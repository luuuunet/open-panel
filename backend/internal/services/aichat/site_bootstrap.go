package aichat

import (
	"encoding/json"
	"fmt"
	"strings"
)

type SiteRepoBrief struct {
	RepoURL        string   `json:"repo_url"`
	Branch         string   `json:"branch"`
	FrameworkHint  string   `json:"framework_hint"`
	FileList       []string `json:"file_list"`
	HasComposer    bool     `json:"has_composer"`
	HasPackageJSON bool     `json:"has_package_json"`
	HasDockerfile  bool     `json:"has_dockerfile"`
	HasDockerCompose bool   `json:"has_docker_compose"`
	HasNodeServer  bool     `json:"has_node_server"`
	ComposerJSON   string   `json:"composer_json,omitempty"`
	PackageJSON    string   `json:"package_json,omitempty"`
	Dockerfile     string   `json:"dockerfile,omitempty"`
	Readme         string   `json:"readme,omitempty"`
	LockfileKind       string `json:"lockfile_kind,omitempty"`
	UsesCatalog        bool   `json:"uses_catalog"`
	PackageManager     string `json:"package_manager,omitempty"`
	NodeMajorRequired  int    `json:"node_major_required,omitempty"`
	PHPVersionRequired string `json:"php_version_required,omitempty"`
	IsMonorepo         bool     `json:"is_monorepo"`
	HasTurbo           bool     `json:"has_turbo"`
	PrimaryAppFilter   string   `json:"primary_app_filter,omitempty"`
	PrimaryAppOutDir   string   `json:"primary_app_out_dir,omitempty"`
	BuildEnvKeys       []string `json:"build_env_keys,omitempty"`
}

type SiteBootstrapRequest struct {
	RepoURL        string
	Branch         string
	Domain         string
	Notes          string
	Repo           SiteRepoBrief
	Panel          string `json:"panel,omitempty"`
	SiteRoot       string
	PhpVersion     string
	DbHost         string
	DbName         string
	DbUser         string
	DbPassword     string
	WebsiteCreated bool
}

type SiteBootstrapPlanResult struct {
	ProjectType  string `json:"project_type"`
	Summary      string `json:"summary"`
	Domain       string `json:"domain"`
	PhpVersion   string `json:"php_version"`
	NeedDatabase bool   `json:"need_database"`
	CreateFTP    bool   `json:"create_ftp"`
	DeployScript string `json:"deploy_script"`
	PostNotes    string `json:"post_notes"`
	Confidence   string `json:"confidence"`
	Framework    string `json:"framework"`
	DocumentRoot   string `json:"document_root"`
	UseDocker      bool   `json:"use_docker"`
	UsePM2         bool   `json:"use_pm2"`
	NodePort       int    `json:"node_port"`
	EnableWebhook  bool   `json:"enable_webhook"`
	RollbackOnFail bool   `json:"rollback_on_fail"`
}

func (s *Service) SiteBootstrapPlan(req SiteBootstrapRequest) (*SiteBootstrapPlanResult, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, fmt.Errorf("请先在面板设置中启用 AI 助手")
	}
	if cfg.APIKey == "" && cfg.Provider != "ollama" && cfg.Provider != "huggingface" {
		return nil, fmt.Errorf("AI API Key 未配置")
	}

	systemPrompt := buildSiteBootstrapSystemPrompt(req)
	messages := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "请为一键全自动部署生成 JSON 方案（deploy_script 在站点根目录执行，面板已负责环境与站点创建）。"},
	}
	reply, err := s.callChatAPI(cfg, messages)
	if err != nil {
		return nil, err
	}
	plan, err := extractSiteBootstrapPlan(reply)
	if err != nil {
		return nil, fmt.Errorf("AI 返回格式无效: %w", err)
	}
	return plan, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func buildSiteBootstrapSystemPrompt(req SiteBootstrapRequest) string {
	var b strings.Builder
	b.WriteString(`You are an expert DevOps engineer for OWPanel hosting panel.

This is ONE-CLICK FULL AUTO deploy: user only provides GitHub URL. The panel automatically:
1) installs Git + runtime (nginx, mysql, php, composer, nodejs, docker, pm2)
2) creates website + database
3) runs install script in site root
4) configures .env, migrate, document_root, PM2 reverse proxy

Supported frameworks: Laravel, WordPress, Symfony/PHP, Vue/React/Next static, Node.js (PM2), Docker Compose, generic composer/npm projects.

Output JSON ONLY in a markdown code block:

` + "```json\n" + `{
  "project_type": "php|static|node",
  "summary": "brief Chinese summary for the user",
  "domain": "suggested.domain.local",
  "php_version": "8.4|8.3|8.2|8.1|static",
  "need_database": true,
  "create_ftp": false,
  "deploy_script": "shell script to run IN the site root after site is created (use {{root}} {{branch}} {{repo}} placeholders)",
  "post_notes": "manual steps if any",
  "confidence": "high|medium|low",
  "framework": "laravel|wordpress|vue|etc",
  "document_root": "public or dist or empty for root",
  "use_docker": false,
  "use_pm2": false,
  "node_port": 31080,
  "enable_webhook": true,
  "rollback_on_fail": true
}
` + "```\n\n")
	b.WriteString("Rules:\n")
	if req.WebsiteCreated {
		b.WriteString("- The website is ALREADY created on the panel; Nginx vhost exists\n")
		b.WriteString("- deploy_script runs with cwd = site root; clone repo into current directory (do NOT cd elsewhere)\n")
		b.WriteString("- Use {{repo}} {{branch}} {{root}} placeholders; panel replaces them before execution\n")
		if req.SiteRoot != "" {
			b.WriteString("- Site root (install here): ")
			b.WriteString(req.SiteRoot)
			b.WriteString("\n")
		}
		if req.PhpVersion != "" {
			b.WriteString("- PHP version already set on site: ")
			b.WriteString(req.PhpVersion)
			b.WriteString("\n")
		}
		if req.DbName != "" {
			b.WriteString("- MySQL database already created; use these credentials in deploy_script (.env etc.):\n")
			b.WriteString("  DB_HOST=")
			b.WriteString(firstNonEmpty(req.DbHost, "127.0.0.1"))
			b.WriteString("\n  DB_DATABASE=")
			b.WriteString(req.DbName)
			b.WriteString("\n  DB_USERNAME=")
			b.WriteString(req.DbUser)
			b.WriteString("\n  DB_PASSWORD=")
			b.WriteString(req.DbPassword)
			b.WriteString("\n")
		}
	} else {
		b.WriteString("- deploy_script must clone repo into current directory (site root is empty except placeholder)\n")
		b.WriteString("- Use {{repo}} {{branch}} {{root}} in deploy_script; panel replaces them\n")
	}
	b.WriteString("- On Linux use bash; prefer composer install / npm run build when detected\n")
	b.WriteString("- Laravel: php_version 8.2+, need_database true, document_root public\n")
	b.WriteString("- WordPress: need_database true\n")
	b.WriteString("- Pure static/SPA after build: php_version static\n")
	b.WriteString("- Node server (Express/Nest): project_type node, use_pm2 true, php_version static\n")
	b.WriteString("- Docker/docker-compose: use_docker true, php_version static, deploy_script runs compose\n")
	b.WriteString("- enable_webhook true for git push auto-deploy; rollback_on_fail true to delete site on failure\n")
	b.WriteString("- OWPanel AUTO-INSTALLS missing runtime via app store BEFORE deploy: nginx, mysql, php, composer, nodejs, docker as needed\n")
	b.WriteString("- OWPanel AUTO-CONFIGURES after deploy: Laravel .env DB credentials, migrate, document_root public — do NOT tell user to apt install composer/npm or manually edit .env\n")
	b.WriteString("- post_notes: only OPTIONAL follow-ups (HTTPS, queue worker, cron schedule); never manual env/Composer/NPM install steps\n")
	b.WriteString("- Respond in Chinese for summary and post_notes\n\n")

	b.WriteString("Repository URL: ")
	b.WriteString(req.RepoURL)
	b.WriteString("\nBranch: ")
	b.WriteString(req.Branch)
	if req.Domain != "" {
		b.WriteString("\nUser domain hint: ")
		b.WriteString(req.Domain)
	}
	if req.Notes != "" {
		b.WriteString("\nUser notes: ")
		b.WriteString(req.Notes)
	}
	b.WriteString("\n\nFramework hint: ")
	b.WriteString(req.Repo.FrameworkHint)
	if req.Repo.PHPVersionRequired != "" {
		b.WriteString("\nPHP required (from composer.json): ")
		b.WriteString(req.Repo.PHPVersionRequired)
	}
	if req.Repo.NodeMajorRequired > 0 {
		b.WriteString("\nNode major required: ")
		b.WriteString(fmt.Sprintf("%d", req.Repo.NodeMajorRequired))
	}
	if req.Repo.LockfileKind != "" {
		b.WriteString("\nJS lockfile: ")
		b.WriteString(req.Repo.LockfileKind)
	}
	if req.Repo.UsesCatalog {
		b.WriteString("\nUses npm catalog: dependencies — prefer pnpm or npm 10.7+")
	}
	if req.Repo.PackageManager != "" {
		b.WriteString("\npackageManager: ")
		b.WriteString(req.Repo.PackageManager)
	}
	b.WriteString("\nFiles: ")
	b.WriteString(strings.Join(req.Repo.FileList, ", "))
	b.WriteString("\n\ncomposer.json:\n")
	b.WriteString(req.Repo.ComposerJSON)
	b.WriteString("\n\npackage.json:\n")
	b.WriteString(req.Repo.PackageJSON)
	b.WriteString("\n\nREADME excerpt:\n")
	b.WriteString(req.Repo.Readme)
	if req.Panel != "" {
		b.WriteString("\n\nPanel environment:\n")
		b.WriteString(req.Panel)
	}
	return b.String()
}

func extractSiteBootstrapPlan(text string) (*SiteBootstrapPlanResult, error) {
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
		return nil, fmt.Errorf("no JSON found in AI response")
	}
	var plan SiteBootstrapPlanResult
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return nil, err
	}
	if plan.DeployScript == "" {
		return nil, fmt.Errorf("deploy_script is required")
	}
	return &plan, nil
}
