package cluster

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

type LBRequest struct {
	Name           string `json:"name"`
	Domain         string `json:"domain"`
	ListenPort     int    `json:"listen_port"`
	SSL            bool   `json:"ssl"`
	Algorithm      string `json:"algorithm"`
	HealthCheck    bool   `json:"health_check"`
	HealthPath     string `json:"health_path"`
	HealthInterval int    `json:"health_interval"`
	StickySession  bool   `json:"sticky_session"`
	WebSocket      bool   `json:"websocket"`
	Enabled        bool   `json:"enabled"`
	Remark         string `json:"remark"`
}

type BackendRequest struct {
	NodeID  uint   `json:"node_id"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Weight  int    `json:"weight"`
	Enabled bool   `json:"enabled"`
}

func (s *Service) ListLoadBalancers() ([]models.LoadBalancer, error) {
	var list []models.LoadBalancer
	if err := s.db.Preload("Backends").Order("id desc").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *Service) GetLoadBalancer(id uint) (*models.LoadBalancer, error) {
	var lb models.LoadBalancer
	if err := s.db.Preload("Backends").First(&lb, id).Error; err != nil {
		return nil, err
	}
	return &lb, nil
}

func (s *Service) CreateLoadBalancer(req LBRequest) (*models.LoadBalancer, error) {
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Domain) == "" {
		return nil, fmt.Errorf("名称和域名不能为空")
	}
	if req.ListenPort <= 0 {
		req.ListenPort = 80
	}
	if req.Algorithm == "" {
		req.Algorithm = "round_robin"
	}
	if req.HealthPath == "" {
		req.HealthPath = "/"
	}
	if req.HealthInterval <= 0 {
		req.HealthInterval = 10
	}
	lb := models.LoadBalancer{
		Name: req.Name, Domain: strings.TrimSpace(req.Domain),
		ListenPort: req.ListenPort, SSL: req.SSL, Algorithm: req.Algorithm,
		HealthCheck: req.HealthCheck, HealthPath: req.HealthPath,
		HealthInterval: req.HealthInterval, StickySession: req.StickySession,
		WebSocket: req.WebSocket, Enabled: req.Enabled, Remark: req.Remark,
		Status: "pending",
	}
	if err := s.db.Create(&lb).Error; err != nil {
		return nil, err
	}
	return &lb, nil
}

func (s *Service) UpdateLoadBalancer(id uint, req LBRequest) (*models.LoadBalancer, error) {
	lb, err := s.GetLoadBalancer(id)
	if err != nil {
		return nil, err
	}
	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Domain != "" {
		updates["domain"] = strings.TrimSpace(req.Domain)
	}
	if req.ListenPort > 0 {
		updates["listen_port"] = req.ListenPort
	}
	updates["ssl"] = req.SSL
	if req.Algorithm != "" {
		updates["algorithm"] = req.Algorithm
	}
	updates["health_check"] = req.HealthCheck
	if req.HealthPath != "" {
		updates["health_path"] = req.HealthPath
	}
	if req.HealthInterval > 0 {
		updates["health_interval"] = req.HealthInterval
	}
	updates["sticky_session"] = req.StickySession
	updates["websocket"] = req.WebSocket
	updates["enabled"] = req.Enabled
	updates["remark"] = req.Remark
	s.db.Model(lb).Updates(updates)
	return s.GetLoadBalancer(id)
}

func (s *Service) DeleteLoadBalancer(id uint) error {
	lb, err := s.GetLoadBalancer(id)
	if err != nil {
		return err
	}
	if lb.NginxConf != "" {
		_ = os.Remove(lb.NginxConf)
	}
	s.db.Where("load_balancer_id = ?", id).Delete(&models.LoadBalancerBackend{})
	return s.db.Delete(lb).Error
}

func (s *Service) AddBackend(lbID uint, req BackendRequest) (*models.LoadBalancerBackend, error) {
	if _, err := s.GetLoadBalancer(lbID); err != nil {
		return nil, err
	}
	host := strings.TrimSpace(req.Host)
	port := req.Port
	if req.NodeID > 0 {
		node, err := s.GetNode(req.NodeID)
		if err != nil {
			return nil, err
		}
		if host == "" {
			if node.WebsiteHost != "" {
				host = node.WebsiteHost
			} else {
				host = node.Host
			}
		}
		if port <= 0 {
			if node.WebsitePort > 0 {
				port = node.WebsitePort
			} else {
				port = 80
			}
		}
	}
	if host == "" {
		return nil, fmt.Errorf("后端地址不能为空")
	}
	if port <= 0 {
		port = 80
	}
	weight := req.Weight
	if weight <= 0 {
		weight = 1
	}
	b := models.LoadBalancerBackend{
		LoadBalancerID: lbID, NodeID: req.NodeID, Host: host, Port: port,
		Weight: weight, Enabled: req.Enabled, Status: "unknown",
	}
	// 避免流程重复执行时叠加相同后端
	var existing models.LoadBalancerBackend
	q := s.db.Where("load_balancer_id = ? AND host = ? AND port = ?", lbID, host, port)
	if req.NodeID > 0 {
		q = s.db.Where("load_balancer_id = ? AND node_id = ?", lbID, req.NodeID)
	}
	if q.First(&existing).Error == nil {
		s.db.Model(&existing).Updates(map[string]interface{}{
			"host": host, "port": port, "weight": weight, "enabled": req.Enabled,
		})
		return &existing, nil
	}
	if err := s.db.Create(&b).Error; err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Service) DeleteBackend(lbID, backendID uint) error {
	return s.db.Where("id = ? AND load_balancer_id = ?", backendID, lbID).Delete(&models.LoadBalancerBackend{}).Error
}

func (s *Service) ApplyLoadBalancer(id uint) error {
	lb, err := s.GetLoadBalancer(id)
	if err != nil {
		return err
	}
	if len(lb.Backends) == 0 {
		return fmt.Errorf("请至少添加一个后端节点")
	}
	enabled := 0
	for _, b := range lb.Backends {
		if b.Enabled {
			enabled++
		}
	}
	if enabled == 0 {
		return fmt.Errorf("请至少启用一个后端")
	}
	conf, path, err := s.generateNginxLB(lb)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(conf), 0644); err != nil {
		return err
	}
	status := "active"
	if !lb.Enabled {
		status = "disabled"
	}
	s.db.Model(lb).Updates(map[string]interface{}{
		"nginx_conf": path, "status": status,
	})
	if s.ws != nil {
		key := s.ws.GetActive()
		s.ws.EnsureVhostInclude(key)
		return s.ws.Reload(key)
	}
	return nil
}

func (s *Service) lbConfDir() string {
	dir := filepath.Join(s.dataDir, "nginx", "vhosts")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func (s *Service) generateNginxLB(lb *models.LoadBalancer) (string, string, error) {
	upName := fmt.Sprintf("op_lb_%d", lb.ID)
	slug := strings.NewReplacer(".", "-", ":", "-").Replace(lb.Domain)
	path := filepath.Join(s.lbConfDir(), "lb-"+slug+".conf")

	var upstreamDirectives []string
	switch lb.Algorithm {
	case "least_conn":
		upstreamDirectives = append(upstreamDirectives, "    least_conn;")
	case "ip_hash":
		upstreamDirectives = append(upstreamDirectives, "    ip_hash;")
	case "random":
		upstreamDirectives = append(upstreamDirectives, "    random;")
	}

	var servers []string
	for _, b := range lb.Backends {
		if !b.Enabled {
			continue
		}
		line := fmt.Sprintf("    server %s:%d weight=%d max_fails=3 fail_timeout=10s;", b.Host, b.Port, b.Weight)
		servers = append(servers, line)
	}
	if len(servers) == 0 {
		return "", "", fmt.Errorf("no enabled backends")
	}

	sticky := ""
	if lb.StickySession && lb.Algorithm != "ip_hash" {
		sticky = `
    ip_hash;`
	}

	wsHeaders := ""
	if lb.WebSocket {
		wsHeaders = `
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";`
	}

	health := ""
	if lb.HealthCheck && lb.HealthPath != "" {
		health = fmt.Sprintf(`
    # health check hint: GET %s every %ds`, lb.HealthPath, lb.HealthInterval)
	}

	upstream := fmt.Sprintf(`upstream %s {%s%s
%s
}%s`, upName, strings.Join(upstreamDirectives, "\n"), sticky, strings.Join(servers, "\n"), health)

	accessLog := filepath.ToSlash(filepath.Join(s.dataDir, "logs", lb.Domain+"_lb_access.log"))
	errorLog := filepath.ToSlash(filepath.Join(s.dataDir, "logs", lb.Domain+"_lb_error.log"))

	server := fmt.Sprintf(`
server {
    listen %d;
    server_name %s;
    access_log %s;
    error_log %s;

    location / {
        proxy_pass http://%s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;%s
    }
}`, lb.ListenPort, lb.Domain, accessLog, errorLog, upName, wsHeaders)

	conf := fmt.Sprintf("# OWPanel Load Balancer — %s\n%s\n%s\n", lb.Name, upstream, server)
	return conf, path, nil
}

func (s *Service) checkBackendsHealth() {
	var backends []models.LoadBalancerBackend
	s.db.Where("enabled = ?", true).Find(&backends)
	client := &http.Client{Timeout: 5 * time.Second}
	for _, b := range backends {
		url := fmt.Sprintf("http://%s:%d/", b.Host, b.Port)
		status := "down"
		if resp, err := client.Get(url); err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				status = "up"
			}
		}
		now := time.Now()
		s.db.Model(&b).Updates(map[string]interface{}{"status": status, "last_check_at": now})
	}
}
