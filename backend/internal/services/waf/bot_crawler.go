package waf

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

type CrawlerRuleView struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Icon            string `json:"icon"`
	Patterns        []string `json:"patterns"`
	DefaultAction   string `json:"default_action"`
	ConfiguredAction string `json:"configured_action"` // stored action or empty
	EffectiveAction string `json:"effective_action"`
}

type CrawlerRulesResponse struct {
	WebsiteID uint              `json:"website_id"`
	Crawlers  []CrawlerRuleView `json:"crawlers"`
}

type CrawlerRuleUpdate struct {
	CrawlerID string `json:"crawler_id"`
	Action    string `json:"action"`
}

type SaveCrawlerRulesRequest struct {
	WebsiteID uint                `json:"website_id"`
	Rules     []CrawlerRuleUpdate `json:"rules"`
}

func (s *Service) loadBotRules(websiteID uint) map[string]string {
	var rows []models.BotCrawlerRule
	s.db.Where("website_id = ?", websiteID).Find(&rows)
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.CrawlerID] = r.Action
	}
	return out
}

func (s *Service) resolveEffectiveAction(crawlerID, globalAction, siteAction, presetDefault string) string {
	if siteAction != "" && siteAction != "inherit" {
		return siteAction
	}
	if globalAction != "" && globalAction != "inherit" {
		return globalAction
	}
	if presetDefault != "" {
		return presetDefault
	}
	return "allow"
}

func (s *Service) GetCrawlerRules(websiteID uint) CrawlerRulesResponse {
	globalRules := s.loadBotRules(0)
	siteRules := map[string]string{}
	if websiteID > 0 {
		siteRules = s.loadBotRules(websiteID)
	}

	var crawlers []CrawlerRuleView
	for _, preset := range ListCrawlerPresets() {
		globalAction := globalRules[preset.ID]
		siteAction := siteRules[preset.ID]
		configured := globalAction
		if websiteID > 0 {
			configured = siteAction
		}
		effective := s.resolveEffectiveAction(preset.ID, globalAction, siteAction, preset.DefaultAction)
		crawlers = append(crawlers, CrawlerRuleView{
			ID:               preset.ID,
			Name:             preset.Name,
			Icon:             preset.Icon,
			Patterns:         preset.Patterns,
			DefaultAction:    preset.DefaultAction,
			ConfiguredAction: configured,
			EffectiveAction:  effective,
		})
	}
	return CrawlerRulesResponse{WebsiteID: websiteID, Crawlers: crawlers}
}

func (s *Service) SaveCrawlerRules(req *SaveCrawlerRulesRequest) error {
	if req == nil {
		return fmt.Errorf("request required")
	}
	for _, rule := range req.Rules {
		action := strings.ToLower(strings.TrimSpace(rule.Action))
		if req.WebsiteID == 0 {
			if action != "allow" && action != "block" {
				return fmt.Errorf("global action for %s must be allow or block", rule.CrawlerID)
			}
		} else if action != "allow" && action != "block" && action != "inherit" {
			return fmt.Errorf("site action for %s must be allow, block, or inherit", rule.CrawlerID)
		}
		if _, ok := GetCrawlerPreset(rule.CrawlerID); !ok {
			return fmt.Errorf("unknown crawler: %s", rule.CrawlerID)
		}
	}

	for _, rule := range req.Rules {
		action := strings.ToLower(strings.TrimSpace(rule.Action))
		var existing models.BotCrawlerRule
		err := s.db.Where("website_id = ? AND crawler_id = ?", req.WebsiteID, rule.CrawlerID).First(&existing).Error
		if err != nil {
			if err := s.db.Create(&models.BotCrawlerRule{
				WebsiteID: req.WebsiteID,
				CrawlerID: rule.CrawlerID,
				Action:    action,
			}).Error; err != nil {
				return err
			}
			continue
		}
		existing.Action = action
		if err := s.db.Save(&existing).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) BlockedCrawlersForSite(websiteID uint) []CrawlerPreset {
	resp := s.GetCrawlerRules(websiteID)
	var blocked []CrawlerPreset
	for _, c := range resp.Crawlers {
		if c.EffectiveAction != "block" {
			continue
		}
		if preset, ok := GetCrawlerPreset(c.ID); ok {
			blocked = append(blocked, preset)
		}
	}
	return blocked
}

func (s *Service) CrawlerRulesSummary() map[string]interface{} {
	resp := s.GetCrawlerRules(0)
	blocked := 0
	allowed := 0
	for _, c := range resp.Crawlers {
		if c.EffectiveAction == "block" {
			blocked++
		} else {
			allowed++
		}
	}
	var siteOverrides int64
	s.db.Model(&models.BotCrawlerRule{}).Where("website_id > 0 AND action <> ?", "inherit").Count(&siteOverrides)
	return map[string]interface{}{
		"global_blocked":   blocked,
		"global_allowed":   allowed,
		"site_overrides":   siteOverrides,
		"preset_count":     len(resp.Crawlers),
	}
}
