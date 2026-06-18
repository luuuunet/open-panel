package webserver

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const panelIncludeMarker = "owpanel-vhosts"

// Bootstrap prepares vhost directory, panel include snippet, and injects into the system main config.
func (m *Manager) Bootstrap(key string) error {
	if !IsWebServerKey(key) {
		return fmt.Errorf("unsupported web server: %s", key)
	}
	app, err := m.apps.Get(key)
	if err != nil {
		return err
	}
	m.logStep(key, fmt.Sprintf("正在配置 %s …", app.Name))

	m.ensureVhostInclude(key)
	vhostDir := VhostDir(m.dataDir, key)
	_ = os.MkdirAll(vhostDir, 0755)
	_ = os.MkdirAll(filepath.Join(m.dataDir, "logs"), 0755)

	panelConf := panelIncludePath(m.dataDir, key)
	if err := m.injectMainInclude(key, app.ConfigPath, panelConf); err != nil {
		m.logStep(key, "主配置 include 提示: "+err.Error())
	}

	if runtime.GOOS == "linux" {
		m.disableConflictingDefaults(key)
	}

	if _, err := m.TestConfig(key); err != nil {
		return fmt.Errorf("配置测试失败: %w", err)
	}
	m.logStep(key, "Web 服务器配置已完成")
	return nil
}

func (m *Manager) logStep(key, msg string) {
	if m.apps != nil {
		m.apps.AppendInstallLog(key, msg)
	}
}

func panelIncludePath(dataDir, key string) string {
	if key == "apache" {
		return filepath.Join(dataDir, "apache", "owpanel.conf")
	}
	return filepath.Join(dataDir, "nginx", "owpanel.conf")
}

func (m *Manager) injectMainInclude(key, configFallback, panelConf string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	mainConf := m.resolveConfigPath(key, configFallback)
	if mainConf == "" || !fileExists(mainConf) {
		return fmt.Errorf("未找到主配置文件（请安装 %s 后重试）", key)
	}

	includeLine := fmt.Sprintf("include %s;", filepath.ToSlash(panelConf))
	data, err := os.ReadFile(mainConf)
	if err != nil {
		return err
	}
	content := string(data)
	if strings.Contains(content, panelIncludeMarker) || strings.Contains(content, filepath.ToSlash(panelConf)) {
		return nil
	}

	updated, ok := injectHTTPInclude(content, includeLine, panelIncludeMarker)
	if !ok {
		return fmt.Errorf("请手动在 %s 的 http {} 内添加: %s", mainConf, includeLine)
	}

	backup := mainConf + ".owpanel.bak." + time.Now().Format("20060102-150405")
	_ = os.WriteFile(backup, data, 0644)
	if err := os.WriteFile(mainConf, []byte(updated), 0644); err != nil {
		return err
	}
	m.logStep(key, fmt.Sprintf("已写入主配置 include: %s", mainConf))
	return nil
}

func injectHTTPInclude(content, includeLine, marker string) (string, bool) {
	if strings.Contains(content, marker) {
		return content, true
	}
	lower := strings.ToLower(content)
	idx := strings.Index(lower, "http")
	if idx < 0 {
		return content, false
	}
	brace := strings.Index(content[idx:], "{")
	if brace < 0 {
		return content, false
	}
	insertAt := idx + brace + 1
	for insertAt < len(content) && (content[insertAt] == ' ' || content[insertAt] == '\t') {
		insertAt++
	}
	if insertAt < len(content) && content[insertAt] == '\r' {
		insertAt++
	}
	if insertAt < len(content) && content[insertAt] == '\n' {
		insertAt++
	}
	block := fmt.Sprintf("    # %s\n    %s\n", marker, includeLine)
	return content[:insertAt] + block + content[insertAt:], true
}

func (m *Manager) disableConflictingDefaults(key string) {
	switch key {
	case "nginx", "openresty":
		candidates := []string{
			"/etc/nginx/sites-enabled/default",
			"/etc/nginx/conf.d/default.conf",
			"/etc/nginx/conf.d/welcome.conf",
		}
		for _, p := range candidates {
			if !fileExists(p) {
				continue
			}
			backup := p + ".owpanel-disabled"
			if fileExists(backup) {
				continue
			}
			if err := os.Rename(p, backup); err == nil {
				m.logStep(key, "已禁用默认站点: "+p)
			}
		}
	case "apache":
		for _, p := range []string{
			"/etc/apache2/sites-enabled/000-default.conf",
			"/etc/httpd/conf.d/welcome.conf",
		} {
			if fileExists(p) {
				_ = os.Rename(p, p+".owpanel-disabled")
			}
		}
	}
}
