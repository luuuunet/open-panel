package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/scheduleutil"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"gorm.io/gorm"
)

type Service struct {
	db       *gorm.DB
	dataDir  string
	settings *settings.Service
}

type RuleRequest struct {
	Name       string `json:"name"`
	Preset     string `json:"preset"`
	PathGlob   string `json:"path_glob"`
	MaxAgeDays int    `json:"max_age_days"`
	MaxTotalMB int    `json:"max_total_mb"`
	Schedule   string `json:"schedule"`
	Enabled    bool   `json:"enabled"`
}

type RunResult struct {
	DeletedFiles int   `json:"deleted_files"`
	BytesFreed   int64 `json:"bytes_freed"`
	Message      string `json:"message"`
}

func NewService(db *gorm.DB, dataDir string, settingsSvc *settings.Service) *Service {
	return &Service{db: db, dataDir: dataDir, settings: settingsSvc}
}

func (s *Service) ListRules() ([]models.LocalCleanupRule, error) {
	var list []models.LocalCleanupRule
	return list, s.db.Order("id desc").Find(&list).Error
}

func (s *Service) CreateRule(req *RuleRequest) (*models.LocalCleanupRule, error) {
	rule := s.requestToModel(req)
	if err := s.db.Create(rule).Error; err != nil {
		return nil, err
	}
	return rule, nil
}

func (s *Service) UpdateRule(id uint, req *RuleRequest) (*models.LocalCleanupRule, error) {
	var rule models.LocalCleanupRule
	if err := s.db.First(&rule, id).Error; err != nil {
		return nil, err
	}
	updated := s.requestToModel(req)
	updated.ID = rule.ID
	if err := s.db.Save(updated).Error; err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *Service) DeleteRule(id uint) error {
	return s.db.Delete(&models.LocalCleanupRule{}, id).Error
}

func (s *Service) RunRuleByID(id uint) (*RunResult, error) {
	var rule models.LocalCleanupRule
	if err := s.db.First(&rule, id).Error; err != nil {
		return nil, ErrRuleNotFound
	}
	return s.RunRule(&rule)
}

var ErrRuleNotFound = errors.New("rule not found")

func (s *Service) requestToModel(req *RuleRequest) *models.LocalCleanupRule {
	if req.MaxAgeDays <= 0 {
		req.MaxAgeDays = 7
	}
	if req.Schedule == "" {
		req.Schedule = "0 5 * * *"
	}
	return &models.LocalCleanupRule{
		Name:       strings.TrimSpace(req.Name),
		Preset:     strings.TrimSpace(req.Preset),
		PathGlob:   strings.TrimSpace(req.PathGlob),
		MaxAgeDays: req.MaxAgeDays,
		MaxTotalMB: req.MaxTotalMB,
		Schedule:   req.Schedule,
		Enabled:    req.Enabled,
	}
}

func (s *Service) ListPresets() []map[string]string {
	return []map[string]string{
		{"key": "panel_logs", "name": "面板日志目录", "path": "logs"},
		{"key": "backup_staging", "name": "迁移临时目录", "path": "panel-migration/staging-*"},
		{"key": "panel_migration", "name": "旧迁移包（超龄）", "path": "panel-migration"},
	}
}

func (s *Service) RunDueRules() int {
	var rules []models.LocalCleanupRule
	if err := s.db.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		return 0
	}
	n := 0
	now := time.Now()
	for i := range rules {
		if !scheduleutil.DueNow(rules[i].Schedule, rules[i].LastRunAt, now) {
			continue
		}
		if _, err := s.RunRule(&rules[i]); err == nil {
			n++
		}
	}
	return n
}

func (s *Service) RunRule(rule *models.LocalCleanupRule) (*RunResult, error) {
	now := time.Now()
	s.db.Model(rule).Updates(map[string]interface{}{"last_status": "running"})
	result, err := s.runRuleInternal(rule)
	status := "success"
	msg := result.Message
	if err != nil {
		status = "failed"
		msg = err.Error()
	}
	t := now
	s.db.Model(rule).Updates(map[string]interface{}{
		"last_run_at": &t,
		"last_status": status,
		"last_result": msg,
	})
	return result, err
}

func (s *Service) runRuleInternal(rule *models.LocalCleanupRule) (*RunResult, error) {
	cutoff := time.Now().AddDate(0, 0, -rule.MaxAgeDays)
	basePaths, err := s.resolvePresetPaths(rule)
	if err != nil {
		return nil, err
	}
	res := &RunResult{}
	for _, base := range basePaths {
		_ = filepath.Walk(base, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info == nil || info.IsDir() {
				return nil
			}
			if !s.isAllowedPath(path) {
				return nil
			}
			if info.ModTime().After(cutoff) {
				return nil
			}
			if rule.Preset == "backup_staging" && !strings.Contains(filepath.Base(path), "staging-") {
				return nil
			}
			if rule.Preset == "panel_migration" && !strings.HasSuffix(strings.ToLower(path), ".tar.gz") {
				return nil
			}
			size := info.Size()
			if err := os.Remove(path); err != nil {
				return nil
			}
			res.DeletedFiles++
			res.BytesFreed += size
			return nil
		})
	}
	res.Message = fmt.Sprintf("deleted %d files, freed %d bytes", res.DeletedFiles, res.BytesFreed)
	return res, nil
}

func (s *Service) resolvePresetPaths(rule *models.LocalCleanupRule) ([]string, error) {
	preset := strings.TrimSpace(rule.Preset)
	if preset == "" && rule.PathGlob != "" {
		p := rule.PathGlob
		if !filepath.IsAbs(p) {
			p = filepath.Join(s.dataDir, p)
		}
		return []string{filepath.Clean(p)}, nil
	}
	switch preset {
	case "panel_logs":
		return []string{filepath.Join(s.dataDir, "logs")}, nil
	case "backup_staging":
		return []string{filepath.Join(s.dataDir, "panel-migration")}, nil
	case "panel_migration":
		return []string{filepath.Join(s.dataDir, "panel-migration")}, nil
	default:
		if rule.PathGlob == "" {
			return nil, fmt.Errorf("unknown preset: %s", preset)
		}
		p := rule.PathGlob
		if !filepath.IsAbs(p) {
			p = filepath.Join(s.dataDir, p)
		}
		return []string{filepath.Clean(p)}, nil
	}
}

func (s *Service) isAllowedPath(path string) bool {
	path = filepath.Clean(path)
	allowedRoots := []string{
		filepath.Clean(s.dataDir),
	}
	all, _ := s.settings.GetAll()
	for _, key := range []string{"backup_path", "website_path"} {
		if v := strings.TrimSpace(all[key]); v != "" {
			if !filepath.IsAbs(v) {
				v = filepath.Join(s.dataDir, v)
			}
			allowedRoots = append(allowedRoots, filepath.Clean(v))
		}
	}
	for _, root := range allowedRoots {
		if path == root || strings.HasPrefix(path, root+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}
