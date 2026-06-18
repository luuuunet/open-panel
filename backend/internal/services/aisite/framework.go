package aisite

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/website"
)

func resolveFramework(plan DeployPlan, snap *RepoSnapshot) string {
	fw := frameworkHint(plan, snap)
	if fw != "" && fw != "unknown" {
		return fw
	}
	if snap == nil {
		return "static"
	}
	if snap.FrameworkHint != "" && snap.FrameworkHint != "unknown" {
		return snap.FrameworkHint
	}
	if snap.HasDockerCompose || snap.HasDockerfile {
		return "docker"
	}
	if snap.HasNodeServer {
		return "nodejs"
	}
	if snap.HasPackageJSON && !snap.HasComposer {
		return "static"
	}
	if snap.HasComposer {
		return "php"
	}
	return "static"
}

func hasKnownDeployTemplate(fw string) bool {
	switch fw {
	case "laravel", "wordpress", "symfony", "php", "nextjs", "vue", "react", "nodejs", "docker":
		return true
	default:
		return false
	}
}

func applyFrameworkDefaults(plan *DeployPlan, snap *RepoSnapshot, dataDir string, panel PanelContext, dbConfigured bool) {
	fw := resolveFramework(*plan, snap)
	plan.Framework = fw

	switch fw {
	case "laravel":
		plan.ProjectType = "php"
		plan.PhpVersion = firstNonEmpty(plan.PhpVersion, "8.4")
		plan.NeedDatabase = true
		plan.DocumentRoot = firstNonEmpty(plan.DocumentRoot, "public")
		plan.DeployScript = laravelDeployScript(dataDir, panel)
		plan.Summary = "Laravel：自动安装环境、部署代码、配置数据库与 public 目录"
	case "wordpress":
		plan.ProjectType = "php"
		plan.PhpVersion = firstNonEmpty(plan.PhpVersion, "8.2")
		plan.NeedDatabase = true
		plan.DeployScript = wordpressDeployScript()
		plan.Summary = "WordPress：自动部署并配置数据库"
	case "symfony", "php":
		plan.ProjectType = "php"
		plan.PhpVersion = firstNonEmpty(plan.PhpVersion, "8.3")
		if plan.NeedDatabase || strings.Contains(strings.ToLower(snap.ComposerJSON), "doctrine") {
			plan.NeedDatabase = true
		}
		plan.DocumentRoot = firstNonEmpty(plan.DocumentRoot, "public")
		plan.DeployScript = phpComposerDeployScript(dataDir, panel, snap)
		plan.Summary = "PHP 项目：Composer 安装与 Web 配置"
	case "nextjs":
		plan.PhpVersion = "static"
		if snap != nil && snap.IsMonorepo && snap.PrimaryAppPath != "" {
			plan.DeployScript = genericBuildDeployScript(dataDir, panel, snap)
			if snap.PrimaryAppOutDir != "" && !nextNeedsPM2(snap.PrimaryAppOutDir) {
				plan.ProjectType = "static"
				plan.DocumentRoot = snap.PrimaryAppOutDir
				plan.Summary = "Monorepo/Next.js：构建 " + snap.PrimaryAppFilter + "，Nginx 托管 " + snap.PrimaryAppOutDir
			} else {
				plan.ProjectType = "node"
				plan.UsePM2 = true
				plan.DocumentRoot = snap.PrimaryAppPath
				plan.Summary = "Monorepo/Next.js：构建 " + snap.PrimaryAppFilter + "，PM2 运行 next start + Nginx 反代"
			}
		} else {
			plan.ProjectType = "static"
			plan.DocumentRoot = firstNonEmpty(plan.DocumentRoot, "out", ".next")
			plan.DeployScript = frontendBuildDeployScript(snap, "next")
			plan.Summary = "Next.js：构建静态/SSR 产物并由 Nginx 托管"
		}
	case "vue", "react":
		plan.ProjectType = "static"
		plan.PhpVersion = "static"
		plan.DocumentRoot = firstNonEmpty(plan.DocumentRoot, "dist")
		plan.DeployScript = frontendBuildDeployScript(snap, fw)
		plan.Summary = fmt.Sprintf("%s 前端：npm build 后由 Nginx 托管", fw)
	case "nodejs":
		plan.ProjectType = "node"
		plan.PhpVersion = "static"
		plan.UsePM2 = true
		plan.DeployScript = nodeServiceDeployScript(snap)
		plan.Summary = "Node 服务：npm 安装 + PM2 常驻 + Nginx 反代"
	case "docker":
		plan.UseDocker = true
		plan.ProjectType = "docker"
		plan.PhpVersion = "static"
		plan.DeployScript = dockerDeployScript(snap)
		plan.Summary = "Docker 项目：compose 构建并启动"
	default:
		if snap != nil && snap.HasPackageJSON {
			plan.DeployScript = genericBuildDeployScript(dataDir, panel, snap)
			plan.Summary = "通用项目：git clone 并按 package.json/composer 自动构建"
		}
	}

	if plan.DeployScript == "" {
		plan.DeployScript = defaultDeployScript(dataDir, snap, panel, "")
	}
	plan.PostNotes = automatedPostNotes(fw, panel, plan.NeedDatabase && dbConfigured)
	if plan.EnableWebhook == false {
		plan.EnableWebhook = true
	}
	if plan.RollbackOnFail == false {
		plan.RollbackOnFail = true
	}
}

func nodeAppKeyForSnap(snap *RepoSnapshot) string {
	if snap == nil {
		return "nodejs20"
	}
	return snap.suggestedNodeAppKey()
}

func snapSuggestedPHP(snap *RepoSnapshot, fallback string) string {
	if snap == nil {
		return fallback
	}
	if v := snap.suggestedPHPVersion(); v != "" {
		return v
	}
	return fallback
}

func profileRequiredApps(fw string, plan DeployPlan, snap *RepoSnapshot) []string {
	var keys []string
	add := func(k ...string) { keys = append(keys, k...) }

	switch fw {
	case "laravel":
		add("mysql", phpAppKey(firstNonEmpty(plan.PhpVersion, snapSuggestedPHP(snap, "8.4"))), "composer", nodeAppKeyForSnap(snap))
	case "wordpress":
		add("mysql", phpAppKey("8.2"))
	case "symfony", "php":
		add(phpAppKey(firstNonEmpty(plan.PhpVersion, snapSuggestedPHP(snap, "8.3"))), "composer")
		if plan.NeedDatabase {
			add("mysql")
		}
	case "nextjs", "vue", "react":
		add(nodeAppKeyForSnap(snap))
	case "nodejs":
		add(nodeAppKeyForSnap(snap), "pm2")
	case "docker":
		add("docker")
	default:
		if snap != nil {
			if snap.HasComposer {
				add("composer", phpAppKey(firstNonEmpty(plan.PhpVersion, snapSuggestedPHP(snap, "8.3"))))
			}
			if snap.HasPackageJSON {
				add(nodeAppKeyForSnap(snap))
			}
			if snap.HasNodeServer {
				add("pm2")
			}
			if plan.NeedDatabase {
				add("mysql")
			}
		}
	}
	if plan.UseDocker && fw != "docker" {
		add("docker")
	}
	if plan.UsePM2 && fw != "nodejs" {
		add("pm2", "nodejs20")
	}
	return dedupeKeys(keys)
}

func (s *Service) requiredDeployApps(plan DeployPlan, snap *RepoSnapshot) []string {
	fw := resolveFramework(plan, snap)
	keys := profileRequiredApps(fw, plan, snap)
	for _, k := range s.missingEnvApps(plan, snap) {
		if !deployAppReady(s, k) && k != "" {
			keys = append(keys, k)
		}
	}
	return dedupeKeys(keys)
}

func (s *Service) finalizeDeployPlan(plan *DeployPlan, snap *RepoSnapshot, panel PanelContext, createRes *website.CreateResult, aiPlan *DeployPlan) {
	hasDB := createRes != nil && createRes.DbName != ""
	fw := resolveFramework(*plan, snap)

	if aiPlan != nil {
		if aiPlan.Summary != "" && plan.Summary == "" {
			plan.Summary = aiPlan.Summary
		}
		if aiPlan.DocumentRoot != "" {
			plan.DocumentRoot = aiPlan.DocumentRoot
		}
		if aiPlan.UsePM2 {
			plan.UsePM2 = true
		}
		if aiPlan.UseDocker {
			plan.UseDocker = true
		}
	}

	if hasKnownDeployTemplate(fw) {
		applyFrameworkDefaults(plan, snap, s.dataDir, panel, hasDB)
	} else if aiPlan != nil && strings.TrimSpace(aiPlan.DeployScript) != "" {
		plan.DeployScript = aiPlan.DeployScript
		if aiPlan.Framework != "" {
			plan.Framework = aiPlan.Framework
		}
	} else {
		applyFrameworkDefaults(plan, snap, s.dataDir, panel, hasDB)
	}
	if snap != nil && snap.IsMonorepo && !strings.Contains(plan.DeployScript, "pnpm --filter") {
		plan.DeployScript = genericBuildDeployScript(s.dataDir, panel, snap)
		if snap.PrimaryAppOutDir != "" && !nextNeedsPM2(snap.PrimaryAppOutDir) {
			plan.DocumentRoot = snap.PrimaryAppOutDir
		}
	}
	plan.PostNotes = sanitizePostNotes(plan.PostNotes, resolveFramework(*plan, snap), panel, hasDB)
	if plan.PostNotes == "" {
		plan.PostNotes = automatedPostNotes(resolveFramework(*plan, snap), panel, hasDB)
	}
}

func automatedPostNotes(fw string, panel PanelContext, dbConfigured bool) string {
	var notes []string
	notes = append(notes, "面板已全自动完成：Git/运行环境安装、站点创建、代码拉取与构建。")
	switch fw {
	case "laravel", "symfony", "php":
		if dbConfigured {
			notes = append(notes, "数据库、.env、migrate、public 运行目录已由面板自动配置。")
		}
	case "wordpress":
		if dbConfigured {
			notes = append(notes, "MySQL 与 wp-config.php 已由面板自动配置。")
		}
	case "nodejs":
		notes = append(notes, "Node 进程已通过 PM2 启动，Nginx 反代已配置。")
	case "nextjs", "vue", "react", "static":
		notes = append(notes, "前端构建产物目录已设为 Nginx 文档根。")
	case "docker":
		notes = append(notes, "Docker Compose 已启动，请确认端口映射与域名反代。")
	}
	notes = append(notes, "可选：申请 HTTPS、配置队列 worker（Supervisor php artisan queue:work）；schedule:run 已由面板自动注册 cron。")
	var b strings.Builder
	for i, n := range notes {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, n))
	}
	return strings.TrimSpace(b.String())
}

func wordpressDeployScript() string {
	return `set -euo pipefail
` + gitCloneIntoSiteRootBlock()
}

func phpComposerDeployScript(dataDir string, panel PanelContext, snap *RepoSnapshot) string {
	composer := laravelComposerInstallBlock(dataDir, panel)
	var b strings.Builder
	b.WriteString("set -euo pipefail\n")
	b.WriteString(gitCloneIntoSiteRootBlock())
	b.WriteString(composer)
	if snap != nil && snap.HasArtisan {
		b.WriteString("cp -n .env.example .env 2>/dev/null || true\n")
		b.WriteString("php artisan key:generate --force 2>/dev/null || true\n")
	}
	if snap != nil && snap.HasPackageJSON {
		b.WriteString(laravelNPMBuildBlock(panel))
	}
	return b.String()
}

func frontendBuildDeployScript(snap *RepoSnapshot, fw string) string {
	_ = snap
	_ = fw
	return fmt.Sprintf(`set -euo pipefail
%s
%s
`, gitCloneIntoSiteRootBlock(), npmInstallBlock()+npmBuildBlock())
}

func nodeServiceDeployScript(snap *RepoSnapshot) string {
	startScript := "npm start"
	if snap != nil && snap.PackageJSON != "" {
		if strings.Contains(strings.ToLower(snap.PackageJSON), `"start"`) {
			startScript = "npm start"
		} else if strings.Contains(strings.ToLower(snap.PackageJSON), `"dev"`) {
			startScript = "npm run dev"
		}
	}
	_ = startScript
	return `set -euo pipefail
` + gitCloneIntoSiteRootBlock() + npmInstallBlock() + npmBuildBlock()
}

func genericBuildDeployScript(dataDir string, panel PanelContext, snap *RepoSnapshot) string {
	var b strings.Builder
	b.WriteString("set -euo pipefail\n")
	b.WriteString(gitCloneIntoSiteRootBlock())
	if snap.HasComposer {
		b.WriteString(laravelComposerInstallBlock(dataDir, panel))
	}
	if snap.HasPackageJSON {
		b.WriteString(laravelNPMBuildBlock(panel))
	}
	return b.String()
}
