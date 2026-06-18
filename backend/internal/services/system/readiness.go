package system

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/luuuunet/owpanel/internal/platform"
	"github.com/luuuunet/owpanel/internal/services/appstore"
)

type CheckItem struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Status  string `json:"status"` // ok | warn | missing | simulated
	Detail  string `json:"detail,omitempty"`
	Action  string `json:"action,omitempty"`
	Group   string `json:"group"`
}

type ReadinessReport struct {
	Score    int           `json:"score"`
	Platform platform.Info `json:"platform"`
	Checks   []CheckItem   `json:"checks"`
}

func BuildReadiness(apps *appstore.Service, dataDir string) ReadinessReport {
	_ = apps.SyncCatalog()

	checks := []CheckItem{}
	checks = append(checks, runtimeChecks(apps, dataDir)...)
	checks = append(checks, securityChecks(apps)...)
	checks = append(checks, opsChecks(dataDir)...)

	score := 0
	for _, c := range checks {
		switch c.Status {
		case "ok":
			score += 100 / len(checks)
		case "warn", "simulated":
			score += 40 / len(checks)
		}
	}
	if score > 100 {
		score = 100
	}

	return ReadinessReport{
		Score:    score,
		Platform: platform.Detect(),
		Checks:   checks,
	}
}

func runtimeChecks(apps *appstore.Service, dataDir string) []CheckItem {
	items := []struct {
		key, label, group string
	}{
		{"nginx", "Nginx Web 服务器", "runtime"},
		{"mysql", "MySQL 数据库", "runtime"},
		{"php83", "PHP 8.3", "runtime"},
		{"certbot", "Certbot SSL", "runtime"},
		{"redis", "Redis", "runtime"},
		{"docker", "Docker", "runtime"},
		{"composer", "Composer", "runtime"},
	}
	var out []CheckItem
	for _, it := range items {
		out = append(out, appCheck(apps, dataDir, it.key, it.label, it.group))
	}
	return out
}

func securityChecks(apps *appstore.Service) []CheckItem {
	return []CheckItem{
		appCheckSimple(apps, "fail2ban", "Fail2ban", "security"),
		appCheckSimple(apps, "pureftpd", "Pure-FTPd", "security"),
	}
}

func opsChecks(dataDir string) []CheckItem {
	backupDir := filepath.Join(dataDir, "backup")
	ok := dirHasFiles(backupDir)
	st := "missing"
	detail := "尚未产生备份文件"
	if ok {
		st = "ok"
		detail = backupDir
	}
	return []CheckItem{{
		Key: "backup", Label: "本地备份目录", Status: st, Detail: detail, Group: "ops",
		Action: "/backup",
	}}
}

func appCheck(apps *appstore.Service, dataDir, key, label, group string) CheckItem {
	item := appCheckSimple(apps, key, label, group)
	if appstore.IsSimulatedInstall(key, dataDir) {
		item.Status = "simulated"
		item.Detail = "当前为模拟安装，请在 Linux 服务器上一键安装真实环境"
		item.Action = "/nginx"
	}
	return item
}

func appCheckSimple(apps *appstore.Service, key, label, group string) CheckItem {
	app, err := apps.Get(key)
	if err != nil {
		return CheckItem{Key: key, Label: label, Status: "missing", Group: group, Action: "/nginx"}
	}
	if !app.Installed {
		return CheckItem{Key: key, Label: label, Status: "missing", Group: group, Action: "/nginx"}
	}
	st := apps.LiveStatus(key)
	if st == "running" {
		return CheckItem{Key: key, Label: label, Status: "ok", Detail: st, Group: group}
	}
	if st == "simulated" {
		return CheckItem{Key: key, Label: label, Status: "simulated", Detail: st, Group: group, Action: "/nginx"}
	}
	return CheckItem{Key: key, Label: label, Status: "warn", Detail: "已安装但未运行: " + st, Group: group, Action: "/nginx"}
}

func dirHasFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}

func BinaryOnPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func IsLinuxServer() bool {
	return runtime.GOOS == "linux" && platform.PackageManager() != ""
}
