package aichat

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type DeployRepairRequest struct {
	Domain       string        `json:"domain"`
	SiteRoot     string        `json:"site_root"`
	DeployScript string        `json:"deploy_script"`
	BuildLog     string        `json:"build_log"`
	Attempt      int           `json:"attempt"`
	Repo         SiteRepoBrief `json:"repo"`
	Framework    string        `json:"framework"`
	DocumentRoot string        `json:"document_root"`
}

type DeployRepairPlan struct {
	Summary      string   `json:"summary"`
	Diagnosis    string   `json:"diagnosis"`
	EnvExports   []string `json:"env_exports"`
	BuildCommand string   `json:"build_command"`
	DeployScript string   `json:"deploy_script"`
	DocumentRoot string   `json:"document_root"`
	Confidence   string   `json:"confidence"`
}

var deployRepairDangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)rm\s+-rf\s+/\s`),
	regexp.MustCompile(`(?i)rm\s+-rf\s+/\*`),
	regexp.MustCompile(`(?i)\bmkfs\b`),
	regexp.MustCompile(`(?i)\bdd\s+if=`),
	regexp.MustCompile(`(?i)curl\s+[^\n|]*\|\s*(ba)?sh`),
	regexp.MustCompile(`(?i)wget\s+[^\n|]*\|\s*(ba)?sh`),
}

func sanitizeDeployRepairCommand(cmd string) string {
	for _, re := range deployRepairDangerousPatterns {
		cmd = re.ReplaceAllString(cmd, "# blocked-dangerous")
	}
	return strings.TrimSpace(cmd)
}

func (s *Service) AnalyzeDeployRepair(req DeployRepairRequest) (*DeployRepairPlan, error) {
	if err := s.EnsureConfigured(); err != nil {
		return nil, err
	}

	system := `你是 OWPanel 一键部署专家。根据构建失败日志、仓库快照与当前部署脚本，输出 JSON 修复方案（不要 markdown 代码块外的多余文字）。

场景：仓库已克隆到站点根目录，这是第 N 次增量修复重试，不要建议重新 git clone，除非必须提供完整 deploy_script。

要求:
- summary: 一句话中文总结
- diagnosis: 中文问题分析（2-5 句）
- env_exports: 数组，每项为单行 shell export（可用 {{domain_host}} 占位符），例如 export DOCS_ORIGIN="https://docs.{{domain_host}}"
- build_command: 在站点根目录执行的增量构建命令（仓库已存在，仅 build/install 修复），不要包含 git clone
- deploy_script: 仅当必须完整重跑时才提供（含 clone 的完整脚本）；多数情况留空
- document_root: 若需修正 Nginx 文档根目录则填写（如 apps/site/out、dist、public），否则留空
- confidence: high/medium/low
- 常见修复：补充 DOCS_ORIGIN/BLOG_ORIGIN/NEXT_PUBLIC_SITE_URL、pnpm approve-builds、monorepo pnpm --filter 构建、修正 document_root
- 禁止 rm -rf /、mkfs、dd、curl|sh 等危险操作

JSON 格式:
{"summary":"","diagnosis":"","env_exports":[],"build_command":"","deploy_script":"","document_root":"","confidence":"medium"}`

	payload, _ := json.Marshal(req)
	messages := []Message{
		{Role: "system", Content: system},
		{Role: "user", Content: "请分析以下部署失败信息并给出增量修复 JSON（attempt=" + fmt.Sprintf("%d", req.Attempt) + "）:\n\n" + string(payload)},
	}
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	reply, err := s.callChatAPI(cfg, messages)
	if err != nil {
		return nil, err
	}
	return extractDeployRepairPlan(reply)
}

func extractDeployRepairPlan(text string) (*DeployRepairPlan, error) {
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
	var plan DeployRepairPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return nil, err
	}
	plan.BuildCommand = sanitizeDeployRepairCommand(plan.BuildCommand)
	plan.DeployScript = sanitizeDeployRepairCommand(plan.DeployScript)
	if plan.Summary == "" {
		plan.Summary = "已生成部署修复方案"
	}
	return &plan, nil
}
