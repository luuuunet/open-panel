package aisite

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/website"
)

func (s *Service) runBootstrap(jobID uint, req DeployRequest) {
	var pipe *pipelineTracker
	defer func() {
		if r := recover(); r != nil {
			stack := string(debug.Stack())
			msg := fmt.Sprintf("内部错误: %v", r)
			log.Printf("[aisite] bootstrap panic job=%d: %v\n%s", jobID, r, stack)
			if pipe != nil {
				pipe.finishPhase(pipe.runningPhase(), StepFailed, msg)
				s.finishJob(jobID, "failed", pipe.combinedLog(), 0, 0, msg)
			} else {
				s.updateJobLog(jobID, fmt.Sprintf("[%s] %s\n", ts(), msg))
				s.finishJob(jobID, "failed", fmt.Sprintf("[%s] %s", ts(), msg), 0, 0, msg)
			}
		}
	}()

	pipe = &pipelineTracker{
		jobID:   jobID,
		service: s,
		steps:   newPipelineSteps(),
	}
	pipe.appendLog(PhaseAnalyze, fmt.Sprintf("开始 AI 全自动建站: %s", req.Plan.Domain))

	manualPlan := strings.TrimSpace(req.Plan.DeployScript) != "" && !req.Plan.UseAI

	var snap *RepoSnapshot
	var plan DeployPlan
	var panel PanelContext
	var createRes *website.CreateResult
	var site models.Website
	var siteID uint
	var aiPlan *DeployPlan

	fail := func(phase, errMsg string) {
		pipe.finishPhase(phase, StepFailed, errMsg)
		s.finishJob(jobID, "failed", pipe.combinedLog(), siteID, 0, errMsg)
	}

	// ── Phase 1: Analyze (项目理解) ──
	pipe.startPhase(PhaseAnalyze)
	logAnalyze := pipe.phaseLog(PhaseAnalyze)

	logAnalyze("检测 Git…")
	if err := ensureGitAvailable(logAnalyze); err != nil {
		fail(PhaseAnalyze, err.Error())
		return
	}

	logAnalyze("拉取 GitHub 仓库结构（clone 预检，最多约 2 分钟）…")
	var snapErr error
	snap, snapErr = s.fetchRepoSnapshot(req.RepoURL, req.Branch, req.GithubToken)
	if snapErr != nil {
		logAnalyze("仓库预检: " + snapErr.Error())
		branch := resolveGitBranch(normalizeRepoURL(req.RepoURL), req.Branch, req.GithubToken)
		snap = &RepoSnapshot{RepoURL: req.RepoURL, Branch: branch, FrameworkHint: req.Plan.Framework}
	} else {
		logAnalyze(fmt.Sprintf("识别框架: %s（分支 %s）", snap.FrameworkHint, snap.Branch))
		if snap.PHPVersionRequired != "" {
			logAnalyze(fmt.Sprintf("composer.json 要求 PHP %s", snap.PHPVersionRequired))
		}
		if snap.NodeMajorRequired > 0 {
			logAnalyze(fmt.Sprintf("package.json / .nvmrc 要求 Node %d+", snap.NodeMajorRequired))
		}
		if snap.LockfileKind != "" && snap.LockfileKind != "none" {
			logAnalyze(fmt.Sprintf("检测到锁文件: %s", snap.LockfileKind))
		} else if snap.HasPackageJSON {
			logAnalyze("未检测到 npm/pnpm/yarn 锁文件，将使用 npm install")
		}
		if snap.IsMonorepo {
			logAnalyze(fmt.Sprintf("Monorepo（turbo=%v），主应用: %s → 输出目录 %s",
				snap.HasTurbo, firstNonEmpty(snap.PrimaryAppFilter, "?"), firstNonEmpty(snap.PrimaryAppOutDir, "auto")))
			if len(snap.BuildEnvKeys) > 0 {
				logAnalyze("构建环境变量将自动注入: " + strings.Join(snap.BuildEnvKeys, ", "))
			}
		}
		if snap.UsesCatalog {
			logAnalyze("package.json 使用 catalog: 依赖，需要 Node 20 + npm 10.7+ 或 pnpm")
		}
	}

	panel = s.collectPanelContext()
	wizardPlan := req.Plan
	hasWizardPlan := strings.TrimSpace(wizardPlan.Framework) != "" && strings.TrimSpace(wizardPlan.Summary) != ""
	analyzeBranch := branchFromSnap(req, snap)

	if !manualPlan {
		if hasWizardPlan {
			logAnalyze("向导已确认框架，结合最新仓库与面板环境深度分析…")
		} else {
			logAnalyze("调用 Cursor/AI 分析技术栈与构建命令…")
		}
		aiDeployPlan, aiReply, _ := s.buildPlan(snap, panel, AnalyzeRequest{
			RepoURL: req.RepoURL, Branch: analyzeBranch, GithubToken: req.GithubToken,
			Domain: firstNonEmpty(wizardPlan.Domain, req.Plan.Domain), Notes: req.Notes,
		})
		if aiReply != "" {
			logAnalyze(aiReply)
		}
		plan = aiDeployPlan
		if hasWizardPlan {
			mergeWizardPlanChoices(&plan, wizardPlan)
		}
		mergeHeuristicSiteParams(&plan, s.dataDir, snap, panel)
		enrichPlanDefaults(&plan, snap, panel)
		if plan.DeployScript == "" {
			applyFrameworkDefaults(&plan, snap, s.dataDir, panel, false)
		}
		plan.PostNotes = sanitizePostNotes(plan.PostNotes, resolveFramework(plan, snap), panel, false)
	} else if hasWizardPlan {
		logAnalyze("使用手动部署脚本与向导配置…")
		plan = wizardPlan
		mergeHeuristicSiteParams(&plan, s.dataDir, snap, panel)
		enrichPlanDefaults(&plan, snap, panel)
		if plan.DeployScript == "" {
			applyFrameworkDefaults(&plan, snap, s.dataDir, panel, false)
		}
		plan.PostNotes = sanitizePostNotes(plan.PostNotes, resolveFramework(plan, snap), panel, false)
	} else {
		plan = s.buildAutoDeployPlan(req, snap, panel, true)
	}
	if analyzeBranch != "" && analyzeBranch != strings.TrimSpace(req.Branch) {
		logAnalyze(fmt.Sprintf("使用 Git 分支: %s", analyzeBranch))
	}
	logAnalyze(fmt.Sprintf("分析完成: %s — %s", plan.Framework, plan.Summary))
	s.updateJobPlan(jobID, plan)
	pipe.finishPhase(PhaseAnalyze, StepDone, "")

	// ── Phase 2: Plan (环境规划) ──
	pipe.startPhase(PhasePlan)
	logPlan := pipe.phaseLog(PhasePlan)

	panel, envErr := s.ensureDeployEnvironment(plan, snap, req.SelectedApps, logPlan)
	if envErr != nil {
		fail(PhasePlan, envErr.Error())
		return
	}

	logPlan("创建网站、Nginx 虚拟主机与数据库…")
	var err error
	createRes, site, siteID, err = s.createSiteForPlan(plan, req.RepoURL)
	if err != nil {
		fail(PhasePlan, err.Error())
		return
	}
	logPlan(fmt.Sprintf("站点 ID=%d 路径=%s", siteID, site.RootPath))
	if createRes.DbName != "" {
		logPlan(fmt.Sprintf("数据库: %s / %s", createRes.DbName, createRes.DbUser))
	}

	if !manualPlan {
		logPlan("基于站点信息 refine AI 安装脚本…")
		installPlan, aiReply, _ := s.planInstallAfterSiteCreate(snap, panel, AnalyzeRequest{
			RepoURL: req.RepoURL, Branch: req.Branch, GithubToken: req.GithubToken,
			Domain: plan.Domain, Notes: req.Notes,
		}, &site, createRes)
		if aiReply != "" {
			logPlan(aiReply)
		}
		aiPlan = &installPlan
		mergeInstallPlan(&plan, installPlan)
		s.updateJobPlan(jobID, installPlan)
	}

	panel = s.collectPanelContext()
	s.finalizeDeployPlan(&plan, snap, panel, createRes, aiPlan)
	hasDB := createRes != nil && createRes.DbName != ""
	plan.PostNotes = sanitizePostNotes(plan.PostNotes, resolveFramework(plan, snap), panel, hasDB)
	s.updateJobPlan(jobID, plan)

	if hasDB {
		_ = s.prepareFrameworkEnvBeforeDeploy(plan, snap, site.RootPath, createRes, plan.Domain, logPlan)
	}

	panel, envErr = s.preflightDeployEnvironment(plan, snap, req.SelectedApps, logPlan)
	if envErr != nil {
		fail(PhasePlan, envErr.Error())
		return
	}
	s.finalizeDeployPlan(&plan, snap, panel, createRes, aiPlan)
	plan.PostNotes = sanitizePostNotes(plan.PostNotes, resolveFramework(plan, snap), panel, hasDB)
	s.updateJobPlan(jobID, plan)
	pipe.finishPhase(PhasePlan, StepDone, "")

	// ── Phase 3: Execute (代码执行) ──
	pipe.startPhase(PhaseExecute)
	logExecute := pipe.phaseLog(PhaseExecute)

	script := s.prepareDeployScript(req, plan, site.RootPath, snap)
	if plan.UseDocker && !panel.DockerAvailable {
		logExecute("仓库标记 Docker 部署，但面板未检测到 Docker，改用 shell 脚本执行")
		plan.UseDocker = false
	} else if plan.UseDocker {
		logExecute("使用 Docker 构建并启动（docker compose）…")
	} else {
		logExecute("在站点目录执行构建脚本（npm/composer/git 等）…")
	}

	out, err := s.runDeployScriptWithRetry(req, &plan, snap, site.RootPath, createRes, aiPlan, script, logExecute)
	if strings.TrimSpace(out) != "" {
		for _, line := range splitJobLog(out) {
			if strings.TrimSpace(line) != "" {
				logExecute(line)
			}
		}
	}
	if err != nil {
		logExecute("安装失败: " + err.Error())
		if plan.RollbackOnFail {
			s.rollbackSite(siteID, logExecute)
		}
		fail(PhaseExecute, err.Error())
		return
	}
	pipe.finishPhase(PhaseExecute, StepDone, "")

	// ── Phase 4: Deploy (自动部署) ──
	pipe.startPhase(PhaseDeploy)
	logDeploy := pipe.phaseLog(PhaseDeploy)

	if err := s.saveDeployWebhook(siteID, req, plan, script, logDeploy); err != nil {
		if plan.RollbackOnFail {
			s.rollbackSite(siteID, logDeploy)
		}
		fail(PhaseDeploy, err.Error())
		return
	}

	if plan.NodePort <= 0 && plan.UsePM2 {
		plan.NodePort = defaultNodePort(siteID)
	}
	if err := s.postDeploySteps(bootstrapContext{plan: plan, site: &site, createRes: createRes}, logDeploy); err != nil {
		logDeploy("后置: " + err.Error())
	}

	if err := s.finalizeDeploy(siteID, plan.Domain, logDeploy); err != nil {
		if plan.RollbackOnFail {
			s.rollbackSite(siteID, logDeploy)
		}
		fail(PhaseDeploy, err.Error())
		return
	}

	plan.PostNotes = automatedPostNotes(resolveFramework(plan, snap), s.collectPanelContext(), hasDB)
	if plan.PostNotes != "" {
		logDeploy("完成说明: " + strings.ReplaceAll(plan.PostNotes, "\n", "；"))
	}

	logDeploy("✓ AI 全自动建站完成，站点已在列表中显示")
	pipe.finishPhase(PhaseDeploy, StepDone, "")
	s.finishJob(jobID, "success", pipe.combinedLog(), siteID, 0, "")
}

func (s *Service) finalizeDeploy(siteID uint, domain string, appendLog func(string)) error {
	appendLog("重载 Nginx / Web 服务器…")
	if err := s.website.ApplyVhost(siteID); err != nil {
		return fmt.Errorf("Nginx 重载失败: %w", err)
	}
	appendLog("Nginx 已重载")

	appendLog("执行健康检查…")
	ok, detail := s.deployHealthCheck(siteID, domain)
	if ok {
		appendLog("健康检查通过: " + detail)
		return nil
	}
	appendLog("健康检查未通过: " + detail)
	return fmt.Errorf("健康检查未通过: %s", detail)
}

func (s *Service) deployHealthCheck(siteID uint, domain string) (bool, string) {
	diag, err := s.website.CollectDiagnostics(siteID)
	if err != nil {
		return false, err.Error()
	}
	if diag.Status == "stopped" {
		return false, "站点处于停止状态"
	}
	if !diag.WebServerRunning {
		return false, "Web 服务器未运行"
	}
	if !diag.RootExists {
		return false, "网站根目录不存在"
	}

	site, _ := s.website.Get(siteID)
	if site != nil {
		if upstreamPort := parseProxyPort(site.ProxyPass); upstreamPort > 0 {
			if !waitLocalPortOpen(upstreamPort, 45*time.Second) {
				return false, fmt.Sprintf("反代上游端口 %d 未监听", upstreamPort)
			}
		}
	}

	port := 80
	if strings.TrimSpace(domain) == "" {
		domain = diag.Domain
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false, err.Error()
	}
	req.Host = domain
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, "HTTP 探测失败: " + err.Error()
	}
	defer resp.Body.Close()
	if !isHealthyHTTPStatus(resp.StatusCode) {
		return false, fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return true, fmt.Sprintf("HTTP %d", resp.StatusCode)
}

func parseProxyPort(proxy string) int {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return 0
	}
	i := strings.LastIndex(proxy, ":")
	if i < 0 {
		return 0
	}
	rest := strings.TrimSuffix(proxy[i+1:], "/")
	port, err := strconv.Atoi(rest)
	if err != nil || port <= 0 {
		return 0
	}
	return port
}

func isLocalPortOpen(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func waitLocalPortOpen(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isLocalPortOpen(port) {
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return isLocalPortOpen(port)
}

func isHealthyHTTPStatus(code int) bool {
	return code == 200 || code == 301 || code == 302 || code == 304
}

func (s *Service) buildAutoDeployPlan(req DeployRequest, snap *RepoSnapshot, panel PanelContext, manual bool) DeployPlan {
	plan := req.Plan
	if plan.Domain == "" {
		plan.Domain = suggestDomain("", snap.RepoURL)
	}
	if !manual {
		mergeHeuristicSiteParams(&plan, s.dataDir, snap, panel)
	}
	enrichPlanDefaults(&plan, snap, panel)
	savedScript := plan.DeployScript
	if !manual {
		applyFrameworkDefaults(&plan, snap, s.dataDir, panel, false)
	} else if savedScript != "" {
		applyFrameworkDefaults(&plan, snap, s.dataDir, panel, false)
		plan.DeployScript = savedScript
	}
	plan.PostNotes = sanitizePostNotes(plan.PostNotes, resolveFramework(plan, snap), panel, false)
	return plan
}

func (s *Service) createSiteForPlan(plan DeployPlan, repoURL string) (*website.CreateResult, models.Website, uint, error) {
	phpVer := plan.PhpVersion
	if phpVer == "" {
		phpVer = "static"
	}
	if plan.ProjectType == "node" || plan.UsePM2 || plan.UseDocker {
		phpVer = "static"
	}
	dbOpt := "none"
	if plan.NeedDatabase {
		dbOpt = "mysql"
	}
	ftpOpt := "none"
	if plan.CreateFTP {
		ftpOpt = "create"
	}
	createRes, err := s.website.Create(&website.CreateRequest{
		DomainsText: plan.Domain,
		Description: "AI GitHub: " + repoURL,
		PhpVersion:  phpVer,
		Database:    dbOpt,
		Ftp:         ftpOpt,
		DnsMode:     "manual",
	})
	if err != nil {
		return nil, models.Website{}, 0, err
	}
	return createRes, createRes.Site, createRes.Site.ID, nil
}

func (s *Service) saveDeployWebhook(siteID uint, req DeployRequest, plan DeployPlan, script string, appendLog func(string)) error {
	appendLog("保存 WebHook 部署配置…")
	cfg := &models.SiteDeployConfig{
		WebsiteID: siteID, Enabled: plan.EnableWebhook,
		RepoURL: normalizeRepoURL(req.RepoURL), Branch: req.Branch,
		DeployScript: script, AutoRestart: true,
	}
	saved, err := s.devops.SaveDeployConfig(cfg)
	if err != nil {
		return err
	}
	if note := webhookNote(saved.HookURL); note != "" && plan.EnableWebhook {
		appendLog(note)
	}
	return nil
}

func mergeHeuristicSiteParams(plan *DeployPlan, dataDir string, snap *RepoSnapshot, panel PanelContext) {
	h := heuristicPlan(dataDir, snap, panel, plan.Domain)
	if plan.Domain == "" {
		plan.Domain = h.Domain
	}
	if plan.ProjectType == "" {
		plan.ProjectType = h.ProjectType
	}
	if plan.PhpVersion == "" {
		plan.PhpVersion = h.PhpVersion
	}
	if plan.Framework == "" {
		plan.Framework = h.Framework
	}
	if plan.DocumentRoot == "" {
		plan.DocumentRoot = h.DocumentRoot
	}
	plan.NeedDatabase = plan.NeedDatabase || h.NeedDatabase
	plan.CreateFTP = plan.CreateFTP || h.CreateFTP
	if !plan.UseDocker && h.UseDocker {
		plan.UseDocker = h.UseDocker
	}
	if !plan.UsePM2 && h.UsePM2 {
		plan.UsePM2 = h.UsePM2
	}
	if plan.Summary == "" {
		plan.Summary = h.Summary
	}
}

func mergeInstallPlan(plan *DeployPlan, install DeployPlan) {
	if install.Framework != "" {
		plan.Framework = install.Framework
	}
	if install.DocumentRoot != "" {
		plan.DocumentRoot = install.DocumentRoot
	}
	if install.ProjectType != "" {
		plan.ProjectType = install.ProjectType
	}
	if install.UseDocker {
		plan.UseDocker = install.UseDocker
	}
	if install.UsePM2 {
		plan.UsePM2 = install.UsePM2
	}
	if install.NodePort > 0 {
		plan.NodePort = install.NodePort
	}
	if install.UseAI {
		plan.UseAI = true
	}
	if install.DeployScript != "" {
		plan.DeployScript = install.DeployScript
	}
	if install.Summary != "" {
		plan.Summary = install.Summary
	}
}

func defaultNodePort(siteID uint) int {
	return 31000 + int(siteID%500)
}

func (s *Service) prepareDeployScript(req DeployRequest, plan DeployPlan, siteRoot string, snap *RepoSnapshot) string {
	script := plan.DeployScript
	if script == "" {
		if snap == nil {
			var err error
			snap, err = s.fetchRepoSnapshot(req.RepoURL, req.Branch, req.GithubToken)
			if err != nil {
				snap = nil
			}
		}
		panel := s.collectPanelContext()
		if snap != nil {
			script = defaultDeployScript(s.dataDir, snap, panel, req.GithubToken)
		}
	}
	repo := cloneURLWithToken(normalizeRepoURL(req.RepoURL), req.GithubToken)
	branch := strings.TrimSpace(req.Branch)
	if snap != nil && strings.TrimSpace(snap.Branch) != "" {
		branch = snap.Branch
	}
	branch = resolveGitBranch(normalizeRepoURL(req.RepoURL), branch, req.GithubToken)
	return applyPlanPlaceholders(script, repo, branch, plan.Domain, siteRoot)
}
