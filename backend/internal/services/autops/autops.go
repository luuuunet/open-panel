package autops

import (
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/luuuunet/owpanel/internal/bootstrap"
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"github.com/luuuunet/owpanel/internal/services/dashboard"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"github.com/luuuunet/owpanel/internal/services/website"
	"gorm.io/gorm"
)

type Service struct {
	db        *gorm.DB
	settings  *settings.Service
	apps      *appstore.Service
	dashboard *dashboard.Service
	website   *website.Service
	dataDir   string

	mu         sync.Mutex
	lastScan   time.Time
	auditStore *websiteAuditStore
}

func NewService(db *gorm.DB, settingsSvc *settings.Service, apps *appstore.Service, dash *dashboard.Service, websiteSvc *website.Service, dataDir string) *Service {
	return &Service{
		db: db, settings: settingsSvc, apps: apps, dashboard: dash, website: websiteSvc,
		dataDir: dataDir, auditStore: newWebsiteAuditStore(),
	}
}

type Config struct {
	Enabled         bool   `json:"enabled"`
	IntervalSec     int    `json:"interval_sec"`
	CooldownSec     int    `json:"cooldown_sec"`
	MaxRestarts     int    `json:"max_restarts"`
	NotifyWebhook   string `json:"notify_webhook"`
	NotifyOnDown    bool   `json:"notify_on_down"`
	NotifyOnFail    bool   `json:"notify_on_fail"`
	ResourceEnabled bool   `json:"resource_enabled"`
	CPUThreshold    int    `json:"cpu_threshold"`
	MemThreshold    int    `json:"mem_threshold"`
	MemAutoRelief   bool   `json:"mem_auto_relief"`
	DiskThreshold   int    `json:"disk_threshold"`
	SSLAutoRenew    bool   `json:"ssl_auto_renew"`
	AlertDaysSSL    int    `json:"alert_days_ssl"`
	AlertDaysSite   int    `json:"alert_days_site"`
	WebsiteScanEnabled bool `json:"website_scan_enabled"`
}

type WatchItem struct {
	Key          string    `json:"key"`
	Name         string    `json:"name"`
	Category     string    `json:"category"`
	Installed    bool      `json:"installed"`
	WatchEnabled bool      `json:"watch_enabled"`
	AutoRestart  bool      `json:"auto_restart"`
	LiveStatus   string    `json:"live_status"`
	DBStatus     string    `json:"db_status"`
	LastEvent    string    `json:"last_event,omitempty"`
	LastEventAt  time.Time `json:"last_event_at,omitempty"`
}

type StatusResponse struct {
	Config    Config      `json:"config"`
	Watches   []WatchItem `json:"watches"`
	LastScan  time.Time   `json:"last_scan"`
	WatchCount int        `json:"watch_count"`
	DownCount  int        `json:"down_count"`
}

func (s *Service) Start() {
	go func() {
		for {
			cfg := s.loadConfig()
			sleep := time.Duration(cfg.IntervalSec) * time.Second
			if sleep < 10*time.Second {
				sleep = 10 * time.Second
			}
			if cfg.Enabled {
				s.ensureK3sWatch()
				s.apps.SyncInstalledStatuses()
				s.runCheck(false)
			} else {
				s.checkMemoryPressure(cfg, time.Now())
			}
			time.Sleep(sleep)
		}
	}()
}

func bootstrapSmallMachine() bool {
	return bootstrap.SmallMachine()
}

func (s *Service) loadConfig() Config {
	cfg := Config{
		Enabled: true, IntervalSec: 30, CooldownSec: 300, MaxRestarts: 5,
		NotifyOnDown: true, NotifyOnFail: true,
		CPUThreshold: 90, MemThreshold: 90, MemAutoRelief: true, DiskThreshold: 90,
		SSLAutoRenew: true, AlertDaysSSL: 14, AlertDaysSite: 14,
		WebsiteScanEnabled: true,
	}
	all, err := s.settings.GetAll()
	if err != nil {
		return cfg
	}
	cfg.Enabled = all["auto_ops_enabled"] != "false"
	if v, _ := strconv.Atoi(all["auto_ops_interval"]); v >= 10 {
		cfg.IntervalSec = v
	}
	if v, _ := strconv.Atoi(all["auto_ops_cooldown"]); v >= 60 {
		cfg.CooldownSec = v
	}
	if v, _ := strconv.Atoi(all["auto_ops_max_restarts"]); v >= 1 {
		cfg.MaxRestarts = v
	}
	return s.loadExpiryConfig(s.loadWebsiteScanConfig(s.loadNotifyConfig(cfg)))
}

func (s *Service) UpdateConfig(patch Config) error {
	data := map[string]string{
		"auto_ops_enabled":      "false",
		"auto_ops_interval":     strconv.Itoa(patch.IntervalSec),
		"auto_ops_cooldown":     strconv.Itoa(patch.CooldownSec),
		"auto_ops_max_restarts": strconv.Itoa(patch.MaxRestarts),
	}
	if patch.Enabled {
		data["auto_ops_enabled"] = "true"
	}
	if patch.IntervalSec < 10 {
		patch.IntervalSec = 10
		data["auto_ops_interval"] = "10"
	}
	if patch.CooldownSec < 60 {
		patch.CooldownSec = 60
		data["auto_ops_cooldown"] = "60"
	}
	if patch.MaxRestarts < 1 {
		patch.MaxRestarts = 1
		data["auto_ops_max_restarts"] = "1"
	}
	s.saveNotifyConfig(patch, data)
	s.saveExpiryConfig(patch, data)
	s.saveWebsiteScanConfig(patch, data)
	return s.settings.Update(data)
}

func (s *Service) GetStatus() (*StatusResponse, error) {
	s.ensureK3sWatch()
	s.apps.SyncInstalledStatuses()
	cfg := s.loadConfig()
	apps, err := s.apps.ListInstalled()
	if err != nil {
		return nil, err
	}
	items := make([]WatchItem, 0, len(apps))
	watchCount, downCount := 0, 0
	for _, app := range apps {
		if app.Status == "simulated" {
			continue
		}
		live := s.apps.LiveStatus(app.Key)
		item := WatchItem{
			Key: app.Key, Name: app.Name, Category: app.Category,
			Installed: app.Installed, WatchEnabled: app.WatchEnabled,
			AutoRestart: app.AutoRestart, LiveStatus: live, DBStatus: app.Status,
		}
		var ev models.AutoOpsEvent
		if err := s.db.Where("app_key = ?", app.Key).Order("created_at desc").First(&ev).Error; err == nil {
			item.LastEvent = ev.EventType
			item.LastEventAt = ev.CreatedAt
		}
		if app.WatchEnabled {
			watchCount++
			if live != "running" {
				downCount++
			}
		}
		items = append(items, item)
	}
	s.mu.Lock()
	last := s.lastScan
	s.mu.Unlock()
	return &StatusResponse{
		Config: cfg, Watches: items, LastScan: last,
		WatchCount: watchCount, DownCount: downCount,
	}, nil
}

func (s *Service) UpdateWatch(key string, watchEnabled, autoRestart *bool) error {
	app, err := s.apps.Get(key)
	if err != nil {
		return err
	}
	updates := map[string]interface{}{}
	if watchEnabled != nil {
		updates["watch_enabled"] = *watchEnabled
	}
	if autoRestart != nil {
		updates["auto_restart"] = *autoRestart
		if *autoRestart {
			updates["watch_enabled"] = true
		}
	}
	if watchEnabled != nil && !*watchEnabled {
		updates["auto_restart"] = false
	}
	if len(updates) == 0 {
		return nil
	}
	return s.db.Model(app).Updates(updates).Error
}

func (s *Service) BulkUpdateWatch(keys []string, watchEnabled, autoRestart bool) error {
	if len(keys) == 0 {
		return nil
	}
	if !watchEnabled {
		autoRestart = false
	}
	return s.db.Model(&models.App{}).Where("app_key IN ?", keys).Updates(map[string]interface{}{
		"watch_enabled": watchEnabled,
		"auto_restart":  autoRestart,
	}).Error
}

func (s *Service) ScanNow() error {
	s.runCheck(true)
	s.ScanWebsiteAudits(true)
	return nil
}

func (s *Service) ListEvents(limit int) ([]models.AutoOpsEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var list []models.AutoOpsEvent
	return list, s.db.Order("created_at desc").Limit(limit).Find(&list).Error
}

func (s *Service) runCheck(manual bool) {
	s.mu.Lock()
	s.lastScan = time.Now()
	s.mu.Unlock()

	s.apps.SyncInstalledStatuses()

	cfg := s.loadConfig()

	apps, err := s.apps.ListInstalled()
	if err != nil {
		return
	}
	now := time.Now()
	s.checkMemoryPressure(cfg, now)
	if cfg.Enabled || manual {
		s.checkResources(cfg, now)
	}
	if !cfg.Enabled && !manual {
		return
	}
	cooldown := time.Duration(cfg.CooldownSec) * time.Second

	for _, app := range apps {
		if !app.Installed || !app.WatchEnabled {
			continue
		}
		if app.Status == "simulated" {
			continue
		}
		live := s.apps.LiveStatus(app.Key)
		if live == "running" {
			continue
		}

		if !app.AutoRestart {
			if s.inEventCooldown(app.Key, []string{"down_detected"}, now, cooldown) {
				continue
			}
			s.logEvent(app, "down_detected", "检测到服务未运行（仅监控，未重启）", live)
			continue
		}

		if s.inEventCooldown(app.Key, []string{"restart_ok", "restart_fail"}, now, cooldown) {
			continue
		}
		if s.restartsInLastHour(app.Key, now) >= cfg.MaxRestarts {
			if !s.inEventCooldown(app.Key, []string{"restart_skipped"}, now, cooldown) {
				s.logEvent(app, "restart_skipped", "已达每小时最大重启次数", live)
			}
			continue
		}

		s.logEvent(app, "down_detected", "检测到服务未运行", live)
		if err := s.apps.ServiceAction(app.Key, "restart"); err != nil {
			s.logEvent(app, "restart_fail", err.Error(), live)
			log.Printf("[autops] restart %s failed: %v", app.Key, err)
			continue
		}
		newStatus := s.apps.LiveStatus(app.Key)
		_ = s.db.Model(&app).Update("status", newStatus).Error
		if newStatus == "running" {
			s.logEvent(app, "restart_ok", "已自动重启", newStatus)
		} else {
			s.logEvent(app, "restart_fail", "重启后仍未运行", newStatus)
		}
	}
}

func (s *Service) inEventCooldown(appKey string, eventTypes []string, now time.Time, cooldown time.Duration) bool {
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

func (s *Service) restartsInLastHour(appKey string, now time.Time) int {
	var count int64
	hourAgo := now.Add(-time.Hour)
	s.db.Model(&models.AutoOpsEvent{}).
		Where("app_key = ? AND event_type = ? AND created_at > ?", appKey, "restart_ok", hourAgo).
		Count(&count)
	return int(count)
}

func (s *Service) logEvent(app models.App, eventType, message, status string) {
	_ = s.db.Create(&models.AutoOpsEvent{
		AppKey: app.Key, AppName: app.Name,
		EventType: eventType, Message: message, Status: status,
	}).Error
	cfg := s.loadConfig()
	s.maybeNotify(cfg, app, eventType, message, status)
}

func (s *Service) ensureK3sWatch() {
	if bootstrapSmallMachine() {
		return
	}
	app, err := s.apps.Get("k3s")
	if err != nil || !app.Installed {
		return
	}
	if app.WatchEnabled && app.AutoRestart {
		return
	}
	_ = s.db.Model(app).Updates(map[string]interface{}{
		"watch_enabled": true,
		"auto_restart":  true,
	}).Error
}
