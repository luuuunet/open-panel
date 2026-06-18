package cluster

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

type QuickLBRequest struct {
	Name       string `json:"name"`
	Domain     string `json:"domain"`
	ListenPort int    `json:"listen_port"`
	Algorithm  string `json:"algorithm"`
	NodeIDs    []uint `json:"node_ids"`
	AutoSetup  bool   `json:"auto_setup"`
}

type QuickReplRequest struct {
	MasterNodeID uint   `json:"master_node_id"`
	SlaveNodeID  uint   `json:"slave_node_id"`
	ReplUser     string `json:"repl_user"`
	DBName       string `json:"db_name"`
	AutoSetup    bool   `json:"auto_setup"`
}

type QuickResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Log     string `json:"log,omitempty"`
	LBID    uint   `json:"lb_id,omitempty"`
	Script  string `json:"script_path,omitempty"`
}

func (s *Service) QuickCreateLB(req QuickLBRequest) (*QuickResult, error) {
	if len(req.NodeIDs) == 0 {
		return nil, fmt.Errorf("请至少选择一个后端节点")
	}
	domain := strings.TrimSpace(req.Domain)
	if domain == "" {
		return nil, fmt.Errorf("请填写域名")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "LB-" + domain
	}
	port := req.ListenPort
	if port <= 0 {
		port = 80
	}
	algo := strings.TrimSpace(req.Algorithm)
	if algo == "" {
		algo = "round_robin"
	}

	var logLines []string
	if req.AutoSetup {
		for _, id := range req.NodeIDs {
			node, err := s.GetNode(id)
			if err != nil {
				continue
			}
			if node.IsLocal || !nodeHasSSH(node) {
				continue
			}
			if node.ProvisionRole != "lb_backend" && node.ProvisionRole != "worker" {
				s.db.Model(node).Update("provision_role", "lb_backend")
			}
			if node.ProvisionStatus != "ready" {
				logLines = append(logLines, fmt.Sprintf("搭建 %s…", node.Name))
				if _, err := s.ProvisionNode(id); err != nil {
					logLines = append(logLines, fmt.Sprintf("  警告: %s — %s", node.Name, err.Error()))
				} else {
					logLines = append(logLines, fmt.Sprintf("  %s 就绪", node.Name))
				}
			}
		}
	}

	lb, err := s.CreateLoadBalancer(LBRequest{
		Name: name, Domain: domain, ListenPort: port, Algorithm: algo,
		Enabled: true, HealthCheck: true, HealthPath: "/",
	})
	if err != nil {
		return nil, err
	}

	added := 0
	for _, id := range req.NodeIDs {
		if _, err := s.AddBackend(lb.ID, BackendRequest{
			NodeID: id, Weight: 1, Enabled: true,
		}); err == nil {
			added++
		}
	}
	if added == 0 {
		_ = s.DeleteLoadBalancer(lb.ID)
		return nil, fmt.Errorf("未能添加任何后端，请检查节点是否存在")
	}

	if err := s.ApplyLoadBalancer(lb.ID); err != nil {
		return &QuickResult{
			Status: "failed", Message: err.Error(), Log: strings.Join(logLines, "\n"), LBID: lb.ID,
		}, err
	}
	_ = s.settings.Update(map[string]string{"cluster_default_domain": domain})

	msg := fmt.Sprintf("负载均衡已创建并应用（%d 个后端）", added)
	logLines = append(logLines, msg)
	return &QuickResult{
		Status: "ok", Message: msg, Log: strings.Join(logLines, "\n"), LBID: lb.ID,
	}, nil
}

func (s *Service) QuickReplication(req QuickReplRequest) (*QuickResult, error) {
	if req.MasterNodeID == 0 || req.SlaveNodeID == 0 {
		return nil, fmt.Errorf("请选择主库和从库节点")
	}
	if req.MasterNodeID == req.SlaveNodeID {
		return nil, fmt.Errorf("主库和从库不能是同一节点")
	}
	masterNode, err := s.GetNode(req.MasterNodeID)
	if err != nil {
		return nil, fmt.Errorf("主库节点不存在")
	}
	slaveNode, err := s.GetNode(req.SlaveNodeID)
	if err != nil {
		return nil, fmt.Errorf("从库节点不存在")
	}

	var logLines []string
	if req.AutoSetup {
		for _, pair := range []struct {
			node *models.ClusterNode
			role string
		}{
			{masterNode, "db_master"},
			{slaveNode, "db_slave"},
		} {
			if pair.node.IsLocal || !nodeHasSSH(pair.node) {
				logLines = append(logLines, fmt.Sprintf("%s: 跳过 SSH 搭建（本机或无 SSH）", pair.node.Name))
				continue
			}
			s.db.Model(pair.node).Update("provision_role", pair.role)
			if pair.node.ProvisionStatus != "ready" {
				if _, err := s.ProvisionNode(pair.node.ID); err != nil {
					logLines = append(logLines, fmt.Sprintf("%s 搭建失败: %s", pair.node.Name, err.Error()))
				} else {
					logLines = append(logLines, fmt.Sprintf("%s MySQL 已安装", pair.node.Name))
				}
			}
		}
	}

	replUser := strings.TrimSpace(req.ReplUser)
	if replUser == "" {
		replUser = "repl"
	}
	dbName := strings.TrimSpace(req.DBName)
	if dbName == "" {
		dbName = "app_db"
	}

	masterFN := &FlowNode{
		ID: "quick-master", Type: "db_master", Label: masterNode.Name, RefID: masterNode.ID,
		Config: map[string]interface{}{"repl_user": replUser, "db_name": dbName},
	}
	slaveFN := &FlowNode{
		ID: "quick-slave", Type: "db_slave", Label: slaveNode.Name, RefID: slaveNode.ID,
		Config: map[string]interface{}{"db_name": dbName},
	}

	script, err := s.generateReplicationScript(masterFN, slaveFN)
	if err != nil {
		return nil, err
	}
	path, err := s.writeReplicationScript(masterFN, slaveFN, script)
	if err != nil {
		return nil, err
	}

	msg := fmt.Sprintf("主从脚本已生成: %s", path)
	logLines = append(logLines, msg)

	if req.AutoSetup {
		applyLog, applyErr := s.ApplyReplicationViaSSH(masterFN, slaveFN)
		if applyLog != "" {
			logLines = append(logLines, applyLog)
		}
		if applyErr != nil {
			logLines = append(logLines, "SSH 应用警告: "+applyErr.Error())
		} else if strings.Contains(applyLog, "主从复制已应用") {
			msg = fmt.Sprintf("主从复制已通过 SSH 配置完成")
		}
	} else {
		_ = s.ensureReplicationDBRecords(masterFN, slaveFN)
	}

	return &QuickResult{
		Status: "ok", Message: msg, Log: strings.Join(logLines, "\n"), Script: path,
	}, nil
}
