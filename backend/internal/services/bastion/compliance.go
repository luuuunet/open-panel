package bastion

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

type ComplianceExportInput struct {
	From  time.Time `json:"from"`
	To    time.Time `json:"to"`
	Types []string  `json:"types"`
}

type ComplianceScoreFactor struct {
	Key    string  `json:"key"`
	Name   string  `json:"name"`
	Score  float64 `json:"score"`
	Max    float64 `json:"max"`
	Detail string  `json:"detail"`
}

type ComplianceScoreReport struct {
	Score   float64                 `json:"score"`
	Grade   string                  `json:"grade"`
	Factors []ComplianceScoreFactor `json:"factors"`
}

func (s *Service) complianceDir() string {
	return filepath.Join(s.dataDir, "bastion", "compliance")
}

func (s *Service) ExportCompliance(in ComplianceExportInput) (string, error) {
	if in.From.IsZero() {
		in.From = time.Now().AddDate(0, -1, 0)
	}
	if in.To.IsZero() {
		in.To = time.Now()
	}
	types := in.Types
	if len(types) == 0 {
		types = []string{"login_events", "bastion_sessions", "command_audits", "rotation_logs", "access_requests"}
	}
	typeSet := map[string]bool{}
	for _, t := range types {
		typeSet[strings.TrimSpace(t)] = true
	}
	_ = os.MkdirAll(s.complianceDir(), 0755)
	name := fmt.Sprintf("compliance-%s.zip", time.Now().Format("20060102-150405"))
	zipPath := filepath.Join(s.complianceDir(), name)
	f, err := os.Create(zipPath)
	if err != nil {
		return "", err
	}
	zw := zip.NewWriter(f)

	writeJSON := func(filename string, v interface{}) error {
		w, err := zw.Create(filename)
		if err != nil {
			return err
		}
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	}

	if typeSet["login_events"] {
		var events []models.LoginEvent
		s.db.Where("created_at >= ? AND created_at <= ?", in.From, in.To).Order("created_at asc").Find(&events)
		if err := writeJSON("login_events.json", events); err != nil {
			zw.Close()
			f.Close()
			return "", err
		}
	}
	if typeSet["bastion_sessions"] {
		var sessions []models.BastionSession
		s.db.Where("start_time >= ? AND start_time <= ?", in.From, in.To).Order("start_time asc").Find(&sessions)
		summary := make([]map[string]interface{}, 0, len(sessions))
		for _, sess := range sessions {
			summary = append(summary, map[string]interface{}{
				"id": sess.ID, "username": sess.Username, "asset_name": sess.AssetName,
				"host": sess.Host, "start_time": sess.StartTime, "end_time": sess.EndTime,
				"status": sess.Status, "log_size": sess.LogSize,
			})
		}
		if err := writeJSON("bastion_sessions.json", summary); err != nil {
			zw.Close()
			f.Close()
			return "", err
		}
	}
	if typeSet["command_audits"] {
		var audits []models.BastionCommandAudit
		s.db.Where("created_at >= ? AND created_at <= ?", in.From, in.To).Order("created_at asc").Find(&audits)
		if err := writeJSON("command_audits.json", audits); err != nil {
			zw.Close()
			f.Close()
			return "", err
		}
	}
	if typeSet["rotation_logs"] {
		var logs []models.BastionAccountRotationLog
		s.db.Where("rotated_at >= ? AND rotated_at <= ?", in.From, in.To).Order("rotated_at asc").Find(&logs)
		if err := writeJSON("rotation_logs.json", logs); err != nil {
			zw.Close()
			f.Close()
			return "", err
		}
	}
	if typeSet["access_requests"] {
		list, err := s.ListAccessRequestsForExport(in.From, in.To)
		if err != nil {
			zw.Close()
			f.Close()
			return "", err
		}
		if err := writeJSON("access_requests.json", list); err != nil {
			zw.Close()
			f.Close()
			return "", err
		}
	}

	if err := zw.Close(); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return name, nil
}

func (s *Service) ComplianceExportPath(filename string) (string, error) {
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return "", fmt.Errorf("invalid filename")
	}
	path := filepath.Join(s.complianceDir(), filename)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("文件不存在")
	}
	return path, nil
}

func (s *Service) ComputeComplianceScore() ComplianceScoreReport {
	factors := []ComplianceScoreFactor{}
	total, maxTotal := 0.0, 0.0

	add := func(key, name, detail string, score, max float64) {
		factors = append(factors, ComplianceScoreFactor{Key: key, Name: name, Score: score, Max: max, Detail: detail})
		total += score
		maxTotal += max
	}

	var userCount, totpCount int64
	s.db.Model(&models.User{}).Count(&userCount)
	s.db.Model(&models.User{}).Where("totp_enabled = ?", true).Count(&totpCount)
	totpPct := 0.0
	if userCount > 0 {
		totpPct = float64(totpCount) / float64(userCount)
	}
	add("totp_adoption", "2FA 启用率", fmt.Sprintf("%d/%d 用户", totpCount, userCount), totpPct*20, 20)

	var rotAccounts, totalAccounts int64
	s.db.Model(&models.BastionAccount{}).Where("auto_rotate = ? OR rotate_after_session = ?", true, true).Count(&rotAccounts)
	s.db.Model(&models.BastionAccount{}).Count(&totalAccounts)
	rotPct := 0.0
	if totalAccounts > 0 {
		rotPct = float64(rotAccounts) / float64(totalAccounts)
	}
	add("rotation_policy", "改密策略覆盖", fmt.Sprintf("%d/%d 账号", rotAccounts, totalAccounts), rotPct*20, 20)

	jit, standing := s.CountJITPermissions()
	jitScore := 20.0
	if standing > 0 && jit == 0 {
		jitScore = 5
	} else if jit > 0 {
		jitScore = 20
	}
	add("jit_access", "JIT 临时授权", fmt.Sprintf("JIT=%d 永久=%d", jit, standing), jitScore, 20)

	var recorded, totalSess int64
	s.db.Model(&models.BastionSession{}).Where("log_size > 0").Count(&recorded)
	s.db.Model(&models.BastionSession{}).Count(&totalSess)
	recPct := 0.0
	if totalSess > 0 {
		recPct = float64(recorded) / float64(totalSess)
	}
	add("session_recording", "会话录制率", fmt.Sprintf("%.0f%%", recPct*100), recPct*20, 20)

	policy := s.LoadCommandPolicy()
	cmdScore := 10.0
	if policy.Mode == "block" && len(policy.Blocklist) > 0 {
		cmdScore = 20
	}
	add("command_policy", "命令策略", policy.Mode, cmdScore, 20)

	score := 0.0
	if maxTotal > 0 {
		score = total / maxTotal * 100
	}
	grade := "D"
	switch {
	case score >= 90:
		grade = "A"
	case score >= 75:
		grade = "B"
	case score >= 60:
		grade = "C"
	}
	return ComplianceScoreReport{Score: score, Grade: grade, Factors: factors}
}
