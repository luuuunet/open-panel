package aisite

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/website"
)

type bootstrapContext struct {
	plan      DeployPlan
	site      *models.Website
	createRes *website.CreateResult
	dbPass    string
}

func (s *Service) postDeploySteps(ctx bootstrapContext, appendLog func(string)) error {
	root := ctx.site.RootPath
	if root == "" {
		return nil
	}

	if ctx.plan.NeedDatabase && ctx.createRes != nil && ctx.createRes.DbName != "" {
		switch ctx.plan.Framework {
		case "laravel", "symfony":
			if err := writeLaravelEnv(root, ctx.createRes, ctx.plan.Domain); err != nil {
				appendLog("Laravel .env 写入: " + err.Error())
			} else {
				appendLog("已自动配置 Laravel .env（production 模式与数据库连接）")
				s.runArtisanMigrate(root, appendLog)
			}
			if err := fixLaravelStoragePermissions(root, appendLog); err != nil {
				appendLog("Laravel 目录权限: " + err.Error())
			}
			s.ensureLaravelScheduleCron(root, ctx.plan.Domain, appendLog)
		case "wordpress":
			if err := writeWordPressConfig(root, ctx.createRes, ctx.plan.Domain); err != nil {
				appendLog("WordPress 配置: " + err.Error())
			} else {
				appendLog("已生成 wp-config.php 数据库配置")
			}
		}
	}

	docRoot := strings.Trim(strings.TrimSpace(ctx.plan.DocumentRoot), "/")
	if docRoot == "" {
		docRoot = defaultDocumentRoot(ctx.plan.Framework)
	}
	if docRoot != "" {
		if err := s.applyDocumentRoot(ctx.site.ID, root, docRoot); err != nil {
			appendLog("文档根目录: " + err.Error())
		} else {
			appendLog("Nginx 文档根目录已设为: " + docRoot)
			site, _ := s.website.Get(ctx.site.ID)
			if site != nil {
				ctx.site = site
			}
		}
	}

	if ctx.plan.UsePM2 && ctx.plan.ProjectType == "node" && s.nodejs != nil {
		if err := s.setupNodePM2(ctx, appendLog); err != nil {
			appendLog("Node PM2: " + err.Error())
		}
	}

	return nil
}

func (s *Service) prepareFrameworkEnvBeforeDeploy(plan DeployPlan, snap *RepoSnapshot, root string, createRes *website.CreateResult, domain string, appendLog func(string)) error {
	if createRes == nil || createRes.DbName == "" {
		return nil
	}
	switch resolveFramework(plan, snap) {
	case "laravel", "symfony":
		if err := writeLaravelEnv(root, createRes, domain); err != nil {
			return err
		}
		appendLog("已预写 Laravel .env（数据库与 APP_ENV=production）")
		return nil
	default:
		return nil
	}
}

func defaultDocumentRoot(framework string) string {
	switch framework {
	case "laravel", "symfony":
		return "public"
	case "nextjs":
		return "out"
	case "vue", "react":
		return "dist"
	default:
		return ""
	}
}

func (s *Service) applyDocumentRoot(siteID uint, siteRoot, subdir string) error {
	subdir = strings.Trim(subdir, "/")
	if subdir == "" {
		return nil
	}
	newRoot := filepath.Join(siteRoot, subdir)
	if _, err := os.Stat(newRoot); err != nil {
		return fmt.Errorf("目录不存在: %s", newRoot)
	}
	p := newRoot
	_, err := s.website.UpdateSite(siteID, &website.UpdateRequest{RootPath: &p})
	return err
}

func writeLaravelEnv(root string, res *website.CreateResult, domain string) error {
	envPath := filepath.Join(root, ".env")
	examplePath := filepath.Join(root, ".env.example")
	content := ""
	if b, err := os.ReadFile(envPath); err == nil {
		content = string(b)
	} else if b, err := os.ReadFile(examplePath); err == nil {
		content = string(b)
	} else {
		content = "APP_NAME=Laravel\nAPP_ENV=production\nAPP_KEY=\nAPP_DEBUG=false\nAPP_URL=http://localhost\n"
	}
	content = setEnvLine(content, "APP_URL", "http://"+domain)
	content = setEnvLine(content, "APP_ENV", "production")
	content = setEnvLine(content, "APP_DEBUG", "false")
	content = setEnvLine(content, "DB_CONNECTION", "mysql")
	content = setEnvLine(content, "DB_HOST", "127.0.0.1")
	content = setEnvLine(content, "DB_PORT", "3306")
	content = setEnvLine(content, "DB_DATABASE", res.DbName)
	content = setEnvLine(content, "DB_USERNAME", res.DbUser)
	content = setEnvLine(content, "DB_PASSWORD", res.DbPassword)
	return os.WriteFile(envPath, []byte(content), 0644)
}

func fixLaravelStoragePermissions(root string, appendLog func(string)) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dirs := []string{
		filepath.Join(root, "storage"),
		filepath.Join(root, "bootstrap", "cache"),
	}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		_ = exec.Command("chmod", "-R", "775", dir).Run()
		_ = exec.Command("find", dir, "-type", "d", "-exec", "chmod", "g+s", "{}", "+").Run()
	}
	if _, err := exec.LookPath("chown"); err == nil {
		for _, dir := range dirs {
			if _, err := os.Stat(dir); err != nil {
				continue
			}
			if out, err := exec.Command("chown", "-R", "www-data:www-data", dir).CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %s", dir, strings.TrimSpace(string(out)))
			}
		}
	}
	appendLog("已修复 storage/bootstrap/cache 权限（www-data 可写）")
	return nil
}

func (s *Service) ensureLaravelScheduleCron(root, domain string, appendLog func(string)) {
	if s.cron == nil || runtime.GOOS == "windows" {
		return
	}
	if _, err := os.Stat(filepath.Join(root, "artisan")); err != nil {
		return
	}
	name := fmt.Sprintf("Laravel schedule (%s)", domain)
	jobs, err := s.cron.List()
	if err == nil {
		for _, job := range jobs {
			if job.Name == name {
				appendLog("Laravel schedule:run 定时任务已存在")
				return
			}
		}
	}
	cmd := fmt.Sprintf("cd %s && php artisan schedule:run >> /dev/null 2>&1", shellQuotePath(root))
	if err := s.cron.Create(&models.CronJob{
		Name:     name,
		Schedule: "* * * * *",
		Command:  cmd,
		Enabled:  true,
	}); err != nil {
		appendLog("Laravel cron 注册: " + err.Error())
		return
	}
	appendLog("已注册 cron：每分钟执行 php artisan schedule:run")
}

func shellQuotePath(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}

func writeWordPressConfig(root string, res *website.CreateResult, domain string) error {
	sample := filepath.Join(root, "wp-config-sample.php")
	target := filepath.Join(root, "wp-config.php")
	if _, err := os.Stat(target); err == nil {
		return nil
	}
	b, err := os.ReadFile(sample)
	if err != nil {
		return err
	}
	content := string(b)
	content = strings.ReplaceAll(content, "database_name_here", res.DbName)
	content = strings.ReplaceAll(content, "username_here", res.DbUser)
	content = strings.ReplaceAll(content, "password_here", res.DbPassword)
	content = strings.ReplaceAll(content, "localhost", "127.0.0.1")
	salt := fmt.Sprintf("define('WP_HOME', 'http://%s');\ndefine('WP_SITEURL', 'http://%s');\n", domain, domain)
	content = strings.Replace(content, "/* That's all, stop editing!", salt+"/* That's all, stop editing!", 1)
	return os.WriteFile(target, []byte(content), 0644)
}

func setEnvLine(content, key, value string) string {
	lines := strings.Split(content, "\n")
	found := false
	prefix := key + "="
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			lines[i] = key + "=" + value
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, key+"="+value)
	}
	return strings.Join(lines, "\n")
}

func (s *Service) runArtisanMigrate(root string, appendLog func(string)) {
	artisan := filepath.Join(root, "artisan")
	if _, err := os.Stat(artisan); err != nil {
		return
	}
	out, err := runDeployShell(s.dataDir, "php artisan migrate --force 2>/dev/null || php artisan migrate --force", root)
	if err != nil {
		appendLog("artisan migrate: " + strings.TrimSpace(out))
	} else {
		appendLog("Laravel 数据库迁移完成")
	}
}

func (s *Service) setupNodePM2(ctx bootstrapContext, appendLog func(string)) error {
	port := ctx.plan.NodePort
	if port <= 0 {
		port = 31000 + int(ctx.site.ID%500)
	}
	name := strings.ReplaceAll(ctx.plan.Domain, ".", "-")
	cwd := ctx.site.RootPath
	if dr := strings.Trim(strings.TrimSpace(ctx.plan.DocumentRoot), "/"); dr != "" {
		candidate := dr
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(ctx.site.RootPath, dr)
		}
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			cwd = candidate
		}
	}
	proj := &models.NodeProject{
		Name:    name,
		Domain:  ctx.plan.Domain,
		Path:    cwd,
		Port:    port,
		NodeVer: "20",
		Status:  "stopped",
		Remark:  "AI GitHub bootstrap",
	}
	if err := s.nodejs.Create(proj); err != nil {
		return err
	}
	if err := s.nodejs.Toggle(proj.ID, "running"); err != nil {
		return err
	}
	proxy := fmt.Sprintf("http://127.0.0.1:%d", port)
	_, err := s.website.UpdateSite(ctx.site.ID, &website.UpdateRequest{ProxyPass: &proxy})
	if err != nil {
		return err
	}
	appendLog(fmt.Sprintf("Node PM2 已启动，端口 %d，Nginx 已反代", port))
	return nil
}

func (s *Service) rollbackSite(siteID uint, appendLog func(string)) {
	if siteID == 0 {
		return
	}
	appendLog("部署失败，正在回滚删除站点…")
	if err := s.website.Delete(siteID); err != nil {
		appendLog("回滚失败: " + err.Error())
	} else {
		appendLog("已回滚并删除未完成站点")
	}
}

func runShellInDir(script, cwd string) (string, error) {
	return runShell(script, cwd)
}
