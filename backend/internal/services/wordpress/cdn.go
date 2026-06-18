package wordpress

import (
	"github.com/luuuunet/owpanel/internal/models"
)

// applyCDNMode updates WordPress URLs, linked website flags, and nginx after CDN mode changes.
func (s *Service) applyCDNMode(site *models.WordPressSite) error {
	if site.RootPath != "" {
		_ = s.FixWPConfig(site.RootPath)
		https := site.CloudflareCDN || s.siteSSLEnabled(site)
		_ = s.patchWPSiteURL(site.RootPath, site.Domain, https)
	}
	if site.WebsiteID > 0 {
		forceHTTPS := !site.CloudflareCDN && s.siteSSLEnabled(site)
		_ = s.db.Model(&models.Website{}).Where("id = ?", site.WebsiteID).Updates(map[string]interface{}{
			"force_https": forceHTTPS,
		}).Error
	}
	if err := s.regenerateVhost(site.ID); err != nil {
		return err
	}
	return reloadNginxIfAvailable()
}
