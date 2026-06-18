package cache

import (
	"github.com/luuuunet/owpanel/internal/models"
)

func (s *Service) ListRules(websiteID uint) ([]models.CacheRule, error) {
	var rules []models.CacheRule
	q := s.db.Where("enabled = ?", true).Order("priority ASC, id ASC")
	if websiteID > 0 {
		q = q.Where("website_id = ? OR website_id = 0", websiteID)
	} else {
		q = q.Where("website_id = 0")
	}
	return rules, q.Find(&rules).Error
}

func (s *Service) ListAllRules() ([]models.CacheRule, error) {
	var rules []models.CacheRule
	return rules, s.db.Order("priority ASC, id ASC").Find(&rules).Error
}

func (s *Service) CreateRule(rule *models.CacheRule) (*models.CacheRule, error) {
	if rule.Action == "" {
		rule.Action = "bypass"
	}
	if err := s.db.Create(rule).Error; err != nil {
		return nil, err
	}
	return rule, nil
}

func (s *Service) UpdateRule(id uint, patch *models.CacheRule) (*models.CacheRule, error) {
	var rule models.CacheRule
	if err := s.db.First(&rule, id).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&rule).Updates(patch).Error; err != nil {
		return nil, err
	}
	return &rule, s.db.First(&rule, id).Error
}

func (s *Service) DeleteRule(id uint) error {
	return s.db.Delete(&models.CacheRule{}, id).Error
}

func (s *Service) globalRules() []models.CacheRule {
	var rules []models.CacheRule
	_ = s.db.Where("enabled = ? AND website_id = 0 AND action = ?", true, "bypass").Order("priority ASC, id ASC").Find(&rules).Error
	return rules
}

func (s *Service) siteRules(siteID uint) []models.CacheRule {
	var rules []models.CacheRule
	_ = s.db.Where("enabled = ? AND website_id = ?", true, siteID).Order("priority ASC, id ASC").Find(&rules).Error
	return rules
}
