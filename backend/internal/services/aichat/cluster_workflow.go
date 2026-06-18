package aichat

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ClusterWorkflowContext struct {
	Nodes    []ClusterNodeBrief `json:"nodes"`
	Graph    interface{}        `json:"graph,omitempty"`
	Balancers []map[string]interface{} `json:"balancers,omitempty"`
}

type ClusterNodeBrief struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Role     string `json:"role"`
	IsLocal  bool   `json:"is_local"`
	Status   string `json:"status"`
}

type ClusterWorkflowChatRequest struct {
	Message string                 `json:"message"`
	History []Message              `json:"history"`
	Context ClusterWorkflowContext `json:"context"`
}

type ClusterWorkflowChatResult struct {
	Reply           string      `json:"reply"`
	SuggestedGraph  interface{} `json:"suggested_graph,omitempty"`
}

func (s *Service) ClusterWorkflowChat(req ClusterWorkflowChatRequest) (*ClusterWorkflowChatResult, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, fmt.Errorf("请先在面板设置中启用 AI 助手并配置 API Key")
	}
	if cfg.APIKey == "" && cfg.Provider != "ollama" {
		return nil, fmt.Errorf("AI API Key 未配置")
	}
	if strings.TrimSpace(req.Message) == "" {
		return nil, fmt.Errorf("message is required")
	}

	systemPrompt := buildClusterWorkflowSystemPrompt(req.Context)
	messages := []Message{{Role: "system", Content: systemPrompt}}
	for _, h := range req.History {
		if h.Role == "user" || h.Role == "assistant" {
			messages = append(messages, h)
		}
	}
	messages = append(messages, Message{Role: "user", Content: req.Message})

	reply, err := s.callChatAPI(cfg, messages)
	if err != nil {
		return nil, err
	}

	result := &ClusterWorkflowChatResult{Reply: strings.TrimSpace(reply)}
	if g := extractSuggestedGraph(reply); g != nil {
		result.SuggestedGraph = g
	}
	return result, nil
}

func buildClusterWorkflowSystemPrompt(ctx ClusterWorkflowContext) string {
	var b strings.Builder
	b.WriteString(`You are a Kubernetes-style cluster orchestration expert for OWPanel.

Help users design visual workflow graphs for multi-server automation:
- Node types: master (panel master), worker (panel worker), lb (HTTP load balancer), db_master, db_slave, web_sync
- Edge kinds: routes (lb -> worker), replicates (db_master -> db_slave), manages (master -> worker)

When proposing a workflow, include JSON in a markdown code block:
` + "```json\n" + `{
  "nodes": [
    {"id": "node-1", "type": "master", "label": "Master", "x": 120, "y": 120, "ref_id": 1},
    {"id": "lb-1", "type": "lb", "label": "Web LB", "x": 400, "y": 40, "config": {"domain": "app.example.com", "listen_port": 80}},
    {"id": "db-master", "type": "db_master", "label": "MySQL Master", "x": 120, "y": 380, "config": {"repl_user": "repl"}},
    {"id": "db-slave", "type": "db_slave", "label": "MySQL Slave", "x": 380, "y": 380}
  ],
  "edges": [
    {"id": "e1", "from": "lb-1", "to": "node-2", "kind": "routes"},
    {"id": "e2", "from": "db-master", "to": "db-slave", "kind": "replicates"}
  ]
}
` + "```\n")
	b.WriteString("Explain in the user's language. ref_id should match cluster node id when linking panel nodes.\n\n")

	b.WriteString("Cluster nodes:\n")
	for _, n := range ctx.Nodes {
		b.WriteString(fmt.Sprintf("- id=%d name=%s host=%s role=%s local=%v status=%s\n",
			n.ID, n.Name, n.Host, n.Role, n.IsLocal, n.Status))
	}
	if ctx.Graph != nil {
		raw, _ := json.Marshal(ctx.Graph)
		b.WriteString("\nCurrent graph:\n")
		b.Write(raw)
		b.WriteString("\n")
	}
	return b.String()
}

func extractSuggestedGraph(text string) interface{} {
	var raw string
	if m := jsonBlockRe.FindStringSubmatch(text); len(m) >= 2 {
		raw = m[1]
	} else {
		start := strings.Index(text, "{")
		end := strings.LastIndex(text, "}")
		if start >= 0 && end > start {
			raw = text[start : end+1]
		}
	}
	if raw == "" {
		return nil
	}
	var g map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		return nil
	}
	if _, ok := g["nodes"]; !ok {
		return nil
	}
	return g
}
