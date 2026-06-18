package aichat

import (
	"fmt"
	"strings"
)

type LogChatRequest struct {
	Message    string    `json:"message"`
	SourceID   string    `json:"source_id"`
	SourceName string    `json:"source_name"`
	Category   string    `json:"category"`
	Path       string    `json:"path"`
	LogContent string    `json:"log_content"`
	History    []Message `json:"history"`
}

type LogChatResult struct {
	Reply string `json:"reply"`
}

func (s *Service) LogChat(req LogChatRequest) (*LogChatResult, error) {
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

	name := strings.TrimSpace(req.SourceName)
	if name == "" {
		name = req.SourceID
	}
	category := strings.TrimSpace(req.Category)
	if category == "" {
		category = "unknown"
	}
	path := strings.TrimSpace(req.Path)
	logText := strings.TrimSpace(req.LogContent)
	if len(logText) > 80000 {
		logText = logText[len(logText)-80000:]
		logText = "...(truncated)\n" + logText
	}

	system := fmt.Sprintf(`你是 OWPanel 日志分析助手，帮助运维人员读懂服务器日志。
当前日志来源: %s
分类: %s
文件路径: %s

要求:
- 用简洁中文回答
- 指出错误、警告、异常模式及可能原因
- 给出可操作的排查或修复建议
- 若日志正常，简要说明当前状态
- 不要编造日志中不存在的内容

日志内容:
%s`, name, category, path, logText)

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
	return &LogChatResult{Reply: reply}, nil
}
