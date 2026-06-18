package enterprise

import (
	"fmt"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

type ComplianceCheck struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type ComplianceReport struct {
	Score   int               `json:"score"`
	Grade   string            `json:"grade"`
	Checks  []ComplianceCheck `json:"checks"`
	Summary string            `json:"summary"`
}

func (s *Service) RunComplianceChecks() ComplianceReport {
	checks := []ComplianceCheck{}
	pass, warn, fail := 0, 0, 0

	add := func(key, name, status, detail string) {
		checks = append(checks, ComplianceCheck{Key: key, Name: name, Status: status, Detail: detail})
		switch status {
		case "pass":
			pass++
		case "warn":
			warn++
		case "fail":
			fail++
		}
	}

	all, _ := s.settings.GetAll()
	if all["panel_ip_whitelist_enabled"] == "true" {
		add("panel_ip_whitelist", "面板 IP 白名单", "pass", "已启用")
	} else {
		add("panel_ip_whitelist", "面板 IP 白名单", "warn", "未启用，建议限制管理入口 IP")
	}

	if all["password_require_strong"] != "false" {
		add("strong_password", "强密码策略", "pass", "已启用")
	} else {
		add("strong_password", "强密码策略", "warn", "仅最小长度要求")
	}

	since := time.Now().Add(-24 * time.Hour)
	var failLogins int64
	s.db.Model(&models.LoginEvent{}).Where("success = ? AND created_at >= ?", false, since).Count(&failLogins)
	if failLogins == 0 {
		add("login_failures_24h", "24h 登录失败", "pass", "无失败记录")
	} else if failLogins < 10 {
		add("login_failures_24h", "24h 登录失败", "warn", fmt.Sprintf("%d 次失败尝试", failLogins))
	} else {
		add("login_failures_24h", "24h 登录失败", "fail", fmt.Sprintf("%d 次失败尝试，可能存在暴力破解", failLogins))
	}

	if s.AuditLoggingEnabled() {
		cfg := s.GetAuditSettings()
		add("audit_logging", "审计日志", "pass", fmt.Sprintf("已启用，保留 %d 天", cfg.RetentionDays))
	} else {
		add("audit_logging", "审计日志", "warn", "尚无审计记录，请确认 retention 配置")
	}

	var expiring int64
	deadline := time.Now().AddDate(0, 0, 30)
	s.db.Model(&models.SSLCertificate{}).Where("expires_at IS NOT NULL AND expires_at < ? AND status = ?", deadline, "issued").Count(&expiring)
	if expiring == 0 {
		add("ssl_expiry", "SSL 证书有效期", "pass", "30 天内无即将过期证书")
	} else {
		add("ssl_expiry", "SSL 证书有效期", "warn", fmt.Sprintf("%d 张证书将在 30 天内过期", expiring))
	}

	var offline int64
	s.db.Model(&models.ClusterNode{}).Where("status != ? AND is_local = ?", "online", false).Count(&offline)
	if offline == 0 {
		add("cluster_nodes", "集群节点状态", "pass", "所有远程节点在线")
	} else {
		add("cluster_nodes", "集群节点状态", "fail", fmt.Sprintf("%d 个节点离线", offline))
	}

	var downMonitors int64
	s.db.Model(&models.UptimeMonitor{}).Where("enabled = ? AND last_status = ?", true, "down").Count(&downMonitors)
	if downMonitors == 0 {
		add("uptime_monitors", "可用性监控", "pass", "无宕机告警")
	} else {
		add("uptime_monitors", "可用性监控", "fail", fmt.Sprintf("%d 个监控项处于 down 状态", downMonitors))
	}

	total := pass + warn + fail
	score := 0
	if total > 0 {
		score = (pass*100 + warn*50) / total
	}
	grade := "F"
	switch {
	case score >= 90:
		grade = "A"
	case score >= 80:
		grade = "B"
	case score >= 70:
		grade = "C"
	case score >= 60:
		grade = "D"
	}
	summary := fmt.Sprintf("通过 %d / 警告 %d / 失败 %d", pass, warn, fail)
	return ComplianceReport{Score: score, Grade: grade, Checks: checks, Summary: summary}
}
