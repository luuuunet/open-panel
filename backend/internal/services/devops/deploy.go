package devops

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/settings"
)

type DeployConfigView struct {
	models.SiteDeployConfig
	Domain   string `json:"domain"`
	RootPath string `json:"root_path"`
	HookURL  string `json:"hook_url"`
	CIURL    string `json:"ci_url"`
}

func (s *Service) ListDeployConfigs() ([]DeployConfigView, error) {
	var configs []models.SiteDeployConfig
	if err := s.db.Order("id desc").Find(&configs).Error; err != nil {
		return nil, err
	}
	out := make([]DeployConfigView, 0, len(configs))
	for _, cfg := range configs {
		out = append(out, s.enrichDeployConfig(&cfg))
	}
	return out, nil
}

func (s *Service) GetDeployConfig(websiteID uint) (*DeployConfigView, error) {
	var cfg models.SiteDeployConfig
	err := s.db.Where("website_id = ?", websiteID).First(&cfg).Error
	if err != nil {
		cfg = models.SiteDeployConfig{WebsiteID: websiteID, Branch: "main", CIProvider: "manual"}
	}
	v := s.enrichDeployConfig(&cfg)
	return &v, nil
}

func (s *Service) SaveDeployConfig(cfg *models.SiteDeployConfig) (*DeployConfigView, error) {
	cfg.Branch = strings.TrimSpace(cfg.Branch)
	if cfg.Branch == "" {
		cfg.Branch = "main"
	}
	if cfg.WebhookToken == "" {
		cfg.WebhookToken = newWebhookToken()
	}
	var existing models.SiteDeployConfig
	if err := s.db.Where("website_id = ?", cfg.WebsiteID).First(&existing).Error; err == nil {
		cfg.ID = existing.ID
		if cfg.WebhookSecret == "" {
			cfg.WebhookSecret = existing.WebhookSecret
		}
		if cfg.WebhookSecret == "" {
			cfg.WebhookSecret = newWebhookSecret()
		}
		if err := s.db.Save(cfg).Error; err != nil {
			return nil, err
		}
	} else {
		if cfg.WebhookSecret == "" {
			cfg.WebhookSecret = newWebhookSecret()
		}
		if err := s.db.Create(cfg).Error; err != nil {
			return nil, err
		}
	}
	v := s.enrichDeployConfig(cfg)
	return &v, nil
}

func (s *Service) enrichDeployConfig(cfg *models.SiteDeployConfig) DeployConfigView {
	v := DeployConfigView{SiteDeployConfig: *cfg}
	var site models.Website
	if s.db.First(&site, cfg.WebsiteID).Error == nil {
		v.Domain = site.Domain
		v.RootPath = site.RootPath
	}
	base := panelPublicBase(s.settings)
	if cfg.WebhookToken != "" {
		v.HookURL = fmt.Sprintf("%s/api/v1/deploy/hook/%s", base, cfg.WebhookToken)
		v.CIURL = fmt.Sprintf("%s/api/v1/deploy/ci/%s", base, cfg.WebhookToken)
	}
	return v
}

func panelPublicBase(st *settings.Service) string {
	all, _ := st.GetAll()
	port := strings.TrimSpace(all["panel_port"])
	if port == "" {
		port = "8888"
	}
	safe := strings.Trim(strings.TrimSpace(all["panel_safe_path"]), "/")
	path := ""
	if safe != "" {
		path = "/" + safe
	}
	return fmt.Sprintf("http://127.0.0.1:%s%s", port, path)
}

func newWebhookToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func newWebhookSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Service) ListDeployJobs(websiteID uint, limit int) ([]models.SiteDeployJob, error) {
	if limit <= 0 {
		limit = 20
	}
	var jobs []models.SiteDeployJob
	q := s.db.Order("id desc").Limit(limit)
	if websiteID > 0 {
		q = q.Where("website_id = ?", websiteID)
	}
	return jobs, q.Find(&jobs).Error
}

func (s *Service) TriggerDeploy(websiteID uint, trigger string) (*models.SiteDeployJob, error) {
	var cfg models.SiteDeployConfig
	if err := s.db.Where("website_id = ? AND enabled = ?", websiteID, true).First(&cfg).Error; err != nil {
		return nil, fmt.Errorf("站点未启用自动部署")
	}
	return s.runDeploy(&cfg, trigger)
}

func (s *Service) HandleWebhook(token, trigger string, body []byte, signature string) (*models.SiteDeployJob, error) {
	var cfg models.SiteDeployConfig
	if err := s.db.Where("webhook_token = ? AND enabled = ?", token, true).First(&cfg).Error; err != nil {
		return nil, fmt.Errorf("无效的 WebHook 或部署未启用")
	}
	if strings.TrimSpace(cfg.WebhookSecret) == "" {
		return nil, fmt.Errorf("Webhook 未配置签名密钥，请在部署设置中重新保存以生成密钥")
	}
	if signature == "" {
		return nil, fmt.Errorf("Webhook 签名校验失败：缺少 X-Hub-Signature-256 请求头")
	}
	if !verifyGitHubSignature(cfg.WebhookSecret, body, signature) {
		return nil, fmt.Errorf("Webhook 签名校验失败")
	}
	if trigger == "" {
		trigger = "webhook"
	}
	return s.runDeploy(&cfg, trigger)
}

func verifyGitHubSignature(secret string, body []byte, signature string) bool {
	sig := strings.TrimPrefix(signature, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(sig))
}

func (s *Service) runDeploy(cfg *models.SiteDeployConfig, trigger string) (*models.SiteDeployJob, error) {
	var site models.Website
	if err := s.db.First(&site, cfg.WebsiteID).Error; err != nil {
		return nil, fmt.Errorf("站点不存在")
	}

	job := models.SiteDeployJob{
		WebsiteID: cfg.WebsiteID,
		Trigger:   trigger,
		Status:    "running",
		StartedAt: time.Now(),
	}
	if err := s.db.Create(&job).Error; err != nil {
		return nil, err
	}

	go s.executeDeploy(&job, cfg, &site)
	return &job, nil
}

func (s *Service) executeDeploy(job *models.SiteDeployJob, cfg *models.SiteDeployConfig, site *models.Website) {
	logs := []string{fmt.Sprintf("[%s] 开始部署 %s", time.Now().Format("15:04:05"), site.Domain)}
	appendLog := func(msg string) {
		logs = append(logs, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg))
	}

	script := strings.TrimSpace(cfg.DeployScript)
	if script == "" {
		script = defaultDeployScript(site.RootPath, cfg)
	} else {
		script = strings.ReplaceAll(script, "{{root}}", site.RootPath)
		script = strings.ReplaceAll(script, "{{branch}}", cfg.Branch)
		script = strings.ReplaceAll(script, "{{domain}}", site.Domain)
	}

	appendLog("执行部署脚本…")
	out, err := runShell(script, site.RootPath)
	logs = append(logs, splitLines(out)...)
	if err != nil {
		s.finishDeployJob(job, "failed", strings.Join(logs, "\n"), err.Error())
		return
	}

	if cfg.ComposeAppID != nil && *cfg.ComposeAppID > 0 && s.compose != nil {
		appendLog("更新 Docker Compose 栈…")
		if _, err := s.compose.RollingUpdate(*cfg.ComposeAppID); err != nil {
			appendLog("Compose 滚动更新: " + err.Error())
		} else {
			appendLog("Compose 滚动更新完成")
		}
	}

	if cfg.AutoRestart && s.webserver != nil {
		appendLog("重载 Web 服务器…")
		if err := s.webserver.Reload(s.webserver.GetActive()); err != nil {
			appendLog("重载失败: " + err.Error())
		}
	}

	s.finishDeployJob(job, "success", strings.Join(logs, "\n"), "")
}

func defaultDeployScript(root string, cfg *models.SiteDeployConfig) string {
	if strings.TrimSpace(cfg.RepoURL) != "" {
		if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
			return fmt.Sprintf("git fetch --all && git checkout %s && git pull origin %s", cfg.Branch, cfg.Branch)
		}
		return fmt.Sprintf("git clone -b %s %s .", cfg.Branch, cfg.RepoURL)
	}
	return "echo no git repo configured"
}

func runShell(script, cwd string) (string, error) {
	if cwd == "" {
		cwd = "."
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", script)
	} else {
		cmd = exec.Command("bash", "-c", script)
	}
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func splitLines(s string) []string {
	parts := strings.Split(strings.TrimSpace(s), "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *Service) finishDeployJob(job *models.SiteDeployJob, status, log, errMsg string) {
	now := time.Now()
	job.Status = status
	job.Log = log
	job.Error = errMsg
	job.EndedAt = &now
	_ = s.db.Save(job).Error
}

type CIRequest struct {
	Event  string `json:"event"`
	Branch string `json:"branch"`
	Ref    string `json:"ref"`
}

func (s *Service) HandleCIWebhook(token string, req CIRequest, body []byte, gitlabToken string) (*models.SiteDeployJob, error) {
	var cfg models.SiteDeployConfig
	if err := s.db.Where("webhook_token = ? AND enabled = ?", token, true).First(&cfg).Error; err != nil {
		return nil, fmt.Errorf("无效的 CI 端点")
	}
	if cfg.WebhookSecret != "" && gitlabToken != "" && cfg.WebhookSecret != gitlabToken {
		return nil, fmt.Errorf("GitLab Token 校验失败")
	}
	branch := strings.TrimSpace(req.Branch)
	if branch == "" && req.Ref != "" {
		branch = strings.TrimPrefix(req.Ref, "refs/heads/")
	}
	if branch != "" && branch != cfg.Branch {
		return nil, fmt.Errorf("分支 %s 与配置 %s 不匹配，跳过部署", branch, cfg.Branch)
	}
	trigger := "ci"
	if cfg.CIProvider != "" {
		trigger = cfg.CIProvider
	}
	return s.runDeploy(&cfg, trigger)
}

func (s *Service) ExportDockerfile(websiteID uint) (string, error) {
	var site models.Website
	if err := s.db.First(&site, websiteID).Error; err != nil {
		return "", fmt.Errorf("站点不存在")
	}
	phpVer := site.PhpVersion
	if phpVer == "" || phpVer == "static" {
		phpVer = "8.3"
	}
	phpVer = strings.TrimPrefix(strings.ToLower(phpVer), "php")

	var b strings.Builder
	b.WriteString("# OWPanel 自动导出的 Dockerfile\n")
	b.WriteString("# 站点: " + site.Domain + "\n\n")
	if site.PHP || site.PhpVersion != "static" {
		b.WriteString(fmt.Sprintf("FROM php:%s-fpm-alpine\n\n", phpVer))
		b.WriteString("RUN apk add --no-cache nginx git \\\n")
		b.WriteString("    && docker-php-ext-install pdo_mysql mysqli opcache\n\n")
	} else {
		b.WriteString("FROM nginx:alpine\n\n")
	}
	b.WriteString(fmt.Sprintf("WORKDIR /var/www/%s\n", site.Domain))
	b.WriteString(fmt.Sprintf("COPY . /var/www/%s\n\n", site.Domain))
	if site.PHP || site.PhpVersion != "static" {
		b.WriteString("EXPOSE 9000\n")
	} else {
		b.WriteString("EXPOSE 80\n")
	}
	b.WriteString("\n# 构建: docker build -t " + site.Domain + " .\n")
	b.WriteString("# 运行: docker run -d -p 8080:80 " + site.Domain + "\n")
	return b.String(), nil
}

func (s *Service) SaveDockerfileExport(websiteID uint, content string) (string, error) {
	var site models.Website
	if err := s.db.First(&site, websiteID).Error; err != nil {
		return "", err
	}
	path := filepath.Join(site.RootPath, "Dockerfile.exported")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return path, nil
}

func ParseCIBody(raw []byte) CIRequest {
	var req CIRequest
	_ = json.Unmarshal(raw, &req)
	return req
}
