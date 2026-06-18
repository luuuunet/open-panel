package website

import (
	"github.com/luuuunet/owpanel/internal/models"
)

// ToggleCrossSiteProtect enables or disables anti cross-site access for a website.
// When enabled: static hotlink protection + X-Frame-Options / CSP frame-ancestors headers.
func (s *Service) ToggleCrossSiteProtect(siteID uint, enabled *bool) (*models.Website, error) {
	site, err := s.Get(siteID)
	if err != nil {
		return nil, err
	}
	on := !site.CrossSiteProtectEnabled
	if enabled != nil {
		on = *enabled
	}
	updates := map[string]interface{}{
		"cross_site_protect_enabled": on,
		"hotlink_enabled":            on,
	}
	if on {
		updates["hotlink_allow_empty"] = true
	}
	if err := s.db.Model(site).Updates(updates).Error; err != nil {
		return nil, err
	}
	site, err = s.Get(siteID)
	if err != nil {
		return nil, err
	}
	if err := s.regenerateVhost(site); err != nil {
		return nil, err
	}
	return site, nil
}
