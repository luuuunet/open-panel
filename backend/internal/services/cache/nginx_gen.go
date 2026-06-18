package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

func (s *Service) Preview() (string, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return "", err
	}
	return s.generateHTTP(cfg)
}

type ApplyResult struct {
	ConfPath    string `json:"conf_path"`
	Preview     string `json:"preview"`
	NginxReload bool   `json:"nginx_reloaded"`
	Message     string `json:"message"`
}

func (s *Service) Apply() (*ApplyResult, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	content, err := s.generateHTTP(cfg)
	if err != nil {
		return nil, err
	}
	confPath := s.ConfPath()
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		return nil, err
	}

	included, includeHint := s.ensureCacheInclude()

	reloaded := false
	if s.regen != nil {
		_ = s.regen()
	}
	if s.reload != nil {
		if err := s.reload(); err == nil {
			reloaded = true
		}
	}

	include := filepath.ToSlash(confPath)
	msg := fmt.Sprintf("缓存配置已写入 %s，请在 nginx http {} 中添加: include %s;", include, include)
	if included {
		msg = "CDN 缓存配置已应用，已自动写入 Nginx http include"
	} else if includeHint != "" {
		msg = "CDN 缓存配置已写入；" + includeHint
	}
	if reloaded {
		if included {
			msg = "CDN 缓存配置已应用，Nginx 已重载，站点虚拟主机已更新"
		} else {
			msg = "CDN 缓存配置已应用，站点虚拟主机已更新"
		}
	}
	if cfg.DevMode {
		msg += "（开发模式已开启，所有请求跳过缓存）"
	}
	return &ApplyResult{
		ConfPath:    confPath,
		Preview:     content,
		NginxReload: reloaded,
		Message:     msg,
	}, nil
}

func (s *Service) generateHTTP(cfg *models.CacheConfig) (string, error) {
	if !cfg.Enabled {
		return "# OWPanel CDN Cache — disabled\n", nil
	}
	var b strings.Builder
	b.WriteString("# OWPanel CDN Cache — auto generated (Cloudflare-style edge cache)\n\n")

	sites, _ := s.cachedSites()
	perSiteMem := "10m"
	if len(sites) > 0 {
		perSiteMem = cfg.ZoneMemory
	}
	for _, site := range sites {
		pxDir := filepath.ToSlash(s.SiteProxyCacheDir(&site))
		fcDir := filepath.ToSlash(s.SiteFastCGICacheDir(&site))
		pxZone := s.siteProxyZone(&site)
		fcZone := s.siteFastCGIZone(&site)
		b.WriteString(fmt.Sprintf("proxy_cache_path %s levels=1:2 keys_zone=%s:%s max_size=%s inactive=%s use_temp_path=off;\n",
			pxDir, pxZone, perSiteMem, cfg.ProxyMaxSize, cfg.ProxyInactive))
		b.WriteString(fmt.Sprintf("fastcgi_cache_path %s levels=1:2 keys_zone=%s:%s max_size=%s inactive=%s use_temp_path=off;\n",
			fcDir, fcZone, perSiteMem, cfg.FastcgiMaxSize, cfg.FastcgiInactive))
	}
	if len(sites) == 0 {
		pxDir := filepath.ToSlash(s.ProxyCacheDir())
		fcDir := filepath.ToSlash(s.FastCGICacheDir())
		b.WriteString(fmt.Sprintf("proxy_cache_path %s levels=1:2 keys_zone=opanel_proxy:%s max_size=%s inactive=%s use_temp_path=off;\n",
			pxDir, cfg.ZoneMemory, cfg.ProxyMaxSize, cfg.ProxyInactive))
		b.WriteString(fmt.Sprintf("fastcgi_cache_path %s levels=1:2 keys_zone=opanel_fcgi:%s max_size=%s inactive=%s use_temp_path=off;\n",
			fcDir, cfg.ZoneMemory, cfg.FastcgiMaxSize, cfg.FastcgiInactive))
	}
	b.WriteString("\n")
	b.WriteString(s.CacheLogFormatBlock())

	b.WriteString(`map $request_method $op_no_cache {
    default 1;
    GET     0;
    HEAD    0;
}
`)
	b.WriteString(buildBypassCookieMap(cfg.BypassCookies))
	b.WriteString(buildBypassPathMap(cfg.BypassPaths))
	b.WriteString(buildDevModeMap(cfg.DevMode))
	devSites, _ := s.devModeSites()
	b.WriteString(buildSiteDevModeMap(devSites))
	b.WriteString(buildGlobalRulesMap(s.globalRules()))
	b.WriteString(buildHostRulesMap(s.hostScopedRules()))
	b.WriteString(buildForceCacheMap(s.globalForceCacheRules()))
	b.WriteString(buildHostForceCacheMap(s.hostForceCacheRules()))
	b.WriteString(`
map "$op_force_cache$op_host_force_cache" $op_any_force_cache {
    default 0;
    ~*[1-9] 1;
}
map "$op_bypass_cookie$op_any_force_cache" $op_cookie_skip {
    default 0;
    "10" 1;
}
map "$op_bypass_path$op_any_force_cache" $op_path_skip {
    default 0;
    "10" 1;
}
map "$op_no_cache$op_cookie_skip$op_path_skip$op_dev_mode$op_rule_bypass$op_host_rule_bypass" $op_skip_cache {
    default 0;
    ~*[1-9] 1;
}
map "$op_skip_cache$op_site_dev" $op_final_skip {
    default 0;
    ~*[1-9] 1;
}
`)
	return b.String(), nil
}

func buildDevModeMap(devMode bool) string {
	if devMode {
		return `map $host $op_dev_mode {
    default 1;
}
`
	}
	return `map $host $op_dev_mode {
    default 0;
}
`
}

func (s *Service) devModeSites() ([]models.Website, error) {
	var sites []models.Website
	err := s.db.Preload("Aliases").Where("cache_dev_mode = ?", true).Find(&sites).Error
	return sites, err
}

func buildSiteDevModeMap(sites []models.Website) string {
	var b strings.Builder
	b.WriteString("map $host $op_site_dev {\n    default 0;\n")
	seen := make(map[string]struct{})
	for _, site := range sites {
		if !site.CacheDevMode {
			continue
		}
		hosts := []string{site.Domain}
		for _, a := range site.Aliases {
			hosts = append(hosts, a.Domain)
		}
		for _, h := range hosts {
			h = strings.TrimSpace(h)
			if h == "" {
				continue
			}
			if _, ok := seen[h]; ok {
				continue
			}
			seen[h] = struct{}{}
			b.WriteString(fmt.Sprintf("    %s 1;\n", h))
		}
	}
	b.WriteString("}\n")
	return b.String()
}

func buildGlobalRulesMap(rules []models.CacheRule) string {
	return buildRuleBypassMap("$request_uri", "op_rule_bypass", rules)
}

func buildRuleBypassMap(varName, mapName string, rules []models.CacheRule) string {
	if len(rules) == 0 {
		return fmt.Sprintf("map %s $%s {\n    default 0;\n}\n", varName, mapName)
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("map %s $%s {\n    default 0;\n", varName, mapName))
	for _, r := range rules {
		if r.Action != "bypass" {
			continue
		}
		pat := strings.TrimSpace(r.Pattern)
		if pat == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("    ~*%s 1;\n", pat))
	}
	b.WriteString("}\n")
	return b.String()
}

func buildForceCacheMap(rules []models.CacheRule) string {
	return buildForceCacheMapNamed("$request_uri", "op_force_cache", rules)
}

func buildForceCacheMapNamed(varName, mapName string, rules []models.CacheRule) string {
	if len(rules) == 0 {
		return fmt.Sprintf("map %s $%s {\n    default 0;\n}\n", varName, mapName)
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("map %s $%s {\n    default 0;\n", varName, mapName))
	for _, r := range rules {
		if r.Action != "cache" {
			continue
		}
		pat := strings.TrimSpace(r.Pattern)
		if pat == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("    ~*%s 1;\n", pat))
	}
	b.WriteString("}\n")
	return b.String()
}

func buildHostRulesMap(rules []hostRule) string {
	if len(rules) == 0 {
		return `map "$host$request_uri" $op_host_rule_bypass {
    default 0;
}
`
	}
	var b strings.Builder
	b.WriteString(`map "$host$request_uri" $op_host_rule_bypass {
    default 0;
`)
	for _, r := range rules {
		host := regexp.QuoteMeta(r.Host)
		pat := strings.TrimSpace(r.Pattern)
		if pat == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("    ~*^%s%s 1;\n", host, pat))
	}
	b.WriteString("}\n")
	return b.String()
}

type hostRule struct {
	Host    string
	Pattern string
}

func (s *Service) globalForceCacheRules() []models.CacheRule {
	var rules []models.CacheRule
	_ = s.db.Where("enabled = ? AND website_id = 0 AND action = ?", true, "cache").Order("priority ASC, id ASC").Find(&rules).Error
	return rules
}

func (s *Service) hostForceCacheRules() []hostRule {
	var rules []models.CacheRule
	_ = s.db.Where("enabled = ? AND website_id > 0 AND action = ?", true, "cache").Order("priority ASC, id ASC").Find(&rules).Error
	out := make([]hostRule, 0, len(rules))
	for _, r := range rules {
		var site models.Website
		if s.db.First(&site, r.WebsiteID).Error != nil {
			continue
		}
		out = append(out, hostRule{Host: site.Domain, Pattern: r.Pattern})
	}
	return out
}

func buildHostForceCacheMap(rules []hostRule) string {
	if len(rules) == 0 {
		return `map "$host$request_uri" $op_host_force_cache {
    default 0;
}
`
	}
	var b strings.Builder
	b.WriteString(`map "$host$request_uri" $op_host_force_cache {
    default 0;
`)
	for _, r := range rules {
		host := regexp.QuoteMeta(r.Host)
		pat := strings.TrimSpace(r.Pattern)
		if pat == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("    ~*^%s%s 1;\n", host, pat))
	}
	b.WriteString("}\n")
	return b.String()
}

func (s *Service) hostScopedRules() []hostRule {
	var rules []models.CacheRule
	_ = s.db.Where("enabled = ? AND website_id > 0 AND action = ?", true, "bypass").Order("priority ASC, id ASC").Find(&rules).Error
	out := make([]hostRule, 0, len(rules))
	for _, r := range rules {
		var site models.Website
		if s.db.First(&site, r.WebsiteID).Error != nil {
			continue
		}
		out = append(out, hostRule{Host: site.Domain, Pattern: r.Pattern})
	}
	return out
}

func buildBypassCookieMap(patterns string) string {
	patterns = strings.TrimSpace(patterns)
	if patterns == "" {
		return `map $http_cookie $op_bypass_cookie {
    default 0;
}
`
	}
	var b strings.Builder
	b.WriteString("map $http_cookie $op_bypass_cookie {\n    default 0;\n")
	for _, p := range strings.Split(patterns, "|") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("    ~*%s 1;\n", p))
	}
	b.WriteString("}\n")
	return b.String()
}

func buildBypassPathMap(patterns string) string {
	patterns = strings.TrimSpace(patterns)
	if patterns == "" {
		return `map $request_uri $op_bypass_path {
    default 0;
}
`
	}
	var b strings.Builder
	b.WriteString("map $request_uri $op_bypass_path {\n    default 0;\n")
	for _, p := range strings.Split(patterns, "|") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("    ~*%s 1;\n", p))
	}
	b.WriteString("}\n")
	return b.String()
}

func (s *Service) ServerBlockDirectives(site *models.Website) string {
	return ""
}

func (s *Service) skipCacheVar(site *models.Website) string {
	return "$op_final_skip"
}

func (s *Service) cacheKey(site *models.Website, cfg *models.CacheConfig) string {
	if cfg != nil && !cfg.CacheQueryString {
		return `"$scheme$request_method$host$uri"`
	}
	return `"$scheme$request_method$host$request_uri"`
}

func (s *Service) proxyZone(site *models.Website) string {
	if site != nil && site.CacheEnabled {
		return s.siteProxyZone(site)
	}
	return "opanel_proxy"
}

func (s *Service) fastcgiZone(site *models.Website) string {
	if site != nil && site.CacheEnabled {
		return s.siteFastCGIZone(site)
	}
	return "opanel_fcgi"
}

func (s *Service) ProxyLocationDirectives(site *models.Website) string {
	if !s.SiteEnabled(site) {
		return ""
	}
	cfg, err := s.GetConfig()
	if err != nil {
		return ""
	}
	htmlTTL := s.htmlTTL(site, cfg)
	skip := s.skipCacheVar(site)
	stale := ""
	if cfg.StaleEnabled {
		stale = `
        proxy_cache_use_stale error timeout updating http_500 http_502 http_503 http_504;
        proxy_cache_background_update on;
        proxy_cache_lock on;`
	}
	revalidate := ""
	ignoreHeaders := `
        proxy_ignore_headers Cache-Control Expires Set-Cookie Vary;`
	if cfg.HonorOrigin {
		revalidate = `
        proxy_cache_revalidate on;`
		ignoreHeaders = `
        proxy_ignore_headers Set-Cookie Vary;`
	}
	return fmt.Sprintf(`
        proxy_cache %s;
        proxy_cache_key %s;
        proxy_cache_valid 200 301 302 %dm;
        proxy_cache_valid 404 1m;
        proxy_no_cache %s;
        proxy_cache_bypass %s;%s
        add_header X-Cache-Status $upstream_cache_status always;
        add_header Cache-Control "public, max-age=%d" always;%s%s`, s.proxyZone(site), s.cacheKey(site, cfg), htmlTTL, skip, skip, ignoreHeaders, htmlTTL*60, stale, revalidate)
}

func (s *Service) PHPLocationDirectives(site *models.Website) string {
	if !s.SiteEnabled(site) {
		return ""
	}
	cfg, err := s.GetConfig()
	if err != nil {
		return ""
	}
	htmlTTL := s.htmlTTL(site, cfg)
	skip := s.skipCacheVar(site)
	stale := ""
	if cfg.StaleEnabled {
		stale = `
        fastcgi_cache_use_stale error timeout updating;
        fastcgi_cache_background_update on;
        fastcgi_cache_lock on;`
	}
	return fmt.Sprintf(`
        fastcgi_cache %s;
        fastcgi_cache_key %s;
        fastcgi_cache_valid 200 %dm;
        fastcgi_no_cache %s;
        fastcgi_cache_bypass %s;
        add_header X-Cache-Status $upstream_cache_status always;%s`, s.fastcgiZone(site), s.cacheKey(site, cfg), htmlTTL, skip, skip, stale)
}

func (s *Service) StaticLocationDirectives(site *models.Website) string {
	if !s.SiteEnabled(site) {
		return ""
	}
	hours := s.staticTTL(site, nil)
	if cfg, err := s.GetConfig(); err == nil {
		hours = s.staticTTL(site, cfg)
	}
	return fmt.Sprintf(`
        expires %dh;
        add_header Cache-Control "public, max-age=%d, immutable" always;
        add_header X-Cache-Status "STATIC" always;`, hours, hours*3600)
}

func (s *Service) RootLocationDirectives(site *models.Website) string {
	if !s.SiteEnabled(site) {
		return ""
	}
	if strings.TrimSpace(site.ProxyPass) != "" || strings.TrimSpace(site.RedirectURL) != "" {
		return ""
	}
	cfg, err := s.GetConfig()
	if err != nil {
		return ""
	}
	htmlTTL := s.htmlTTL(site, cfg)
	return fmt.Sprintf(`
        add_header Cache-Control "public, max-age=%d" always;
        add_header X-Cache-Status "BROWSER" always;`, htmlTTL*60)
}
