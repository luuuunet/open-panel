package devops

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

type CVEItem struct {
	Software    string `json:"software"`
	Version     string `json:"version"`
	CVE         string `json:"cve"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Fix         string `json:"fix"`
}

type CVEScanResult struct {
	Items      []CVEItem `json:"items"`
	HighCount  int       `json:"high_count"`
	MediumCount int      `json:"medium_count"`
	PackageUpdates int   `json:"package_updates"`
}

var knownVulnChecks = []struct {
	keyPrefix string
	minSafe   string
	cve       string
	severity  string
	desc      string
	fix       string
}{
	{"nginx", "1.24", "CVE-2024-7347", "medium", "Nginx 旧版本可能存在 HTTP/2 漏洞", "升级至 1.26+ 或软件商店重装"},
	{"mysql", "8.0.36", "CVE-2024-20963", "high", "MySQL 8.0 旧版本安全更新", "apt upgrade mysql 或软件商店升级"},
	{"mariadb", "10.11.6", "CVE-2024-21096", "medium", "MariaDB 安全更新", "升级 MariaDB 至最新稳定版"},
	{"redis", "7.2.4", "CVE-2024-31449", "high", "Redis 旧版本 RCE 风险", "升级 Redis 7.2.5+"},
	{"php83", "8.3.8", "CVE-2024-4577", "high", "PHP CGI 参数注入（Windows）", "升级 PHP 8.3.8+"},
	{"openresty", "1.21", "CVE-2024-32760", "medium", "OpenResty 跟随 Nginx 安全公告", "升级 OpenResty"},
}

func (s *Service) ScanCVE() (*CVEScanResult, error) {
	result := &CVEScanResult{}
	var apps []models.App
	_ = s.db.Where("installed = ?", true).Find(&apps).Error

	for _, app := range apps {
		ver := strings.TrimSpace(app.Version)
		if ver == "" {
			ver = "unknown"
		}
		key := strings.ToLower(app.Key)
		for _, check := range knownVulnChecks {
			if strings.HasPrefix(key, check.keyPrefix) || strings.Contains(key, check.keyPrefix) {
				if versionLT(ver, check.minSafe) {
					result.Items = append(result.Items, CVEItem{
						Software: app.Name, Version: ver, CVE: check.cve,
						Severity: check.severity, Description: check.desc, Fix: check.fix,
					})
					if check.severity == "high" {
						result.HighCount++
					} else {
						result.MediumCount++
					}
				}
			}
		}
	}

	result.PackageUpdates = countPackageUpdates()
	return result, nil
}

func countPackageUpdates() int {
	if runtime.GOOS == "linux" {
		out, err := exec.Command("sh", "-c", "apt list --upgradable 2>/dev/null | wc -l").Output()
		if err == nil {
			var n int
			fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &n)
			if n > 1 {
				return n - 1
			}
		}
	}
	return 0
}

func versionLT(current, minimum string) bool {
	current = normalizeVer(current)
	minimum = normalizeVer(minimum)
	if current == "" || current == "unknown" || current == "latest" {
		return false
	}
	c := strings.Split(current, ".")
	m := strings.Split(minimum, ".")
	for i := 0; i < len(m) && i < len(c); i++ {
		var cv, mv int
		fmt.Sscanf(c[i], "%d", &cv)
		fmt.Sscanf(m[i], "%d", &mv)
		if cv < mv {
			return true
		}
		if cv > mv {
			return false
		}
	}
	return len(c) < len(m)
}

func normalizeVer(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	for _, prefix := range []string{"nginx/", "mysql/", "php"} {
		v = strings.TrimPrefix(v, prefix)
	}
	return v
}
