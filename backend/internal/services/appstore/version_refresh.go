package appstore

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

// VersionRefreshResult summarizes a store version refresh run.
type VersionRefreshResult struct {
	Added       int       `json:"added"`
	Updated     int       `json:"updated"`
	PHPVersions []string  `json:"php_versions"`
	RefreshedAt time.Time `json:"refreshed_at"`
	Message     string    `json:"message"`
}

var phpStoreVersions = []string{
	"8.4", "8.3", "8.2", "8.1", "8.0",
	"7.4", "7.3", "7.2", "7.1", "7.0",
	"5.6", "5.5", "5.4", "5.3",
}

var phpDefaultConfig = map[string]interface{}{
	"memory_limit":        "128M",
	"upload_max_filesize": "50M",
	"post_max_size":       "50M",
	"max_execution_time":  "300",
	"date.timezone":       "Asia/Shanghai",
	"open_basedir":        "",
	"disable_functions":   "exec,passthru,shell_exec,system,proc_open,popen",
}

var phpPortByVersion = map[string]int{
	"8.3": 9000, "8.2": 9001, "8.1": 9002, "7.4": 9003,
	"8.4": 9004, "8.0": 9005, "7.3": 9006, "7.2": 9007,
	"7.1": 9008, "7.0": 9009, "5.6": 9010, "5.5": 9011,
	"5.4": 9012, "5.3": 9013,
}

// multiVersionUpdates refreshes the Versions field on single-key apps with multiple upstream releases.
var multiVersionUpdates = map[string]string{
	"nginx":      "1.27,1.26,1.25,1.24",
	"mysql":      "8.4,8.0,5.7,5.6,5.5",
	"mariadb":    "11.4,10.11,10.6",
	"postgresql": "17,16,15,14",
	"redis":      "7.4,7.2,6.2",
	"mongodb":    "8.0,7.0,6.0",
	"python":     "3.13,3.12,3.11,3.10",
	"nodejs20":   "22,20,18,16",
	"java21":     "21,17,11,8",
	"tomcat10":   "10.1,9.0",
}

func PHPKeyFromVersion(ver string) string {
	return "php" + strings.ReplaceAll(ver, ".", "")
}

func phpCatalogItem(ver string) catalogItem {
	key := PHPKeyFromVersion(ver)
	verPath := strings.ReplaceAll(ver, ".", "")
	port := phpPortByVersion[ver]
	if port == 0 {
		port = 9020
	}
	return catalogItem{
		App: models.App{
			Key:         key,
			Name:        fmt.Sprintf("PHP-%s", ver),
			Category:    "运行环境",
			Versions:    ver,
			Version:     ver,
			Description: fmt.Sprintf("PHP %s 运行环境", ver),
			Port:        port,
			InstallPath: fmt.Sprintf("server/php/%s", verPath),
			ConfigPath:  fmt.Sprintf("server/php/%s/etc/php.ini", verPath),
			Icon:        "Coffee",
		},
		defaultConfig: copyMap(phpDefaultConfig),
	}
}

func copyMap(src map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func catalogKeySet() map[string]struct{} {
	set := make(map[string]struct{})
	for _, item := range catalog {
		set[item.Key] = struct{}{}
	}
	for _, item := range catalogExtraApps {
		set[item.Key] = struct{}{}
	}
	return set
}

func buildDynamicPHPCatalog() []catalogItem {
	static := catalogKeySet()
	var out []catalogItem
	for _, ver := range phpStoreVersions {
		key := PHPKeyFromVersion(ver)
		if _, ok := static[key]; ok {
			continue
		}
		out = append(out, phpCatalogItem(ver))
	}
	return out
}

func fetchPHPLatestPatch(majorMinor string) string {
	url := fmt.Sprintf("https://windows.php.net/downloads/releases/php-%s-src-latest.zip", majorMinor)
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Head(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		return majorMinor
	}
	_ = resp.Body.Close()
	// HEAD succeeded — version is available upstream; keep major.minor for install UX.
	return majorMinor
}

func fetchNginxLatestStable() string {
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get("https://nginx.org/en/download.html")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`nginx-(\d+\.\d+\.\d+)`)
	m := re.FindStringSubmatch(string(body))
	if len(m) < 2 {
		return ""
	}
	parts := strings.Split(m[1], ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return m[1]
}

// RefreshStoreVersions discovers upstream versions and updates the software store catalog.
func (s *Service) RefreshStoreVersions() (*VersionRefreshResult, error) {
	result := &VersionRefreshResult{RefreshedAt: time.Now()}

	dynamic := buildDynamicPHPCatalog()
	if err := saveDynamicCatalog(s.dataDir, dynamic); err != nil {
		return nil, err
	}

	for _, ver := range phpStoreVersions {
		result.PHPVersions = append(result.PHPVersions, ver)
	}
	result.Added = len(dynamic)

	nginxLatest := fetchNginxLatestStable()

	s.catalogMu.Lock()
	s.catalogSyncedAt = time.Time{}
	s.syncCatalogLocked()
	s.catalogSyncedAt = time.Now()

	for key, versions := range multiVersionUpdates {
		if key == "nginx" && nginxLatest != "" && !strings.Contains(versions, nginxLatest) {
			versions = nginxLatest + "," + versions
		}
		res := s.db.Model(&models.App{}).Where("app_key = ?", key).Update("versions", versions)
		if res.Error == nil && res.RowsAffected > 0 {
			result.Updated++
		}
	}
	s.catalogMu.Unlock()

	if result.Added > 0 {
		result.Message = fmt.Sprintf("已更新 %d 个 PHP 版本，刷新 %d 个软件版本信息", result.Added, result.Updated)
	} else {
		result.Message = fmt.Sprintf("版本列表已是最新，刷新了 %d 个软件版本信息", result.Updated)
	}
	return result, nil
}
