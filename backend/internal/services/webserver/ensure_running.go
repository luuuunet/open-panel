package webserver

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/services/appstore"
)

var cacheWebServerKeys = []string{"nginx", "openresty"}

// EnsureRunning installs or starts a web server for cache / PHP acceleration.
func (m *Manager) EnsureRunning(steps *[]string) (string, error) {
	if m.apps == nil {
		return "", fmt.Errorf("应用商店不可用")
	}
	m.apps.ReconcileInstalledFromSystem()

	preferred := m.GetActive()
	if preferred != "nginx" && preferred != "openresty" {
		preferred = "nginx"
	}
	order := orderedKeys(preferred, cacheWebServerKeys)

	for _, key := range order {
		if m.isWebServerRunning(key) {
			if key != m.GetActive() {
				m.SetActive(key)
			}
			*steps = append(*steps, fmt.Sprintf("Web 服务器 %s 已运行", key))
			return key, nil
		}
	}

	for _, key := range order {
		app, err := m.apps.Get(key)
		if err != nil {
			continue
		}
		if appstore.IsSimulatedInstall(key, m.dataDir) && !appstore.SystemPackagePresent(key, m.dataDir) {
			continue
		}
		if !app.Installed {
			*steps = append(*steps, fmt.Sprintf("正在安装 %s …", app.Name))
			if err := m.apps.Install(key, "latest"); err != nil && !installInProgress(err) {
				*steps = append(*steps, fmt.Sprintf("%s 安装启动失败: %v", app.Name, err))
				continue
			}
			if err := m.apps.WaitInstall(key, 15*time.Minute); err != nil {
				*steps = append(*steps, fmt.Sprintf("%s 安装未完成: %v", app.Name, err))
				continue
			}
			m.apps.ReconcileInstalledFromSystem()
			*steps = append(*steps, fmt.Sprintf("%s 安装完成", app.Name))
		}
		*steps = append(*steps, fmt.Sprintf("正在启动 %s …", app.Name))
		if err := m.StartExclusive(key); err != nil {
			detail := err.Error()
			if testOut, testErr := m.TestConfig(key); testErr != nil {
				detail = fmt.Sprintf("%v; nginx -t: %s", err, testOut)
			}
			*steps = append(*steps, fmt.Sprintf("启动 %s 失败: %s", app.Name, detail))
			m.apps.InvalidateLiveStatus(key)
			if m.isWebServerRunning(key) {
				*steps = append(*steps, fmt.Sprintf("Web 服务器 %s 已就绪", key))
				return key, nil
			}
			continue
		}
		m.apps.InvalidateLiveStatus(key)
		time.Sleep(2 * time.Second)
		if m.isWebServerRunning(key) {
			*steps = append(*steps, fmt.Sprintf("Web 服务器 %s 已就绪", key))
			return key, nil
		}
	}

	return "", fmt.Errorf("未能安装或启动 Nginx/OpenResty，请先在软件商店安装 Web 服务器")
}

// EnsureInstalled installs the given web server if it is not present on the host.
func (m *Manager) EnsureInstalled(key string) error {
	if !IsWebServerKey(key) {
		return fmt.Errorf("unsupported web server: %s", key)
	}
	if m.apps == nil {
		return fmt.Errorf("应用商店不可用")
	}
	m.apps.ReconcileInstalledFromSystem()
	app, err := m.apps.Get(key)
	if err != nil {
		return err
	}
	if m.webServerInstalled(key) {
		return nil
	}
	if err := m.installWebServer(key, app.Name); err != nil {
		return err
	}
	m.apps.ReconcileInstalledFromSystem()
	if !m.webServerInstalled(key) {
		return fmt.Errorf("%s 安装后仍不可用，请查看软件商店安装日志", app.Name)
	}
	return nil
}

// SwitchExclusive installs the target web server if needed, then starts it exclusively.
func (m *Manager) SwitchExclusive(key string) error {
	if err := m.stopOtherWebServers(key); err != nil {
		return err
	}
	if err := m.EnsureInstalled(key); err != nil {
		return err
	}
	return m.StartExclusive(key)
}

func (m *Manager) stopOtherWebServers(except string) error {
	for _, other := range webServerKeys {
		if other == except {
			continue
		}
		if m.apps != nil && m.apps.LiveStatus(other) == "running" {
			_ = m.apps.ServiceAction(other, "stop")
			continue
		}
		if svc := webServerServiceName(other); svc != "" {
			_ = exec.Command("systemctl", "stop", svc).Run()
		}
	}
	return nil
}

func webServerServiceName(key string) string {
	switch key {
	case "nginx":
		return "nginx"
	case "openresty":
		return "openresty"
	case "apache":
		return "apache2"
	default:
		return ""
	}
}

func (m *Manager) webServerInstalled(key string) bool {
	if m.apps == nil {
		return false
	}
	m.apps.ClearSimulatedIfRealPresent(key)
	app, err := m.apps.Get(key)
	if err != nil {
		return false
	}
	if app.Installed && !appstore.IsSimulatedInstall(key, m.dataDir) {
		if webServerBinary(key) != "" {
			return true
		}
		if m.apps.LiveStatus(key) == "running" {
			return true
		}
	}
	return appstore.SystemPackagePresent(key, m.dataDir) && webServerBinary(key) != ""
}

func (m *Manager) installWebServer(key, name string) error {
	if err := m.apps.Install(key, ""); err != nil && !installInProgress(err) {
		return fmt.Errorf("安装 %s 失败: %w", name, err)
	}
	if err := m.apps.WaitInstall(key, 15*time.Minute); err != nil {
		return fmt.Errorf("等待 %s 安装: %w", name, err)
	}
	return nil
}

func (m *Manager) isWebServerRunning(key string) bool {
	if m.apps != nil {
		m.apps.ClearSimulatedIfRealPresent(key)
		if m.apps.LiveStatus(key) == "running" {
			return true
		}
	}
	if webServerBinary(key) == "" {
		return false
	}
	return m.tryDirectReload(key) == nil
}

func orderedKeys(preferred string, keys []string) []string {
	out := []string{preferred}
	for _, k := range keys {
		if k != preferred {
			out = append(out, k)
		}
	}
	return out
}

func installInProgress(err error) bool {
	if err == nil {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "already") || strings.Contains(msg, "in progress")
}
