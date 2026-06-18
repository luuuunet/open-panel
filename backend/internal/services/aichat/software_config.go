package aichat

import (
	"encoding/json"
	"fmt"
	"strings"
)

type SoftwareConfigChatRequest struct {
	Message    string                 `json:"message"`
	AppKey     string                 `json:"app_key"`
	AppName    string                 `json:"app_name"`
	Category   string                 `json:"category"`
	ConfigKind string                 `json:"config_kind"`
	Config     map[string]interface{} `json:"config"`
	RawContent string                 `json:"raw_content"`
	History    []Message              `json:"history"`
}

type SoftwareConfigChatResult struct {
	Reply           string                 `json:"reply"`
	SuggestedConfig map[string]interface{} `json:"suggested_config,omitempty"`
	SuggestedRaw    string                 `json:"suggested_raw,omitempty"`
}

func (s *Service) SoftwareConfigChat(req SoftwareConfigChatRequest) (*SoftwareConfigChatResult, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, fmt.Errorf("请先在面板设置中启用 AI 助手")
	}
	if cfg.APIKey == "" && cfg.Provider != "ollama" && cfg.Provider != "huggingface" {
		return nil, fmt.Errorf("AI API Key 未配置")
	}

	name := strings.TrimSpace(req.AppName)
	if name == "" {
		name = req.AppKey
	}
	configJSON, _ := json.MarshalIndent(req.Config, "", "  ")
	raw := strings.TrimSpace(req.RawContent)
	if len(raw) > 60000 {
		raw = raw[:60000] + "\n...(truncated)"
	}

	system := fmt.Sprintf(`你是 OWPanel 软件商店配置助手，帮助用户修改已安装软件的配置。

软件: %s (%s)
分类: %s
配置类型: %s

要求:
- 用简洁中文回答
- 根据用户需求解释配置项含义与推荐值
- 若用户明确要求修改配置，在回复末尾附加 JSON 代码块，格式如下:

`+"```json\n"+`{
  "suggested_config": { "key": "value" },
  "suggested_raw": "完整配置文件内容（可选，仅当需要改 raw 时）"
}
`+"```\n\n"+`
- suggested_config 只需包含要修改的键
- 不要编造不存在的配置项
- PHP 扩展、disable_functions 等请给出具体函数名/扩展名

当前常用配置:
%s

当前配置文件内容:
%s`, name, req.AppKey, req.Category, req.ConfigKind, string(configJSON), raw)

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
	result := &SoftwareConfigChatResult{Reply: reply}
	if block := extractSoftwareConfigSuggestion(reply); block != nil {
		result.SuggestedConfig = block.SuggestedConfig
		result.SuggestedRaw = block.SuggestedRaw
		result.Reply = stripSoftwareConfigJSONBlock(reply)
	}
	return result, nil
}

type softwareConfigSuggestion struct {
	SuggestedConfig map[string]interface{} `json:"suggested_config"`
	SuggestedRaw    string                 `json:"suggested_raw"`
}

func extractSoftwareConfigSuggestion(text string) *softwareConfigSuggestion {
	if m := jsonBlockRe.FindStringSubmatch(text); len(m) >= 2 {
		var s softwareConfigSuggestion
		if err := json.Unmarshal([]byte(m[1]), &s); err == nil {
			if len(s.SuggestedConfig) > 0 || s.SuggestedRaw != "" {
				return &s
			}
		}
	}
	return nil
}

func stripSoftwareConfigJSONBlock(text string) string {
	if loc := jsonBlockRe.FindStringIndex(text); loc != nil {
		out := strings.TrimSpace(text[:loc[0]] + text[loc[1]:])
		return strings.TrimSuffix(out, "\n")
	}
	return text
}
