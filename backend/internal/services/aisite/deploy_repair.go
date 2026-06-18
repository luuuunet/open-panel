package aisite

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/aichat"
)

const maxDeployAIRepairAttempts = 3

var repairScriptUnsafePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)rm\s+-rf\s+/\s`),
	regexp.MustCompile(`(?i)rm\s+-rf\s+/\*`),
	regexp.MustCompile(`(?i)\bmkfs\b`),
	regexp.MustCompile(`(?i)\bdd\s+if=`),
	regexp.MustCompile(`(?i)curl\s+[^\n|]*\|\s*(ba)?sh`),
	regexp.MustCompile(`(?i)wget\s+[^\n|]*\|\s*(ba)?sh`),
}

func monorepoBuildEnvExports() []string {
	return []string{
		`export NODE_ENV=production`,
		`SITE_DOMAIN="{{domain_host}}"`,
		`export DOCS_ORIGIN="${DOCS_ORIGIN:-https://docs.${SITE_DOMAIN}}"`,
		`export BLOG_ORIGIN="${BLOG_ORIGIN:-https://blog.${SITE_DOMAIN}}"`,
		`export NEXT_PUBLIC_SITE_URL="${NEXT_PUBLIC_SITE_URL:-https://${SITE_DOMAIN}}"`,
		`export SITE_URL="${SITE_URL:-https://${SITE_DOMAIN}}"`,
	}
}

func heuristicDeployRepair(log string, plan DeployPlan, snap *RepoSnapshot) *aichat.DeployRepairPlan {
	lower := strings.ToLower(log)
	var exports []string
	var diagnosis []string
	buildCmd := ""

	if strings.Contains(lower, "approve-builds") || strings.Contains(lower, "ignored build scripts") {
		diagnosis = append(diagnosis, "pnpm 需要批准构建脚本")
		buildCmd = "pnpm approve-builds -y 2>/dev/null || true"
		if snap != nil && snap.IsMonorepo && snap.PrimaryAppFilter != "" {
			buildCmd += "\npnpm --filter \"" + snap.PrimaryAppFilter + "\" build"
		} else {
			buildCmd += "\npnpm run build"
		}
	}

	for _, key := range buildEnvKeysForRepair(snap) {
		if envVarLikelyMissing(log, key) {
			if line := buildEnvExportLine(key); line != "" {
				exports = append(exports, line)
				diagnosis = append(diagnosis, "可能缺少环境变量 "+key)
			}
		}
	}

	if snap != nil && snap.IsMonorepo && (strings.Contains(lower, "turbo") || strings.Contains(lower, "failed to compile") || strings.Contains(lower, "monorepo")) {
		if buildCmd == "" {
			diagnosis = append(diagnosis, "Monorepo/Turbo 构建失败，补充环境变量后重试")
			exports = append(exports, monorepoBuildEnvExports()...)
			if snap.PrimaryAppFilter != "" {
				buildCmd = "pnpm --filter \"" + snap.PrimaryAppFilter + "\" build"
			} else {
				buildCmd = "npm run build"
			}
		}
	}

	exports = dedupeStrings(exports)
	if buildCmd == "" && len(exports) > 0 {
		buildCmd = defaultIncrementalBuild(snap)
	}

	if buildCmd == "" && len(exports) == 0 {
		return nil
	}

	repair := &aichat.DeployRepairPlan{
		Summary:      "根据构建日志启发式生成增量修复",
		Diagnosis:    strings.Join(diagnosis, "；"),
		EnvExports:   exports,
		BuildCommand: buildCmd,
		Confidence:   "low",
	}
	if snap != nil && snap.IsMonorepo && snap.PrimaryAppOutDir != "" && strings.TrimSpace(plan.DocumentRoot) == "" {
		repair.DocumentRoot = snap.PrimaryAppOutDir
	}
	return repair
}

func buildEnvKeysForRepair(snap *RepoSnapshot) []string {
	if snap != nil && len(snap.BuildEnvKeys) > 0 {
		return snap.BuildEnvKeys
	}
	return []string{"DOCS_ORIGIN", "BLOG_ORIGIN", "NEXT_PUBLIC_SITE_URL", "SITE_URL"}
}

func envVarLikelyMissing(log, key string) bool {
	if !strings.Contains(log, key) {
		return false
	}
	lower := strings.ToLower(log)
	markers := []string{
		"required", "missing", "undefined", "not defined", "must be set",
		"environment variable", "env variable", "process.env",
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return strings.Contains(lower, strings.ToLower(key)+" is required") ||
		strings.Contains(lower, "invalid "+strings.ToLower(key))
}

func buildEnvExportLine(key string) string {
	switch key {
	case "DOCS_ORIGIN":
		return `export DOCS_ORIGIN="${DOCS_ORIGIN:-https://docs.{{domain_host}}}"`
	case "BLOG_ORIGIN":
		return `export BLOG_ORIGIN="${BLOG_ORIGIN:-https://blog.{{domain_host}}}"`
	case "NEXT_PUBLIC_SITE_URL", "NEXT_PUBLIC_APP_URL":
		return `export ` + key + `="${` + key + `:-https://{{domain_host}}}"`
	case "SITE_URL", "APP_URL":
		return `export ` + key + `="${` + key + `:-https://{{domain_host}}}"`
	case "NODE_ENV":
		return `export NODE_ENV=production`
	default:
		if strings.HasSuffix(key, "_ORIGIN") || strings.HasSuffix(key, "_URL") {
			return `export ` + key + `="${` + key + `:-https://{{domain_host}}}"`
		}
		return ""
	}
}

func applyDeployRepairToPlan(plan *DeployPlan, repair *aichat.DeployRepairPlan) {
	if plan == nil || repair == nil {
		return
	}
	if dr := strings.TrimSpace(repair.DocumentRoot); dr != "" {
		plan.DocumentRoot = dr
	}
}

func buildIncrementalRepairScript(repair *aichat.DeployRepairPlan, domain string) string {
	if repair == nil {
		return "set -e\n"
	}
	var b strings.Builder
	b.WriteString("set -e\n")
	for _, line := range repair.EnvExports {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		b.WriteString(applyPlanPlaceholders(line, "", "", domain, ""))
		b.WriteString("\n")
	}
	buildCmd := strings.TrimSpace(repair.BuildCommand)
	if buildCmd == "" && len(repair.EnvExports) > 0 {
		buildCmd = "npm run build"
	}
	if buildCmd != "" {
		b.WriteString(applyPlanPlaceholders(buildCmd, "", "", domain, ""))
		if !strings.HasSuffix(buildCmd, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func isRepairScriptSafe(script string) bool {
	script = strings.TrimSpace(script)
	if script == "" {
		return false
	}
	for _, re := range repairScriptUnsafePatterns {
		if re.MatchString(script) {
			return false
		}
	}
	return true
}

func repairHasAction(repair *aichat.DeployRepairPlan) bool {
	if repair == nil {
		return false
	}
	if strings.TrimSpace(repair.DeployScript) != "" || strings.TrimSpace(repair.BuildCommand) != "" {
		return true
	}
	return len(repair.EnvExports) > 0
}

func defaultIncrementalBuild(snap *RepoSnapshot) string {
	if snap != nil && snap.IsMonorepo && snap.PrimaryAppFilter != "" {
		return "pnpm --filter \"" + snap.PrimaryAppFilter + "\" build"
	}
	if snap != nil && snap.HasPnpmLock {
		return "pnpm run build"
	}
	return "npm run build"
}

func (s *Service) resolveDeployRepair(log, script string, plan DeployPlan, snap *RepoSnapshot, req DeployRequest, attempt int) *aichat.DeployRepairPlan {
	fw := resolveFramework(plan, snap)
	repoBrief := aichat.SiteRepoBrief{RepoURL: req.RepoURL, Branch: branchFromSnap(req, snap)}
	if snap != nil {
		repoBrief = snapToAIRepo(snap)
	}

	aiReq := aichat.DeployRepairRequest{
		Domain:       plan.Domain,
		SiteRoot:     "",
		DeployScript: script,
		BuildLog:     trimTail(log, 12000),
		Attempt:      attempt,
		Repo:         repoBrief,
		Framework:    fw,
		DocumentRoot: plan.DocumentRoot,
	}

	if repair, err := s.aichat.AnalyzeDeployRepair(aiReq); err == nil && repairHasAction(repair) {
		if strings.TrimSpace(repair.BuildCommand) == "" && len(repair.EnvExports) > 0 {
			repair.BuildCommand = defaultIncrementalBuild(snap)
		}
		return repair
	}

	if repair := heuristicDeployRepair(log, plan, snap); repairHasAction(repair) {
		return repair
	}
	return nil
}

func deployRepairPlaceholderScript(req DeployRequest, plan DeployPlan, siteRoot string, snap *RepoSnapshot, script string) string {
	repo := cloneURLWithToken(normalizeRepoURL(req.RepoURL), req.GithubToken)
	branch := branchFromSnap(req, snap)
	branch = resolveGitBranch(normalizeRepoURL(req.RepoURL), branch, req.GithubToken)
	return applyPlanPlaceholders(script, repo, branch, plan.Domain, siteRoot)
}

func formatDeployRepairAttempt(n int) string {
	return fmt.Sprintf("部署失败，AI 诊断修复中 (%d/%d)…", n, maxDeployAIRepairAttempts)
}
