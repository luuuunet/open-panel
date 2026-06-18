package waf

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/settings"
)

type ApplyResult struct {
	ConfPath    string `json:"conf_path"`
	Preview     string `json:"preview"`
	NginxReload bool   `json:"nginx_reloaded"`
	Message     string `json:"message"`
}

type headerPresetValues struct {
	CSP              string
	XFrameOptions    string
	HSTSEnabled      bool
	HSTSMaxAge       int
	XContentTypeOpts bool
	ReferrerPolicy   string
	PermissionsPolicy string
}

func resolveHeaderPreset(cfg *models.SecurityConfig) headerPresetValues {
	switch strings.ToLower(strings.TrimSpace(cfg.HeaderPreset)) {
	case "strict":
		return headerPresetValues{
			CSP:              "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'",
			XFrameOptions:    "DENY",
			HSTSEnabled:      true,
			HSTSMaxAge:       63072000,
			XContentTypeOpts: true,
			ReferrerPolicy:   "no-referrer",
			PermissionsPolicy: "geolocation=(), microphone=(), camera=(), payment=(), usb=()",
		}
	case "balanced":
		return headerPresetValues{
			CSP:              "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:; connect-src 'self'",
			XFrameOptions:    "SAMEORIGIN",
			HSTSEnabled:      true,
			HSTSMaxAge:       31536000,
			XContentTypeOpts: true,
			ReferrerPolicy:   "strict-origin-when-cross-origin",
			PermissionsPolicy: "geolocation=(), microphone=(), camera=()",
		}
	case "none":
		return headerPresetValues{}
	default:
		return headerPresetValues{
			CSP:              cfg.CSP,
			XFrameOptions:    cfg.XFrameOptions,
			HSTSEnabled:      cfg.HSTSEnabled,
			HSTSMaxAge:       cfg.HSTSMaxAge,
			XContentTypeOpts: cfg.XContentTypeOpts,
			ReferrerPolicy:   cfg.ReferrerPolicy,
			PermissionsPolicy: "geolocation=(), microphone=(), camera=()",
		}
	}
}

func (s *Service) Preview() (string, error) {
	s.ensureDefaults()
	cfg, err := s.GetConfig()
	if err != nil {
		return "", err
	}
	rules, err := s.List()
	if err != nil {
		return "", err
	}
	return s.generateNginx(cfg, rules), nil
}

func (s *Service) Apply() (*ApplyResult, error) {
	content, err := s.Preview()
	if err != nil {
		return nil, err
	}
	confPath := s.ConfPath()
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		return nil, err
	}
	_ = s.writeBlacklistMap()
	_ = s.writeWhitelistMap()

	if cfg, _ := s.GetConfig(); cfg != nil && cfg.LogFormatEnabled {
		logConf := generateLogFormat(cfg)
		_ = os.WriteFile(s.LogFormatPath(), []byte(logConf), 0644)
		_ = os.WriteFile(s.Fail2BanFilterPath(), []byte(generateFail2BanFilter()), 0644)
	}

	reloaded := false
	confInclude := nginxPath(confPath)
	msg := "配置已写入 " + confInclude + "，请在 nginx http {} 中添加: include " + confInclude + ";"
	if err := exec.Command("nginx", "-t").Run(); err == nil {
		if err := exec.Command("nginx", "-s", "reload").Run(); err == nil {
			reloaded = true
			msg = "Nginx 安全配置已应用并重载"
		}
	}

	return &ApplyResult{
		ConfPath:    confPath,
		Preview:     content,
		NginxReload: reloaded,
		Message:     msg,
	}, nil
}

func (s *Service) generateNginx(cfg *models.SecurityConfig, rules []models.WAFRule) string {
	var b strings.Builder
	b.WriteString("# OWPanel Nginx Security — auto generated\n")
	b.WriteString("# Modules: rate_limit, conn_limit, access_control, request_filter, security_headers, security_log, edge_policies\n\n")

	if cfg.LogFormatEnabled {
		b.WriteString(generateLogFormat(cfg))
		b.WriteString("\n")
	}

	if cfg.RateLimitEnabled {
		rate := cfg.RateLimitRate
		if rate == "" {
			rate = "10r/s"
		}
		b.WriteString("limit_req_zone $binary_remote_addr zone=op_ratelimit:10m rate=" + rate + ";\n")
	}

	if cfg.ApiRateLimitEnabled {
		rate := cfg.ApiRateLimitRate
		if rate == "" {
			rate = "30r/s"
		}
		b.WriteString("limit_req_zone $binary_remote_addr zone=op_api_ratelimit:10m rate=" + rate + ";\n")
	}

	if cfg.ConnLimitEnabled {
		b.WriteString("limit_conn_zone $binary_remote_addr zone=op_connlimit:10m;\n")
	}

	if cfg.WhitelistEnabled {
		b.WriteString("map $remote_addr $op_whitelisted {\n    default 0;\n")
		b.WriteString("    include " + nginxPath(s.WhitelistMapPath()) + ";\n}\n")
	} else {
		b.WriteString("map $remote_addr $op_whitelisted { default 0; }\n")
	}

	if cfg.BlacklistEnabled {
		b.WriteString("map $remote_addr $op_blocked_ip {\n    default 0;\n")
		b.WriteString("    include " + nginxPath(s.BlacklistMapPath()) + ";\n}\n")
		b.WriteString("map \"$op_whitelisted$op_blocked_ip\" $op_deny_ip {\n    default 0;\n    \"01\" 1;\n}\n")
	}

	if cfg.GeoBlockEnabled && cfg.BlockedCountries != "" {
		dbPath := nginxPath(s.GeoDBPath(cfg))
		b.WriteString("# GeoIP2 country access control\n")
		b.WriteString("geoip2 " + dbPath + " {\n")
		b.WriteString("    auto_reload 5m;\n")
		b.WriteString("    $geoip2_country_code country iso_code;\n")
		b.WriteString("}\n\n")
		b.WriteString("map $geoip2_country_code $op_blocked_geo {\n")
		codes := parseCountryCodes(cfg.BlockedCountries)
		mode := cfg.GeoMode
		if mode == "" {
			mode = "block"
		}
		if mode == "allow" {
			b.WriteString("    default 1;\n")
			for _, cc := range codes {
				b.WriteString("    " + cc + " 0;\n")
			}
		} else {
			b.WriteString("    default 0;\n")
			for _, cc := range codes {
				b.WriteString("    " + cc + " 1;\n")
			}
		}
		b.WriteString("}\n")
		b.WriteString("map \"$op_whitelisted$op_blocked_geo\" $op_deny_geo {\n    default 0;\n    \"01\" 1;\n}\n")
	}

	methods := parseBlockedMethods(cfg.BlockHttpMethods)
	if len(methods) > 0 {
		b.WriteString("map $request_method $op_bad_method {\n    default 0;\n")
		for _, m := range methods {
			b.WriteString("    " + m + " 1;\n")
		}
		b.WriteString("}\n")
		b.WriteString("map \"$op_whitelisted$op_bad_method\" $op_deny_method {\n    default 0;\n    \"01\" 1;\n}\n")
	}

	if cfg.FilterEnabled {
		b.WriteString("map $http_user_agent $op_bad_ua {\n    default 0;\n")
		if cfg.AllowSearchBots {
			b.WriteString("    ~*(?i)(googlebot|bingbot|slurp|duckduckbot|yandexbot|baiduspider|sogou|360spider) 0;\n")
		}
		if cfg.BlockHeadlessBots {
			b.WriteString("    ~*(?i)(headlesschrome|phantomjs|puppeteer|selenium|webdriver) 1;\n")
		}
		if cfg.BlockScannerUA {
			b.WriteString("    ~*sqlmap 1;\n    ~*nikto 1;\n    ~*nmap 1;\n    ~*masscan 1;\n    ~*acunetix 1;\n    ~*nessus 1;\n")
		}
		if cfg.BlockBadUserAgent {
			b.WriteString("    ~*libwww-perl 1;\n    ~*wget 1;\n    \"\" 1;\n")
		}
		for _, r := range rules {
			if !r.Enabled || r.Pattern == "" {
				continue
			}
			if r.Type == "ua" {
				b.WriteString(fmt.Sprintf("    ~*(?i)(%s) 1;\n", escapeNginxRegex(r.Pattern)))
			}
		}
		b.WriteString("}\n")
		b.WriteString("map \"$op_whitelisted$op_bad_ua\" $op_deny_ua {\n    default 0;\n    \"01\" 1;\n}\n")

		b.WriteString("map $request_uri $op_bad_uri {\n    default 0;\n")
		b.WriteString("    ~*(?i)(union.*select|select.*from|insert.*into|delete.*from|drop.*table) 1;\n")
		b.WriteString("    ~*(?i)(<script|javascript:|onerror=|onload=|xss_payload) 1;\n")
		b.WriteString("    ~*(?i)(\\.\\./|/etc/passwd|/proc/self) 1;\n")
		for _, r := range rules {
			if !r.Enabled || r.Pattern == "" {
				continue
			}
			if r.Type == "uri" || r.Type == "sql" || r.Type == "xss" || r.Type == "path" || r.Type == "custom" {
				b.WriteString(fmt.Sprintf("    ~*(?i)(%s) 1;\n", escapeNginxRegex(r.Pattern)))
			}
		}
		b.WriteString("}\n")
		b.WriteString("map \"$op_whitelisted$op_bad_uri\" $op_deny_uri {\n    default 0;\n    \"01\" 1;\n}\n")

		b.WriteString("map $http_cookie $op_bad_header {\n    default 0;\n")
		b.WriteString("    ~*(?i)(union.*select|<script|xss_payload|sql_inject) 1;\n")
		for _, r := range rules {
			if !r.Enabled || r.Pattern == "" {
				continue
			}
			if r.Type == "header" {
				b.WriteString(fmt.Sprintf("    ~*(?i)(%s) 1;\n", escapeNginxRegex(r.Pattern)))
			}
		}
		b.WriteString("}\n")
		b.WriteString("map \"$op_whitelisted$op_bad_header\" $op_deny_header {\n    default 0;\n    \"01\" 1;\n}\n")
	}

	b.WriteString("\n# ── server 块内 include 以下片段 ──\n")
	b.WriteString("# include " + nginxPath(s.confSnippetPath()) + ";\n\n")
	b.WriteString(s.generateServerSnippet(cfg))
	return b.String()
}

func parseBlockedMethods(raw string) []string {
	if raw == "" {
		raw = "TRACE,TRACK,DEBUG,CONNECT"
	}
	var out []string
	for _, m := range strings.Split(raw, ",") {
		m = strings.TrimSpace(strings.ToUpper(m))
		if m != "" {
			out = append(out, m)
		}
	}
	return out
}

func (s *Service) confSnippetPath() string {
	return s.confDir + "/server_security.conf"
}

func (s *Service) generateServerSnippet(cfg *models.SecurityConfig) string {
	snippetPath := s.confSnippetPath()
	var b strings.Builder
	b.WriteString("# server snippet → " + snippetPath + "\n")

	var sn strings.Builder

	if cfg.SlowAttackEnabled {
		bodyTO := cfg.ClientBodyTimeoutSec
		if bodyTO <= 0 {
			bodyTO = 12
		}
		headerTO := cfg.ClientHeaderTimeoutSec
		if headerTO <= 0 {
			headerTO = 12
		}
		sn.WriteString(fmt.Sprintf("client_body_timeout %ds;\n", bodyTO))
		sn.WriteString(fmt.Sprintf("client_header_timeout %ds;\n", headerTO))
		sn.WriteString(fmt.Sprintf("send_timeout %ds;\n", headerTO))
	}

	if cfg.RateLimitEnabled {
		nodelay := ""
		if cfg.RateLimitNodelay {
			nodelay = " nodelay"
		}
		burst := cfg.RateLimitBurst
		if burst <= 0 {
			burst = 20
		}
		sn.WriteString(fmt.Sprintf("limit_req zone=op_ratelimit burst=%d%s;\n", burst, nodelay))
	}

	if cfg.ConnLimitEnabled {
		perIP := cfg.ConnLimitPerIP
		if perIP <= 0 {
			perIP = 50
		}
		sn.WriteString(fmt.Sprintf("limit_conn op_connlimit %d;\n", perIP))
	}

	if cfg.BlacklistEnabled {
		sn.WriteString("if ($op_deny_ip) { return 403; }\n")
	}
	if cfg.GeoBlockEnabled {
		sn.WriteString("if ($op_deny_geo) { return 403; }\n")
	}
	if len(parseBlockedMethods(cfg.BlockHttpMethods)) > 0 {
		sn.WriteString("if ($op_deny_method) { return 405; }\n")
	}
	if cfg.FilterEnabled {
		sn.WriteString("if ($op_deny_ua) { return 403; }\n")
		sn.WriteString("if ($op_deny_uri) { return 403; }\n")
		sn.WriteString("if ($op_deny_header) { return 403; }\n")
	}

	headersOn := cfg.HeadersEnabled && strings.ToLower(cfg.HeaderPreset) != "none"
	if headersOn {
		preset := resolveHeaderPreset(cfg)
		if preset.XFrameOptions != "" {
			sn.WriteString("add_header X-Frame-Options \"" + preset.XFrameOptions + "\" always;\n")
		}
		if preset.XContentTypeOpts {
			sn.WriteString("add_header X-Content-Type-Options \"nosniff\" always;\n")
		}
		if preset.ReferrerPolicy != "" {
			sn.WriteString("add_header Referrer-Policy \"" + preset.ReferrerPolicy + "\" always;\n")
		}
		if preset.CSP != "" {
			sn.WriteString("add_header Content-Security-Policy \"" + strings.ReplaceAll(preset.CSP, "\"", "\\\"") + "\" always;\n")
		}
		if preset.HSTSEnabled {
			maxAge := preset.HSTSMaxAge
			if maxAge <= 0 {
				maxAge = 31536000
			}
			sn.WriteString(fmt.Sprintf("add_header Strict-Transport-Security \"max-age=%d; includeSubDomains\" always;\n", maxAge))
		}
		sn.WriteString("add_header X-XSS-Protection \"1; mode=block\" always;\n")
		if preset.PermissionsPolicy != "" {
			sn.WriteString("add_header Permissions-Policy \"" + preset.PermissionsPolicy + "\" always;\n")
		}
	}

	if cfg.LogFormatEnabled {
		logPath := cfg.SecurityLogPath
		if logPath == "" {
			logPath = settings.DefaultSecurityLogPath(s.dataDir)
		}
		sn.WriteString("access_log " + logPath + " security_detailed;\n")
	}

	content := sn.String()

	if cfg.ApiRateLimitEnabled {
		burst := cfg.ApiRateLimitBurst
		if burst <= 0 {
			burst = 60
		}
		content += fmt.Sprintf(`
location ^~ /api/ {
    limit_req zone=op_api_ratelimit burst=%d nodelay;
}
`, burst)
	}

	if cfg.HotlinkEnabled {
		content += generateHotlinkBlock(cfg)
	}

	b.WriteString(content)
	_ = os.WriteFile(snippetPath, []byte(content), 0644)
	return b.String()
}

func generateHotlinkBlock(cfg *models.SecurityConfig) string {
	var refs []string
	if cfg.HotlinkAllowEmpty {
		refs = append(refs, "none", "blocked")
	}
	refs = append(refs, "server_names")
	for _, d := range strings.Split(cfg.HotlinkAllowDomains, ",") {
		d = strings.TrimSpace(d)
		if d != "" {
			refs = append(refs, d)
		}
	}
	return fmt.Sprintf(`
location ~* \.(jpg|jpeg|png|gif|ico|svg|webp|bmp|mp4|mp3|avi|mov|wmv|flv|css|js|woff|woff2|ttf|eot)$ {
    valid_referers %s;
    if ($invalid_referer) { return 403; }
}
`, strings.Join(refs, " "))
}

func generateLogFormat(cfg *models.SecurityConfig) string {
	return `# Security log format — Fail2Ban / ELK friendly
log_format security_detailed '$remote_addr - $request_id [$time_local] '
    '"$request" $status $body_bytes_sent '
    'rt=$request_time urt=$upstream_response_time '
    'uaddr=$upstream_addr '
    '"$http_user_agent" "$http_x_forwarded_for" '
    'blocked_ip=$op_blocked_ip bad_uri=$op_bad_uri whitelisted=$op_whitelisted';
`
}

func generateFail2BanFilter() string {
	return `[Definition]
failregex = ^<HOST> - .* "(GET|POST|HEAD).*"(403|444|429)
ignoreregex =
`
}

func escapeNginxRegex(p string) string {
	p = strings.ReplaceAll(p, `"`, `\"`)
	return p
}

func nginxPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return filepath.ToSlash(p)
}

func (s *Service) StatusSummary() map[string]interface{} {
	cfg, _ := s.GetConfig()
	rules, _ := s.List()
	bl, _ := s.ListBlacklist()
	wl, _ := s.ListWhitelist()
	enabledRules := 0
	for _, r := range rules {
		if r.Enabled {
			enabledRules++
		}
	}
	geoCountries := 0
	geoDBExists := false
	if cfg != nil {
		geoCountries = len(parseCountryCodes(cfg.BlockedCountries))
		geoDBExists = fileExists(s.GeoDBPath(cfg))
	}
	edgePolicies := map[string]interface{}{}
	if cfg != nil {
		edgePolicies = map[string]interface{}{
			"whitelist_enabled":  cfg.WhitelistEnabled,
			"allow_search_bots":  cfg.AllowSearchBots,
			"block_headless_bots": cfg.BlockHeadlessBots,
			"block_http_methods": len(parseBlockedMethods(cfg.BlockHttpMethods)) > 0,
			"slow_attack":        cfg.SlowAttackEnabled,
			"api_rate_limit":     cfg.ApiRateLimitEnabled,
			"hotlink":            cfg.HotlinkEnabled,
			"header_preset":      cfg.HeaderPreset,
		}
	}
	return map[string]interface{}{
		"rate_limit":      cfg != nil && cfg.RateLimitEnabled,
		"conn_limit":      cfg != nil && cfg.ConnLimitEnabled,
		"blacklist_count": len(bl),
		"whitelist_count": len(wl),
		"geo_block":       cfg != nil && cfg.GeoBlockEnabled,
		"geo_countries":   geoCountries,
		"geo_db_exists":   geoDBExists,
		"filter_rules":    enabledRules,
		"headers":         cfg != nil && cfg.HeadersEnabled,
		"security_log":    cfg != nil && cfg.LogFormatEnabled,
		"conf_exists":     fileExists(s.ConfPath()),
		"edge_policies":   edgePolicies,
		"crawler_rules":   s.CrawlerRulesSummary(),
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
