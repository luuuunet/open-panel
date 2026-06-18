package aisite

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/services/appstore"
)

const appInstallTimeout = 20 * time.Minute

func (s *Service) ensureDeployEnvironment(plan DeployPlan, snap *RepoSnapshot, selectedApps []string, appendLog func(string)) (PanelContext, error) {
	if s.appstore == nil {
		return s.collectPanelContext(), fmt.Errorf("软件商店不可用，无法自动安装运行环境")
	}
	s.appstore.ReconcileInstalledFromSystem()

	appendLog("正在检测面板运行环境…")
	missing := s.requiredDeployApps(plan, snap)
	missing = filterAppsBySelection(missing, selectedApps)
	if !gitAvailable() {
		if err := ensureGitAvailable(appendLog); err != nil {
			return s.collectPanelContext(), fmt.Errorf("Git 不可用: %w", err)
		}
	}

	if len(missing) == 0 && s.webServerReady() {
		appendLog("运行环境已就绪")
		return s.collectPanelContext(), nil
	}

	if !s.webServerReady() {
		appendLog("未检测到 Web 服务器，准备安装 Nginx/OpenResty…")
		if err := s.ensureWebServer(appendLog); err != nil {
			return s.collectPanelContext(), err
		}
		missing = removeKeys(missing, "nginx", "openresty")
	}

	for _, key := range missing {
		if key == "nginx" || key == "openresty" {
			continue
		}
		if err := s.ensureAppInstalled(key, appendLog); err != nil {
			return s.collectPanelContext(), fmt.Errorf("安装 %s 失败: %w", key, err)
		}
		if err := s.startAppIfNeeded(key, appendLog); err != nil {
			appendLog(fmt.Sprintf("启动 %s: %v", key, err))
		}
		if strings.HasPrefix(key, "php") {
			s.ensurePHPCLI(key, appendLog)
		}
	}

	s.appstore.ReconcileInstalledFromSystem()
	panel := s.collectPanelContext()
	appendLog("运行环境检测与安装完成")
	return panel, nil
}

func (s *Service) missingEnvApps(plan DeployPlan, snap *RepoSnapshot) []string {
	var keys []string
	if plan.NeedDatabase && !s.envMySQLReady() {
		keys = append(keys, mysqlAppKey())
	}
	if phpKey := phpAppKey(plan.PhpVersion); phpKey != "" && planNeedsPHP(plan) && !s.isAppReady(phpKey) {
		keys = append(keys, phpKey)
	}
	if planNeedsComposer(plan, snap) && !s.envComposerReady() {
		keys = append(keys, "composer")
	}
	if planNeedsNode(plan, snap) && !s.envNodeReady() {
		keys = append(keys, "nodejs20")
	}
	if plan.UsePM2 && !s.isAppReady("pm2") {
		keys = append(keys, "pm2")
	}
	if plan.UseDocker && !s.envDockerReady() {
		keys = append(keys, "docker")
	}
	if !s.webServerReady() {
		keys = append(keys, s.webServerAppKeys()[0])
	}
	return dedupeKeys(keys)
}

func planNeedsPHP(plan DeployPlan) bool {
	return plan.ProjectType == "php" || (plan.PhpVersion != "" && plan.PhpVersion != "static")
}

func planNeedsComposer(plan DeployPlan, snap *RepoSnapshot) bool {
	if plan.ProjectType == "php" {
		return true
	}
	if snap == nil {
		return false
	}
	return snap.HasComposer || snap.FrameworkHint == "laravel" || snap.FrameworkHint == "symfony" || snap.FrameworkHint == "wordpress"
}

func planNeedsNode(plan DeployPlan, snap *RepoSnapshot) bool {
	if plan.Framework == "laravel" {
		return true
	}
	if plan.UsePM2 || plan.ProjectType == "node" {
		return true
	}
	if snap == nil {
		return false
	}
	return snap.HasPackageJSON || snap.HasNodeServer ||
		snap.FrameworkHint == "nextjs" || snap.FrameworkHint == "vue" ||
		snap.FrameworkHint == "react" || snap.FrameworkHint == "nodejs"
}

func phpAppKey(version string) string {
	v := strings.TrimSpace(version)
	if v == "" || v == "static" {
		return ""
	}
	return "php" + strings.ReplaceAll(v, ".", "")
}

func mysqlAppKey() string {
	return "mysql"
}

func (s *Service) webServerAppKeys() []string {
	preferred := "nginx"
	if all, _ := s.settings.GetAll(); all != nil {
		if v := strings.TrimSpace(all["active_web_server"]); v != "" {
			preferred = v
		}
	}
	if preferred == "openresty" {
		return []string{"openresty", "nginx"}
	}
	return []string{"nginx", "openresty"}
}

func (s *Service) webServerReady() bool {
	for _, key := range s.webServerAppKeys() {
		if s.isAppReady(key) {
			return true
		}
	}
	return false
}

func (s *Service) envMySQLReady() bool {
	if s.isAppReady("mysql") || s.isAppReady("mariadb") {
		return true
	}
	panel := s.collectPanelContext()
	return panel.MySQLAvailable
}

func (s *Service) envComposerReady() bool {
	if appstore.ComposerBinary(s.dataDir) != "" {
		return true
	}
	panel := s.collectPanelContext()
	return panel.ComposerAvail
}

func (s *Service) envNodeReady() bool {
	return appstore.NodeMajorAvailable(s.dataDir, 20) || appstore.NodeMajorAvailable(s.dataDir, 18)
}

func (s *Service) envDockerReady() bool {
	panel := s.collectPanelContext()
	return panel.DockerAvailable
}

func (s *Service) isAppReady(key string) bool {
	if s.appstore == nil {
		return false
	}
	if strings.HasPrefix(key, "nodejs") {
		majorStr := strings.TrimPrefix(key, "nodejs")
		if major, err := strconv.Atoi(majorStr); err == nil && major > 0 {
			return appstore.NodeMajorAvailable(s.dataDir, major)
		}
	}
	if appstore.SystemPackagePresent(key, s.dataDir) {
		return true
	}
	app, err := s.appstore.Get(key)
	if err != nil {
		return false
	}
	return app.Installed && !appstore.IsSimulatedInstall(key, s.dataDir) && app.Status != "failed"
}

func (s *Service) ensureWebServer(appendLog func(string)) error {
	for _, key := range s.webServerAppKeys() {
		if s.isAppReady(key) {
			return s.startAppIfNeeded(key, appendLog)
		}
	}
	for _, key := range s.webServerAppKeys() {
		if err := s.ensureAppInstalled(key, appendLog); err != nil {
			appendLog(fmt.Sprintf("%s 安装失败: %v", key, err))
			continue
		}
		return s.startAppIfNeeded(key, appendLog)
	}
	return fmt.Errorf("未能安装或启动 Web 服务器 (Nginx/OpenResty)")
}

func (s *Service) ensureAppInstalled(key string, appendLog func(string)) error {
	if strings.HasPrefix(key, "nodejs") {
		majorStr := strings.TrimPrefix(key, "nodejs")
		if major, err := strconv.Atoi(majorStr); err == nil && major > 0 {
			if !appstore.NodeMajorAvailable(s.dataDir, major) {
				appendLog(fmt.Sprintf("正在安装 Node.js %d（官方二进制，满足 Vite/Laravel 构建要求）…", major))
				if err := appstore.EnsureNodeMajor(s.dataDir, major); err != nil {
					return err
				}
				appendLog(fmt.Sprintf("Node.js %d 安装完成", major))
			}
			return nil
		}
	}
	if s.isAppReady(key) {
		return nil
	}
	app, err := s.appstore.Get(key)
	if err != nil {
		return err
	}
	if app.Status == "installing" {
		appendLog(fmt.Sprintf("等待 %s 安装完成…", app.Name))
		return s.waitInstallWithLog(key, app.Name, appendLog)
	}
	appendLog(fmt.Sprintf("正在通过软件商店安装 %s …", app.Name))
	if err := s.appstore.Install(key, ""); err != nil && !installInProgress(err) {
		return err
	}
	if err := s.waitInstallWithLog(key, app.Name, appendLog); err != nil {
		return err
	}
	appendLog(fmt.Sprintf("%s 安装完成", app.Name))
	return nil
}

func (s *Service) startAppIfNeeded(key string, appendLog func(string)) error {
	if s.appstore == nil {
		return nil
	}
	status := s.appstore.LiveStatus(key)
	if status == "running" {
		return nil
	}
	app, err := s.appstore.Get(key)
	if err != nil {
		return err
	}
	appendLog(fmt.Sprintf("正在启动 %s …", app.Name))
	if err := s.appstore.ServiceAction(key, "start"); err != nil {
		return err
	}
	s.appstore.InvalidateLiveStatus(key)
	time.Sleep(2 * time.Second)
	if s.appstore.LiveStatus(key) == "running" {
		appendLog(fmt.Sprintf("%s 已就绪", app.Name))
	}
	return nil
}

func (s *Service) ensurePHPCLI(phpKey string, appendLog func(string)) {
	if s.appstore == nil {
		return
	}
	if err := s.appstore.ServiceAction(phpKey, "start"); err != nil {
		appendLog(fmt.Sprintf("PHP 启动: %v", err))
	}
}

func (s *Service) waitInstallWithLog(key, name string, appendLog func(string)) error {
	if name == "" {
		name = key
	}
	done := make(chan error, 1)
	go func() { done <- s.appstore.WaitInstall(key, appInstallTimeout) }()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	elapsed := 0
	for {
		select {
		case err := <-done:
			return err
		case <-ticker.C:
			elapsed += 10
			appendLog(fmt.Sprintf("仍在安装 %s…（已等待 %ds）", name, elapsed))
		}
	}
}

func installInProgress(err error) bool {
	if err == nil {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "already") || strings.Contains(msg, "in progress")
}

func dedupeKeys(keys []string) []string {
	seen := make(map[string]bool, len(keys))
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	return out
}

func removeKeys(keys []string, drop ...string) []string {
	dropSet := make(map[string]bool, len(drop))
	for _, d := range drop {
		dropSet[d] = true
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if !dropSet[k] {
			out = append(out, k)
		}
	}
	return out
}
