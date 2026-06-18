package aichat

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type CacheRuleContext struct {
	Sites         []CacheRuleSite  `json:"sites"`
	ExistingRules []CacheRuleBrief `json:"existing_rules"`
	FormDraft     *CacheRuleBrief  `json:"form_draft,omitempty"`
	GlobalConfig  map[string]any   `json:"global_config,omitempty"`
}

type CacheRuleSite struct {
	ID     uint   `json:"id"`
	Domain string `json:"domain"`
}

type CacheRuleBrief struct {
	Name       string `json:"name"`
	Pattern    string `json:"pattern"`
	Action     string `json:"action"`
	TTLMinutes int    `json:"ttl_minutes"`
	WebsiteID  uint   `json:"website_id"`
	Priority   int    `json:"priority"`
	Enabled    bool   `json:"enabled"`
}

type CacheRuleChatRequest struct {
	Message string           `json:"message"`
	History []Message        `json:"history"`
	Context CacheRuleContext `json:"context"`
}

type CacheRuleChatResult struct {
	Reply          string          `json:"reply"`
	SuggestedRule  *CacheRuleBrief `json:"suggested_rule,omitempty"`
}

var jsonBlockRe = regexp.MustCompile(`(?s)` + "```(?:json)?\\s*(\\{.*?\\})\\s*```")

func (s *Service) CacheRuleChat(req CacheRuleChatRequest) (*CacheRuleChatResult, error) {
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
	if cfg.Model == "" {
		return nil, fmt.Errorf("AI 模型未配置")
	}
	if strings.TrimSpace(req.Message) == "" {
		return nil, fmt.Errorf("message is required")
	}

	systemPrompt := buildCacheRuleSystemPrompt(req.Context)
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

	result := &CacheRuleChatResult{Reply: strings.TrimSpace(reply)}
	if rule := extractSuggestedRule(reply); rule != nil {
		result.SuggestedRule = rule
	}
	return result, nil
}

func buildCacheRuleSystemPrompt(ctx CacheRuleContext) string {
	var b strings.Builder
	b.WriteString(`You are a CDN cache expert for OWPanel (Nginx proxy_cache / fastcgi_cache), similar to Cloudflare Page Rules.

Help the user write cache rules. Rules match request URI using Nginx map regex (~* pattern).

Supported actions:
- bypass: skip CDN cache for matching paths
- cache: force cache (override cookie/path bypass) for matching paths; use ttl_minutes for custom TTL via dedicated location blocks

Pattern tips:
- Use Nginx regex fragments without delimiters, e.g. /wp-admin|/api/|\.json$
- ^/admin for prefix, login for substring match
- Escape dots: \.css not .css when matching extensions

Priority: lower number = higher priority (1-9999). Default 100.
Website scope: website_id 0 = global, otherwise use site id from the list.

When you propose a concrete rule, ALWAYS include a JSON object in a markdown code block:
` + "```json\n" + `{
  "name": "short label",
  "pattern": "/api/",
  "action": "bypass",
  "ttl_minutes": 0,
  "website_id": 0,
  "priority": 100,
  "enabled": true
}
` + "```\n")
	b.WriteString("Also explain in plain language (same language as user).\n\n")

	b.WriteString("Sites:\n")
	if len(ctx.Sites) == 0 {
		b.WriteString("- (none)\n")
	} else {
		for _, site := range ctx.Sites {
			b.WriteString(fmt.Sprintf("- id=%d domain=%s\n", site.ID, site.Domain))
		}
	}

	b.WriteString("\nExisting rules:\n")
	if len(ctx.ExistingRules) == 0 {
		b.WriteString("- (none)\n")
	} else {
		for _, r := range ctx.ExistingRules {
			b.WriteString(fmt.Sprintf("- [%d] %s pattern=%s action=%s scope=%d priority=%d\n",
				r.WebsiteID, r.Name, r.Pattern, r.Action, r.WebsiteID, r.Priority))
		}
	}

	if ctx.FormDraft != nil {
		raw, _ := json.Marshal(ctx.FormDraft)
		b.WriteString("\nCurrent form draft:\n")
		b.Write(raw)
		b.WriteString("\n")
	}

	if len(ctx.GlobalConfig) > 0 {
		raw, _ := json.Marshal(ctx.GlobalConfig)
		b.WriteString("\nGlobal cache config:\n")
		b.Write(raw)
		b.WriteString("\n")
	}

	return b.String()
}

func extractSuggestedRule(text string) *CacheRuleBrief {
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
	var rule CacheRuleBrief
	if err := json.Unmarshal([]byte(raw), &rule); err != nil {
		return nil
	}
	if strings.TrimSpace(rule.Pattern) == "" {
		return nil
	}
	if rule.Action == "" {
		rule.Action = "bypass"
	}
	if rule.Priority <= 0 {
		rule.Priority = 100
	}
	if rule.Name == "" {
		rule.Name = "AI Rule"
	}
	rule.Enabled = true
	return &rule
}
