package dashboard

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

// InstalledAppMetrics is an installed app with live status and resource usage.
type InstalledAppMetrics struct {
	RunningAppInfo
	LiveStatus string  `json:"live_status"`
	CPU        float64 `json:"cpu"`
	Memory     float32 `json:"memory"`
}

func BuildInstalledAppMetrics(apps []models.App, liveStatus func(key string) string, procs []ProcessBrief) []InstalledAppMetrics {
	out := make([]InstalledAppMetrics, 0, len(apps))
	for _, a := range apps {
		live := liveStatus(a.Key)
		cpu, mem := matchAppResources(a.Key, a.Port, a.InstallPath, procs)
		out = append(out, InstalledAppMetrics{
			RunningAppInfo: RunningAppInfo{
				Key: a.Key, Name: a.Name, Category: a.Category,
				Status: live, Port: a.Port, Version: a.Version,
			},
			LiveStatus: live,
			CPU:        cpu,
			Memory:     mem,
		})
	}
	return out
}

func matchAppResources(key string, port int, installPath string, procs []ProcessBrief) (cpu float64, mem float32) {
	for _, p := range procs {
		if processMatchesApp(key, port, installPath, p) {
			cpu += p.CPU
			mem += p.Memory
		}
	}
	return
}

func processMatchesApp(key string, port int, installPath string, p ProcessBrief) bool {
	lcKey := strings.ToLower(key)
	ln := strings.ToLower(p.Name)
	lc := strings.ToLower(p.Command)

	switch {
	case strings.HasPrefix(key, "php") && key != "phpmyadmin":
		if !strings.Contains(ln, "php") {
			return false
		}
		if port > 0 && (strings.Contains(lc, fmt.Sprintf(":%d", port)) || strings.Contains(lc, fmt.Sprintf(" %d", port))) {
			return true
		}
		ver := strings.TrimPrefix(lcKey, "php")
		return strings.Contains(lc, ver) || strings.Contains(lc, lcKey)
	case key == "nginx" || key == "openresty":
		return strings.Contains(ln, "nginx")
	case key == "mysql" || key == "mariadb":
		return strings.Contains(ln, "mysql") || strings.Contains(ln, "mariadb")
	case key == "redis":
		return strings.Contains(ln, "redis")
	case key == "memcached":
		return strings.Contains(ln, "memcached")
	case key == "owpanel":
		return strings.Contains(ln, "owpanel")
	case key == "phpmyadmin":
		return strings.Contains(lc, "phpmyadmin") || strings.Contains(installPath, "phpmyadmin") && strings.Contains(ln, "php")
	case key == "pm2" || key == "nodejs":
		return strings.Contains(ln, "node") || strings.Contains(ln, "pm2")
	default:
		if port > 0 && strings.Contains(lc, fmt.Sprintf(":%d", port)) {
			return true
		}
		if installPath != "" {
			ip := strings.ToLower(strings.ReplaceAll(installPath, `\`, `/`))
			if ip != "" && strings.Contains(lc, ip) {
				return true
			}
		}
		for _, token := range []string{lcKey, strings.ReplaceAll(lcKey, "-", ""), strings.ReplaceAll(lcKey, "_", "")} {
			if len(token) >= 3 && (strings.Contains(ln, token) || strings.Contains(lc, token)) {
				return true
			}
		}
		if port > 0 && strings.Contains(lc, strconv.Itoa(port)) {
			return true
		}
	}
	return false
}
