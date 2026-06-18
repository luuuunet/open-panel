package cache

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

var domainSafe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func siteZoneName(domain string) string {
	sum := md5.Sum([]byte(strings.ToLower(strings.TrimSpace(domain))))
	return fmt.Sprintf("op_%x", sum[:6])
}

func sanitizeDomain(domain string) string {
	d := domainSafe.ReplaceAllString(strings.TrimSpace(domain), "_")
	if d == "" {
		d = "default"
	}
	return d
}

func (s *Service) SiteProxyCacheDir(site *models.Website) string {
	dir := filepath.Join(s.ProxyCacheDir(), sanitizeDomain(site.Domain))
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func (s *Service) SiteFastCGICacheDir(site *models.Website) string {
	dir := filepath.Join(s.FastCGICacheDir(), sanitizeDomain(site.Domain))
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func (s *Service) siteProxyZone(site *models.Website) string {
	return siteZoneName(site.Domain) + "_px"
}

func (s *Service) siteFastCGIZone(site *models.Website) string {
	return siteZoneName(site.Domain) + "_fc"
}

func (s *Service) cachedSites() ([]models.Website, error) {
	var sites []models.Website
	err := s.db.Where("cache_enabled = ?", true).Order("domain").Find(&sites).Error
	return sites, err
}
