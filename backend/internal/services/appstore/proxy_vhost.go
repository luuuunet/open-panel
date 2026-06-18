package appstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/domaincheck"
)

type WebServerHooks struct {
	GetActive func() string
	Reload    func(key string) error
	EnsureInc func(key string)
	IsRunning func(key string) bool
}

func (s *Service) SetWebServerHooks(h WebServerHooks) {
	s.ws = h
}

func IsDockerStoreApp(key string) bool {
	_, ok := dockerSpec(key)
	return ok
}

func (s *Service) ProxyPort(key string, appPort int) int {
	if appPort > 0 {
		return appPort
	}
	spec, ok := dockerSpec(key)
	if !ok || spec.Port == "" {
		return 0
	}
	parts := strings.SplitN(spec.Port, ":", 2)
	if len(parts) == 0 {
		return 0
	}
	p, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	return p
}

func (s *Service) AccessURL(key string, bindDomain string, port int) string {
	host := strings.TrimSpace(bindDomain)
	if host != "" {
		return "http://" + domaincheck.HostOnly(host)
	}
	p := s.ProxyPort(key, port)
	if p <= 0 {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d", p)
}

func proxyVhostFile(dataDir, key string) string {
	return filepath.Join(dataDir, "nginx", "vhosts", fmt.Sprintf("owpanel-app-%s.conf", key))
}

func (s *Service) ApplyProxyVhost(key string) error {
	app, err := s.Get(key)
	if err != nil {
		return err
	}
	domain := domaincheck.HostOnly(app.BindDomain)
	if domain == "" {
		return s.RemoveProxyVhost(key)
	}
	port := s.ProxyPort(key, app.Port)
	if port <= 0 {
		return fmt.Errorf("无法确定 %s 的本地端口，请先在设置中填写端口", app.Name)
	}
	if err := s.ensureProxyWebServer(); err != nil {
		return err
	}
	conf := fmt.Sprintf(`# OWPanel — %s (%s)
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
`, app.Name, key, domain, port)
	vhostPath := proxyVhostFile(s.dataDir, key)
	if err := os.MkdirAll(filepath.Dir(vhostPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(vhostPath, []byte(conf), 0644); err != nil {
		return err
	}
	return s.reloadProxyWebServer()
}

func (s *Service) RemoveProxyVhost(key string) error {
	_ = os.Remove(proxyVhostFile(s.dataDir, key))
	return s.reloadProxyWebServer()
}

func (s *Service) ensureProxyWebServer() error {
	active := s.GetActiveWebServer()
	if active == "" {
		active = "nginx"
	}
	app, err := s.Get(active)
	if err != nil || !app.Installed {
		return fmt.Errorf("请先安装并启动 Nginx 或 OpenResty，才能绑定域名")
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
	active := s.GetActiveWebServer()
	if active == "" {
		active = "nginx"
	}
	if s.ws.Reload != nil {
		if err := s.ws.Reload(active); err != nil {
			return fmt.Errorf("Nginx 配置已写入，但重载失败: %w", err)
		}
		return nil
	}
	return nil
}
