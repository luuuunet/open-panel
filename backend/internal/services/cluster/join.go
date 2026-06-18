package cluster

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

type AgentRegisterRequest struct {
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Name          string `json:"name"`
	Hostname      string `json:"hostname"`
	Role          string `json:"role"`
	ProvisionRole string `json:"provision_role"`
	WebsiteHost   string `json:"website_host"`
	WebsitePort   int    `json:"website_port"`
	SSHHost       string `json:"ssh_host"`
	SSHPort       int    `json:"ssh_port"`
	SSHUser       string `json:"ssh_user"`
	SSHPassword   string `json:"ssh_password"`
	AutoProvision bool   `json:"auto_provision"`
}

type AgentRegisterResult struct {
	NodeID      uint   `json:"node_id"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	WorkflowWired bool `json:"workflow_wired"`
	Provisioned   bool `json:"provisioned"`
}

type JoinInfo struct {
	MasterURL   string            `json:"master_url"`
	APIBase     string            `json:"api_base"`
	Token       string            `json:"token"`
	Commands    map[string]string `json:"commands"`
	ScriptURL   string            `json:"script_url"`
}

func (s *Service) PanelAPIBase(requestHost string) string {
	all, _ := s.settings.GetAll()
	port := 8888
	if p := all["panel_port"]; p != "" {
		fmt.Sscanf(p, "%d", &port)
	}
	host := strings.TrimSpace(all["server_public_ip"])
	if host == "" {
		host = strings.TrimSpace(requestHost)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	sp := strings.Trim(all["panel_safe_path"], "/")
	base := fmt.Sprintf("http://%s:%d", host, port)
	if sp != "" {
		base += "/" + sp
	}
	return base + "/api/v1"
}

func (s *Service) JoinInfo(requestHost string) (*JoinInfo, error) {
	tok, err := s.AgentToken()
	if err != nil {
		return nil, err
	}
	apiBase := s.PanelAPIBase(requestHost)
	masterURL := strings.TrimSuffix(apiBase, "/api/v1")
	scriptBase := apiBase + "/cluster/agent/join.sh"
	info := &JoinInfo{
		MasterURL: masterURL,
		APIBase:   apiBase,
		Token:     tok,
		ScriptURL: scriptBase,
		Commands:  map[string]string{},
	}
	for _, role := range []string{"worker", "lb_backend", "db_slave", "db_master"} {
		info.Commands[role] = fmt.Sprintf(
			`curl -fsSL "%s?role=%s&token=%s" | bash`,
			scriptBase, role, tok,
		)
	}
	return info, nil
}

func (s *Service) GenerateJoinScript(role, token, apiBase string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		role = "worker"
	}
	provisionRole := role
	if role == "worker" {
		provisionRole = "worker"
	}
	apiBase = strings.TrimSuffix(apiBase, "/")
	script := fmt.Sprintf(`#!/bin/bash
# OWPanel cluster join script — role: %s
set -e
API_BASE="%s"
TOKEN="%s"
ROLE="%s"
PROVISION_ROLE="%s"

detect_ip() {
  for cmd in "curl -fsSL --max-time 3 ifconfig.me" "curl -fsSL --max-time 3 icanhazip.com" "hostname -I"; do
    ip=$(eval "$cmd" 2>/dev/null | awk '{print $1}' | head -1)
    if [ -n "$ip" ] && [ "$ip" != "127.0.0.1" ]; then echo "$ip"; return; fi
  done
  hostname -I 2>/dev/null | awk '{print $1}'
}

HOST="${JOIN_HOST:-$(detect_ip)}"
HOSTNAME="$(hostname -f 2>/dev/null || hostname)"
NAME="${JOIN_NAME:-$HOSTNAME}"

echo "[owpanel] Joining cluster as $PROVISION_ROLE ($HOST)..."

# Optional local bootstrap before register
case "$PROVISION_ROLE" in
  worker|lb_backend)
    if ! command -v nginx >/dev/null 2>&1; then
      echo "[owpanel] Installing nginx..."
      export DEBIAN_FRONTEND=noninteractive
      if command -v apt-get >/dev/null 2>&1; then apt-get update -qq && apt-get install -y nginx curl
      elif command -v yum >/dev/null 2>&1; then yum install -y nginx curl; fi
    fi
    mkdir -p /var/www/owpanel-backend
    echo 'OWPanel LB Backend OK' > /var/www/owpanel-backend/index.html
    ;;
  db_slave|db_master)
    if ! command -v mysql >/dev/null 2>&1; then
      echo "[owpanel] Installing MySQL..."
      export DEBIAN_FRONTEND=noninteractive
      if command -v apt-get >/dev/null 2>&1; then apt-get update -qq && apt-get install -y mysql-server curl
      elif command -v yum >/dev/null 2>&1; then yum install -y mysql-server || yum install -y mariadb-server; fi
    fi
    ;;
esac

PAYLOAD=$(cat <<EOF
{"host":"$HOST","name":"$NAME","hostname":"$HOSTNAME","role":"worker","provision_role":"$PROVISION_ROLE","website_host":"$HOST","website_port":80,"auto_provision":true}
EOF
)

HTTP_CODE=$(curl -fsSL -w "%%{http_code}" -o /tmp/op-join-resp.json \
  -X POST "$API_BASE/cluster/agent/register" \
  -H "Content-Type: application/json" \
  -H "X-Cluster-Token: $TOKEN" \
  -d "$PAYLOAD" 2>/dev/null || echo "000")

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
  echo "[owpanel] Registered successfully:"
  cat /tmp/op-join-resp.json
  echo ""
  echo "[owpanel] Done. Return to master panel → 可视化工作流 → 运行流水线"
else
  echo "[owpanel] Register failed (HTTP $HTTP_CODE):"
  cat /tmp/op-join-resp.json 2>/dev/null || true
  exit 1
fi
`, role, apiBase, token, role, provisionRole)
	return script
}

func (s *Service) RegisterAgentNode(req AgentRegisterRequest) (*AgentRegisterResult, error) {
	host := strings.TrimSpace(req.Host)
	if host == "" {
		return nil, fmt.Errorf("host required")
	}
	provisionRole := strings.TrimSpace(req.ProvisionRole)
	if provisionRole == "" {
		provisionRole = "worker"
	}
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "worker"
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		if req.Hostname != "" {
			name = req.Hostname
		} else {
			name = host
		}
	}
	websitePort := req.WebsitePort
	if websitePort <= 0 {
		websitePort = 80
	}
	websiteHost := strings.TrimSpace(req.WebsiteHost)
	if websiteHost == "" {
		websiteHost = host
	}

	var node models.ClusterNode
	err := s.db.Where("host = ? AND is_local = ?", host, false).First(&node).Error
	now := time.Now()
	if err != nil {
		masterTok, _ := s.AgentToken()
		node = models.ClusterNode{
			Name: name, Host: host, Port: 8888, Role: role,
			ProvisionRole: provisionRole, WebsiteHost: websiteHost, WebsitePort: websitePort,
			Hostname: req.Hostname, Status: "online", LastSeenAt: &now,
			AgentToken: masterTok,
		}
		if req.Port > 0 {
			node.Port = req.Port
		}
		if strings.TrimSpace(req.SSHPassword) != "" {
			node.SSHHost = strings.TrimSpace(req.SSHHost)
			if node.SSHHost == "" {
				node.SSHHost = host
			}
			node.SSHPort = req.SSHPort
			if node.SSHPort <= 0 {
				node.SSHPort = 22
			}
			node.SSHUser = strings.TrimSpace(req.SSHUser)
			if node.SSHUser == "" {
				node.SSHUser = "root"
			}
			node.SSHPassword = strings.TrimSpace(req.SSHPassword)
		}
		if err := s.db.Create(&node).Error; err != nil {
			return nil, err
		}
	} else {
		updates := map[string]interface{}{
			"name": name, "hostname": req.Hostname, "provision_role": provisionRole,
			"website_host": websiteHost, "website_port": websitePort,
			"status": "online", "last_seen_at": now, "last_error": "",
		}
		if req.Port > 0 {
			updates["port"] = req.Port
		}
		if strings.TrimSpace(req.SSHPassword) != "" {
			updates["ssh_host"] = strings.TrimSpace(req.SSHHost)
			if strings.TrimSpace(req.SSHHost) == "" {
				updates["ssh_host"] = host
			}
			if req.SSHPort > 0 {
				updates["ssh_port"] = req.SSHPort
			}
			updates["ssh_user"] = strings.TrimSpace(req.SSHUser)
			if strings.TrimSpace(req.SSHUser) == "" {
				updates["ssh_user"] = "root"
			}
			updates["ssh_password"] = strings.TrimSpace(req.SSHPassword)
		}
		s.db.Model(&node).Updates(updates)
	}
	if fresh, err := s.GetNode(node.ID); err == nil && fresh != nil {
		node = *fresh
	}

	result := &AgentRegisterResult{
		NodeID:        node.ID,
		Status:        "ok",
		Message:       fmt.Sprintf("节点 %s 已接入集群", node.Name),
		WorkflowWired: true,
	}

	autoProv := req.AutoProvision
	if autoProv && nodeHasSSH(&node) {
		if _, err := s.ProvisionNode(node.ID); err == nil {
			result.Provisioned = true
			result.Message += "，环境已自动搭建"
		}
	} else if autoProv && !nodeHasSSH(&node) {
		s.db.Model(&node).Updates(map[string]interface{}{
			"provision_status": "ready",
			"provision_log":    "join script local bootstrap",
		})
		result.Provisioned = true
	}

	applyMsg := s.AutoApplyAfterJoin(node.ID, provisionRole)
	if applyMsg != "" {
		result.Message += "；" + applyMsg
	}

	return result, nil
}

func (s *Service) integrateNodeIntoWorkflow(nodeID uint, provisionRole string) bool {
	_, graph, err := s.GetWorkflow()
	if err != nil || graph == nil {
		return false
	}
	node, err := s.GetNode(nodeID)
	if err != nil {
		return false
	}

	existing := false
	for _, n := range graph.Nodes {
		if n.RefID == nodeID {
			existing = true
			break
		}
	}
	if !existing {
		typ := flowTypeFromProvision(provisionRole)
		x := 120.0
		for _, n := range graph.Nodes {
			if n.X+180 > x {
				x = n.X + 180
			}
		}
		graph.Nodes = append(graph.Nodes, FlowNode{
			ID: fmt.Sprintf("node-%d", nodeID), Type: typ,
			Label: node.Name, X: x, Y: 200,
			RefID: nodeID, Status: node.Status,
		})
	}

	nodeFlowID := fmt.Sprintf("node-%d", nodeID)
	wired := false

	switch provisionRole {
	case "worker", "lb_backend":
		for _, n := range graph.Nodes {
			if n.Type != "lb" {
				continue
			}
			already := false
			for _, e := range graph.Edges {
				if e.From == n.ID && e.To == nodeFlowID && e.Kind == "routes" {
					already = true
					break
				}
			}
			if !already {
				graph.Edges = append(graph.Edges, FlowEdge{
					ID: fmt.Sprintf("e-lb-%s-%d", n.ID, nodeID),
					From: n.ID, To: nodeFlowID, Kind: "routes",
				})
				wired = true
			}
		}
	case "db_slave":
		for _, n := range graph.Nodes {
			if n.Type != "db_master" {
				continue
			}
			already := false
			for _, e := range graph.Edges {
				if e.From == n.ID && e.To == nodeFlowID && e.Kind == "replicates" {
					already = true
					break
				}
			}
			if !already {
				graph.Edges = append(graph.Edges, FlowEdge{
					ID: fmt.Sprintf("e-repl-%s-%d", n.ID, nodeID),
					From: n.ID, To: nodeFlowID, Kind: "replicates",
				})
				wired = true
			}
		}
	}

	_, _ = s.SaveWorkflow(graph)
	return wired || !existing
}

func flowTypeFromProvision(role string) string {
	switch role {
	case "db_master":
		return "db_master"
	case "db_slave":
		return "db_slave"
	case "worker":
		return "worker"
	default:
		return "worker"
	}
}

func (s *Service) DetectPublicIP() string {
	all, _ := s.settings.GetAll()
	if ip := strings.TrimSpace(all["server_public_ip"]); ip != "" {
		return ip
	}
	out, err := exec.Command("curl", "-fsSL", "--max-time", "3", "ifconfig.me").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}
