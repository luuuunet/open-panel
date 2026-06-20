package ossstorage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/scheduleutil"
)

type ArchiveRuleRequest struct {
	Name             string `json:"name"`
	LocalPath        string `json:"local_path"`
	MinSizeMB        int    `json:"min_size_mb"`
	FilePatterns     string `json:"file_patterns"`
	TargetStorageID  uint   `json:"target_storage_id"`
	TargetPrefix     string `json:"target_prefix"`
	DeleteLocalAfter bool   `json:"delete_local_after"`
	Schedule         string `json:"schedule"`
	Enabled          bool   `json:"enabled"`
}

type ArchiveRunResult struct {
	UploadedCount int    `json:"uploaded_count"`
	BytesUploaded int64  `json:"bytes_uploaded"`
	DeletedLocal  int    `json:"deleted_local"`
	Message       string `json:"message"`
}

func (s *Service) ListArchiveRules() ([]models.OSSArchiveRule, error) {
	var list []models.OSSArchiveRule
	return list, s.db.Order("id desc").Find(&list).Error
}

func (s *Service) CreateArchiveRule(req *ArchiveRuleRequest) (*models.OSSArchiveRule, error) {
	rule := s.archiveRequestToModel(req)
	if err := s.db.Create(rule).Error; err != nil {
		return nil, err
	}
	return rule, nil
}

func (s *Service) UpdateArchiveRule(id uint, req *ArchiveRuleRequest) (*models.OSSArchiveRule, error) {
	var rule models.OSSArchiveRule
	if err := s.db.First(&rule, id).Error; err != nil {
		return nil, err
	}
	updated := s.archiveRequestToModel(req)
	updated.ID = rule.ID
	if err := s.db.Save(updated).Error; err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *Service) DeleteArchiveRule(id uint) error {
	return s.db.Delete(&models.OSSArchiveRule{}, id).Error
}

func (s *Service) archiveRequestToModel(req *ArchiveRuleRequest) *models.OSSArchiveRule {
	if req.MinSizeMB <= 0 {
		req.MinSizeMB = 100
	}
	if req.TargetPrefix == "" {
		req.TargetPrefix = "archives/"
	}
	if req.FilePatterns == "" {
		req.FilePatterns = "*"
	}
	if req.Schedule == "" {
		req.Schedule = "0 7 * * *"
	}
	local := strings.TrimSpace(req.LocalPath)
	if local != "" && !filepath.IsAbs(local) {
		local = filepath.Join(s.dataDir, local)
	}
	return &models.OSSArchiveRule{
		Name:             strings.TrimSpace(req.Name),
		LocalPath:        filepath.Clean(local),
		MinSizeMB:        req.MinSizeMB,
		FilePatterns:     req.FilePatterns,
		TargetStorageID:  req.TargetStorageID,
		TargetPrefix:     strings.Trim(req.TargetPrefix, "/") + "/",
		DeleteLocalAfter: req.DeleteLocalAfter,
		Schedule:         req.Schedule,
		Enabled:          req.Enabled,
	}
}

func (s *Service) RunDueArchiveRules() int {
	var rules []models.OSSArchiveRule
	if err := s.db.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		return 0
	}
	n := 0
	now := time.Now()
	for i := range rules {
		if !scheduleutil.DueNow(rules[i].Schedule, rules[i].LastRunAt, now) {
			continue
		}
		if _, err := s.RunArchiveRule(&rules[i]); err == nil {
			n++
		}
	}
	return n
}

func (s *Service) RunArchiveRule(rule *models.OSSArchiveRule) (*ArchiveRunResult, error) {
	now := time.Now()
	res, err := s.executeArchiveRule(rule)
	status := "success"
	logMsg := res.Message
	if err != nil {
		status = "failed"
		logMsg = err.Error()
	}
	t := now
	s.db.Model(rule).Updates(map[string]interface{}{
		"last_run_at": &t,
		"last_status": status,
		"last_log":    logMsg,
	})
	return res, err
}

func (s *Service) executeArchiveRule(rule *models.OSSArchiveRule) (*ArchiveRunResult, error) {
	if rule.LocalPath == "" {
		return nil, fmt.Errorf("local_path required")
	}
	if !s.isArchivePathAllowed(rule.LocalPath) {
		return nil, fmt.Errorf("local path not allowed: %s", rule.LocalPath)
	}
	store, err := s.openStore(&rule.TargetStorageID)
	if err != nil {
		return nil, err
	}
	minBytes := int64(rule.MinSizeMB) * 1024 * 1024
	patterns := strings.Split(rule.FilePatterns, ",")
	res := &ArchiveRunResult{}
	err = filepath.Walk(rule.LocalPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info == nil || info.IsDir() {
			return nil
		}
		if info.Size() < minBytes {
			return nil
		}
		if !matchArchivePattern(filepath.Base(path), patterns) {
			return nil
		}
		rel, err := filepath.Rel(rule.LocalPath, path)
		if err != nil {
			return nil
		}
		key := joinKey(rule.TargetPrefix, strings.ReplaceAll(rel, string(os.PathSeparator), "/"))
		if err := store.UploadFile(path, key); err != nil {
			return err
		}
		res.UploadedCount++
		res.BytesUploaded += info.Size()
		if rule.DeleteLocalAfter {
			if err := os.Remove(path); err == nil {
				res.DeletedLocal++
			}
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	res.Message = fmt.Sprintf("uploaded %d files (%d bytes), deleted local %d", res.UploadedCount, res.BytesUploaded, res.DeletedLocal)
	return res, nil
}

func (s *Service) isArchivePathAllowed(path string) bool {
	path = filepath.Clean(path)
	root := filepath.Clean(s.dataDir)
	return path == root || strings.HasPrefix(path, root+string(os.PathSeparator))
}

func matchArchivePattern(name string, patterns []string) bool {
	name = strings.ToLower(name)
	for _, p := range patterns {
		p = strings.TrimSpace(strings.ToLower(p))
		if p == "" || p == "*" {
			return true
		}
		if strings.HasPrefix(p, "*.") {
			if strings.HasSuffix(name, strings.TrimPrefix(p, "*")) {
				return true
			}
		}
		if name == p {
			return true
		}
	}
	return false
}
