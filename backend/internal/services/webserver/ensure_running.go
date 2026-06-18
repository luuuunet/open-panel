package webserver

import (
	"fmt"
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
