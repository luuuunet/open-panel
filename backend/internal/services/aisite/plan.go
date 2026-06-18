package aisite

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/services/aichat"
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"github.com/luuuunet/owpanel/internal/services/website"
)

const defaultAIPlanTimeout = 60 * time.Second
const cursorAIPlanTimeout = 6 * time.Minute

func aiPlanWaitTimeout(st aichat.AssistantStatus) time.Duration {
	if strings.EqualFold(strings.TrimSpace(st.Provider), "cursor") {
		return cursorAIPlanTimeout
	}
	return defaultAIPlanTimeout
}

type DeployPlan struct {
	ProjectType    string `json:"project_type"`
	Summary        string `json:"summary"`
	Domain         string `json:"domain"`
	PhpVersion     string `json:"php_version"`
	NeedDatabase   bool   `json:"need_database"`
	CreateFTP      bool   `json:"create_ftp"`
	DeployScript   string `json:"deploy_script"`
	PostNotes      string `json:"post_notes"`
	Confidence     string `json:"confidence"`
	Framework      string `json:"framework"`
	DocumentRoot   string `json:"document_root,omitempty"`
	UseDocker      bool   `json:"use_docker"`
	UsePM2         bool   `json:"use_pm2"`
	NodePort       int    `json:"node_port"`
	EnableWebhook  bool   `json:"enable_webhook"`
	RollbackOnFail bool   `json:"rollback_on_fail"`
	UseAI          bool   `json:"use_ai"`
}

type AnalyzeRequest struct {
	RepoURL     string `json:"repo_url"`
	Branch      string `json:"branch"`
	GithubToken string `json:"github_token"`
	Domain      string `json:"domain"`
	Notes       string `json:"notes"`
}

type AnalyzeResult struct {
	Repo         *RepoSnapshot    `json:"repo"`
	Panel        PanelContext     `json:"panel"`
	Plan         DeployPlan       `json:"plan"`
	RequiredEnv  []EnvRequirement `json:"required_env"`
	AIReply      string           `json:"ai_reply,omitempty"`
}

func (s *Service) Analyze(req AnalyzeRequest) (*AnalyzeResult, error) {
	if strings.TrimSpace(req.RepoURL) == "" {
		return nil, fmt.Errorf("请输入 GitHub 仓库地址")
	}
	if err := ensureGitAvailable(func(string) {}); err != nil {
		return nil, fmt.Errorf("Git 不可用: %w", err)
	}
	snap, err := s.fetchRepoSnapshot(req.RepoURL, req.Branch, req.GithubToken)
	if err != nil {
		return nil, err
	}
	panel := s.collectPanelContext()
	plan, aiReply, err := s.buildPlan(snap, panel, req)
	if err != nil && aiReply == "" {
		aiReply = err.Error()
	}
	enrichPlanDefaults(&plan, snap, panel)
	if plan.Domain == "" {
		plan.Domain = suggestDomain(req.Domain, snap.RepoURL)
	}
	plan.PostNotes = sanitizePostNotes(plan.PostNotes, resolveFramework(plan, snap), panel, false)
	requiredEnv := s.buildEnvRequirements(plan, snap)
	return &AnalyzeResult{
		Repo:        snap,
		Panel:       panel,
		Plan:        plan,
		RequiredEnv: requiredEnv,
		AIReply:     aiReply,
	}, nil
}

func (s *Service) buildPlan(snap *RepoSnapshot, panel PanelContext, req AnalyzeRequest) (DeployPlan, string, error) {
	st := s.aichat.AssistantStatus()
	if !st.Configured {
		plan := heuristicPlan(s.dataDir, snap, panel, req.Domain)
		plan.UseAI = false
		msg := "AI 未配置，已使用规则引擎生成部署方案"
		if st.Message != "" {
			msg = st.Message + "；" + msg
		}
		return plan, msg, nil
	}

	type planOutcome struct {
		plan DeployPlan
		err  error
	}
	ch := make(chan planOutcome, 1)
	go func() {
		aiPlan, err := s.aichat.SiteBootstrapPlan(aichat.SiteBootstrapRequest{
			Repo:    snapToAIRepo(snap),
			Panel:   panelContextJSON(panel),
			Domain:  req.Domain,
			Notes:   req.Notes,
			Branch:  snap.Branch,
			RepoURL: snap.RepoURL,
		})
		if err != nil {
			ch <- planOutcome{plan: heuristicPlan(s.dataDir, snap, panel, req.Domain), err: err}
			return
		}
		plan := DeployPlan{
			ProjectType:    aiPlan.ProjectType,
			Summary:        aiPlan.Summary,
			Domain:         firstNonEmpty(aiPlan.Domain, req.Domain, suggestDomain("", snap.RepoURL)),
			PhpVersion:     aiPlan.PhpVersion,
			NeedDatabase:   aiPlan.NeedDatabase,
			CreateFTP:      aiPlan.CreateFTP,
			DeployScript:   aiPlan.DeployScript,
			PostNotes:      aiPlan.PostNotes,
			Confidence:     aiPlan.Confidence,
			Framework:      aiPlan.Framework,
			DocumentRoot:   aiPlan.DocumentRoot,
			UseDocker:      aiPlan.UseDocker,
			UsePM2:         aiPlan.UsePM2,
			NodePort:       aiPlan.NodePort,
			EnableWebhook:  aiPlan.EnableWebhook,
			RollbackOnFail: aiPlan.RollbackOnFail,
			UseAI:          true,
		}
		normalizePlan(&plan, snap, panel)
		enrichPlanDefaults(&plan, snap, panel)
		if plan.DeployScript == "" {
			plan.DeployScript = heuristicPlan(s.dataDir, snap, panel, plan.Domain).DeployScript
		}
		ch <- planOutcome{plan: plan}
	}()

	select {
	case out := <-ch:
		if out.err != nil {
			plan := out.plan
			plan.UseAI = false
			applyFrameworkDefaults(&plan, snap, s.dataDir, panel, false)
			plan.PostNotes = sanitizePostNotes(plan.PostNotes, resolveFramework(plan, snap), panel, false)
			return plan, "AI 分析失败，已使用规则引擎：" + out.err.Error(), nil
		}
		applyFrameworkDefaults(&out.plan, snap, s.dataDir, panel, false)
		out.plan.PostNotes = sanitizePostNotes(out.plan.PostNotes, resolveFramework(out.plan, snap), panel, false)
		return out.plan, "", nil
	case <-time.After(aiPlanWaitTimeout(st)):
		plan := heuristicPlan(s.dataDir, snap, panel, req.Domain)
		plan.UseAI = false
		applyFrameworkDefaults(&plan, snap, s.dataDir, panel, false)
		plan.PostNotes = sanitizePostNotes(plan.PostNotes, resolveFramework(plan, snap), panel, false)
		return plan, "AI 响应超时，已使用规则引擎生成部署方案", nil
	}
}

func (s *Service) planInstallAfterSiteCreate(snap *RepoSnapshot, panel PanelContext, req AnalyzeRequest, site *models.Website, createRes *website.CreateResult) (DeployPlan, string, error) {
	st := s.aichat.AssistantStatus()
	sitePanel := buildSiteContextJSON(site, createRes, panel, s.dataDir)

	if !st.Configured {
		plan := heuristicPlan(s.dataDir, snap, panel, req.Domain)
		plan.UseAI = false
		msg := "AI 未配置，已使用规则引擎生成安装脚本"
		if st.Message != "" {
			msg = st.Message + "；" + msg
		}
		return plan, msg, nil
	}

	type planOutcome struct {
		plan DeployPlan
		err  error
	}
	ch := make(chan planOutcome, 1)
	go func() {
		aiPlan, err := s.aichat.SiteBootstrapPlan(aichat.SiteBootstrapRequest{
			Repo:           snapToAIRepo(snap),
			Panel:          sitePanel,
			Domain:         req.Domain,
			Notes:          req.Notes,
			Branch:         snap.Branch,
			RepoURL:        snap.RepoURL,
			SiteRoot:       site.RootPath,
			PhpVersion:     site.PhpVersion,
			DbHost:         "127.0.0.1",
			DbName:         dbField(createRes, func(r *website.CreateResult) string { return r.DbName }),
			DbUser:         dbField(createRes, func(r *website.CreateResult) string { return r.DbUser }),
			DbPassword:     dbField(createRes, func(r *website.CreateResult) string { return r.DbPassword }),
			WebsiteCreated: true,
		})
		if err != nil {
			ch <- planOutcome{plan: heuristicPlan(s.dataDir, snap, panel, req.Domain), err: err}
			return
		}
		plan := aiPlanToDeployPlan(aiPlan, req.Domain, snap)
		plan.UseAI = true
		normalizePlan(&plan, snap, panel)
		if plan.DeployScript == "" {
			plan.DeployScript = heuristicPlan(s.dataDir, snap, panel, plan.Domain).DeployScript
		}
		ch <- planOutcome{plan: plan}
	}()

	select {
	case out := <-ch:
		if out.err != nil {
			plan := out.plan
			plan.UseAI = false
			return plan, "AI 生成安装脚本失败，已使用规则引擎：" + out.err.Error(), nil
		}
		applyFrameworkDefaults(&out.plan, snap, s.dataDir, panel, false)
		return out.plan, "", nil
	case <-time.After(aiPlanWaitTimeout(st)):
		plan := heuristicPlan(s.dataDir, snap, panel, req.Domain)
		plan.UseAI = false
		applyFrameworkDefaults(&plan, snap, s.dataDir, panel, false)
		return plan, "AI 响应超时，已使用规则引擎生成安装脚本", nil
	}
}

func dbField(res *website.CreateResult, fn func(*website.CreateResult) string) string {
	if res == nil {
		return ""
	}
	return fn(res)
}

func aiPlanToDeployPlan(aiPlan *aichat.SiteBootstrapPlanResult, domain string, snap *RepoSnapshot) DeployPlan {
	return DeployPlan{
		ProjectType:    aiPlan.ProjectType,
		Summary:        aiPlan.Summary,
		Domain:         firstNonEmpty(aiPlan.Domain, domain, suggestDomain("", snap.RepoURL)),
		PhpVersion:     aiPlan.PhpVersion,
		NeedDatabase:   aiPlan.NeedDatabase,
		CreateFTP:      aiPlan.CreateFTP,
		DeployScript:   aiPlan.DeployScript,
		PostNotes:      aiPlan.PostNotes,
		Confidence:     aiPlan.Confidence,
		Framework:      aiPlan.Framework,
		DocumentRoot:   aiPlan.DocumentRoot,
		UseDocker:      aiPlan.UseDocker,
		UsePM2:         aiPlan.UsePM2,
		NodePort:       aiPlan.NodePort,
		EnableWebhook:  aiPlan.EnableWebhook,
		RollbackOnFail: aiPlan.RollbackOnFail,
		UseAI:          true,
	}
}

func applyLaravelDefaults(plan *DeployPlan, snap *RepoSnapshot, dataDir string, panel PanelContext) {
	applyFrameworkDefaults(plan, snap, dataDir, panel, plan.NeedDatabase)
}

func snapToAIRepo(s *RepoSnapshot) aichat.SiteRepoBrief {
	return aichat.SiteRepoBrief{
		RepoURL:          s.RepoURL,
		Branch:           s.Branch,
		FrameworkHint:    s.FrameworkHint,
		FileList:         s.FileList,
		HasComposer:      s.HasComposer,
		HasPackageJSON:   s.HasPackageJSON,
		HasDockerfile:    s.HasDockerfile,
		HasDockerCompose: s.HasDockerCompose,
		HasNodeServer:    s.HasNodeServer,
		ComposerJSON:     s.ComposerJSON,
		PackageJSON:      s.PackageJSON,
		Dockerfile:       s.Dockerfile,
		Readme:           s.Readme,
		LockfileKind:       s.LockfileKind,
		UsesCatalog:        s.UsesCatalog,
		PackageManager:     s.PackageManager,
		NodeMajorRequired:  s.NodeMajorRequired,
		PHPVersionRequired: s.PHPVersionRequired,
		IsMonorepo:         s.IsMonorepo,
		HasTurbo:           s.HasTurbo,
		PrimaryAppFilter:   s.PrimaryAppFilter,
		PrimaryAppOutDir:   s.PrimaryAppOutDir,
		BuildEnvKeys:       s.BuildEnvKeys,
	}
}

func normalizePlan(plan *DeployPlan, snap *RepoSnapshot, panel PanelContext) {
	if plan.ProjectType == "" {
		plan.ProjectType = mapFrameworkType(snap.FrameworkHint)
	}
	if plan.Framework == "" {
		plan.Framework = snap.FrameworkHint
	}
	if plan.PhpVersion == "" {
		if php := snap.suggestedPHPVersion(); php != "" {
			plan.PhpVersion = php
		} else {
			switch plan.Framework {
			case "laravel":
				plan.PhpVersion = "8.4"
			case "wordpress":
				plan.PhpVersion = "8.2"
			default:
				plan.PhpVersion = "8.3"
			}
		}
	}
	if plan.PhpVersion == "static" || plan.ProjectType == "static" {
		plan.PhpVersion = "static"
	}
	if plan.Confidence == "" {
		plan.Confidence = "medium"
	}
	_ = panel
}

func mapFrameworkType(fw string) string {
	switch fw {
	case "laravel", "symfony", "php", "wordpress":
		return "php"
	case "nodejs":
		return "node"
	default:
		return "static"
	}
}

func heuristicPlan(dataDir string, snap *RepoSnapshot, panel PanelContext, domainHint string) DeployPlan {
	domain := suggestDomain(domainHint, snap.RepoURL)
	plan := DeployPlan{
		ProjectType:    "static",
		Summary:        fmt.Sprintf("检测到 %s 项目，将创建站点并 git clone 部署", snap.FrameworkHint),
		Domain:         domain,
		PhpVersion:     "static",
		Framework:      snap.FrameworkHint,
		Confidence:     "heuristic",
		UseAI:          false,
		RollbackOnFail: true,
		EnableWebhook:  true,
		DeployScript:   defaultDeployScript(dataDir, snap, panel, ""),
	}
	switch snap.FrameworkHint {
	case "laravel":
		plan.ProjectType = "php"
		plan.PhpVersion = "8.4"
		plan.NeedDatabase = true
		plan.DocumentRoot = "public"
		plan.DeployScript = laravelDeployScript(dataDir, panel)
		plan.PostNotes = laravelPostNotes(dataDir, panel, true, true)
		plan.Summary = "Laravel 项目：clone、Composer、前端构建与缓存优化，面板将自动配置数据库与 public 目录"
	case "symfony", "php":
		plan.ProjectType = "php"
		plan.PhpVersion = firstNonEmpty(snap.suggestedPHPVersion(), "8.3")
		plan.NeedDatabase = strings.Contains(strings.ToLower(snap.ComposerJSON), "laravel") || snap.FrameworkHint == "laravel"
		plan.DocumentRoot = "public"
	case "wordpress":
		plan.ProjectType = "php"
		plan.PhpVersion = "8.2"
		plan.NeedDatabase = true
	case "nextjs", "vue", "react":
		plan.ProjectType = "static"
		plan.PhpVersion = "static"
		plan.DeployScript = genericBuildDeployScript(dataDir, panel, snap)
		if snap.IsMonorepo && snap.PrimaryAppOutDir != "" {
			plan.DocumentRoot = snap.PrimaryAppOutDir
			plan.Summary = "Monorepo：构建 " + snap.PrimaryAppFilter + "，Nginx 托管 " + snap.PrimaryAppOutDir
		} else {
			plan.DocumentRoot = firstNonEmpty(plan.DocumentRoot, "dist")
			plan.Summary = "前端项目：clone 后 npm build，Nginx 托管静态资源"
		}
	case "nodejs":
		plan.ProjectType = "node"
		plan.PhpVersion = "static"
		plan.UsePM2 = true
		plan.Summary = "Node 服务：clone 后 npm install，PM2 常驻 + Nginx 反代"
	}
	if snap.HasNodeServer && !plan.UseDocker {
		plan.ProjectType = "node"
		plan.UsePM2 = true
		plan.PhpVersion = "static"
	}
	if (snap.HasDockerfile || snap.HasDockerCompose) && panel.DockerAvailable {
		plan.UseDocker = true
		plan.ProjectType = "docker"
		plan.Framework = "docker"
		plan.DeployScript = dockerDeployScript(snap)
		plan.Summary = "Docker 项目：clone 后 docker compose 构建并启动"
	} else if snap.HasDockerfile && !panel.DockerAvailable {
		plan.PostNotes = "仓库含 Dockerfile，但面板未检测到 Docker，已使用 git clone 方式部署"
	}
	return plan
}

func suggestDomain(hint, repoURL string) string {
	if d := strings.TrimSpace(hint); d != "" {
		return d
	}
	u := strings.TrimSuffix(repoURL, "/")
	u = strings.TrimSuffix(u, ".git")
	parts := strings.Split(u, "/")
	if len(parts) >= 2 {
		name := parts[len(parts)-1]
		name = strings.ToLower(name)
		return name + ".local"
	}
	return "github-site.local"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func defaultDeployScript(dataDir string, snap *RepoSnapshot, panel PanelContext, token string) string {
	repo := normalizeRepoURL(snap.RepoURL)
	repo = cloneURLWithToken(repo, token)
	branch := snap.Branch
	if snap.FrameworkHint == "laravel" {
		return laravelDeployScript(dataDir, panel)
	}
	if runtime.GOOS == "windows" {
		return windowsDeployScript(repo, branch, snap, panel)
	}
	var b strings.Builder
	b.WriteString("set -e\n")
	b.WriteString(gitCloneIntoSiteRootBlock())
	switch snap.FrameworkHint {
	case "laravel", "symfony", "php":
		if bin := strings.TrimSpace(appstore.ComposerBinary(dataDir)); bin != "" {
			b.WriteString(bin + " install --no-dev --optimize-autoloader 2>/dev/null || " + bin + " install --no-dev || true\n")
		} else if panel.ComposerAvail {
			b.WriteString("composer install --no-dev --optimize-autoloader 2>/dev/null || composer install --no-dev || true\n")
		}
		if snap.HasArtisan {
			b.WriteString("cp -n .env.example .env 2>/dev/null || true\n")
			b.WriteString("php artisan key:generate --force 2>/dev/null || true\n")
		}
	case "wordpress":
		b.WriteString("# WordPress: 请在面板中完成 wp-config.php 与数据库配置\n")
	case "nextjs", "vue", "react", "nodejs":
		if panel.NPMAvailable {
			b.WriteString(npmInstallBlock())
			b.WriteString("npm run build 2>/dev/null || true\n")
		}
	}
	return b.String()
}

func windowsDeployScript(repo, branch string, snap *RepoSnapshot, panel PanelContext) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("git clone --depth 1 -b %s %s .\r\n", branch, repo))
	if snap.HasComposer && panel.ComposerAvail {
		b.WriteString("composer install --no-dev\r\n")
	}
	if snap.HasPackageJSON && panel.NPMAvailable {
		b.WriteString("npm install\r\nnpm run build\r\n")
	}
	return b.String()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func planToJSON(plan DeployPlan) string {
	b, _ := json.Marshal(plan)
	return string(b)
}
