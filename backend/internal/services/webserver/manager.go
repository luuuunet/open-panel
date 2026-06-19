package webserver

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"gorm.io/gorm"
)

var webServerKeys = []string{"openresty", "nginx", "apache"}

type ServerInfo struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	Status       string `json:"status"`
	Installed    bool   `json:"installed"`
	ConfigPath   string `json:"config_path"`
	VhostDir     string `json:"vhost_dir"`
	SitesEnabled int    `json:"sites_enabled"`
	IsActive     bool   `json:"is_active"`
	Binary       string `json:"binary"`
}

type Overview struct {
	Active  string       `json:"active"`
	Servers []ServerInfo `json:"servers"`
}

type Manager struct {
	db      *gorm.DB
	dataDir string
	apps    *appstore.Service
}

func NewManager(db *gorm.DB, dataDir string, apps *appstore.Service) *Manager {
	return &Manager{db: db, dataDir: dataDir, apps: apps}
}

func (m *Manager) GetActive() string {
	v := m.getSetting("active_web_server")
	if v == "" {
		return "nginx"
	}
	return v
}

func (m *Manager) SetActive(key string) {
	m.setSetting("active_web_server", key)
}

func (m *Manager) Overview() (*Overview, error) {
	active := m.GetActive()
	var servers []ServerInfo
	for _, key := range webServerKeys {
		app, err := m.apps.Get(key)
		if err != nil {
			continue
		}
		status := app.Status
		installed := app.Installed && !appstore.IsSimulatedInstall(key, m.dataDir)
		if IsWebServerKey(key) && installed {
			installed = webServerBinary(key) != ""
		}
		if installed {
			status = m.apps.LiveStatus(key)
		}
		vhostDir := VhostDir(m.dataDir, key)
		cfgPath := m.resolveConfigPath(key, app.ConfigPath)
		servers = append(servers, ServerInfo{
			Key:          key,
			Name:         app.Name,
			Version:      m.detectVersion(key, app.Version),
			Status:       status,
			Installed:    installed,
			ConfigPath:   cfgPath,
			VhostDir:     vhostDir,
			SitesEnabled: countVhosts(vhostDir),
			IsActive:     key == active,
			Binary:       webServerBinary(key),
		})
	}
	return &Overview{Active: active, Servers: servers}, nil
}

func (m *Manager) StartExclusive(key string) error {
	if !IsWebServerKey(key) {
		return fmt.Errorf("unsupported web server: %s", key)
	}
	app, err := m.apps.Get(key)
	if err != nil {
		return err
	}
	if !app.Installed {
		return fmt.Errorf("%s 未安装，请先在软件商店安装", app.Name)
	}
	if err := m.Bootstrap(key); err != nil {
		return fmt.Errorf("配置 %s 失败: %w", app.Name, err)
	}

	for _, other := range webServerKeys {
		if other == key {
			continue
		}
		otherApp, err := m.apps.Get(other)
		if err != nil || !otherApp.Installed {
			continue
		}
		if m.apps.LiveStatus(other) == "running" {
			if err := m.apps.ServiceAction(other, "stop"); err != nil {
				return fmt.Errorf("停止 %s 失败: %w", otherApp.Name, err)
			}
		}
	}

	if err := m.apps.ServiceAction(key, "start"); err != nil {
		return err
	}
	m.SetActive(key)
	m.ensureVhostInclude(key)
	return m.Reload(key)
}

func (m *Manager) Stop(key string) error {
	return m.apps.ServiceAction(key, "stop")
}

func (m *Manager) Reload(key string) error {
	if key == "" {
		key = m.GetActive()
	}
	app, err := m.apps.Get(key)
	if err != nil || !app.Installed {
		m.apps.ReconcileInstalledFromSystem()
		app, err = m.apps.Get(key)
	}
	if err != nil || !app.Installed {
		if err := m.tryDirectReload(key); err == nil {
			return nil
		}
		return fmt.Errorf("web server not installed")
	}
	if m.apps.LiveStatus(key) != "running" {
		if err := m.tryDirectReload(key); err == nil {
			return nil
		}
		return fmt.Errorf("服务未运行")
	}
	if err := m.apps.ServiceAction(key, "reload"); err != nil {
		_ = m.apps.ServiceAction(key, "restart")
	}
	return m.tryDirectReload(key)
}

func (m *Manager) TestConfig(key string) (string, error) {
	if key == "" {
		key = m.GetActive()
	}
	bin := webServerBinary(key)
	if bin == "" {
		return "", fmt.Errorf("binary not found for %s", key)
	}
	app, _ := m.apps.Get(key)
	cfg := m.resolveConfigPath(key, "")
	if app != nil && app.ConfigPath != "" {
		cfg = m.resolveConfigPath(key, app.ConfigPath)
	}
	cmd := exec.Command(bin, "-t")
	if cfg != "" {
		cmd = exec.Command(bin, "-t", "-c", cfg)
	}
	out, err := cmd.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		if msg == "" {
			msg = err.Error()
		}
		return msg, fmt.Errorf("config test failed: %s", msg)
	}
	return msg, nil
}

func (m *Manager) ReadMainConfig(key string) (string, error) {
	app, err := m.apps.Get(key)
	if err != nil {
		return "", err
	}
	path := m.resolveConfigPath(key, app.ConfigPath)
	data, err := os.ReadFile(path)
	if err != nil {
		panelConf := filepath.Join(m.dataDir, "nginx", "owpanel.conf")
		if key == "apache" {
			panelConf = filepath.Join(m.dataDir, "apache", "owpanel.conf")
		}
		data, err = os.ReadFile(panelConf)
		if err != nil {
			return "", err
		}
	}
	return string(data), nil
}

func (m *Manager) WriteMainConfig(key, content string) error {
	app, err := m.apps.Get(key)
	if err != nil {
		return err
	}
	path := m.resolveConfigPath(key, app.ConfigPath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	return m.Reload(key)
}

func (m *Manager) Setup(key string, start bool) error {
	if err := m.Bootstrap(key); err != nil {
		return err
	}
	if start {
		return m.StartExclusive(key)
	}
	return nil
}

func (m *Manager) EnsureVhostInclude(key string) {
	m.ensureVhostInclude(key)
}

func (m *Manager) ensureVhostInclude(key string) {
	switch key {
	case "nginx", "openresty":
		m.writeNginxMainInclude()
	case "apache":
		m.writeApacheMainInclude()
	}
}

func (m *Manager) writeNginxMainInclude() {
	vhostDir := filepath.Join(m.dataDir, "nginx", "vhosts")
	_ = os.MkdirAll(vhostDir, 0755)
	mainConf := filepath.Join(m.dataDir, "nginx", "owpanel.conf")
	includeLine := fmt.Sprintf("include %s/*.conf;", filepath.ToSlash(vhostDir))
	content := fmt.Sprintf("# OWPanel auto-generated\n%s\n", includeLine)
	_ = os.WriteFile(mainConf, []byte(content), 0644)
}

func (m *Manager) writeApacheMainInclude() {
	vhostDir := filepath.Join(m.dataDir, "apache", "vhosts")
	_ = os.MkdirAll(vhostDir, 0755)
	mainConf := filepath.Join(m.dataDir, "apache", "owpanel.conf")
	includeLine := fmt.Sprintf("IncludeOptional %s/*.conf", filepath.ToSlash(vhostDir))
	content := fmt.Sprintf("# OWPanel auto-generated\n%s\n", includeLine)
	_ = os.WriteFile(mainConf, []byte(content), 0644)
}

func (m *Manager) tryDirectReload(key string) error {
	bin := webServerBinary(key)
	if bin == "" {
		return nil
	}
	cmd := exec.Command(bin, "-s", "reload")
	if runtime.GOOS == "windows" {
		cmd = exec.Command(bin, "-s", "reload")
	}
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func (m *Manager) ResolveConfigPath(key, fallback string) string {
	return m.resolveConfigPath(key, fallback)
}

func (m *Manager) resolveConfigPath(key, fallback string) string {
	candidates := []string{}
	if fallback != "" {
		candidates = append(candidates, fallback)
	}
	switch key {
	case "openresty":
		candidates = append(candidates,
			"/usr/local/openresty/nginx/conf/nginx.conf",
			"/etc/openresty/nginx.conf",
		)
	case "nginx":
		candidates = append(candidates,
			"/etc/nginx/nginx.conf",
			filepath.Join(m.dataDir, "server", "nginx", "conf", "nginx.conf"),
			filepath.Join(m.dataDir, "nginx", "nginx.conf"),
		)
	case "apache":
		candidates = append(candidates,
			"/etc/apache2/apache2.conf",
			"/etc/httpd/conf/httpd.conf",
		)
	}
	for _, p := range candidates {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func webServerBinary(key string) string {
	var names []string
	switch key {
	case "openresty":
		names = []string{"openresty", "/usr/local/openresty/nginx/sbin/nginx"}
	case "nginx":
		names = []string{"nginx", "/usr/sbin/nginx", "/usr/local/nginx/sbin/nginx"}
	case "apache":
		names = []string{"apache2ctl", "apachectl", "httpd"}
	default:
		return ""
	}
	for _, n := range names {
		if strings.Contains(n, "/") {
			if fileExists(n) {
				return n
			}
			continue
		}
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

func (m *Manager) detectVersion(key, fallback string) string {
	bin := webServerBinary(key)
	if bin == "" {
		return fallback
	}
	out, err := exec.Command(bin, "-v").CombinedOutput()
	if err != nil {
		out, _ = exec.Command(bin, "-V").CombinedOutput()
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return fallback
	}
	return s
}

func countVhosts(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".conf") {
			n++
		}
	}
	return n
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (m *Manager) getSetting(key string) string {
	var row models.PanelSetting
	if m.db.Where("key = ?", key).First(&row).Error != nil {
		return ""
	}
	return row.Value
}

func (m *Manager) setSetting(key, value string) {
	m.db.Where("key = ?", key).Assign(models.PanelSetting{Value: value}).FirstOrCreate(&models.PanelSetting{Key: key})
}

func (m *Manager) EnforceExclusiveOnStart(key string) error {
	if !IsWebServerKey(key) {
		return nil
	}
	for _, other := range webServerKeys {
		if other == key {
			continue
		}
		otherApp, err := m.apps.Get(other)
		if err != nil || !otherApp.Installed {
			continue
		}
		if m.apps.LiveStatus(other) == "running" {
			_ = m.apps.ServiceAction(other, "stop")
		}
	}
	m.SetActive(key)
	m.ensureVhostInclude(key)
	return nil
}

func IsWebServerKey(key string) bool {
	return key == "nginx" || key == "openresty" || key == "apache"
}

func OtherWebServer(key string) string {
	if key == "nginx" {
		return "openresty"
	}
	if key == "openresty" {
		return "nginx"
	}
	if key == "apache" {
		return "nginx"
	}
	return ""
}

func VhostDir(dataDir, webServer string) string {
	if webServer == "apache" {
		return filepath.Join(dataDir, "apache", "vhosts")
	}
	return filepath.Join(dataDir, "nginx", "vhosts")
}

func ConfFileName(domain, webServer string) string {
	domain = strings.TrimSpace(domain)
	return domain + ".conf"
}

// NginxFamily returns true for nginx-compatible servers (site vhosts use nginx format).
func NginxFamily(key string) bool {
	return key == "nginx" || key == "openresty" || key == ""
}
