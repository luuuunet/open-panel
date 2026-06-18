package enterprise

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

type HAStatus struct {
	Healthy         bool                   `json:"healthy"`
	Grade           string                 `json:"grade"`
	NodeTotal       int                    `json:"node_total"`
	NodeOnline      int                    `json:"node_online"`
	LBTotal         int                    `json:"lb_total"`
	LBActive        int                    `json:"lb_active"`
	BackendTotal    int                    `json:"backend_total"`
	LoadBalancers   []HALoadBalancer       `json:"load_balancers"`
	Nodes           []HANode               `json:"nodes"`
	Replication     []HAReplicationHint    `json:"replication_hints"`
	Recommendations []string               `json:"recommendations"`
}

type HALoadBalancer struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Domain   string `json:"domain"`
	Status   string `json:"status"`
	Enabled  bool   `json:"enabled"`
	Backends int    `json:"backends"`
}

type HANode struct {
	ID         uint    `json:"id"`
	Name       string  `json:"name"`
	Host       string  `json:"host"`
	Role       string  `json:"role"`
	Status     string  `json:"status"`
	IsLocal    bool    `json:"is_local"`
	CPUPercent float64 `json:"cpu_percent"`
	MemPercent float64 `json:"mem_percent"`
}

type HAReplicationHint struct {
	MasterNode string `json:"master_node"`
	SlaveNode  string `json:"slave_node"`
	Role       string `json:"role"`
	Status     string `json:"status"`
}

func (s *Service) GetHAStatus() (HAStatus, error) {
	out := HAStatus{
		Recommendations: []string{},
		Replication:     []HAReplicationHint{},
	}
	if s.cluster == nil {
		out.Recommendations = append(out.Recommendations, "集群服务未初始化")
		return out, nil
	}
	ov, err := s.cluster.Overview()
	if err != nil {
		return out, err
	}
	out.NodeTotal = ov.NodeTotal
	out.NodeOnline = ov.NodeOnline
	out.LBTotal = ov.LBTotal
	out.LBActive = ov.LBActive
	out.BackendTotal = ov.BackendTotal

	var nodes []models.ClusterNode
	s.db.Order("is_local desc, id asc").Find(&nodes)
	for _, n := range nodes {
		out.Nodes = append(out.Nodes, HANode{
			ID: n.ID, Name: n.Name, Host: n.Host, Role: n.Role,
			Status: n.Status, IsLocal: n.IsLocal, CPUPercent: n.CPUPercent, MemPercent: n.MemPercent,
		})
		if n.ProvisionRole == "db_master" || n.ProvisionRole == "db_slave" {
			status := n.ProvisionStatus
			if status == "" {
				status = "none"
			}
			hint := HAReplicationHint{Role: n.ProvisionRole, Status: status}
			if n.ProvisionRole == "db_master" {
				hint.MasterNode = n.Name
			} else {
				hint.SlaveNode = n.Name
			}
			out.Replication = append(out.Replication, hint)
		}
	}

	var lbs []models.LoadBalancer
	s.db.Preload("Backends").Find(&lbs)
	for _, lb := range lbs {
		out.LoadBalancers = append(out.LoadBalancers, HALoadBalancer{
			ID: lb.ID, Name: lb.Name, Domain: lb.Domain, Status: lb.Status,
			Enabled: lb.Enabled, Backends: len(lb.Backends),
		})
	}

	out.Healthy = out.NodeOnline > 0
	if out.NodeTotal > 1 && out.NodeOnline < out.NodeTotal {
		out.Recommendations = append(out.Recommendations,
			fmt.Sprintf("%d/%d 节点离线，请检查网络与 Agent 连接", out.NodeTotal-out.NodeOnline, out.NodeTotal))
		out.Healthy = false
	}
	if out.LBTotal > 0 && out.LBActive == 0 {
		out.Recommendations = append(out.Recommendations, "已配置负载均衡但无活跃实例，请检查 Nginx 应用状态")
		out.Healthy = false
	}
	if out.NodeTotal == 1 {
		out.Recommendations = append(out.Recommendations, "当前为单节点部署，建议添加 Worker 节点实现高可用")
	}
	if len(out.Replication) == 0 && out.NodeTotal > 1 {
		out.Recommendations = append(out.Recommendations, "未检测到 MySQL 主从复制节点，数据库层存在单点风险")
	}
	for _, lb := range out.LoadBalancers {
		if lb.Enabled && lb.Status != "active" && !strings.EqualFold(lb.Status, "up") {
			out.Recommendations = append(out.Recommendations, fmt.Sprintf("负载均衡 %s 未处于活跃状态", lb.Name))
			out.Healthy = false
		}
	}

	out.Grade = haGrade(out)
	return out, nil
}

func haGrade(h HAStatus) string {
	if h.NodeTotal == 0 {
		return "F"
	}
	score := 0
	if h.NodeOnline == h.NodeTotal {
		score += 40
	} else if h.NodeOnline > 0 {
		score += 20
	}
	if h.LBActive > 0 {
		score += 30
	} else if h.LBTotal == 0 && h.NodeTotal == 1 {
		score += 15
	}
	if len(h.Replication) > 0 {
		score += 20
	}
	if h.Healthy {
		score += 10
	}
	switch {
	case score >= 85:
		return "A"
	case score >= 70:
		return "B"
	case score >= 55:
		return "C"
	case score >= 40:
		return "D"
	default:
		return "F"
	}
}
