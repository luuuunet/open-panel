package cluster

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/dashboard"
	"github.com/luuuunet/owpanel/internal/services/performance"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"github.com/luuuunet/owpanel/internal/services/webserver"
	"gorm.io/gorm"
)

type Service struct {
	db        *gorm.DB
	dataDir   string
	settings  *settings.Service
	dashboard *dashboard.Service
	ws        *webserver.Manager
	perf      *performance.Service
}

func NewService(db *gorm.DB, dataDir string, settingsSvc *settings.Service, dash *dashboard.Service, ws *webserver.Manager, perf *performance.Service) *Service {
	s := &Service{db: db, dataDir: dataDir, settings: settingsSvc, dashboard: dash, ws: ws, perf: perf}
	s.ensureAgentToken()
	s.ensureLocalNode()
	return s
}

func (s *Service) ensureAgentToken() {
	s.settings.EnsureKeys("cluster_agent_token")
	all, _ := s.settings.GetAll()
	if strings.TrimSpace(all["cluster_agent_token"]) == "" {
		_ = s.settings.Update(map[string]string{"cluster_agent_token": randomToken(24)})
	}
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Service) AgentToken() (string, error) {
	s.ensureAgentToken()
	all, err := s.settings.GetAll()
	if err != nil {
		return "", err
	}
	return all["cluster_agent_token"], nil
}

func (s *Service) RegenerateAgentToken() (string, error) {
	tok := randomToken(24)
	if err := s.settings.Update(map[string]string{"cluster_agent_token": tok}); err != nil {
		return "", err
	}
	return tok, nil
}

func (s *Service) ValidateAgentToken(header string) bool {
	tok, err := s.AgentToken()
	if err != nil || tok == "" {
		return false
	}
	return header == tok
}

func (s *Service) AgentInfo() map[string]interface{} {
	st, _ := s.dashboard.GetStats()
	hostname := ""
	cpu := 0.0
	mem := 0.0
	if st != nil {
		hostname = st.System.Hostname
		cpu = st.CPU.UsagePercent
		mem = st.Memory.UsedPercent
	}
	all, _ := s.settings.GetAll()
	return map[string]interface{}{
		"hostname":   hostname,
		"cpu":        cpu,
		"memory":     mem,
		"panel_name": all["panel_name"],
		"version":    "owpanel",
		"role":       "master",
	}
}

func (s *Service) ensureLocalNode() {
	var n models.ClusterNode
	if s.db.Where("is_local = ?", true).First(&n).Error == nil {
		return
	}
	all, _ := s.settings.GetAll()
	port := 8888
	if p := all["panel_port"]; p != "" {
		fmt.Sscanf(p, "%d", &port)
	}
	safePath := strings.Trim(all["panel_safe_path"], "/")
	host := "127.0.0.1"
	if ip := all["server_public_ip"]; ip != "" {
		host = ip
	}
	st, _ := s.dashboard.GetStats()
	hostname := "local"
	if st != nil && st.System.Hostname != "" {
		hostname = st.System.Hostname
	}
	now := time.Now()
	node := models.ClusterNode{
		Name: "本机 Master", Host: host, Port: port, SafePath: safePath,
		Role: "master", IsLocal: true, Status: "online",
		Hostname: hostname, LastSeenAt: &now,
	}
	if st != nil {
		node.CPUPercent = st.CPU.UsagePercent
		node.MemPercent = st.Memory.UsedPercent
	}
	_ = s.db.Create(&node).Error
}

type Overview struct {
	NodeTotal    int `json:"node_total"`
	NodeOnline   int `json:"node_online"`
	LBTotal      int `json:"lb_total"`
	LBActive     int `json:"lb_active"`
	BackendTotal int `json:"backend_total"`
}

func (s *Service) Overview() (Overview, error) {
	var o Overview
	var nodeCount int64
	s.db.Model(&models.ClusterNode{}).Count(&nodeCount)
	o.NodeTotal = int(nodeCount)
	var nodes []models.ClusterNode
	s.db.Find(&nodes)
	for _, n := range nodes {
		if n.Status == "online" {
			o.NodeOnline++
		}
	}
	var lbCount int64
	s.db.Model(&models.LoadBalancer{}).Count(&lbCount)
	o.LBTotal = int(lbCount)
	var lbs []models.LoadBalancer
	s.db.Where("enabled = ? AND status = ?", true, "active").Find(&lbs)
	o.LBActive = len(lbs)
	var bc int64
	s.db.Model(&models.LoadBalancerBackend{}).Count(&bc)
	o.BackendTotal = int(bc)
	return o, nil
}

func (s *Service) ListNodes() ([]models.ClusterNode, error) {
	var list []models.ClusterNode
	if err := s.db.Order("is_local desc, id asc").Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		list[i].HasSSHPassword = strings.TrimSpace(list[i].SSHPassword) != ""
	}
	return list, nil
}

func (s *Service) GetNode(id uint) (*models.ClusterNode, error) {
	var n models.ClusterNode
	if err := s.db.First(&n, id).Error; err != nil {
		return nil, err
	}
	enrichNode(&n)
	return &n, nil
}

func enrichNode(n *models.ClusterNode) {
	n.HasSSHPassword = strings.TrimSpace(n.SSHPassword) != ""
}

type NodeRequest struct {
	Name          string `json:"name"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	SafePath      string `json:"safe_path"`
	AgentToken    string `json:"agent_token"`
	Role          string `json:"role"`
	Tags          string `json:"tags"`
	Remark        string `json:"remark"`
	WebsiteHost   string `json:"website_host"`
	WebsitePort   int    `json:"website_port"`
	SSHHost       string `json:"ssh_host"`
	SSHPort       int    `json:"ssh_port"`
	SSHUser       string `json:"ssh_user"`
	SSHPassword   string `json:"ssh_password"`
	ProvisionRole string `json:"provision_role"`
	AutoProvision bool   `json:"auto_provision"`
}

func (s *Service) CreateNode(req NodeRequest) (*models.ClusterNode, error) {
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Host) == "" {
		return nil, fmt.Errorf("名称和主机地址不能为空")
	}
	if req.Port <= 0 {
		req.Port = 8888
	}
	if req.Role == "" {
		req.Role = "worker"
	}
	if req.WebsitePort <= 0 {
		req.WebsitePort = 80
	}
	sshPort := req.SSHPort
	if sshPort <= 0 {
		sshPort = 22
	}
	sshUser := strings.TrimSpace(req.SSHUser)
	if sshUser == "" {
		sshUser = "root"
	}
	provisionRole := strings.TrimSpace(req.ProvisionRole)
	if provisionRole == "" {
		provisionRole = "lb_backend"
	}
	node := models.ClusterNode{
		Name: strings.TrimSpace(req.Name), Host: strings.TrimSpace(req.Host),
		Port: req.Port, SafePath: strings.Trim(req.SafePath, "/"),
		AgentToken: strings.TrimSpace(req.AgentToken), Role: req.Role,
		Tags: req.Tags, Remark: req.Remark,
		WebsiteHost: strings.TrimSpace(req.WebsiteHost), WebsitePort: req.WebsitePort,
		SSHHost: strings.TrimSpace(req.SSHHost), SSHPort: sshPort, SSHUser: sshUser,
		SSHPassword: strings.TrimSpace(req.SSHPassword), ProvisionRole: provisionRole,
		Status: "unknown",
	}
	if err := s.db.Create(&node).Error; err != nil {
		return nil, err
	}
	_ = s.SyncNode(node.ID)
	if req.AutoProvision && nodeHasSSH(&node) {
		_, _ = s.ProvisionNode(node.ID)
	}
	out, err := s.GetNode(node.ID)
	if err != nil {
		return nil, err
	}
	if pr := strings.TrimSpace(out.ProvisionRole); pr == "worker" || pr == "lb_backend" || pr == "db_slave" {
		_ = s.AutoApplyAfterJoin(out.ID, pr)
	}
	return out, nil
}

func (s *Service) UpdateNode(id uint, req NodeRequest) (*models.ClusterNode, error) {
	node, err := s.GetNode(id)
	if err != nil {
		return nil, err
	}
	if node.IsLocal {
		return nil, fmt.Errorf("不能修改本机节点的基础连接信息")
	}
	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Host != "" {
		updates["host"] = strings.TrimSpace(req.Host)
	}
	if req.Port > 0 {
		updates["port"] = req.Port
	}
	updates["safe_path"] = strings.Trim(req.SafePath, "/")
	if req.AgentToken != "" {
		updates["agent_token"] = req.AgentToken
	}
	if req.Role != "" {
		updates["role"] = req.Role
	}
	updates["tags"] = req.Tags
	updates["remark"] = req.Remark
	if req.WebsiteHost != "" {
		updates["website_host"] = req.WebsiteHost
	}
	if req.WebsitePort > 0 {
		updates["website_port"] = req.WebsitePort
	}
	if req.SSHHost != "" {
		updates["ssh_host"] = strings.TrimSpace(req.SSHHost)
	}
	if req.SSHPort > 0 {
		updates["ssh_port"] = req.SSHPort
	}
	if strings.TrimSpace(req.SSHUser) != "" {
		updates["ssh_user"] = strings.TrimSpace(req.SSHUser)
	}
	if strings.TrimSpace(req.SSHPassword) != "" {
		updates["ssh_password"] = strings.TrimSpace(req.SSHPassword)
	}
	if strings.TrimSpace(req.ProvisionRole) != "" {
		updates["provision_role"] = strings.TrimSpace(req.ProvisionRole)
	}
	if len(updates) > 0 {
		s.db.Model(node).Updates(updates)
	}
	node, _ = s.GetNode(id)
	if req.AutoProvision && nodeHasSSH(node) {
		_, _ = s.ProvisionNode(id)
	}
	return s.GetNode(id)
}

func (s *Service) DeleteNode(id uint) error {
	node, err := s.GetNode(id)
	if err != nil {
		return err
	}
	if node.IsLocal {
		return fmt.Errorf("不能删除本机节点")
	}
	return s.db.Delete(node).Error
}

func (s *Service) SyncNode(id uint) error {
	node, err := s.GetNode(id)
	if err != nil {
		return err
	}
	if node.IsLocal {
		return s.syncLocalNode(node)
	}
	return s.probeRemote(node)
}

func (s *Service) syncLocalNode(node *models.ClusterNode) error {
	st, _ := s.dashboard.GetStats()
	now := time.Now()
	updates := map[string]interface{}{
		"status": "online", "last_seen_at": now, "last_error": "",
	}
	if st != nil {
		updates["hostname"] = st.System.Hostname
		updates["cpu_percent"] = st.CPU.UsagePercent
		updates["mem_percent"] = st.Memory.UsedPercent
		updates["load1"] = st.Load.Load1
		if len(st.Disk) > 0 {
			updates["disk_percent"] = st.Disk[0].UsedPercent
		}
	}
	return s.db.Model(node).Updates(updates).Error
}

func (s *Service) probeRemote(node *models.ClusterNode) error {
	base := nodeBaseURL(node)
	client := &http.Client{Timeout: 8 * time.Second}

	if node.AgentToken != "" {
		req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/cluster/agent/ping", nil)
		req.Header.Set("X-Cluster-Token", node.AgentToken)
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return s.applyProbeResult(node, resp.Body)
			}
		}
	}

	resp, err := client.Get(base + "/api/v1/health")
	if err != nil {
		if nodeHasSSH(node) {
			if sshErr := s.syncNodeViaSSH(node); sshErr == nil {
				return nil
			}
		}
		s.markOffline(node, err.Error())
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if nodeHasSSH(node) {
			if sshErr := s.syncNodeViaSSH(node); sshErr == nil {
				return nil
			}
		}
		s.markOffline(node, fmt.Sprintf("HTTP %d", resp.StatusCode))
		return fmt.Errorf("health check failed: %d", resp.StatusCode)
	}
	now := time.Now()
	updates := map[string]interface{}{
		"status": "online", "last_seen_at": now, "last_error": "",
	}
	// HTTP 可达但无 Agent 指标时，尝试 SSH 补采监控
	if nodeHasSSH(node) && node.CPUPercent == 0 && node.MemPercent == 0 {
		if m, err := s.CollectMonitor(node.ID); err == nil && (m.CPU > 0 || m.Memory > 0) {
			return nil
		}
	}
	return s.db.Model(node).Updates(updates).Error
}

func (s *Service) applyProbeResult(node *models.ClusterNode, body io.Reader) error {
	var data struct {
		Hostname string  `json:"hostname"`
		CPU      float64 `json:"cpu"`
		Memory   float64 `json:"memory"`
	}
	_ = json.NewDecoder(body).Decode(&data)
	now := time.Now()
	return s.db.Model(node).Updates(map[string]interface{}{
		"status": "online", "last_seen_at": now, "last_error": "",
		"hostname": data.Hostname, "cpu_percent": data.CPU, "mem_percent": data.Memory,
	}).Error
}

func (s *Service) markOffline(node *models.ClusterNode, msg string) {
	s.db.Model(node).Updates(map[string]interface{}{
		"status": "offline", "last_error": msg,
	})
}

func nodeBaseURL(node *models.ClusterNode) string {
	sp := strings.Trim(node.SafePath, "/")
	base := fmt.Sprintf("http://%s:%d", node.Host, node.Port)
	if sp != "" {
		base += "/" + sp
	}
	return base
}

func (s *Service) StartWatcher() {
	go func() {
		for {
			s.syncAllNodes()
			interval := 60 * time.Second
			if s.perf != nil {
				interval = s.perf.ClusterSyncInterval()
			}
			timer := time.NewTimer(interval)
			select {
			case <-timer.C:
				timer.Stop()
			}
		}
	}()
}

func (s *Service) syncAllNodes() {
	nodes, _ := s.ListNodes()
	for i := range nodes {
		n := &nodes[i]
		_ = s.SyncNode(n.ID)
		// 仅有 SSH、无面板 Agent 的节点：定期 SSH 拉取监控
		if !n.IsLocal && nodeHasSSH(n) {
			fresh, err := s.GetNode(n.ID)
			if err == nil && fresh.Status == "online" && fresh.CPUPercent == 0 && fresh.MemPercent == 0 {
				_, _ = s.CollectMonitor(n.ID)
			}
		}
	}
	s.checkBackendsHealth()
}

func (s *Service) SyncAllNodes() {
	s.syncAllNodes()
}
