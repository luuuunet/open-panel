package autops

import (
	"strconv"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/dashboard"
	"github.com/luuuunet/owpanel/internal/services/notify"
)

func (s *Service) loadNotifyConfig(cfg Config) Config {
	all, err := s.settings.GetAll()
	if err != nil {
		return cfg
	}
	cfg.NotifyWebhook = strings.TrimSpace(all["auto_ops_notify_webhook"])
	cfg.NotifyOnDown = all["auto_ops_notify_on_down"] != "false"
	cfg.NotifyOnFail = all["auto_ops_notify_on_fail"] != "false"
	cfg.ResourceEnabled = all["auto_ops_resource_enabled"] == "true"
	if v, _ := strconv.Atoi(all["auto_ops_cpu_threshold"]); v >= 50 && v <= 100 {
		cfg.CPUThreshold = v
	} else if cfg.CPUThreshold == 0 {
		cfg.CPUThreshold = 90
	}
	if v, _ := strconv.Atoi(all["auto_ops_mem_threshold"]); v >= 50 && v <= 100 {
		cfg.MemThreshold = v
	} else if cfg.MemThreshold == 0 {
		cfg.MemThreshold = 90
	}
	if v, _ := strconv.Atoi(all["auto_ops_disk_threshold"]); v >= 50 && v <= 100 {
		cfg.DiskThreshold = v
	} else if cfg.DiskThreshold == 0 {
		cfg.DiskThreshold = 90
	}
	return cfg
}

func (s *Service) saveNotifyConfig(patch Config, data map[string]string) {
	data["auto_ops_notify_webhook"] = strings.TrimSpace(patch.NotifyWebhook)
	if patch.NotifyOnDown {
		data["auto_ops_notify_on_down"] = "true"
	} else {
		data["auto_ops_notify_on_down"] = "false"
	}
	if patch.NotifyOnFail {
		data["auto_ops_notify_on_fail"] = "true"
	} else {
		data["auto_ops_notify_on_fail"] = "false"
	}
	if patch.ResourceEnabled {
		data["auto_ops_resource_enabled"] = "true"
	} else {
		data["auto_ops_resource_enabled"] = "false"
	}
	cpu := patch.CPUThreshold
	if cpu < 50 {
		cpu = 90
	}
	mem := patch.MemThreshold
	if mem < 50 {
		mem = 90
	}
	disk := patch.DiskThreshold
	if disk < 50 {
		disk = 90
	}
	data["auto_ops_cpu_threshold"] = strconv.Itoa(cpu)
	data["auto_ops_mem_threshold"] = strconv.Itoa(mem)
	data["auto_ops_disk_threshold"] = strconv.Itoa(disk)
}

func (s *Service) maybeNotify(cfg Config, app models.App, eventType, message, status string) {
	if cfg.NotifyWebhook == "" {
		return
	}
	switch eventType {
	case "down_detected":
		if !cfg.NotifyOnDown {
			return
		}
	case "restart_fail", "restart_skipped":
		if !cfg.NotifyOnFail {
			return
		}
	case "resource_cpu", "resource_memory", "resource_disk", "cron_failed",
		"ssl_expiring", "site_expiring", "ssl_renew_fail":
		// notify when webhook configured
	default:
		return
	}
	notify.PostJSON(cfg.NotifyWebhook, map[string]interface{}{
		"event":      eventType,
		"source":     "auto_ops",
		"app_key":    app.Key,
		"app_name":   app.Name,
		"message":    message,
		"status":     status,
		"timestamp":  time.Now().Format(time.RFC3339),
	})
}

func (s *Service) checkResources(cfg Config, now time.Time) {
	if !cfg.ResourceEnabled || s.dashboard == nil {
		return
	}
	cooldown := time.Duration(cfg.CooldownSec) * time.Second
	mon := s.dashboard.GetMonitor(0.1)
	if mon.Current == nil {
		return
	}
	cur := mon.Current

	if cfg.CPUThreshold > 0 && cur.CPU.UsagePercent >= float64(cfg.CPUThreshold) {
		if !s.inGlobalEventCooldown("resource_cpu", now, cooldown) {
			msg := "CPU 使用率 " + formatPct(cur.CPU.UsagePercent) + "，超过阈值 " + strconv.Itoa(cfg.CPUThreshold) + "%"
			s.logGlobalEvent("resource_cpu", "system", "系统", msg, formatPct(cur.CPU.UsagePercent))
			s.maybeNotify(cfg, models.App{Key: "system", Name: "系统"}, "resource_cpu", msg, formatPct(cur.CPU.UsagePercent))
		}
	}
	if cfg.MemThreshold > 0 && cur.Memory.UsedPercent >= float64(cfg.MemThreshold) {
		if !s.inGlobalEventCooldown("resource_memory", now, cooldown) {
			msg := "内存使用率 " + formatPct(cur.Memory.UsedPercent) + "，超过阈值 " + strconv.Itoa(cfg.MemThreshold) + "%"
			s.logGlobalEvent("resource_memory", "system", "系统", msg, formatPct(cur.Memory.UsedPercent))
			s.maybeNotify(cfg, models.App{Key: "system", Name: "系统"}, "resource_memory", msg, formatPct(cur.Memory.UsedPercent))
		}
	}
	maxDisk := maxDiskUsage(cur.Disk)
	if cfg.DiskThreshold > 0 && maxDisk >= float64(cfg.DiskThreshold) {
		if !s.inGlobalEventCooldown("resource_disk", now, cooldown) {
			msg := "磁盘使用率 " + formatPct(maxDisk) + "，超过阈值 " + strconv.Itoa(cfg.DiskThreshold) + "%"
			s.logGlobalEvent("resource_disk", "system", "系统", msg, formatPct(maxDisk))
			s.maybeNotify(cfg, models.App{Key: "system", Name: "系统"}, "resource_disk", msg, formatPct(maxDisk))
		}
	}
}

func maxDiskUsage(disks []dashboard.DiskStats) float64 {
	var max float64
	for _, d := range disks {
		if d.UsedPercent > max {
			max = d.UsedPercent
		}
	}
	return max
}

func formatPct(v float64) string {
	return strconv.FormatFloat(v, 'f', 1, 64) + "%"
}

func (s *Service) inGlobalEventCooldown(eventType string, now time.Time, cooldown time.Duration) bool {
	var last models.AutoOpsEvent
	err := s.db.Where("app_key = ? AND event_type = ?", "system", eventType).
		Order("created_at desc").First(&last).Error
	if err != nil {
		return false
	}
	return now.Sub(last.CreatedAt) < cooldown
}

func (s *Service) logGlobalEvent(eventType, appKey, appName, message, status string) {
	_ = s.db.Create(&models.AutoOpsEvent{
		AppKey: appKey, AppName: appName,
		EventType: eventType, Message: message, Status: status,
	}).Error
}

// NotifyWebhookURL returns the configured auto-ops webhook for other services (e.g. cron).
func (s *Service) NotifyWebhookURL() string {
	all, err := s.settings.GetAll()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(all["auto_ops_notify_webhook"])
}

// NotifyExternal posts a structured auto-ops style webhook event.
func (s *Service) NotifyExternal(eventType, appKey, appName, message, status string) {
	url := s.NotifyWebhookURL()
	if url == "" {
		return
	}
	notify.PostJSON(url, map[string]interface{}{
		"event":     eventType,
		"source":    "auto_ops",
		"app_key":   appKey,
		"app_name":  appName,
		"message":   message,
		"status":    status,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// LogCronFailure records a cron job failure in auto-ops events and sends webhook if configured.
func (s *Service) LogCronFailure(jobName, message string, jobID uint) {
	key := "cron"
	if jobID > 0 {
		key = "cron:" + strconv.FormatUint(uint64(jobID), 10)
	}
	s.logEvent(models.App{Key: key, Name: jobName}, "cron_failed", message, "failed")
}
