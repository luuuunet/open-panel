package cluster

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

// AutoApplyAfterJoin wires the node into the workflow and applies LB / replication immediately.
func (s *Service) AutoApplyAfterJoin(nodeID uint, provisionRole string) string {
	s.ensureDefaultWorkflow()
	s.integrateNodeIntoWorkflow(nodeID, provisionRole)

	_, graph, err := s.GetWorkflow()
	if err != nil || graph == nil {
		return s.fallbackApplyToExistingLBs(nodeID, provisionRole)
	}

	graph = s.ensureLBInGraph(graph, nodeID, provisionRole)
	_, _ = s.SaveWorkflow(graph)

	nodeByID := map[string]*FlowNode{}
	for i := range graph.Nodes {
		nodeByID[graph.Nodes[i].ID] = &graph.Nodes[i]
	}
	nodeFlowID := fmt.Sprintf("node-%d", nodeID)
	var msgs []string

	switch provisionRole {
	case "worker", "lb_backend":
		worker := nodeByID[nodeFlowID]
		if worker == nil {
			break
		}
		applied := 0
		for _, n := range graph.Nodes {
			if n.Type != "lb" {
				continue
			}
			linked := false
			for _, e := range graph.Edges {
				if e.From == n.ID && e.To == nodeFlowID && e.Kind == "routes" {
					linked = true
					break
				}
			}
			if !linked {
				continue
			}
			if err := s.applyFlowLBNode(&n, []*FlowNode{worker}); err == nil {
				applied++
			}
		}
		if applied == 0 {
			if fb := s.fallbackApplyToExistingLBs(nodeID, provisionRole); fb != "" {
				msgs = append(msgs, fb)
			}
		} else {
			msgs = append(msgs, fmt.Sprintf("已自动加入 %d 个负载均衡并应用到 Nginx", applied))
		}

	case "db_slave":
		for _, e := range graph.Edges {
			if e.To != nodeFlowID || e.Kind != "replicates" {
				continue
			}
			masterN := nodeByID[e.From]
			slaveN := nodeByID[e.To]
			if masterN == nil || slaveN == nil {
				continue
			}
			script, _ := s.generateReplicationScript(masterN, slaveN)
			_, _ = s.writeReplicationScript(masterN, slaveN, script)
			log, applyErr := s.ApplyReplicationViaSSH(masterN, slaveN)
			if applyErr != nil {
				msgs = append(msgs, "主从脚本已生成，SSH 应用需检查节点 SSH 配置")
			} else if strings.Contains(log, "主从复制已应用") {
				msgs = append(msgs, "MySQL 主从已自动配置")
			}
		}
	}

	return strings.Join(msgs, "；")
}

func (s *Service) ensureLBInGraph(graph *FlowGraph, nodeID uint, provisionRole string) *FlowGraph {
	if graph == nil {
		g := s.buildDefaultGraph()
		return &g
	}
	if provisionRole != "worker" && provisionRole != "lb_backend" {
		return graph
	}
	hasLB := false
	for _, n := range graph.Nodes {
		if n.Type == "lb" {
			hasLB = true
			break
		}
	}
	if !hasLB {
		all, _ := s.settings.GetAll()
		domain := strings.TrimSpace(all["cluster_default_domain"])
		if domain == "" {
			domain = "app.example.com"
		}
		lbID := "lb-auto"
		graph.Nodes = append(graph.Nodes, FlowNode{
			ID: lbID, Type: "lb", Label: "Load Balancer",
			X: 400, Y: 40,
			Config: map[string]interface{}{
				"domain": domain, "listen_port": 80, "algorithm": "round_robin",
			},
		})
	}
	nodeFlowID := fmt.Sprintf("node-%d", nodeID)
	for _, n := range graph.Nodes {
		if n.Type != "lb" {
			continue
		}
		found := false
		for _, e := range graph.Edges {
			if e.From == n.ID && e.To == nodeFlowID {
				found = true
				break
			}
		}
		if !found {
			graph.Edges = append(graph.Edges, FlowEdge{
				ID: fmt.Sprintf("e-auto-lb-%s-%d", n.ID, nodeID),
				From: n.ID, To: nodeFlowID, Kind: "routes",
			})
		}
	}
	return graph
}

func (s *Service) applyFlowLBNode(lbNode *FlowNode, workers []*FlowNode) error {
	if lbNode == nil || len(workers) == 0 {
		return fmt.Errorf("no workers")
	}
	cfg := lbNode.Config
	domain := strCfg(cfg, "domain", "cluster.local")
	lbID := uint(numCfg(cfg, "lb_id", 0))
	var lb *models.LoadBalancer
	var err error
	if lbID > 0 {
		lb, err = s.GetLoadBalancer(lbID)
	}
	if lb == nil || err != nil {
		lb, err = s.CreateLoadBalancer(LBRequest{
			Name: lbNode.Label, Domain: domain,
			ListenPort: int(numCfg(cfg, "listen_port", 80)),
			Algorithm:  strCfg(cfg, "algorithm", "round_robin"),
			Enabled:    true, HealthCheck: true, HealthPath: "/",
		})
		if err != nil {
			return err
		}
		if lbNode.Config == nil {
			lbNode.Config = map[string]interface{}{}
		}
		lbNode.Config["lb_id"] = lb.ID
	}
	for _, w := range workers {
		if w.RefID == 0 {
			continue
		}
		host, port := s.workerBackend(w)
		_, _ = s.AddBackend(lb.ID, BackendRequest{
			NodeID: w.RefID, Host: host, Port: port, Weight: 1, Enabled: true,
		})
	}
	return s.ApplyLoadBalancer(lb.ID)
}

func (s *Service) fallbackApplyToExistingLBs(nodeID uint, provisionRole string) string {
	if provisionRole != "worker" && provisionRole != "lb_backend" {
		return ""
	}
	lbs, err := s.ListLoadBalancers()
	if err != nil || len(lbs) == 0 {
		return ""
	}
	applied := 0
	for _, lb := range lbs {
		if !lb.Enabled {
			continue
		}
		if _, err := s.AddBackend(lb.ID, BackendRequest{NodeID: nodeID, Weight: 1, Enabled: true}); err != nil {
			continue
		}
		if err := s.ApplyLoadBalancer(lb.ID); err != nil {
			continue
		}
		applied++
	}
	if applied > 0 {
		return fmt.Sprintf("已自动加入 %d 个负载均衡并应用到 Nginx", applied)
	}
	return ""
}
