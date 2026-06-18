package aichat

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/website"
)

type SiteProjectChatRequest struct {
	Message   string    `json:"message"`
	Images    []string  `json:"images,omitempty"`
	Domain    string    `json:"domain"`
	RootPath  string    `json:"root_path"`
	FocusPath string    `json:"focus_path,omitempty"`
	Scope     string    `json:"scope"` // file | project
	History   []Message `json:"history"`
	Snapshot  *website.ProjectSnapshot
}

type SiteProjectChatResult struct {
	Reply      string          `json:"reply"`
	Summary    string          `json:"summary,omitempty"`
	FileWrites []FileWriteSpec `json:"file_writes,omitempty"`
}

func (s *Service) SiteProjectChat(req SiteProjectChatRequest) (*SiteProjectChatResult, error) {
	if err := s.EnsureConfigured(); err != nil {
		return nil, err
	}
	if req.Snapshot == nil {
		return nil, fmt.Errorf("项目快照为空")
	}
	if strings.TrimSpace(req.Message) == "" && len(req.Images) == 0 {
		return nil, fmt.Errorf("请输入消息或上传图片")
	}
	images, err := normalizeChatImages(req.Images)
	if err != nil {
		return nil, err
	}

	system := buildSiteProjectSystemPrompt(req)
	messages := []Message{{Role: "system", Content: system}}
	for _, h := range req.History {
		if h.Role == "user" || h.Role == "assistant" {
			// 历史消息不再重复上传图片，减小请求体、加快处理
			h.Images = nil
			messages = append(messages, h)
		}
	}
	messages = append(messages, Message{Role: "user", Content: req.Message, Images: images})

	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	reply, err := s.callChatAPI(cfg, messages)
	if err != nil {
		return nil, err
	}
	return parseSiteProjectReply(reply), nil
}

func buildSiteProjectSystemPrompt(req SiteProjectChatRequest) string {
	snap := req.Snapshot
	var b strings.Builder
	b.WriteString("你是 OWPanel 网站项目 AI 助手，可以分析并修改整个网站项目中的多个文件（主题、样式、模板、前端代码等）。\n")
	b.WriteString(fmt.Sprintf("站点域名: %s\n网站根目录: %s\n项目类型: %s\nPHP: %s\n",
		req.Domain, req.RootPath, snap.ProjectType, snap.PhpVersion))
	if req.FocusPath != "" && req.Scope == "file" {
		b.WriteString(fmt.Sprintf("用户当前正在编辑的文件（相对路径）: %s\n", req.FocusPath))
	}
	b.WriteString("\n要求:\n")
	b.WriteString("- 用简洁中文回答\n")
	b.WriteString("- 可跨多个文件修改主题、配色、布局、CSS、模板、组件代码\n")
	b.WriteString("- 修改文件时必须在回复末尾附加 JSON（可用 ```json 包裹），格式:\n")
	b.WriteString(`{"summary":"一句话","file_writes":[{"relative_path":"相对网站根目录的路径","content":"完整文件内容"}]}` + "\n")
	b.WriteString("- file_writes 中每个文件必须是完整内容，不要用 ... 省略\n")
	b.WriteString("- 路径相对网站根目录，禁止 .. 与绝对路径\n")
	b.WriteString("- 禁止修改 wp-config.php、.env、database.php 等敏感配置\n")
	b.WriteString("- 若仅咨询无需改文件，不要输出 file_writes JSON\n")
	b.WriteString("- 用户可能上传设计稿/截图作为参考，请结合图片理解配色、布局与视觉风格后再给出修改方案\n")
	b.WriteString("- 优先修改 themes/、assets/、css/、style.css、theme.json、*.css、前端 app 目录等主题相关文件\n\n")

	// 只列出已读取内容的文件，避免把数百条路径塞进 prompt
	contentPaths := make([]string, 0, len(snap.FileContents))
	for p := range snap.FileContents {
		contentPaths = append(contentPaths, p)
	}
	sort.Strings(contentPaths)

	b.WriteString(fmt.Sprintf("项目规模: 约 %d 个文件；以下为已加载的关键文件（共 %d 个）:\n",
		len(snap.FileList), len(contentPaths)))
	limit := len(contentPaths)
	if limit > 48 {
		limit = 48
	}
	for i := 0; i < limit; i++ {
		b.WriteString("- ")
		b.WriteString(contentPaths[i])
		b.WriteByte('\n')
	}
	if len(contentPaths) > limit {
		b.WriteString(fmt.Sprintf("... 另有 %d 个已加载文件\n", len(contentPaths)-limit))
	}

	const maxPromptChars = 42000
	const maxFileInPrompt = 6000
	b.WriteString("\n关键文件内容:\n")
	for _, path := range contentPaths {
		if b.Len() >= maxPromptChars {
			b.WriteString("\n... (上下文已达上限，其余文件未展开)\n")
			break
		}
		content := snap.FileContents[path]
		b.WriteString(fmt.Sprintf("\n--- %s ---\n", path))
		if len(content) > maxFileInPrompt {
			b.WriteString(content[:maxFileInPrompt])
			b.WriteString("\n... (truncated)")
		} else {
			b.WriteString(content)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func parseSiteProjectReply(text string) *SiteProjectChatResult {
	res := &SiteProjectChatResult{Reply: strings.TrimSpace(text)}
	writes, summary := extractProjectFileWrites(text)
	if len(writes) > 0 {
		res.FileWrites = writes
		res.Summary = summary
		res.Reply = stripProjectJSONBlock(text)
		if res.Reply == "" {
			res.Reply = summary
		}
	}
	return res
}

func extractProjectFileWrites(text string) ([]FileWriteSpec, string) {
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
		return nil, ""
	}
	var payload struct {
		Summary    string          `json:"summary"`
		FileWrites []FileWriteSpec `json:"file_writes"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, ""
	}
	return filterFileWrites(payload.FileWrites), payload.Summary
}

func stripProjectJSONBlock(text string) string {
	if loc := jsonBlockRe.FindStringIndex(text); loc != nil {
		text = text[:loc[0]] + text[loc[1]:]
	} else {
		start := strings.Index(text, "{")
		end := strings.LastIndex(text, "}")
		if start >= 0 && end > start && strings.Contains(text[start:end+1], "file_writes") {
			text = text[:start] + text[end+1:]
		}
	}
	return strings.TrimSpace(text)
}
