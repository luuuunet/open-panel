package website

import (
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/domaincheck"
	"gorm.io/gorm"
)

// ensurePrimaryAlias backfills the primary domain row for sites created without aliases
// (e.g. legacy WordPress sync).
func (s *Service) ensurePrimaryAlias(site *models.Website) {
	if site == nil || site.ID == 0 || site.Domain == "" {
		return
	}
	host := domaincheck.HostOnly(site.Domain)
	port := site.Port
	if port <= 0 {
		port = 80
	}

	var existing models.WebsiteAlias
	err := s.db.Where("website_id = ? AND type = ?", site.ID, "primary").First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		_ = s.db.Create(&models.WebsiteAlias{
			WebsiteID: site.ID,
			Domain:    host,
			Port:      port,
			Type:      "primary",
		}).Error
		return
	}
	if err == nil && existing.Domain != host {
		_ = s.db.Model(&existing).Updates(map[string]interface{}{
			"domain": host,
			"port":   port,
		}).Error
	}
}

func (s *Service) reloadAliases(site *models.Website) {
	if site == nil || site.ID == 0 {
		return
	}
	s.ensurePrimaryAlias(site)
	s.db.Where("website_id = ?", site.ID).Order("type desc, id asc").Find(&site.Aliases)
}
