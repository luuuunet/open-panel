package website

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

func ParseExpiresDate(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return nil, fmt.Errorf("到期时间格式无效，请使用 YYYY-MM-DD")
	}
	end := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, time.Local)
	return &end, nil
}

func IsSiteExpired(expiresAt *time.Time) bool {
	if expiresAt == nil {
		return false
	}
	return time.Now().After(*expiresAt)
}

func expiresLabel(expiresAt *time.Time) string {
	if expiresAt == nil {
		return "永久"
	}
	return expiresAt.Format("2006-01-02")
}

func (s *Service) EnforceExpiredSites() int {
	now := time.Now()
	var sites []models.Website
	if err := s.db.Where("expires_at IS NOT NULL AND expires_at < ? AND status = ?", now, "running").Find(&sites).Error; err != nil {
		return 0
	}
	n := 0
	for _, site := range sites {
		if _, err := s.ToggleSite(site.ID, "stopped"); err != nil {
			log.Printf("[website] auto-stop expired %s: %v", site.Domain, err)
			continue
		}
		log.Printf("[website] auto-stopped expired site %s", site.Domain)
		n++
	}
	return n
}
