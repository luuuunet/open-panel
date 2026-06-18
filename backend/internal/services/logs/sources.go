package logs

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/settings"
)

type Source struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Enabled  bool   `json:"enabled"`
	Exists   bool   `json:"exists"`
	Size     int64  `json:"size"`
	Virtual  bool   `json:"virtual,omitempty"`
	AppKey   string `json:"app_key,omitempty"`
	AppName  string `json:"app_name,omitempty"`
	LogKind  string `json:"log_kind,omitempty"`
}

type CategoryInfo struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

func (s *Service) DiscoverSources() []Source {
	cfg := s.loadConfig()
	var out []Source
	seen := map[string]bool{}
	add := func(src Source) {
		if src.ID == "" || seen[src.ID] {
			return
		}
		if !src.Virtual && !s.offerPath(src.Path) {
			return
		}
		seen[src.ID] = true
		if src.Virtual {
			if src.LogKind == "docker" || strings.HasSuffix(src.ID, ".docker") {
				src.Exists = dockerAvailable()
			} else {
				src.Exists = true
			}
		} else if st, err := os.Stat(src.Path); err == nil && !st.IsDir() {
			src.Exists = true
			src.Size = st.Size()
		}
		src.Enabled = s.isEnabled(src.ID, cfg)
		out = append(out, src)
	}

	add(Source{
		ID: "panel.main", Category: "panel", Name: "Panel log",
		Path: panelServerLogPath(s.dataDir),
	})

	for _, p := range systemLogPaths() {
		id := "system." + slug(filepath.Base(p))
		add(Source{ID: id, Category: "system", Name: filepath.Base(p), Path: p})
	}
	if runtime.GOOS == "linux" {
		if _, err := exec.LookPath("journalctl"); err == nil {
			add(Source{
				ID: "system.journal", Category: "system", Name: "journalctl",
				Path: "journalctl", Virtual: true, LogKind: "journal",
			})
		}
	}

	s.discoverInstalledAppSources(add)

	if s.hasInstalledWebServer() || s.hasInstalledApp("nginx", "openresty") {
		logDir := filepath.Join(s.dataDir, "logs")
		if entries, err := os.ReadDir(logDir); err == nil {
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".log") {
					continue
				}
				name := e.Name()
				lower := strings.ToLower(name)
				cat := "website"
				display := strings.TrimSuffix(name, ".log")
				if strings.Contains(lower, "_cache") {
					cat = "cache"
					display = "CDN " + display
				} else if strings.Contains(lower, "_lb_") {
					cat = "cluster"
					display = "LB " + display
				}
				id := cat + "." + slug(strings.TrimSuffix(name, ".log"))
				add(Source{
					ID: id, Category: cat, Name: display,
					Path: filepath.Join(logDir, name),
				})
			}
		}
	}

	if s.hasInstalledWebServer() {
		path := s.securityLogPath()
		if path != "" && s.offerPath(path) {
			add(Source{
				ID: "waf.security", Category: "waf", Name: "WAF security log",
				Path: path,
			})
		}
	} else if path := s.securityLogPath(); path != "" && s.offerPath(path) {
		add(Source{
			ID: "waf.security", Category: "waf", Name: "WAF security log",
			Path: path,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return categoryRank(out[i].Category) < categoryRank(out[j].Category)
		}
		if out[i].AppKey != out[j].AppKey {
			return out[i].AppKey < out[j].AppKey
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func categoryRank(cat string) int {
	order := []string{"panel", "system", "software", "website", "cache", "cluster", "waf"}
	for i, k := range order {
		if cat == k {
			return i
		}
	}
	return 99
}

func dockerAvailable() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

func (s *Service) securityLogPath() string {
	defaultPath := settings.DefaultSecurityLogPath(s.dataDir)
	if s.db == nil {
		return defaultPath
	}
	var cfg models.SecurityConfig
	if err := s.db.Where("scope = ?", "global").First(&cfg).Error; err != nil {
		return defaultPath
	}
	if p := strings.TrimSpace(cfg.SecurityLogPath); p != "" {
		return p
	}
	return defaultPath
}

func systemLogPaths() []string {
	if runtime.GOOS == "windows" {
		return []string{
			filepath.Join(os.Getenv("SystemRoot"), "Logs", "CBS", "CBS.log"),
		}
	}
	return []string{
		"/var/log/syslog",
		"/var/log/messages",
		"/var/log/kern.log",
		"/var/log/auth.log",
	}
}

func (s *Service) offerPath(path string) bool {
	if path == "" || path == "journalctl" {
		return true
	}
	isWin := runtime.GOOS == "windows"
	isUnixPath := strings.HasPrefix(path, "/")
	isWinPath := len(path) >= 2 && path[1] == ':'
	if isWin && isUnixPath {
		return strings.HasPrefix(filepath.Clean(path), filepath.Clean(s.dataDir))
	}
	if !isWin && isWinPath {
		return false
	}
	return true
}

func slug(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "log"
	}
	return out
}

func (s *Service) Categories(sources []Source) []CategoryInfo {
	counts := map[string]int{}
	for _, src := range sources {
		counts[src.Category]++
	}
	order := []string{"panel", "system", "software", "website", "cache", "cluster", "waf"}
	var out []CategoryInfo
	seen := map[string]bool{}
	for _, key := range order {
		if counts[key] == 0 {
			continue
		}
		out = append(out, CategoryInfo{Key: key, Label: key, Count: counts[key]})
		seen[key] = true
	}
	var rest []string
	for k := range counts {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		out = append(out, CategoryInfo{Key: k, Label: k, Count: counts[k]})
	}
	return out
}
