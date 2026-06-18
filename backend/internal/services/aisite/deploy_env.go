package aisite

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/website"
)

var deployEnvFailurePatterns = []string{
	"command not found",
	"not found",
	"no such file or directory",
	"composer: command",
	"npm: command",
	"node: command",
	"php: command",
	"Cannot find module",
	"env: node",
	"env: npm",
	"env: composer",
	"env: php",
	"Please install composer",
	"Please install node",
	"Node.js",
	"composer.phar",
	"Vite requires Node.js",
	"EBADENGINE",
	"Unsupported engine",
}

func isDeployEnvFailure(output string, err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(output + "\n" + err.Error())
	for _, p := range deployEnvFailurePatterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

func (s *Service) missingAppsFromDeployOutput(output string, plan DeployPlan, snap *RepoSnapshot) []string {
	lower := strings.ToLower(output)
	var keys []string
	if strings.Contains(lower, "composer") {
		keys = append(keys, "composer")
	}
	if strings.Contains(lower, "npm") || strings.Contains(lower, "node") || strings.Contains(lower, "node.js") ||
		strings.Contains(lower, "vite requires node") || strings.Contains(lower, "ebadengine") {
		keys = append(keys, "nodejs20")
	}
	if strings.Contains(lower, "php") {
		if k := phpAppKey(plan.PhpVersion); k != "" {
			keys = append(keys, k)
		}
	}
	if strings.Contains(lower, "mysql") || strings.Contains(lower, "sqlstate") {
		if plan.NeedDatabase {
			keys = append(keys, mysqlAppKey())
		}
	}
	for _, k := range s.missingEnvApps(plan, snap) {
		keys = append(keys, k)
	}
	return dedupeKeys(keys)
}

func sanitizePostNotes(raw, fw string, panel PanelContext, dbConfigured bool) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return automatedPostNotes(fw, panel, dbConfigured)
	}
	lower := strings.ToLower(raw)
	manualHints := []string{
		"手动", "manual", "apt install", "yum install", "自行安装",
		"create mysql", "创建数据库", "编写 .env", "edit .env", "配置 .env",
		"db_host", "composer 未", "npm 未", "node.js 未", "安装 composer",
		"安装 npm", "安装 node", "chmod", "www-data", "www 用户",
		"app_env", "app_debug", "crontab", "schedule:run",
	}
	for _, hint := range manualHints {
		if strings.Contains(lower, strings.ToLower(hint)) {
			return automatedPostNotes(fw, panel, dbConfigured)
		}
	}
	return raw
}

func (s *Service) preflightDeployEnvironment(plan DeployPlan, snap *RepoSnapshot, selectedApps []string, appendLog func(string)) (PanelContext, error) {
	appendLog("部署前运行环境自检…")
	panel, err := s.ensureDeployEnvironment(plan, snap, selectedApps, appendLog)
	if err != nil {
		return panel, err
	}
	missing := filterAppsBySelection(s.requiredDeployApps(plan, snap), selectedApps)
	for _, key := range missing {
		if deployAppReady(s, key) {
			continue
		}
		appendLog(fmt.Sprintf("自检发现缺失组件 %s，正在安装…", key))
		if err := s.ensureAppInstalled(key, appendLog); err != nil {
			return s.collectPanelContext(), fmt.Errorf("安装 %s 失败: %w", key, err)
		}
		_ = s.startAppIfNeeded(key, appendLog)
	}
	return s.collectPanelContext(), nil
}

func (s *Service) runDeployScriptWithRetry(
	req DeployRequest,
	plan *DeployPlan,
	snap *RepoSnapshot,
	siteRoot string,
	createRes *website.CreateResult,
	aiPlan *DeployPlan,
	initialScript string,
	appendLog func(string),
) (string, error) {
	script := initialScript
	var lastOut string
	var lastErr error

	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			appendLog("检测到部署因运行环境缺失失败，正在自动安装缺失组件并重试…")
			panel, envErr := s.preflightDeployEnvironment(*plan, snap, req.SelectedApps, appendLog)
			if envErr != nil {
				return lastOut, lastErr
			}
			hasDB := createRes != nil && createRes.DbName != ""
			s.finalizeDeployPlan(plan, snap, panel, createRes, aiPlan)
			if hasDB {
				_ = s.prepareFrameworkEnvBeforeDeploy(*plan, snap, siteRoot, createRes, plan.Domain, appendLog)
			}
			plan.PostNotes = automatedPostNotes(resolveFramework(*plan, snap), panel, hasDB)
			script = s.prepareDeployScript(req, *plan, siteRoot, snap)
		}

		out, err := runDeployShell(s.dataDir, script, siteRoot)
		lastOut, lastErr = out, err
		if err == nil {
			return out, nil
		}
		if attempt == 0 && isDeployEnvFailure(out, err) {
			if apps := s.missingAppsFromDeployOutput(out, *plan, snap); len(apps) > 0 {
				for _, key := range apps {
					if deployAppReady(s, key) {
						continue
					}
					appendLog(fmt.Sprintf("根据部署日志安装缺失组件: %s", key))
					_ = s.ensureAppInstalled(key, appendLog)
					_ = s.startAppIfNeeded(key, appendLog)
				}
			}
			continue
		}
		break
	}

	for aiAttempt := 1; aiAttempt <= maxDeployAIRepairAttempts; aiAttempt++ {
		appendLog(formatDeployRepairAttempt(aiAttempt))
		repair := s.resolveDeployRepair(lastOut, script, *plan, snap, req, aiAttempt)
		if repair == nil {
			appendLog("AI/启发式未生成可执行的修复方案，停止重试")
			break
		}
		appendLog("修复方案: " + repair.Summary)
		if repair.Diagnosis != "" {
			appendLog(repair.Diagnosis)
		}
		applyDeployRepairToPlan(plan, repair)

		var retryScript string
		if ds := strings.TrimSpace(repair.DeployScript); ds != "" && isRepairScriptSafe(ds) {
			retryScript = deployRepairPlaceholderScript(req, *plan, siteRoot, snap, ds)
		} else {
			retryScript = buildIncrementalRepairScript(repair, plan.Domain)
		}

		out, err := runDeployShell(s.dataDir, retryScript, siteRoot)
		lastOut, lastErr = out, err
		if err == nil {
			return out, nil
		}
	}
	return lastOut, lastErr
}
