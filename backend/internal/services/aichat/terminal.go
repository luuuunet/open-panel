package aichat

import (
	"encoding/json"
	"fmt"
	"strings"
)

type TerminalChatRequest struct {
	Message string    `json:"message"`
	Host    string    `json:"host"`
	User    string    `json:"user"`
	History []Message `json:"history"`
}

type TerminalChatResult struct {
	Reply           string `json:"reply"`
	SuggestedCommand string `json:"suggested_command,omitempty"`
}

func (s *Service) TerminalChat(req TerminalChatRequest) (*TerminalChatResult, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, fmt.Errorf("请先在面板设置中启用 AI 助手")
	}
	if cfg.APIKey == "" && cfg.Provider != "ollama" {
		return nil, fmt.Errorf("未配置 AI API Key")
	}

	host := strings.TrimSpace(req.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	user := strings.TrimSpace(req.User)
	if user == "" {
		user = "root"
	}

	system := fmt.Sprintf(`你是 OWPanel 内置 SSH 终端助手，帮助用户管理 Linux 服务器。
当前连接: %s@%s
- 用简洁中文回答运维、Shell、Nginx、Docker、MySQL 等问题
- 若需要执行命令，在回复末尾用单独一行给出建议命令，格式: CMD: 命令内容
- 不要编造无法确认的服务器状态`, user, host)

	messages := []Message{{Role: "system", Content: system}}
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

	result := &TerminalChatResult{Reply: reply}
	if cmd := extractTerminalCommand(reply); cmd != "" {
		result.SuggestedCommand = cmd
	}
	return result, nil
}

func extractTerminalCommand(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "CMD:") {
			return strings.TrimSpace(line[4:])
		}
	}
	if code := extractCodeBlock(text); code != "" {
		lines := strings.Split(strings.TrimSpace(code), "\n")
		if len(lines) == 1 {
			return lines[0]
		}
	}
	return ""
}

func (s *Service) TerminalChatJSON(req TerminalChatRequest) ([]byte, error) {
	r, err := s.TerminalChat(req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(r)
}
