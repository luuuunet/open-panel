package ossstorage

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/scheduleutil"
)

type LifecycleRuleRequest struct {
	Name         string `json:"name"`
	StorageID    uint   `json:"storage_id"`
	Prefix       string `json:"prefix"`
	MaxAgeDays   int    `json:"max_age_days"`
	KeepMinCount int    `json:"keep_min_count"`
	DryRun       bool   `json:"dry_run"`
	Schedule     string `json:"schedule"`
	Enabled      bool   `json:"enabled"`
}

type LifecycleRunResult struct {
	DeletedCount int    `json:"deleted_count"`
	BytesFreed   int64  `json:"bytes_freed"`
	Preview      []ObjectInfo `json:"preview,omitempty"`
	Message      string `json:"message"`
}

func (s *Service) ListLifecycleRules() ([]models.OSSLifecycleRule, error) {
	var list []models.OSSLifecycleRule
	return list, s.db.Order("id desc").Find(&list).Error
}

func (s *Service) CreateLifecycleRule(req *LifecycleRuleRequest) (*models.OSSLifecycleRule, error) {
	rule := s.lifecycleRequestToModel(req)
	if err := s.db.Create(rule).Error; err != nil {
		return nil, err
	}
	return rule, nil
}

func (s *Service) UpdateLifecycleRule(id uint, req *LifecycleRuleRequest) (*models.OSSLifecycleRule, error) {
	var rule models.OSSLifecycleRule
	if err := s.db.First(&rule, id).Error; err != nil {
		return nil, err
	}
	updated := s.lifecycleRequestToModel(req)
	updated.ID = rule.ID
	if err := s.db.Save(updated).Error; err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *Service) DeleteLifecycleRule(id uint) error {
	return s.db.Delete(&models.OSSLifecycleRule{}, id).Error
}

func (s *Service) lifecycleRequestToModel(req *LifecycleRuleRequest) *models.OSSLifecycleRule {
	if req.MaxAgeDays <= 0 {
		req.MaxAgeDays = 30
	}
	if req.KeepMinCount <= 0 {
		req.KeepMinCount = 1
	}
	if req.Schedule == "" {
		req.Schedule = "0 6 * * *"
	}
	return &models.OSSLifecycleRule{
		Name:         strings.TrimSpace(req.Name),
		StorageID:    req.StorageID,
		Prefix:       strings.Trim(strings.TrimSpace(req.Prefix), "/"),
		MaxAgeDays:   req.MaxAgeDays,
		KeepMinCount: req.KeepMinCount,
		DryRun:       req.DryRun,
		Schedule:     req.Schedule,
		Enabled:      req.Enabled,
	}
}

func (s *Service) RunDueLifecycleRules() int {
	var rules []models.OSSLifecycleRule
	if err := s.db.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		return 0
	}
	n := 0
	now := time.Now()
	for i := range rules {
		if !scheduleutil.DueNow(rules[i].Schedule, rules[i].LastRunAt, now) {
			continue
		}
		if _, err := s.RunLifecycleRule(&rules[i], false); err == nil {
			n++
		}
	}
	return n
}

func (s *Service) RunLifecycleRule(rule *models.OSSLifecycleRule, forceDryRun bool) (*LifecycleRunResult, error) {
	now := time.Now()
	dryRun := rule.DryRun || forceDryRun
	res, err := s.executeLifecycleRule(rule, dryRun)
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

func (s *Service) executeLifecycleRule(rule *models.OSSLifecycleRule, dryRun bool) (*LifecycleRunResult, error) {
	store, err := s.openStore(&rule.StorageID)
	if err != nil {
		return nil, err
	}
	cutoff := time.Now().AddDate(0, 0, -rule.MaxAgeDays)
	var objects []ObjectInfo
	err = store.Walk(rule.Prefix, func(obj ObjectInfo) error {
		if obj.IsDir || obj.Key == "" {
			return nil
		}
		objects = append(objects, obj)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].LastModified > objects[j].LastModified
	})
	res := &LifecycleRunResult{}
	keep := rule.KeepMinCount
	if keep < 0 {
		keep = 0
	}
	for i, obj := range objects {
		if i < keep {
			continue
		}
		mod, _ := time.Parse(time.RFC3339, obj.LastModified)
		if mod.IsZero() || mod.After(cutoff) {
			continue
		}
		if dryRun {
			res.Preview = append(res.Preview, obj)
			continue
		}
		if err := store.Delete(obj.Key); err != nil {
			return res, err
		}
		res.DeletedCount++
		res.BytesFreed += obj.Size
	}
	if dryRun {
		res.Message = fmt.Sprintf("dry-run: would delete %d objects", len(res.Preview))
	} else {
		res.Message = fmt.Sprintf("deleted %d objects, freed %d bytes", res.DeletedCount, res.BytesFreed)
	}
	return res, nil
}
