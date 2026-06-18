package waf

import (
	"fmt"
	"os"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

func (s *Service) ListWhitelist() ([]models.IPWhitelist, error) {
	var list []models.IPWhitelist
	return list, s.db.Where("enabled = ?", true).Order("id desc").Find(&list).Error
}

func (s *Service) AddWhitelist(ip, reason, source string) (*models.IPWhitelist, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return nil, fmt.Errorf("ip is required")
	}
	entry := models.IPWhitelist{IP: ip, Reason: reason, Source: source, Enabled: true}
	if source == "" {
		entry.Source = "manual"
	}
	if err := s.db.Where("ip = ?", ip).Assign(entry).FirstOrCreate(&entry).Error; err != nil {
		return nil, err
	}
	_ = s.writeWhitelistMap()
	return &entry, nil
}

func (s *Service) RemoveWhitelist(id uint) error {
	if err := s.db.Delete(&models.IPWhitelist{}, id).Error; err != nil {
		return err
	}
	return s.writeWhitelistMap()
}

func (s *Service) ImportWhitelist(ips []string, reason string) (int, error) {
	count := 0
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" || strings.HasPrefix(ip, "#") {
			continue
		}
		if _, err := s.AddWhitelist(ip, reason, "batch"); err == nil {
			count++
		}
	}
	return count, nil
}

func (s *Service) writeWhitelistMap() error {
	list, err := s.ListWhitelist()
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# OWPanel IP whitelist map — auto generated\n")
	for _, item := range list {
		b.WriteString(fmt.Sprintf("%s 1;\n", item.IP))
	}
	return os.WriteFile(s.WhitelistMapPath(), []byte(b.String()), 0644)
}
