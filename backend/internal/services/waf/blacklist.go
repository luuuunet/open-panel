package waf

import (
	"fmt"
	"os"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

func (s *Service) ListBlacklist() ([]models.IPBlacklist, error) {
	var list []models.IPBlacklist
	return list, s.db.Where("enabled = ?", true).Order("id desc").Find(&list).Error
}

func (s *Service) AddBlacklist(ip, reason, source string) (*models.IPBlacklist, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return nil, fmt.Errorf("ip is required")
	}
	entry := models.IPBlacklist{IP: ip, Reason: reason, Source: source, Enabled: true}
	if source == "" {
		entry.Source = "manual"
	}
	if err := s.db.Where("ip = ?", ip).Assign(entry).FirstOrCreate(&entry).Error; err != nil {
		return nil, err
	}
	_ = s.writeBlacklistMap()
	return &entry, nil
}

func (s *Service) RemoveBlacklist(id uint) error {
	if err := s.db.Delete(&models.IPBlacklist{}, id).Error; err != nil {
		return err
	}
	return s.writeBlacklistMap()
}

func (s *Service) ImportBlacklist(ips []string, reason string) (int, error) {
	count := 0
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" || strings.HasPrefix(ip, "#") {
			continue
		}
		if _, err := s.AddBlacklist(ip, reason, "batch"); err == nil {
			count++
		}
	}
	return count, nil
}

func (s *Service) writeBlacklistMap() error {
	list, err := s.ListBlacklist()
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# OWPanel IP blacklist map — auto generated\n")
	for _, item := range list {
		b.WriteString(fmt.Sprintf("%s 1;\n", item.IP))
	}
	return os.WriteFile(s.BlacklistMapPath(), []byte(b.String()), 0644)
}
