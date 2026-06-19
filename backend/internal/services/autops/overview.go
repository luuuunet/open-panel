package autops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"github.com/luuuunet/owpanel/internal/services/k8s"
)

type OverviewResponse struct {
	Status           *StatusResponse `json:"status"`
	UptimeTotal      int             `json:"uptime_total"`
	UptimeDown       int             `json:"uptime_down"`
	CronTotal        int             `json:"cron_total"`
	CronEnabled      int             `json:"cron_enabled"`
	CronFailed       int             `json:"cron_failed"`
	BackupTotal      int             `json:"backup_total"`
	BackupEnabled    int             `json:"backup_enabled"`
	SSLExpiringSoon  int             `json:"ssl_expiring_soon"`
	SitesExpiringSoon int            `json:"sites_expiring_soon"`
	WebsiteAudited    int            `json:"website_audited"`
	WebsiteIssues     int            `json:"website_issues"`
	WebsiteAvgScore   int            `json:"website_avg_score"`
	LogAutoCleanup   bool            `json:"log_auto_cleanup"`
	LogRetentionDays int             `json:"log_retention_days"`
	CPU              float64         `json:"cpu_percent"`
	Memory           float64         `json:"memory_percent"`
	Disk             float64         `json:"disk_percent"`
	K8sInstalled     bool            `json:"k8s_installed"`
	K8sReady         bool            `json:"k8s_ready"`
}

func (s *Service) GetOverview() (*OverviewResponse, error) {
	st, err := s.GetStatus()
	if err != nil {
		return nil, err
	}
	out := &OverviewResponse{Status: st}
	now := time.Now()
	alertDays := 14
	cfg := s.loadConfig()
	if cfg.AlertDaysSSL > 0 {
		alertDays = cfg.AlertDaysSSL
	}

	var monitors []models.UptimeMonitor
	if err := s.db.Find(&monitors).Error; err == nil {
		out.UptimeTotal = len(monitors)
		for _, m := range monitors {
			if m.Enabled && m.LastStatus == "down" {
				out.UptimeDown++
			}
		}
	}

	var jobs []models.CronJob
	if err := s.db.Find(&jobs).Error; err == nil {
		out.CronTotal = len(jobs)
		for _, j := range jobs {
			if j.Enabled {
				out.CronEnabled++
			}
			if j.LastStatus == "failed" {
				out.CronFailed++
			}
		}
	}

	var backups []models.BackupTask
	if err := s.db.Find(&backups).Error; err == nil {
		out.BackupTotal = len(backups)
		for _, b := range backups {
			if b.Enabled {
				out.BackupEnabled++
			}
		}
	}

	deadline := now.AddDate(0, 0, alertDays)
	var sslCount int64
	s.db.Model(&models.SSLCertificate{}).Where("expires_at IS NOT NULL AND expires_at > ? AND expires_at <= ?", now, deadline).Count(&sslCount)
	out.SSLExpiringSoon = int(sslCount)

	siteDays := cfg.AlertDaysSite
	if siteDays < 1 {
		siteDays = 14
	}
	siteDeadline := now.AddDate(0, 0, siteDays)
	var siteCount int64
	s.db.Model(&models.Website{}).Where("expires_at IS NOT NULL AND expires_at > ? AND expires_at <= ? AND status = ?", now, siteDeadline, "running").Count(&siteCount)
	out.SitesExpiringSoon = int(siteCount)

	wa := s.ListWebsiteAudits()
	out.WebsiteAudited = wa.Total
	out.WebsiteIssues = wa.Issues
	out.WebsiteAvgScore = wa.AvgScore

	out.loadLogRetention(s.dataDir)

	out.K8sInstalled = appstore.K3sRunning()
	if !out.K8sInstalled {
		if app, err := s.apps.Get("k3s"); err == nil && app.Installed {
			out.K8sInstalled = true
		}
	}
	if out.K8sInstalled {
		out.K8sReady = k8s.ClusterReady()
	}

	if s.dashboard != nil {
		mon := s.dashboard.GetMonitor(0.1)
		if mon.Current != nil {
			out.CPU = mon.Current.CPU.UsagePercent
			out.Memory = mon.Current.Memory.UsedPercent
			out.Disk = maxDiskUsage(mon.Current.Disk)
		}
	}
	return out, nil
}

type logViewConfig struct {
	RetentionDays int  `json:"retention_days"`
	AutoCleanup   bool `json:"auto_cleanup"`
}

func (out *OverviewResponse) loadLogRetention(dataDir string) {
	if dataDir == "" {
		return
	}
	path := filepath.Join(dataDir, "logs-view.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var cfg logViewConfig
	if json.Unmarshal(data, &cfg) == nil {
		out.LogAutoCleanup = cfg.AutoCleanup
		out.LogRetentionDays = cfg.RetentionDays
	}
}

type EventFilter struct {
	AppKey    string
	EventType string
	Limit     int
	Offset    int
}

func (s *Service) ListEventsFiltered(f EventFilter) ([]models.AutoOpsEvent, error) {
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	q := s.db.Order("created_at desc")
	if f.AppKey != "" {
		q = q.Where("app_key = ?", f.AppKey)
	}
	if f.EventType != "" {
		q = q.Where("event_type = ?", f.EventType)
	}
	var list []models.AutoOpsEvent
	return list, q.Offset(offset).Limit(limit).Find(&list).Error
}
