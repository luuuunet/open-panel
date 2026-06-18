package website

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/waf"
)

func (s *Service) ListGeoPolicies(websiteID uint) ([]models.WebsiteGeoPolicy, error) {
	var list []models.WebsiteGeoPolicy
	q := s.db.Order("country_code asc")
	if websiteID > 0 {
		q = q.Where("website_id = ?", websiteID)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

type GeoPolicyRequest struct {
	WebsiteID   uint   `json:"website_id"`
	CountryCode string `json:"country_code"`
	CountryName string `json:"country_name"`
	Action      string `json:"action"`
	RedirectURL string `json:"redirect_url"`
	Enabled     *bool  `json:"enabled"`
	Remark      string `json:"remark"`
}

func (s *Service) CreateGeoPolicy(req *GeoPolicyRequest) (*models.WebsiteGeoPolicy, error) {
	if req.WebsiteID == 0 {
		return nil, fmt.Errorf("website_id required")
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action != "block" && action != "redirect" {
		return nil, fmt.Errorf("action must be block or redirect")
	}
	code := strings.ToUpper(strings.TrimSpace(req.CountryCode))
	if code == "" {
		return nil, fmt.Errorf("country_code required")
	}
	if action == "redirect" && strings.TrimSpace(req.RedirectURL) == "" {
		return nil, fmt.Errorf("redirect_url required for redirect action")
	}
	name := strings.TrimSpace(req.CountryName)
	if name == "" {
		name = countryNameForCode(code)
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	var existing models.WebsiteGeoPolicy
	if err := s.db.Where("website_id = ? AND country_code = ?", req.WebsiteID, code).First(&existing).Error; err == nil {
		existing.Action = action
		existing.RedirectURL = strings.TrimSpace(req.RedirectURL)
		existing.CountryName = name
		existing.Enabled = enabled
		existing.Remark = strings.TrimSpace(req.Remark)
		if err := s.db.Save(&existing).Error; err != nil {
			return nil, err
		}
		return &existing, nil
	}
	p := models.WebsiteGeoPolicy{
		WebsiteID:   req.WebsiteID,
		CountryCode: code,
		CountryName: name,
		Action:      action,
		RedirectURL: strings.TrimSpace(req.RedirectURL),
		Enabled:     enabled,
		Remark:      strings.TrimSpace(req.Remark),
	}
	if err := s.db.Create(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Service) UpdateGeoPolicy(id uint, req *GeoPolicyRequest) (*models.WebsiteGeoPolicy, error) {
	var p models.WebsiteGeoPolicy
	if err := s.db.First(&p, id).Error; err != nil {
		return nil, err
	}
	if req.CountryCode != "" {
		p.CountryCode = strings.ToUpper(strings.TrimSpace(req.CountryCode))
	}
	if req.CountryName != "" {
		p.CountryName = strings.TrimSpace(req.CountryName)
	}
	if req.Action != "" {
		action := strings.ToLower(strings.TrimSpace(req.Action))
		if action != "block" && action != "redirect" {
			return nil, fmt.Errorf("action must be block or redirect")
		}
		p.Action = action
	}
	if req.RedirectURL != "" {
		p.RedirectURL = strings.TrimSpace(req.RedirectURL)
	}
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}
	if req.Remark != "" {
		p.Remark = strings.TrimSpace(req.Remark)
	}
	if p.Action == "redirect" && strings.TrimSpace(p.RedirectURL) == "" {
		return nil, fmt.Errorf("redirect_url required for redirect action")
	}
	if err := s.db.Save(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Service) DeleteGeoPolicy(id uint) error {
	return s.db.Delete(&models.WebsiteGeoPolicy{}, id).Error
}

func (s *Service) ApplyGeoPolicies(websiteID uint) error {
	return s.ApplyVhost(websiteID)
}

func (s *Service) loadGeoPolicies(websiteID uint) []models.WebsiteGeoPolicy {
	var policies []models.WebsiteGeoPolicy
	s.db.Where("website_id = ? AND enabled = ?", websiteID, true).Find(&policies)
	return policies
}

func countryNameForCode(code string) string {
	for _, c := range waf.ListCountries() {
		if c.Code == code {
			return c.Name
		}
	}
	return code
}
