package cluster

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

type FlowNode struct {
	ID     string                 `json:"id"`
	Type   string                 `json:"type"`
	Label  string                 `json:"label"`
	X      float64                `json:"x"`
	Y      float64                `json:"y"`
	RefID  uint                   `json:"ref_id,omitempty"`
	Config map[string]interface{} `json:"config,omitempty"`
	Status string                 `json:"status,omitempty"`
}

type FlowEdge struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type FlowGraph struct {
	Nodes []FlowNode `json:"nodes"`
	Edges []FlowEdge `json:"edges"`
}

type RunStep struct {
	Step    string `json:"step"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type RunResult struct {
	Status string    `json:"status"`
	Steps  []RunStep `json:"steps"`
	Log    string    `json:"log"`
}

func (s *Service) GetWorkflow() (*models.ClusterWorkflow, *FlowGraph, error) {
	s.ensureDefaultWorkflow()
	var wf models.ClusterWorkflow
	if err := s.db.Where("name = ?", "default").First(&wf).Error; err != nil {
		return nil, nil, err
	}
	g, err := parseGraph(wf.GraphJSON)
	if err != nil {
		return &wf, &FlowGraph{}, nil
	}
	return &wf, g, nil
}

func (s *Service) SaveWorkflow(graph *FlowGraph) (*models.ClusterWorkflow, error) {
	if graph == nil {
		return nil, fmt.Errorf("graph required")
	}
	raw, err := json.Marshal(graph)
	if err != nil {
		return nil, err
	}
	s.ensureDefaultWorkflow()
	var wf models.ClusterWorkflow
	if err := s.db.Where("name = ?", "default").First(&wf).Error; err != nil {
		return nil, err
	}
	wf.GraphJSON = string(raw)
	wf.Status = "draft"
	if err := s.db.Save(&wf).Error; err != nil {
		return nil, err
	}
	return &wf, nil
}

func (s *Service) ensureDefaultWorkflow() {
	var n int64
	s.db.Model(&models.ClusterWorkflow{}).Where("name = ?", "default").Count(&n)
	if n > 0 {
		return
	}
	g := s.buildDefaultGraph()
	raw, _ := json.Marshal(g)
	_ = s.db.Create(&models.ClusterWorkflow{
		Name: "default", GraphJSON: string(raw), Status: "draft",
	}).Error
}

func (s *Service) buildDefaultGraph() FlowGraph {
	nodes, _ := s.ListNodes()
	var graph FlowGraph
	x := 120.0
	for i, n := range nodes {
		typ := "worker"
		if n.IsLocal || n.Role == "master" {
			typ = "master"
		}
		graph.Nodes = append(graph.Nodes, FlowNode{
			ID: fmt.Sprintf("node-%d", n.ID), Type: typ,
			Label: n.Name, X: x, Y: 120 + float64(i%3)*140,
			RefID: n.ID, Status: n.Status,
		})
		x += 220
	}
	if len(graph.Nodes) > 0 {
		masterID := graph.Nodes[0].ID
		graph.Nodes = append(graph.Nodes, FlowNode{
			ID: "lb-1", Type: "lb", Label: "Load Balancer",
			X: 400, Y: 40, Config: map[string]interface{}{
				"domain": "app.example.com", "listen_port": 80, "algorithm": "round_robin",
			},
		})
		graph.Nodes = append(graph.Nodes, FlowNode{
			ID: "db-master", Type: "db_master", Label: "MySQL Master",
			X: 120, Y: 380, Config: map[string]interface{}{"db_type": "mysql", "repl_user": "repl"},
		})
		graph.Nodes = append(graph.Nodes, FlowNode{
			ID: "db-slave", Type: "db_slave", Label: "MySQL Slave",
			X: 380, Y: 380, Config: map[string]interface{}{"db_type": "mysql"},
		})
		for _, nd := range graph.Nodes {
			if nd.Type == "worker" {
				graph.Edges = append(graph.Edges, FlowEdge{
					ID: fmt.Sprintf("e-lb-%s", nd.ID), From: "lb-1", To: nd.ID, Kind: "routes",
				})
			}
		}
		graph.Edges = append(graph.Edges, FlowEdge{
			ID: "e-repl", From: "db-master", To: "db-slave", Kind: "replicates",
		})
		if len(graph.Nodes) > 1 {
			graph.Edges = append(graph.Edges, FlowEdge{
				ID: "e-mgmt", From: masterID, To: graph.Nodes[1].ID, Kind: "manages",
			})
		}
	}
	return graph
}

func parseGraph(raw string) (*FlowGraph, error) {
	if strings.TrimSpace(raw) == "" {
		return &FlowGraph{}, nil
	}
	var g FlowGraph
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *Service) SyncGraphFromNodes(graph *FlowGraph) *FlowGraph {
	if graph == nil {
		g := s.buildDefaultGraph()
		return &g
	}
	nodes, _ := s.ListNodes()
	existing := map[uint]bool{}
	for _, n := range graph.Nodes {
		if n.RefID > 0 {
			existing[n.RefID] = true
		}
	}
	x := 600.0
	for _, cn := range nodes {
		if existing[cn.ID] {
			continue
		}
		typ := "worker"
		if cn.IsLocal || cn.Role == "master" {
			typ = "master"
		}
		graph.Nodes = append(graph.Nodes, FlowNode{
			ID: fmt.Sprintf("node-%d", cn.ID), Type: typ,
			Label: cn.Name, X: x, Y: 200, RefID: cn.ID, Status: cn.Status,
		})
		x += 180
	}
	return graph
}

func (s *Service) RunWorkflow(graph *FlowGraph) (*RunResult, error) {
	if graph == nil || len(graph.Nodes) == 0 {
		return nil, fmt.Errorf("流程图为空，请先拖拽编排节点")
	}
	result := &RunResult{Status: "running", Steps: []RunStep{}}
	logLines := []string{}

	addStep := func(step, status, msg string) {
		result.Steps = append(result.Steps, RunStep{Step: step, Status: status, Message: msg})
		logLines = append(logLines, fmt.Sprintf("[%s] %s: %s", status, step, msg))
	}

	// 1. Sync panel nodes
	addStep("sync_nodes", "running", "同步集群节点状态…")
	s.SyncAllNodes()
	for i := range graph.Nodes {
		if graph.Nodes[i].RefID == 0 {
			continue
		}
		node, err := s.GetNode(graph.Nodes[i].RefID)
		if err != nil {
			continue
		}
		needProvision := nodeHasSSH(node) && node.ProvisionStatus != "ready"
		if !needProvision {
			_ = s.SyncNode(graph.Nodes[i].RefID)
			if node, err = s.GetNode(graph.Nodes[i].RefID); err == nil {
				graph.Nodes[i].Status = node.Status
			}
			continue
		}
		switch graph.Nodes[i].Type {
		case "worker", "db_slave", "db_master":
			if role := flowProvisionRole(graph.Nodes[i].Type); role != "" && node.ProvisionRole != role {
				s.db.Model(node).Update("provision_role", role)
				node.ProvisionRole = role
			}
			addStep("provision", "running", fmt.Sprintf("SSH 自动搭建 %s (%s)…", node.Name, graph.Nodes[i].Type))
			if _, perr := s.ProvisionNode(node.ID); perr != nil {
				addStep("provision", "warn", fmt.Sprintf("%s: %s", node.Name, perr.Error()))
			} else {
				addStep("provision", "ok", fmt.Sprintf("%s 已就绪", node.Name))
			}
		}
		_ = s.SyncNode(graph.Nodes[i].RefID)
		if node, err = s.GetNode(graph.Nodes[i].RefID); err == nil {
			graph.Nodes[i].Status = node.Status
		}
	}
	addStep("sync_nodes", "ok", "节点同步完成")

	// 2. DB replication pairs
	for _, e := range graph.Edges {
		if e.Kind != "replicates" {
			continue
		}
		masterN := findNode(graph, e.From)
		slaveN := findNode(graph, e.To)
		if masterN == nil || slaveN == nil {
			addStep("db_replication", "warn", "主从连线缺少节点")
			continue
		}
		script, err := s.generateReplicationScript(masterN, slaveN)
		if err != nil {
			addStep("db_replication", "error", err.Error())
			continue
		}
		path, err := s.writeReplicationScript(masterN, slaveN, script)
		if err != nil {
			addStep("db_replication", "error", err.Error())
			continue
		}
		applyLog, applyErr := s.ApplyReplicationViaSSH(masterN, slaveN)
		if applyErr != nil {
			addStep("db_replication", "warn", fmt.Sprintf("脚本已生成 %s；SSH 应用: %s", path, applyErr.Error()))
			if applyLog != "" {
				logLines = append(logLines, applyLog)
			}
		} else if strings.Contains(applyLog, "主从复制已应用") {
			addStep("db_replication", "ok", fmt.Sprintf("主从复制已通过 SSH 应用 (%s)", path))
			logLines = append(logLines, applyLog)
		} else {
			_ = s.ensureReplicationDBRecords(masterN, slaveN)
			addStep("db_replication", "ok", fmt.Sprintf("已生成主从配置脚本: %s", path))
		}
	}

	// 3. Load balancers
	nodeByID := map[string]*FlowNode{}
	for i := range graph.Nodes {
		nodeByID[graph.Nodes[i].ID] = &graph.Nodes[i]
	}
	for _, n := range graph.Nodes {
		if n.Type != "lb" {
			continue
		}
		cfg := n.Config
		domain := strCfg(cfg, "domain", "cluster.local")
		var workers []*FlowNode
		for _, e := range graph.Edges {
			if e.From == n.ID && e.Kind == "routes" {
				if w := nodeByID[e.To]; w != nil && (w.Type == "worker" || w.Type == "master") {
					workers = append(workers, w)
				}
			}
		}
		if len(workers) == 0 {
			addStep("load_balancer", "warn", fmt.Sprintf("LB %s 未连接 Worker 节点", n.Label))
			continue
		}
		lbID := uint(numCfg(cfg, "lb_id", 0))
		var lb *models.LoadBalancer
		var err error
		if lbID > 0 {
			lb, err = s.GetLoadBalancer(lbID)
		}
		if lb == nil || err != nil {
			lb, err = s.CreateLoadBalancer(LBRequest{
				Name: n.Label, Domain: domain,
				ListenPort: int(numCfg(cfg, "listen_port", 80)),
				Algorithm:  strCfg(cfg, "algorithm", "round_robin"),
				Enabled:    true, HealthCheck: true, HealthPath: "/",
			})
			if err != nil {
				addStep("load_balancer", "error", err.Error())
				continue
			}
			if n.Config == nil {
				n.Config = map[string]interface{}{}
			}
			n.Config["lb_id"] = lb.ID
		}
		for _, w := range workers {
			host, port := s.workerBackend(w)
			_, _ = s.AddBackend(lb.ID, BackendRequest{
				NodeID: w.RefID, Host: host, Port: port, Weight: 1, Enabled: true,
			})
		}
		if err := s.ApplyLoadBalancer(lb.ID); err != nil {
			addStep("load_balancer", "error", err.Error())
			continue
		}
		n.Status = "active"
		addStep("load_balancer", "ok", fmt.Sprintf("负载均衡 %s 已应用 (%d 后端)", domain, len(workers)))
	}

	result.Status = "applied"
	for _, st := range result.Steps {
		if st.Status == "error" {
			result.Status = "failed"
			break
		}
	}
	result.Log = strings.Join(logLines, "\n")

	raw, _ := json.Marshal(graph)
	var wf models.ClusterWorkflow
	if s.db.Where("name = ?", "default").First(&wf).Error == nil {
		now := time.Now()
		wf.GraphJSON = string(raw)
		wf.Status = result.Status
		wf.LastRunAt = &now
		wf.LastRunLog = result.Log
		_ = s.db.Save(&wf).Error
	}
	return result, nil
}

func flowProvisionRole(flowType string) string {
	switch flowType {
	case "worker":
		return "worker"
	case "db_slave":
		return "db_slave"
	case "db_master":
		return "db_master"
	default:
		return ""
	}
}

func findNode(g *FlowGraph, id string) *FlowNode {
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			return &g.Nodes[i]
		}
	}
	return nil
}

func (s *Service) workerBackend(n *FlowNode) (string, int) {
	if n.RefID > 0 {
		if node, err := s.GetNode(n.RefID); err == nil {
			host := node.WebsiteHost
			if host == "" {
				host = node.Host
			}
			port := node.WebsitePort
			if port <= 0 {
				port = 80
			}
			return host, port
		}
	}
	return "127.0.0.1", 80
}

func strCfg(m map[string]interface{}, key, def string) string {
	if m == nil {
		return def
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return def
}

func numCfg(m map[string]interface{}, key string, def float64) float64 {
	if m == nil {
		return def
	}
	if v, ok := m[key]; ok {
		switch t := v.(type) {
		case float64:
			return t
		case int:
			return float64(t)
		case json.Number:
			f, _ := t.Float64()
			return f
		}
	}
	return def
}

func (s *Service) generateReplicationScript(master, slave *FlowNode) (string, error) {
	replUser := strCfg(master.Config, "repl_user", "repl")
	replPass := strCfg(master.Config, "repl_password", "Repl_"+randomToken(4))
	dbName := strCfg(master.Config, "db_name", "app_db")
	mHost := s.nodeHost(master)
	sHost := s.nodeHost(slave)
	var b strings.Builder
	b.WriteString("-- OWPanel auto-generated MySQL replication plan\n")
	b.WriteString(fmt.Sprintf("-- Master: %s (%s)\n", master.Label, mHost))
	b.WriteString(fmt.Sprintf("-- Slave: %s (%s)\n\n", slave.Label, sHost))
	b.WriteString("-- === On MASTER ===\n")
	b.WriteString(fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s';\n", replUser, replPass))
	b.WriteString(fmt.Sprintf("GRANT REPLICATION SLAVE, REPLICATION CLIENT ON *.* TO '%s'@'%%';\n", replUser))
	b.WriteString("FLUSH PRIVILEGES;\n")
	b.WriteString("SHOW MASTER STATUS;\n\n")
	b.WriteString("-- === On SLAVE ===\n")
	b.WriteString("STOP SLAVE;\n")
	b.WriteString(fmt.Sprintf("CHANGE MASTER TO MASTER_HOST='%s', MASTER_USER='%s', MASTER_PASSWORD='%s', MASTER_AUTO_POSITION=1;\n", mHost, replUser, replPass))
	b.WriteString("START SLAVE;\n")
	b.WriteString("SHOW SLAVE STATUS\\G\n\n")
	b.WriteString(fmt.Sprintf("-- Optional database: CREATE DATABASE IF NOT EXISTS `%s`;\n", dbName))
	return b.String(), nil
}

func (s *Service) nodeHost(n *FlowNode) string {
	if n.RefID > 0 {
		if node, err := s.GetNode(n.RefID); err == nil {
			if node.Host != "" {
				return node.Host
			}
		}
	}
	return "127.0.0.1"
}

func (s *Service) writeReplicationScript(master, slave *FlowNode, content string) (string, error) {
	dir := filepath.Join(s.dataDir, "cluster", "replication")
	_ = os.MkdirAll(dir, 0755)
	name := fmt.Sprintf("repl_%s_to_%s.sql", master.ID, slave.ID)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Service) ensureReplicationDBRecords(master, slave *FlowNode) error {
	ensure := func(n *FlowNode, role string) {
		host := s.nodeHost(n)
		var inst models.DatabaseInstance
		if err := s.db.Where("host = ? AND name LIKE ?", host, "%"+role+"%").First(&inst).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			_ = s.db.Create(&models.DatabaseInstance{
				Name: fmt.Sprintf("cluster_%s_%s", role, n.ID),
				Type: "mysql", Host: host, Port: 3306, Username: "root",
			}).Error
		}
	}
	ensure(master, "master")
	ensure(slave, "slave")
	return nil
}
