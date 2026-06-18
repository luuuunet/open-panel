package cache

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

func (s *Service) applicableRules(siteID uint) []models.CacheRule {
	var rules []models.CacheRule
	_ = s.db.Where("enabled = ?", true).
		Where("website_id = 0 OR website_id = ?", siteID).
		Order("priority ASC, id ASC").
		Find(&rules).Error
	return rules
}

// RuleLocationBlocks emits per-rule location blocks for cache action + custom TTL.
func (s *Service) RuleLocationBlocks(site *models.Website) string {
	if site == nil || !s.SiteEnabled(site) {
		return ""
	}
	cfg, err := s.GetConfig()
	if err != nil {
		return ""
	}

	var blocks []string
	for _, r := range s.applicableRules(site.ID) {
		if r.Action != "cache" || r.TTLMinutes <= 0 {
			continue
		}
		pat := strings.TrimSpace(r.Pattern)
		if pat == "" {
			continue
		}
		ttl := r.TTLMinutes
		if proxy := strings.TrimSpace(site.ProxyPass); proxy != "" {
			skip := s.skipCacheVar(site)
			stale := ""
			if cfg.StaleEnabled {
				stale = `
        proxy_cache_use_stale error timeout updating http_500 http_502 http_503 http_504;`
			}
			blocks = append(blocks, fmt.Sprintf(`
    location ~* %s {
        proxy_pass %s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache %s;
        proxy_cache_key %s;
        proxy_cache_valid 200 301 302 %dm;
        proxy_no_cache %s;
        proxy_cache_bypass %s;%s
        add_header X-Cache-Status $upstream_cache_status always;
        add_header Cache-Control "public, max-age=%d" always;
    }`, pat, proxy, s.proxyZone(site), s.cacheKey(site, cfg), ttl, skip, skip, stale, ttl*60))
		} else {
			blocks = append(blocks, fmt.Sprintf(`
    location ~* %s {
        expires %dm;
        add_header Cache-Control "public, max-age=%d, immutable" always;
        add_header X-Cache-Status "RULE" always;
    }`, pat, ttl, ttl*60))
		}
	}
	return strings.Join(blocks, "")
}
