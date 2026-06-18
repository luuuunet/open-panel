package website

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/waf"
)

type nginxFeatureBlocks struct {
	access   string
	traffic  string
	geo      string
	bots     string
	subdirs  string
	hotlink  string
	crossSite string
	staticEx string
}

func (s *Service) buildNginxFeatures(site *models.Website) (*nginxFeatureBlocks, error) {
	out := &nginxFeatureBlocks{}
	subs := s.loadSubdirs(site.ID)

	htpasswd, err := s.ensureHtpasswd(site)
	if err != nil {
		return nil, err
	}
	out.access = buildAccessBlock(site, htpasswd)
	out.geo = buildGeoPolicyBlock(s.loadGeoPolicies(site.ID))
	out.bots = buildBotCrawlerBlock(s.loadBlockedCrawlers(site.ID))
	out.traffic = buildTrafficBlock(site)
	out.subdirs = buildSubdirBlock(subs)
	out.hotlink = buildHotlinkBlock(site)
	out.crossSite = buildCrossSiteProtectBlock(site)
	out.staticEx = buildStaticHotlinkExtras(site)
	return out, nil
}

func buildAccessBlock(site *models.Website, htpasswd string) string {
	var b strings.Builder
	for _, ip := range splitLines(site.AccessAllowIPs) {
		b.WriteString(fmt.Sprintf("\n    allow %s;", ip))
	}
	for _, ip := range splitLines(site.AccessDenyIPs) {
		b.WriteString(fmt.Sprintf("\n    deny %s;", ip))
	}
	if site.AccessAuthEnabled && htpasswd != "" {
		b.WriteString(fmt.Sprintf(`
    auth_basic "Restricted";
    auth_basic_user_file %s;`, htpasswd))
	}
	return b.String()
}

// buildGeoPolicyBlock injects per-site country rules using $geoip2_country_code.
// Requires GeoIP2 at http{} level (WAF security config or ngx_http_geoip2_module + GeoLite2 DB).
func buildGeoPolicyBlock(policies []models.WebsiteGeoPolicy) string {
	if len(policies) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n    # OWPanel — per-site geo policies (needs $geoip2_country_code from WAF GeoIP2)")
	for _, p := range policies {
		if !p.Enabled {
			continue
		}
		cc := strings.ToUpper(strings.TrimSpace(p.CountryCode))
		if cc == "" {
			continue
		}
		switch p.Action {
		case "block":
			b.WriteString(fmt.Sprintf("\n    if ($geoip2_country_code = %s) { return 403; }", cc))
		case "redirect":
			dest := strings.TrimSpace(p.RedirectURL)
			if dest == "" {
				continue
			}
			if !strings.Contains(dest, "$request_uri") {
				if strings.HasSuffix(dest, "/") {
					dest = dest + "$request_uri"
				} else {
					dest = dest + "$request_uri"
				}
			}
			b.WriteString(fmt.Sprintf("\n    if ($geoip2_country_code = %s) { return 301 %s; }", cc, dest))
		}
	}
	return b.String()
}

func (s *Service) loadBlockedCrawlers(websiteID uint) []waf.CrawlerPreset {
	var globalRules []models.BotCrawlerRule
	s.db.Where("website_id = 0").Find(&globalRules)
	global := map[string]string{}
	for _, r := range globalRules {
		global[r.CrawlerID] = r.Action
	}
	var siteRules []models.BotCrawlerRule
	s.db.Where("website_id = ?", websiteID).Find(&siteRules)
	site := map[string]string{}
	for _, r := range siteRules {
		site[r.CrawlerID] = r.Action
	}

	var blocked []waf.CrawlerPreset
	for _, preset := range waf.ListCrawlerPresets() {
		action := resolveBotAction(global[preset.ID], site[preset.ID], preset.DefaultAction)
		if action == "block" {
			blocked = append(blocked, preset)
		}
	}
	return blocked
}

func resolveBotAction(globalAction, siteAction, presetDefault string) string {
	if siteAction != "" && siteAction != "inherit" {
		return siteAction
	}
	if globalAction != "" && globalAction != "inherit" {
		return globalAction
	}
	if presetDefault != "" {
		return presetDefault
	}
	return "allow"
}

func buildBotCrawlerBlock(blocked []waf.CrawlerPreset) string {
	return waf.BuildSiteBotBlock(blocked)
}

func buildTrafficBlock(site *models.Website) string {
	if !site.TrafficLimitEnabled {
		return ""
	}
	burst := site.TrafficBurst
	if burst <= 0 {
		burst = 20
	}
	zone := siteLimitZone(site.Domain)
	return fmt.Sprintf(`
    limit_req zone=%s burst=%d nodelay;`, zone, burst)
}

func buildSubdirBlock(subs []models.WebsiteSubdir) string {
	if len(subs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, sub := range subs {
		prefix := strings.TrimSuffix(sub.Prefix, "/")
		if prefix == "" {
			continue
		}
		root := strings.TrimSuffix(strings.ReplaceAll(sub.RootPath, "\\", "/"), "/")
		b.WriteString(fmt.Sprintf(`
    location ^~ %s/ {
        alias %s/;
        index index.html index.htm index.php;
        try_files $uri $uri/ =404;
    }`, prefix, root))
	}
	return b.String()
}

func buildHotlinkBlock(site *models.Website) string {
	_ = site
	return ""
}

func buildStaticHotlinkExtras(site *models.Website) string {
	if !site.HotlinkEnabled && !site.CrossSiteProtectEnabled {
		return ""
	}
	var refs []string
	if site.HotlinkAllowEmpty {
		refs = append(refs, "none", "blocked")
	}
	refs = append(refs, "server_names")
	refs = append(refs, site.Domain)
	for _, d := range strings.Split(site.HotlinkAllowDomains, "|") {
		d = strings.TrimSpace(d)
		if d != "" {
			refs = append(refs, d)
		}
	}
	return fmt.Sprintf(`
        valid_referers %s;
        if ($invalid_referer) { return 403; }`, strings.Join(refs, " "))
}

func buildCrossSiteProtectBlock(site *models.Website) string {
	if !site.CrossSiteProtectEnabled {
		return ""
	}
	return `
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header Content-Security-Policy "frame-ancestors 'self'" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;`
}

func splitLines(text string) []string {
	text = strings.ReplaceAll(text, ",", "\n")
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// rewriteRulesDefineRootLocation reports whether custom rewrite rules already
// define a catch-all location / block (templates like WordPress include one).
func rewriteRulesDefineRootLocation(rules string) bool {
	rules = strings.ToLower(rules)
	return strings.Contains(rules, "location /") ||
		strings.Contains(rules, "location ^~ /") ||
		strings.Contains(rules, "location = /")
}
