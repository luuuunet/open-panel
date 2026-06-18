package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/domaincheck"
	"gorm.io/gorm"
)

type WebServerHooks struct {
	GetActive   func() string
	Reload      func(key string) error
	EnsureInc   func(key string)
	EnsureReady func() error
}

type BindDomainRequest struct {
	Domain   string `json:"domain"`
	HostPort int    `json:"host_port"`
}

var portBindingRe = regexp.MustCompile(`:(\d+)->\d+/(\w+)`)

func (s *Service) SetWebServerHooks(h WebServerHooks) {
	s.ws = h
}

func ParseHostPortsFromString(ports string) []int {
	var out []int
	seen := map[int]bool{}
	for _, part := range strings.Split(ports, ",") {
		m := portBindingRe.FindStringSubmatch(strings.TrimSpace(part))
		if len(m) < 3 || m[2] != "tcp" {
			continue
		}
		p, _ := strconv.Atoi(m[1])
		if p > 0 && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func (s *Service) ListBindings() ([]models.DockerContainerBinding, error) {
	if s.db == nil {
		return nil, nil
	}
	var list []models.DockerContainerBinding
	if err := s.db.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Service) GetBinding(containerID string) (*models.DockerContainerBinding, error) {
	if s.db == nil {
		return nil, nil
	}
	b, ok := s.findBindingByContainerID(containerID)
	if !ok {
		return nil, nil
	}
	return b, nil
}

func (s *Service) BindDomain(containerID string, req BindDomainRequest) (*models.DockerContainerBinding, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database unavailable")
	}
	domain := domaincheck.HostOnly(req.Domain)
	if domain == "" {
		return nil, fmt.Errorf("请填写域名")
	}
	detail, err := s.InspectContainer(containerID)
	if err != nil {
		return nil, err
	}
	hostPort := req.HostPort
	if hostPort <= 0 {
		var ports []int
		for _, p := range detail.Ports {
			if p.HostPort != "" && (p.Protocol == "" || p.Protocol == "tcp") {
				if n, e := strconv.Atoi(p.HostPort); e == nil && n > 0 {
					ports = append(ports, n)
				}
			}
		}
		if len(ports) == 0 {
			return nil, fmt.Errorf("容器未映射 TCP 端口，请先配置端口映射")
		}
		hostPort = ports[0]
	}
	if err := domaincheck.AssertAvailable(s.db, []string{domain}, domaincheck.Scope{
		IgnoreDockerContainerID: detail.ID,
	}); err != nil {
		return nil, err
	}
	var binding models.DockerContainerBinding
	err = s.db.Where("container_id = ?", detail.ID).First(&binding).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	oldDomain := binding.Domain
	binding.ContainerID = detail.ID
	binding.ContainerName = detail.Name
	binding.Domain = domain
	binding.HostPort = hostPort
	if binding.ID == 0 {
		if err := s.db.Create(&binding).Error; err != nil {
			return nil, err
		}
	} else {
		if err := s.db.Save(&binding).Error; err != nil {
			return nil, err
		}
	}
	if oldDomain != "" && oldDomain != domain {
		_ = s.removeProxyVhost(detail.Name)
	}
	if err := s.applyProxyVhost(&binding); err != nil {
		return nil, err
	}
	return &binding, nil
}

func (s *Service) UnbindDomain(containerID string) error {
	if s.db == nil {
		return fmt.Errorf("database unavailable")
	}
	b, ok := s.findBindingByContainerID(containerID)
	if !ok {
		return nil
	}
	_ = s.removeProxyVhost(b.ContainerName)
	return s.db.Delete(b).Error
}

func (s *Service) RemoveBindingForContainer(containerID string) {
	_ = s.UnbindDomain(containerID)
}

func (s *Service) ReconcileBindings() error {
	if s.db == nil {
		return nil
	}
	list, err := s.ListBindings()
	if err != nil {
		return err
	}
	for i := range list {
		if err := s.applyProxyVhost(&list[i]); err != nil {
			continue
		}
	}
	return nil
}

func (s *Service) enrichContainers(list []Container) []Container {
	if s.db == nil || len(list) == 0 {
		return list
	}
	bindings, err := s.ListBindings()
	if err != nil {
		return list
	}
	for i := range list {
		if b, ok := s.matchBinding(list[i].ID, bindings); ok {
			list[i].BindDomain = b.Domain
			list[i].HostPort = b.HostPort
			list[i].AccessURL = "http://" + b.Domain
		}
	}
	return list
}

func (s *Service) findBindingByContainerID(containerID string) (*models.DockerContainerBinding, bool) {
	var list []models.DockerContainerBinding
	if s.db.Find(&list).Error != nil {
		return nil, false
	}
	b, ok := s.matchBinding(containerID, list)
	return b, ok
}

func (s *Service) matchBinding(containerID string, list []models.DockerContainerBinding) (*models.DockerContainerBinding, bool) {
	for i := range list {
		b := &list[i]
		if b.ContainerID == containerID ||
			strings.HasPrefix(b.ContainerID, containerID) ||
			strings.HasPrefix(containerID, b.ContainerID) {
			return b, true
		}
	}
	return nil, false
}

func dockerVhostFile(dataDir, containerName string) string {
	safe := regexp.MustCompile(`[^a-zA-Z0-9_-]+`).ReplaceAllString(containerName, "-")
	safe = strings.Trim(safe, "-")
	if safe == "" {
		safe = "container"
	}
	return filepath.Join(dataDir, "nginx", "vhosts", fmt.Sprintf("owpanel-docker-%s.conf", safe))
}

func (s *Service) applyProxyVhost(b *models.DockerContainerBinding) error {
	if b == nil || b.Domain == "" || b.HostPort <= 0 {
		return nil
	}
	if err := s.ensureProxyWebServer(); err != nil {
		return err
	}
	name := b.ContainerName
	if name == "" {
		name = b.ContainerID
	}
	conf := fmt.Sprintf(`# OWPanel — Docker %s
server {
    listen 80;
    server_name %s;

    location / {
        proxy_pass http://127.0.0.1:%d;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 86400;
        proxy_send_timeout 86400;
    }
}
`, name, b.Domain, b.HostPort)
	vhostPath := dockerVhostFile(s.dataDir, name)
	if err := os.MkdirAll(filepath.Dir(vhostPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(vhostPath, []byte(conf), 0644); err != nil {
		return err
	}
	return s.reloadProxyWebServer()
}

func (s *Service) removeProxyVhost(containerName string) error {
	_ = os.Remove(dockerVhostFile(s.dataDir, containerName))
	return s.reloadProxyWebServer()
}

func (s *Service) ensureProxyWebServer() error {
	if s.ws.EnsureReady != nil {
		if err := s.ws.EnsureReady(); err != nil {
			return err
		}
	}
	active := "nginx"
	if s.ws.GetActive != nil {
		if v := s.ws.GetActive(); v != "" {
			active = v
		}
	}
	if s.ws.EnsureInc != nil {
		s.ws.EnsureInc(active)
	} else {
		vhostDir := filepath.Join(s.dataDir, "nginx", "vhosts")
		_ = os.MkdirAll(vhostDir, 0755)
		mainConf := filepath.Join(s.dataDir, "nginx", "owpanel.conf")
		includeLine := fmt.Sprintf("include %s/*.conf;", filepath.ToSlash(vhostDir))
		_ = os.WriteFile(mainConf, []byte("# OWPanel auto-generated\n"+includeLine+"\n"), 0644)
	}
	return nil
}

func (s *Service) reloadProxyWebServer() error {
	active := "nginx"
	if s.ws.GetActive != nil {
		if v := s.ws.GetActive(); v != "" {
			active = v
		}
	}
	if s.ws.Reload != nil {
		if err := s.ws.Reload(active); err != nil {
			return fmt.Errorf("Nginx 配置已写入，但重载失败: %w", err)
		}
	}
	return nil
}
