package aichat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/services/settings"
)

type Service struct {
	settings *settings.Service
}

type Message struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"`
}

type ChatRequest struct {
	Path     string    `json:"path"`
	Content  string    `json:"content"`
	Message  string    `json:"message"`
	History  []Message `json:"history"`
}

type ChatResult struct {
	Reply            string `json:"reply"`
	SuggestedContent string `json:"suggested_content,omitempty"`
}

func NewService(settingsSvc *settings.Service) *Service {
	return &Service{settings: settingsSvc}
}

func (s *Service) Chat(req ChatRequest) (*ChatResult, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, fmt.Errorf("AI assistant is disabled in panel settings")
	}
	if cfg.APIKey == "" && cfg.Provider != "ollama" && cfg.Provider != "huggingface" {
		return nil, fmt.Errorf("AI API key is not configured")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("AI model is not configured")
	}

	systemPrompt := buildSystemPrompt(req.Path, req.Content)
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

	result := &ChatResult{Reply: reply}
	if code := extractCodeBlock(reply); code != "" {
		result.SuggestedContent = code
	}
	return result, nil
}

type aiConfig struct {
	Enabled  bool
	Provider string
	APIKey   string
	BaseURL  string
	Model    string
}

func (s *Service) loadConfig() (aiConfig, error) {
	all, err := s.settings.GetAll()
	if err != nil {
		return aiConfig{}, err
	}
	cfg := aiConfig{
		Enabled:  all["ai_enabled"] == "true",
		Provider: all["ai_provider"],
		APIKey:   all["ai_api_key"],
		BaseURL:  strings.TrimRight(all["ai_base_url"], "/"),
		Model:    all["ai_model"],
	}
	if cfg.Provider == "" {
		cfg.Provider = "openai"
	}
	cfg.Provider = normalizeProvider(cfg.Provider)
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL(cfg.Provider)
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel(cfg.Provider)
	}
	return cfg, nil
}

func defaultBaseURL(provider string) string {
	switch provider {
	case "claude":
		return "https://api.anthropic.com/v1"
	case "cursor":
		return "https://api.cursor.com"
	case "deepseek":
		return "https://api.deepseek.com/v1"
	case "ollama":
		return "http://127.0.0.1:11434/v1"
	case "huggingface":
		return "http://127.0.0.1:8095/v1"
	case "openai":
		return "https://api.openai.com/v1"
	default:
		return ""
	}
}

func defaultModel(provider string) string {
	switch provider {
	case "claude":
		return "claude-sonnet-4-6"
	case "cursor":
		return "composer-2.5"
	case "deepseek":
		return "deepseek-chat"
	case "ollama":
		return "llama3.2"
	case "huggingface":
		return "Qwen2.5-0.5B-Instruct"
	default:
		return "gpt-4o-mini"
	}
}

func buildSystemPrompt(path, content string) string {
	var b strings.Builder
	b.WriteString("You are an expert programming assistant embedded in a server panel file editor (similar to Cursor IDE).\n")
	b.WriteString("The user is editing a file. Help explain, debug, refactor, or modify the code.\n")
	b.WriteString("When the user asks you to CHANGE the file, you MUST:\n")
	b.WriteString("1. Briefly explain what you changed in plain language.\n")
	b.WriteString("2. Include the COMPLETE updated file in a single markdown fenced code block (not a snippet).\n")
	b.WriteString("3. Preserve unrelated code exactly; do not use placeholders like \"... unchanged ...\".\n")
	b.WriteString("Use the same language as the file when writing code.\n\n")
	b.WriteString(fmt.Sprintf("File path: %s\n\n", path))
	b.WriteString("Current file content:\n```\n")
	if len(content) > 120000 {
		b.WriteString(content[:120000])
		b.WriteString("\n... (truncated)")
	} else {
		b.WriteString(content)
	}
	b.WriteString("\n```\n")
	return b.String()
}

type openAIChatRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (s *Service) callChatAPI(cfg aiConfig, messages []Message) (string, error) {
	cfg.Provider = normalizeProvider(cfg.Provider)

	useVision := messagesHaveImages(messages)
	if isCursorAPI(cfg) {
		if useVision {
			messages = appendVisionFallbackNote(messages)
		}
		return s.callCursorAgent(cfg, messages)
	}
	if isAnthropicAPI(cfg) {
		if useVision {
			return s.callAnthropicMessagesVision(cfg, messages)
		}
		return s.callAnthropicMessages(cfg, messages)
	}

	openAIMsgs := toOpenAIMessages(messages)
	body, _ := json.Marshal(openAIChatRequest{
		Model:    cfg.Model,
		Messages: openAIMsgs,
		Stream:   false,
	})

	url := cfg.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("AI request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var errResp openAIChatResponse
		_ = json.Unmarshal(raw, &errResp)
		msg := strings.TrimSpace(string(raw))
		if errResp.Error != nil && errResp.Error.Message != "" {
			msg = errResp.Error.Message
		}
		if resp.StatusCode == 404 && strings.Contains(msg, "chat/completions") {
			return "", fmt.Errorf("AI API error (%d): %s — %s", resp.StatusCode, msg, chatProviderHint(cfg))
		}
		return "", fmt.Errorf("AI API error (%d): %s", resp.StatusCode, msg)
	}

	var parsed openAIChatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("invalid AI response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("AI returned empty response")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}

var codeBlockRe = regexp.MustCompile("(?s)```(?:[a-zA-Z0-9_-]+)?\n(.*?)```")

func extractCodeBlock(text string) string {
	m := codeBlockRe.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimRight(m[1], "\n")
}
