package aichat

import (
	"encoding/json"
	"fmt"
	"strings"
)

type SiteLogChatRequest struct {
	Message    string    `json:"message"`
	Domain     string    `json:"domain"`
	RootPath   string    `json:"root_path"`
	AccessLog  string    `json:"access_log"`
	ErrorLog   string    `json:"error_log"`
	AccessPath string    `json:"access_path"`
	ErrorPath  string    `json:"error_path"`
	History    []Message `json:"history"`
}

type FileWriteSpec struct {
	RelativePath string `json:"relative_path"`
	Content      string `json:"content"`
}

type SiteLogRepairPlan struct {
	Summary    string          `json:"summary"`
	Diagnosis  string          `json:"diagnosis"`
	Actions    []string        `json:"actions"`
	FileWrites []FileWriteSpec `json:"file_writes"`
	Confidence string          `json:"confidence"`
}

func (s *Service) SiteLogChat(req SiteLogChatRequest) (*LogChatResult, error) {
	if err := s.EnsureConfigured(); err != nil {
		return nil, err
	}
	access := trimLogForAI(req.AccessLog, 40000)
	errLog := trimLogForAI(req.ErrorLog, 40000)
	domain := strings.TrimSpace(req.Domain)
	root := strings.TrimSpace(req.RootPath)

	system := fmt.Sprintf(`你是 OWPanel 网站日志与运维 AI 助手。
站点域名: %s
网站根目录: %s
访问日志: %s
错误日志: %s

要求:
- 用简洁中文回答
- 分析访问/错误日志中的异常、攻击扫描、404、PHP/Nginx 错误
- 给出可操作的排查与修复建议
- 若用户请求修复，说明应在网站根目录创建/修改哪些文件（相对路径）
- 不要编造日志中不存在的内容
- 可建议创建缺失的 favicon.ico、index 文件、.htaccess 等

访问日志内容:
%s

错误日志内容:
%s`, domain, root, req.AccessPath, req.ErrorPath, access, errLog)

	messages := []Message{{Role: "system", Content: system}}
	for _, h := range req.History {
		if h.Role == "user" || h.Role == "assistant" {
			messages = append(messages, h)
		}
	}
	messages = append(messages, Message{Role: "user", Content: req.Message})

	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	reply, err := s.callChatAPI(cfg, messages)
	if err != nil {
		return nil, err
	}
	return &LogChatResult{Reply: reply}, nil
}

func (s *Service) AnalyzeSiteLogRepair(bundle string) (*SiteLogRepairPlan, error) {
	if err := s.EnsureConfigured(); err != nil {
		return nil, err
	}

	system := `你是 OWPanel 网站运维专家。根据站点诊断与日志，输出 JSON 修复方案（仅 JSON，不要 markdown 代码块外的文字）。

可用 actions（只能从中选择）:
- create_root_dir: 创建网站根目录
- fix_dir_permissions: 修复目录/文件权限
- ensure_index_files: 设置 PHP/HTML 默认索引
- start_php_fpm: 启动 PHP-FPM
- start_site: 启用已停止的站点
- apply_vhost: 重建虚拟主机
- reload_webserver: 重载 Web 服务器

file_writes: 数组，可在网站根目录下创建/覆盖文本文件（相对路径，禁止 .. 与绝对路径）
- 用于修复日志中的 missing file，如 favicon.ico、robots.txt、index.html
- content 为 UTF-8 文本；二进制 favicon 可写空字符串占位
- 单文件 content 不超过 8192 字符
- 禁止删除或覆盖 wp-config.php、.env 等敏感配置

JSON 格式:
{"summary":"","diagnosis":"","actions":[],"file_writes":[{"relative_path":"favicon.ico","content":""}],"confidence":"medium"}`

	messages := []Message{
		{Role: "system", Content: system},
		{Role: "user", Content: "请分析以下站点诊断与日志，给出修复 actions 与 file_writes:\n\n" + bundle},
	}
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	reply, err := s.callChatAPI(cfg, messages)
	if err != nil {
		return nil, err
	}
	return extractSiteLogRepairPlan(reply)
}

func extractSiteLogRepairPlan(text string) (*SiteLogRepairPlan, error) {
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
		return nil, fmt.Errorf("AI 返回格式无效")
	}
	var plan SiteLogRepairPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return nil, err
	}
	plan.Actions = filterSiteRepairActions(plan.Actions)
	plan.FileWrites = filterFileWrites(plan.FileWrites)
	if len(plan.Actions) == 0 && len(plan.FileWrites) == 0 {
		plan.Actions = []string{"apply_vhost", "reload_webserver"}
	}
	if plan.Summary == "" {
		plan.Summary = "已根据日志生成修复方案"
	}
	return &plan, nil
}

func filterFileWrites(writes []FileWriteSpec) []FileWriteSpec {
	var out []FileWriteSpec
	for _, w := range writes {
		p := strings.TrimSpace(w.RelativePath)
		if p == "" || strings.Contains(p, "..") {
			continue
		}
		lower := strings.ToLower(p)
		if strings.Contains(lower, "wp-config") || strings.Contains(lower, ".env") {
			continue
		}
		content := w.Content
		if len(content) > 8192 {
			content = content[:8192]
		}
		out = append(out, FileWriteSpec{RelativePath: p, Content: content})
	}
	return out
}

func DefaultSiteLogRepairPlan() *SiteLogRepairPlan {
	return &SiteLogRepairPlan{
		Summary:    "执行标准修复：重建虚拟主机并重载 Web 服务",
		Diagnosis:  "AI 未返回有效方案，已使用默认修复流程。",
		Actions:    []string{"apply_vhost", "reload_webserver"},
		Confidence: "low",
	}
}

func trimLogForAI(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return "...(truncated)\n" + s[len(s)-max:]
}
