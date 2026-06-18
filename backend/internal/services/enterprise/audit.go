package enterprise

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

type AuditRecordInput struct {
	UserID    uint
	Username  string
	IP        string
	UserAgent string
	Category  string
	Action    string
	Resource  string
	Detail    string
	Level     string
	Success   bool
}

type AuditFilters struct {
	Category string
	Action   string
	Level    string
	Username string
	Success  *bool
	From     time.Time
	To       time.Time
}

type AuditListResult struct {
	Items []models.PanelAuditEvent `json:"items"`
	Total int64                    `json:"total"`
}

type AuditStats struct {
	Total24h    int64            `json:"total_24h"`
	ByCategory  map[string]int64 `json:"by_category"`
	ByLevel     map[string]int64 `json:"by_level"`
	Failed24h   int64            `json:"failed_24h"`
	Critical24h int64            `json:"critical_24h"`
}

type AuditSettings struct {
	RetentionDays   int  `json:"retention_days"`
	SyslogForward   bool `json:"syslog_forward"`
	SyslogEnabled   bool `json:"syslog_enabled"`
	SyslogHost      string `json:"syslog_host,omitempty"`
	SyslogPort      string `json:"syslog_port,omitempty"`
	SyslogProtocol  string `json:"syslog_protocol,omitempty"`
}

func (s *Service) Record(ctx context.Context, in AuditRecordInput) error {
	if in.Level == "" {
		if in.Success {
			in.Level = "info"
		} else {
			in.Level = "warn"
		}
	}
	if len(in.UserAgent) > 512 {
		in.UserAgent = in.UserAgent[:512]
	}
	ev := models.PanelAuditEvent{
		UserID: in.UserID, Username: in.Username, IP: in.IP, UserAgent: in.UserAgent,
		Category: in.Category, Action: in.Action, Resource: in.Resource,
		Detail: in.Detail, Level: in.Level, Success: in.Success,
	}
	if err := s.db.WithContext(ctx).Create(&ev).Error; err != nil {
		return err
	}
	if s.syslogForwardEnabled() && s.syslog != nil {
		msg := fmt.Sprintf("user=%s ip=%s category=%s action=%s resource=%s success=%v %s",
			in.Username, in.IP, in.Category, in.Action, in.Resource, in.Success, in.Detail)
		s.syslog.Emit("panel_audit_"+in.Action, msg)
	}
	return nil
}

func (s *Service) List(filters AuditFilters, limit, offset int) (AuditListResult, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := s.db.Model(&models.PanelAuditEvent{})
	if filters.Category != "" {
		q = q.Where("category = ?", filters.Category)
	}
	if filters.Action != "" {
		q = q.Where("action LIKE ?", "%"+filters.Action+"%")
	}
	if filters.Level != "" {
		q = q.Where("level = ?", filters.Level)
	}
	if filters.Username != "" {
		q = q.Where("username LIKE ?", "%"+filters.Username+"%")
	}
	if filters.Success != nil {
		q = q.Where("success = ?", *filters.Success)
	}
	if !filters.From.IsZero() {
		q = q.Where("created_at >= ?", filters.From)
	}
	if !filters.To.IsZero() {
		q = q.Where("created_at <= ?", filters.To)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return AuditListResult{}, err
	}
	var items []models.PanelAuditEvent
	err := q.Order("created_at desc").Limit(limit).Offset(offset).Find(&items).Error
	return AuditListResult{Items: items, Total: total}, err
}

func (s *Service) Stats(last24h bool) (AuditStats, error) {
	since := time.Time{}
	if last24h {
		since = time.Now().Add(-24 * time.Hour)
	}
	stats := AuditStats{
		ByCategory: map[string]int64{},
		ByLevel:    map[string]int64{},
	}
	q := s.db.Model(&models.PanelAuditEvent{})
	if !since.IsZero() {
		q = q.Where("created_at >= ?", since)
	}
	if err := q.Count(&stats.Total24h).Error; err != nil {
		return stats, err
	}
	type row struct {
		Key   string
		Count int64
	}
	var catRows []row
	cq := s.db.Model(&models.PanelAuditEvent{}).Select("category as key, count(*) as count").Group("category")
	if !since.IsZero() {
		cq = cq.Where("created_at >= ?", since)
	}
	_ = cq.Scan(&catRows)
	for _, r := range catRows {
		stats.ByCategory[r.Key] = r.Count
	}
	var lvlRows []row
	lq := s.db.Model(&models.PanelAuditEvent{}).Select("level as key, count(*) as count").Group("level")
	if !since.IsZero() {
		lq = lq.Where("created_at >= ?", since)
	}
	_ = lq.Scan(&lvlRows)
	for _, r := range lvlRows {
		stats.ByLevel[r.Key] = r.Count
	}
	fq := s.db.Model(&models.PanelAuditEvent{}).Where("success = ?", false)
	if !since.IsZero() {
		fq = fq.Where("created_at >= ?", since)
	}
	_ = fq.Count(&stats.Failed24h).Error
	cfq := s.db.Model(&models.PanelAuditEvent{}).Where("level = ?", "critical")
	if !since.IsZero() {
		cfq = cfq.Where("created_at >= ?", since)
	}
	_ = cfq.Count(&stats.Critical24h).Error
	return stats, nil
}

func (s *Service) Export(filters AuditFilters, format string) ([]byte, string, error) {
	res, err := s.List(filters, 10000, 0)
	if err != nil {
		return nil, "", err
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "csv" {
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		_ = w.Write([]string{"id", "created_at", "user_id", "username", "ip", "category", "action", "resource", "detail", "level", "success"})
		for _, e := range res.Items {
			_ = w.Write([]string{
				strconv.FormatUint(uint64(e.ID), 10),
				e.CreatedAt.Format(time.RFC3339),
				strconv.FormatUint(uint64(e.UserID), 10),
				e.Username, e.IP, e.Category, e.Action, e.Resource, e.Detail, e.Level,
				strconv.FormatBool(e.Success),
			})
		}
		w.Flush()
		name := fmt.Sprintf("panel-audit-%s.csv", time.Now().Format("20060102-150405"))
		return buf.Bytes(), name, w.Error()
	}
	b, err := json.MarshalIndent(res.Items, "", "  ")
	name := fmt.Sprintf("panel-audit-%s.json", time.Now().Format("20060102-150405"))
	return b, name, err
}

func (s *Service) Cleanup(olderThanDays int) (int64, error) {
	if olderThanDays <= 0 {
		olderThanDays = s.GetAuditSettings().RetentionDays
	}
	if olderThanDays <= 0 {
		olderThanDays = 90
	}
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)
	res := s.db.Where("created_at < ?", cutoff).Delete(&models.PanelAuditEvent{})
	return res.RowsAffected, res.Error
}

func (s *Service) GetAuditSettings() AuditSettings {
	cfg := AuditSettings{RetentionDays: 90}
	if s.settings == nil {
		return cfg
	}
	all, _ := s.settings.GetAll()
	if d := strings.TrimSpace(all["audit_retention_days"]); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 {
			cfg.RetentionDays = n
		}
	}
	cfg.SyslogForward = all["audit_syslog_forward"] == "true"
	cfg.SyslogEnabled = all["syslog_enabled"] == "true"
	cfg.SyslogHost = strings.TrimSpace(all["syslog_host"])
	cfg.SyslogPort = strings.TrimSpace(all["syslog_port"])
	cfg.SyslogProtocol = strings.TrimSpace(all["syslog_protocol"])
	return cfg
}

func (s *Service) UpdateAuditSettings(retentionDays int, syslogForward bool) error {
	data := map[string]string{
		"audit_retention_days": strconv.Itoa(retentionDays),
		"audit_syslog_forward": strconv.FormatBool(syslogForward),
	}
	return s.settings.Update(data)
}

func (s *Service) syslogForwardEnabled() bool {
	all, _ := s.settings.GetAll()
	return all["audit_syslog_forward"] == "true" || all["syslog_enabled"] == "true"
}

func (s *Service) StartRetentionJob() {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			_, _ = s.Cleanup(0)
		}
	}()
}

func (s *Service) CountSince(since time.Time) (int64, error) {
	var n int64
	err := s.db.Model(&models.PanelAuditEvent{}).Where("created_at >= ?", since).Count(&n).Error
	return n, err
}

func (s *Service) AuditLoggingEnabled() bool {
	var n int64
	s.db.Model(&models.PanelAuditEvent{}).Count(&n)
	if n > 0 {
		return true
	}
	cfg := s.GetAuditSettings()
	return cfg.RetentionDays > 0
}
