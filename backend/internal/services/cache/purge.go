package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

type SiteCacheStat struct {
	Domain          string `json:"domain"`
	WebsiteID       uint   `json:"website_id"`
	CacheEnabled    bool   `json:"cache_enabled"`
	ProxyBytes      int64  `json:"proxy_bytes"`
	FastcgiBytes    int64  `json:"fastcgi_bytes"`
	TotalBytes      int64  `json:"total_bytes"`
	IncludeIncluded bool   `json:"nginx_include_ok"`
}

type StatusSummary struct {
	Enabled           bool            `json:"enabled"`
	DevMode           bool            `json:"dev_mode"`
	AutoSiteEnable    bool            `json:"auto_site_enable"`
	ConfPath          string          `json:"conf_path"`
	ProxyCacheDir     string          `json:"proxy_cache_dir"`
	ProxyCacheBytes   int64           `json:"proxy_cache_bytes"`
	FastCGICacheBytes int64           `json:"fastcgi_cache_bytes"`
	TotalCacheBytes   int64           `json:"total_cache_bytes"`
	CachedSites       int             `json:"cached_sites"`
	TotalSites        int             `json:"total_sites"`
	RuleCount         int             `json:"rule_count"`
	NginxIncludeOK    bool            `json:"nginx_include_ok"`
	NginxIncludeHint  string          `json:"nginx_include_hint,omitempty"`
	SiteStats         []SiteCacheStat `json:"site_stats"`
}

func (s *Service) StatusSummary() (*StatusSummary, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	var cached int64
	var total int64
	var rules int64
	s.db.Model(&models.Website{}).Count(&total)
	s.db.Model(&models.Website{}).Where("cache_enabled = ?", true).Count(&cached)
	s.db.Model(&models.CacheRule{}).Where("enabled = ?", true).Count(&rules)

	proxyBytes := dirSize(s.ProxyCacheDir())
	fcgiBytes := dirSize(s.FastCGICacheDir())

	var sites []models.Website
	_ = s.db.Order("domain").Find(&sites).Error
	siteStats := make([]SiteCacheStat, 0, len(sites))
	for _, site := range sites {
		px := dirSize(s.SiteProxyCacheDir(&site))
		fc := dirSize(s.SiteFastCGICacheDir(&site))
		siteStats = append(siteStats, SiteCacheStat{
			Domain:       site.Domain,
			WebsiteID:    site.ID,
			CacheEnabled: site.CacheEnabled,
			ProxyBytes:   px,
			FastcgiBytes: fc,
			TotalBytes:   px + fc,
		})
	}

	includeOK := false
	includeHint := ""
	if s.nginxConfPath != nil {
		confPath := strings.TrimSpace(s.nginxConfPath())
		if confPath != "" {
			if data, err := os.ReadFile(confPath); err == nil {
				includeOK = strings.Contains(string(data), "owpanel-cache.conf")
			}
			if !includeOK {
				includeHint = fmt.Sprintf("请在 %s 的 http {} 中添加: include %s;", confPath, filepath.ToSlash(s.ConfPath()))
			}
		}
	}

	return &StatusSummary{
		Enabled:           cfg.Enabled,
		DevMode:           cfg.DevMode,
		AutoSiteEnable:    cfg.AutoSiteEnable,
		ConfPath:          s.ConfPath(),
		ProxyCacheDir:     s.ProxyCacheDir(),
		ProxyCacheBytes:   proxyBytes,
		FastCGICacheBytes: fcgiBytes,
		TotalCacheBytes:   proxyBytes + fcgiBytes,
		CachedSites:       int(cached),
		TotalSites:        int(total),
		RuleCount:         int(rules),
		NginxIncludeOK:    includeOK,
		NginxIncludeHint:  includeHint,
		SiteStats:         siteStats,
	}, nil
}

func dirSize(root string) int64 {
	var total int64
	_ = walkDirSize(root, &total)
	return total
}

func walkDirSize(root string, total *int64) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		path := root + string(os.PathSeparator) + e.Name()
		if e.IsDir() {
			_ = walkDirSize(path, total)
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		*total += info.Size()
	}
	return nil
}

type PurgeResult struct {
	ClearedBytes int64  `json:"cleared_bytes"`
	Message      string `json:"message"`
	Domain       string `json:"domain,omitempty"`
}

func (s *Service) PurgeAll() (*PurgeResult, error) {
	before := dirSize(s.ProxyCacheDir()) + dirSize(s.FastCGICacheDir())
	_ = os.RemoveAll(s.ProxyCacheDir())
	_ = os.RemoveAll(s.FastCGICacheDir())
	_ = os.MkdirAll(s.ProxyCacheDir(), 0755)
	_ = os.MkdirAll(s.FastCGICacheDir(), 0755)
	return &PurgeResult{
		ClearedBytes: before,
		Message:      "已清空全部 CDN 缓存",
	}, nil
}

func (s *Service) PurgeSite(domain string) (*PurgeResult, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return nil, fmt.Errorf("domain required")
	}
	var site models.Website
	if err := s.db.Where("domain = ?", domain).First(&site).Error; err != nil {
		return nil, fmt.Errorf("site not found: %s", domain)
	}
	pxDir := s.SiteProxyCacheDir(&site)
	fcDir := s.SiteFastCGICacheDir(&site)
	before := dirSize(pxDir) + dirSize(fcDir)
	_ = os.RemoveAll(pxDir)
	_ = os.RemoveAll(fcDir)
	_ = os.MkdirAll(pxDir, 0755)
	_ = os.MkdirAll(fcDir, 0755)
	return &PurgeResult{
		ClearedBytes: before,
		Message:      fmt.Sprintf("已清空站点 %s 的 CDN 缓存", domain),
		Domain:       domain,
	}, nil
}

func (s *Service) ListSites() ([]models.Website, error) {
	var sites []models.Website
	return sites, s.db.Order("domain").Find(&sites).Error
}

func (s *Service) ToggleSite(id uint, enabled bool) error {
	return s.db.Model(&models.Website{}).Where("id = ?", id).Update("cache_enabled", enabled).Error
}

func (s *Service) UpdateSiteCache(id uint, enabled, devMode *bool, htmlTTL, staticTTL *int) error {
	updates := map[string]interface{}{}
	if enabled != nil {
		updates["cache_enabled"] = *enabled
	}
	if devMode != nil {
		updates["cache_dev_mode"] = *devMode
	}
	if htmlTTL != nil {
		updates["cache_html_ttl"] = *htmlTTL
	}
	if staticTTL != nil {
		updates["cache_static_ttl"] = *staticTTL
	}
	if len(updates) == 0 {
		return nil
	}
	return s.db.Model(&models.Website{}).Where("id = ?", id).Updates(updates).Error
}

func (s *Service) EnableAllSites() (int64, error) {
	res := s.db.Model(&models.Website{}).Where("status = ?", "running").Update("cache_enabled", true)
	return res.RowsAffected, res.Error
}
