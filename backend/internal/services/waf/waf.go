package waf

import (
	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

func (s *Service) List() ([]models.WAFRule, error) {
	var list []models.WAFRule
	return list, s.db.Order("id desc").Find(&list).Error
}

func (s *Service) Create(rule *models.WAFRule) error {
	return s.db.Create(rule).Error
}

func (s *Service) Delete(id uint) error {
	return s.db.Delete(&models.WAFRule{}, id).Error
}

func (s *Service) Toggle(id uint, enabled bool) error {
	return s.db.Model(&models.WAFRule{}).Where("id = ?", id).Update("enabled", enabled).Error
}

func (s *Service) SeedDefaults() {
	s.SeedFilterRules()
}

func (s *Service) SeedFilterRules() {
	defaults := []models.WAFRule{
		{Name: "SQL Injection — UNION SELECT", Type: "sql", Pattern: "union.*select", Action: "block", Enabled: true},
		{Name: "SQL Injection — DROP TABLE", Type: "sql", Pattern: "drop.*table", Action: "block", Enabled: true},
		{Name: "XSS — Script Tag", Type: "xss", Pattern: "<script", Action: "block", Enabled: true},
		{Name: "XSS — javascript URI", Type: "xss", Pattern: "javascript:", Action: "block", Enabled: true},
		{Name: "Path Traversal", Type: "path", Pattern: "\\.\\./", Action: "block", Enabled: true},
		{Name: "SQL inject keyword", Type: "uri", Pattern: "sql_inject", Action: "block", Enabled: true},
		{Name: "XSS payload keyword", Type: "uri", Pattern: "xss_payload", Action: "block", Enabled: true},
		{Name: "PHP eval", Type: "uri", Pattern: "eval\\(", Action: "block", Enabled: true},
		{Name: "Malicious UA — sqlmap", Type: "ua", Pattern: "sqlmap", Action: "block", Enabled: true},
		{Name: "Cookie SQL inject", Type: "header", Pattern: "union.*select", Action: "block", Enabled: true},
	}
	for _, r := range defaults {
		var existing models.WAFRule
		if s.db.Where("name = ?", r.Name).First(&existing).Error == gorm.ErrRecordNotFound {
			s.db.Create(&r)
		}
	}
}
