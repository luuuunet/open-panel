package autops

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/notify"
	"github.com/luuuunet/owpanel/internal/services/website"
)

type WebsiteAuditSummary struct {
	SiteID      uint      `json:"site_id"`
	Domain      string    `json:"domain"`
	Status      string    `json:"status"`
	Score       int       `json:"score"`
	Grade       string    `json:"grade"`
	HTTPStatus  int       `json:"http_status"`
	LatencyMs   int       `json:"latency_ms"`
	Critical    int       `json:"critical"`
	Warning     int       `json:"warning"`
	BrokenLinks int       `json:"broken_links"`
	ScannedAt   time.Time `json:"scanned_at"`
	TopIssue    string    `json:"top_issue,omitempty"`
}

type WebsiteAuditListResponse struct {
	Enabled    bool                  `json:"enabled"`
	LastScan   time.Time             `json:"last_scan"`
	Total      int                   `json:"total"`
	Issues     int                   `json:"issues_with_problems"`
	AvgScore   int                   `json:"avg_score"`
	Items      []WebsiteAuditSummary `json:"items"`
}

type websiteAuditStore struct {
	mu       sync.RWMutex
	lastScan time.Time
	bySite   map[uint]*website.AuditReport
}

func newWebsiteAuditStore() *websiteAuditStore {
	return &websiteAuditStore{bySite: map[uint]*website.AuditReport{}}
}

func (s *Service) loadWebsiteScanConfig(cfg Config) Config {
	all, err := s.settings.GetAll()
	if err != nil {
		return cfg
	}
	cfg.WebsiteScanEnabled = all["auto_ops_website_scan"] != "false"
	return cfg
}

func (s *Service) saveWebsiteScanConfig(patch Config, data map[string]string) {
	if patch.WebsiteScanEnabled {
		data["auto_ops_website_scan"] = "true"
	} else {
		data["auto_ops_website_scan"] = "false"
	}
}

func (s *Service) ScanWebsiteAudits(manual bool) {
	if s.website == nil {
		return
	}
	cfg := s.loadConfig()
	if !cfg.WebsiteScanEnabled && !manual {
		return
	}

	var sites []models.Website
	if err := s.db.Where("status = ?", "running").Find(&sites).Error; err != nil {
		return
	}

	now := time.Now()
	cooldown := time.Duration(cfg.CooldownSec) * time.Second

	for _, site := range sites {
		report, err := s.website.AuditSite(site.ID)
		if err != nil {
			log.Printf("[autops] website audit %s: %v", site.Domain, err)
			continue
		}
		s.storeWebsiteReport(report)

		eventKey := fmt.Sprintf("site:%d", site.ID)
		if report.Critical > 0 || report.HTTPStatus >= 500 || report.HTTPStatus == 0 {
			if s.inSiteEventCooldown(eventKey, []string{"website_down", "website_issue"}, now, cooldown) {
				continue
			}
			msg := fmt.Sprintf("%s score %d (%s), %d critical issues", site.Domain, report.Score, report.Grade, report.Critical)
			if len(report.Findings) > 0 {
				msg = report.Findings[0].Title + " — " + site.Domain
			}
			s.logWebsiteEvent(site, "website_issue", msg, report.Grade)
			s.maybeNotifyWebsite(cfg, site, "website_issue", msg)
		} else if report.Warning > 0 {
			if s.inSiteEventCooldown(eventKey, []string{"website_issue"}, now, cooldown) {
				continue
			}
			msg := fmt.Sprintf("%s score %d, %d warnings", site.Domain, report.Score, report.Warning)
			s.logWebsiteEvent(site, "website_issue", msg, report.Grade)
		}
	}

	s.auditStore.mu.Lock()
	s.auditStore.lastScan = now
	s.auditStore.mu.Unlock()
}

func (s *Service) storeWebsiteReport(r *website.AuditReport) {
	if r == nil {
		return
	}
	s.auditStore.mu.Lock()
	s.auditStore.bySite[r.SiteID] = r
	s.auditStore.mu.Unlock()
}

func (s *Service) GetWebsiteAudit(siteID uint) (*website.AuditReport, error) {
	s.auditStore.mu.RLock()
	cached := s.auditStore.bySite[siteID]
	s.auditStore.mu.RUnlock()
	if cached != nil {
		return cached, nil
	}
	if s.website == nil {
		return nil, fmt.Errorf("website service unavailable")
	}
	report, err := s.website.AuditSite(siteID)
	if err != nil {
		return nil, err
	}
	s.storeWebsiteReport(report)
	return report, nil
}

func (s *Service) ListWebsiteAudits() *WebsiteAuditListResponse {
	cfg := s.loadConfig()
	out := &WebsiteAuditListResponse{Enabled: cfg.WebsiteScanEnabled, Items: []WebsiteAuditSummary{}}

	s.auditStore.mu.RLock()
	out.LastScan = s.auditStore.lastScan
	for _, r := range s.auditStore.bySite {
		sum := summaryFromReport(r)
		out.Items = append(out.Items, sum)
		if r.Critical > 0 || r.Warning > 0 {
			out.Issues++
		}
		out.AvgScore += r.Score
	}
	s.auditStore.mu.Unlock()

	out.Total = len(out.Items)
	if out.Total > 0 {
		out.AvgScore /= out.Total
	}
	return out
}

func (s *Service) AuditWebsiteNow(siteID uint) (*website.AuditReport, error) {
	if s.website == nil {
		return nil, fmt.Errorf("website service unavailable")
	}
	report, err := s.website.AuditSite(siteID)
	if err != nil {
		return nil, err
	}
	s.storeWebsiteReport(report)
	return report, nil
}

func summaryFromReport(r *website.AuditReport) WebsiteAuditSummary {
	sum := WebsiteAuditSummary{
		SiteID: r.SiteID, Domain: r.Domain, Status: r.Status,
		Score: r.Score, Grade: r.Grade, HTTPStatus: r.HTTPStatus,
		LatencyMs: r.LatencyMs, Critical: r.Critical, Warning: r.Warning,
		BrokenLinks: r.BrokenLinks, ScannedAt: r.ScannedAt,
	}
	if len(r.Findings) > 0 {
		sum.TopIssue = r.Findings[0].Title
	}
	return sum
}

func (s *Service) logWebsiteEvent(site models.Website, eventType, message, status string) {
	_ = s.db.Create(&models.AutoOpsEvent{
		AppKey: fmt.Sprintf("site:%d", site.ID), AppName: site.Domain,
		EventType: eventType, Message: message, Status: status,
	}).Error
}

func (s *Service) inSiteEventCooldown(appKey string, eventTypes []string, now time.Time, cooldown time.Duration) bool {
	if len(eventTypes) == 0 {
		return false
	}
	var last models.AutoOpsEvent
	err := s.db.Where("app_key = ? AND event_type IN ?", appKey, eventTypes).
		Order("created_at desc").First(&last).Error
	if err != nil {
		return false
	}
	return now.Sub(last.CreatedAt) < cooldown
}

func (s *Service) maybeNotifyWebsite(cfg Config, site models.Website, eventType, message string) {
	if cfg.NotifyWebhook == "" || !cfg.NotifyOnFail {
		return
	}
	payload := map[string]interface{}{
		"event": eventType, "site": site.Domain, "site_id": site.ID,
		"message": message, "timestamp": time.Now().Format(time.RFC3339),
	}
	notify.PostJSON(cfg.NotifyWebhook, payload)
}
