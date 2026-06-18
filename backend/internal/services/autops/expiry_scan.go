package autops

import (
	"fmt"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

func (s *Service) loadExpiryConfig(cfg Config) Config {
	all, _ := s.settings.GetAll()
	if v, ok := all["auto_ops_ssl_auto_renew"]; ok {
		cfg.SSLAutoRenew = v != "false"
	} else {
		cfg.SSLAutoRenew = true
	}
	if v, _ := parseInt(all["auto_ops_alert_days_ssl"], 14); v > 0 {
		cfg.AlertDaysSSL = v
	}
	if v, _ := parseInt(all["auto_ops_alert_days_site"], 14); v > 0 {
		cfg.AlertDaysSite = v
	}
	return cfg
}

func parseInt(s string, def int) (int, bool) {
	if s == "" {
		return def, false
	}
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return def, false
	}
	return n, true
}

func (s *Service) saveExpiryConfig(patch Config, data map[string]string) {
	if patch.SSLAutoRenew {
		data["auto_ops_ssl_auto_renew"] = "true"
	} else {
		data["auto_ops_ssl_auto_renew"] = "false"
	}
	days := patch.AlertDaysSSL
	if days < 1 {
		days = 14
	}
	data["auto_ops_alert_days_ssl"] = fmt.Sprintf("%d", days)
	days = patch.AlertDaysSite
	if days < 1 {
		days = 14
	}
	data["auto_ops_alert_days_site"] = fmt.Sprintf("%d", days)
}

// ScanExpiryAlerts checks SSL certs and websites nearing expiry and emits events/webhooks.
func (s *Service) ScanExpiryAlerts() {
	cfg := s.loadExpiryConfig(s.loadConfig())
	now := time.Now()
	cooldown := time.Duration(cfg.CooldownSec) * time.Second
	if cooldown < time.Hour {
		cooldown = time.Hour
	}

	deadline := now.AddDate(0, 0, cfg.AlertDaysSSL)
	var certs []models.SSLCertificate
	if err := s.db.Where("expires_at IS NOT NULL AND expires_at > ? AND expires_at <= ?", now, deadline).Find(&certs).Error; err == nil {
		for _, c := range certs {
			if c.ExpiresAt == nil {
				continue
			}
			days := int(c.ExpiresAt.Sub(now).Hours() / 24)
			key := "ssl:" + c.Domain
			if s.inEventCooldown(key, []string{"ssl_expiring"}, now, cooldown) {
				continue
			}
			msg := fmt.Sprintf("证书 %s 将在 %d 天后到期", c.Domain, days)
			s.logEvent(models.App{Key: key, Name: c.Domain}, "ssl_expiring", msg, fmt.Sprintf("%d", days))
		}
	}

	siteDeadline := now.AddDate(0, 0, cfg.AlertDaysSite)
	var sites []models.Website
	if err := s.db.Where("expires_at IS NOT NULL AND expires_at > ? AND expires_at <= ? AND status = ?", now, siteDeadline, "running").Find(&sites).Error; err == nil {
		for _, site := range sites {
			if site.ExpiresAt == nil {
				continue
			}
			days := int(site.ExpiresAt.Sub(now).Hours() / 24)
			key := fmt.Sprintf("site:%d", site.ID)
			if s.inEventCooldown(key, []string{"site_expiring"}, now, cooldown) {
				continue
			}
			msg := fmt.Sprintf("网站 %s 将在 %d 天后到期", site.Domain, days)
			s.logEvent(models.App{Key: key, Name: site.Domain}, "site_expiring", msg, fmt.Sprintf("%d", days))
		}
	}
}

// RunSSLAutoRenew renews certificates with auto_renew enabled; logs failures.
func (s *Service) RunSSLAutoRenew(renew func() (int, []string, error)) {
	if renew == nil {
		return
	}
	cfg := s.loadExpiryConfig(s.loadConfig())
	if !cfg.SSLAutoRenew {
		return
	}
	n, failed, err := renew()
	if err != nil {
		s.logEvent(models.App{Key: "ssl", Name: "SSL"}, "ssl_renew_fail", err.Error(), "failed")
		return
	}
	for _, f := range failed {
		s.logEvent(models.App{Key: "ssl", Name: "SSL"}, "ssl_renew_fail", f, "failed")
	}
	if n > 0 {
		s.logEvent(models.App{Key: "ssl", Name: "SSL"}, "ssl_renew_ok", fmt.Sprintf("已续期 %d 个证书", n), "success")
	}
}
