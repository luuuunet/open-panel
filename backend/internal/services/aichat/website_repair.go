package aichat

import (
	"encoding/json"
	"fmt"
	"strings"
)

type SiteRepairPlan struct {
	Summary    string   `json:"summary"`
	Diagnosis  string   `json:"diagnosis"`
	Actions    []string `json:"actions"`
	Confidence string   `json:"confidence"`
}

var allowedSiteRepairActions = map[string]bool{
	"apply_vhost":          true,
	"reload_webserver":     true,
	"fix_dir_permissions":  true,
	"ensure_index_files":   true,
	"start_site":           true,
	"start_php_fpm":        true,
	"create_root_dir":      true,
}

func (s *Service) AnalyzeSiteRepair(bundle string) (*SiteRepairPlan, error) {
	if err := s.EnsureConfigured(); err != nil {
		return nil, err
	}

	system := `你是 OWPanel 网站运维专家。根据站点诊断信息，输出 JSON 修复方案（不要 markdown 代码块外的多余文字）。

可用 actions（只能从中选择，按推荐顺序排列）:
- create_root_dir: 网站根目录不存在时创建
- fix_dir_permissions: 修复目录/文件权限（目录755 文件644）
- ensure_index_files: 为 PHP 站点设置 index.php index.html 索引
- start_php_fpm: 启动对应 PHP-FPM 服务
- start_site: 站点处于 stopped 时启用
- apply_vhost: 重新生成并应用 Nginx/Apache 虚拟主机
- reload_webserver: 重载 Web 服务器

要求:
- summary: 一句话中文总结
- diagnosis: 中文问题分析（2-5 句）
- actions: 数组，仅包含上述 action 字符串
- confidence: high/medium/low
- 若日志无明显错误，至少包含 apply_vhost 与 reload_webserver
- 不要建议修改数据库密码、删除文件等危险操作

JSON 格式:
{"summary":"","diagnosis":"","actions":[],"confidence":"medium"}`

	messages := []Message{
		{Role: "system", Content: system},
		{Role: "user", Content: "请分析以下站点诊断信息并给出修复 actions:\n\n" + bundle},
	}
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	reply, err := s.callChatAPI(cfg, messages)
	if err != nil {
		return nil, err
	}
	return extractSiteRepairPlan(reply)
}

func extractSiteRepairPlan(text string) (*SiteRepairPlan, error) {
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
	var plan SiteRepairPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return nil, err
	}
	plan.Actions = filterSiteRepairActions(plan.Actions)
	if len(plan.Actions) == 0 {
		plan.Actions = []string{"apply_vhost", "reload_webserver"}
	}
	if plan.Summary == "" {
		plan.Summary = "已生成修复方案"
	}
	return &plan, nil
}

func filterSiteRepairActions(actions []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range actions {
		a = strings.TrimSpace(a)
		if !allowedSiteRepairActions[a] || seen[a] {
			continue
		}
		seen[a] = true
		out = append(out, a)
	}
	return out
}

func DefaultSiteRepairPlan() *SiteRepairPlan {
	return &SiteRepairPlan{
		Summary:    "执行标准修复：重建虚拟主机并重载 Web 服务",
		Diagnosis:  "AI 未返回有效方案，已使用默认修复流程。",
		Actions:    []string{"apply_vhost", "reload_webserver"},
		Confidence: "low",
	}
}
