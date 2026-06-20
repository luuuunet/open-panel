package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/migration"
)

type PanelBackupOptions struct {
	OSSStorageID *uint
	RemoteID     *uint
	IncludeLogs  bool
	KeepCount    int
}

type PanelBackupConfig struct {
	Enabled      bool   `json:"enabled"`
	Schedule     string `json:"schedule"`
	KeepCount    int    `json:"keep_count"`
	OSSStorageID *uint  `json:"oss_storage_id"`
	RemoteID     *uint  `json:"remote_id"`
	IncludeLogs  bool   `json:"include_logs"`
	TaskID       *uint  `json:"task_id"`
}

func (s *Service) GetPanelBackupConfig() (*PanelBackupConfig, error) {
	cfg := &PanelBackupConfig{
		Schedule:  "0 4 * * *",
		KeepCount: 5,
	}
	var task models.BackupTask
	if err := s.db.Where("type = ?", "panel").Order("id desc").First(&task).Error; err == nil {
		cfg.Enabled = task.Enabled
		cfg.Schedule = task.Schedule
		cfg.KeepCount = task.KeepCount
		cfg.OSSStorageID = task.OSSStorageID
		cfg.RemoteID = task.RemoteID
		cfg.IncludeLogs = strings.EqualFold(strings.TrimSpace(task.Target), "logs")
		id := task.ID
		cfg.TaskID = &id
	}
	if cfg.KeepCount <= 0 {
		cfg.KeepCount = 5
	}
	return cfg, nil
}

func (s *Service) UpdatePanelBackupConfig(cfg PanelBackupConfig) (*PanelBackupConfig, error) {
	if cfg.KeepCount <= 0 {
		cfg.KeepCount = 5
	}
	if cfg.Schedule == "" {
		cfg.Schedule = "0 4 * * *"
	}
	target := ""
	if cfg.IncludeLogs {
		target = "logs"
	}
	var task models.BackupTask
	err := s.db.Where("type = ?", "panel").Order("id desc").First(&task).Error
	if err != nil {
		task = models.BackupTask{
			Name:     "面板云备份",
			Type:     "panel",
			Target:   target,
			Schedule: cfg.Schedule,
			Enabled:  cfg.Enabled,
		}
	} else {
		task.Name = "面板云备份"
		task.Target = target
		task.Schedule = cfg.Schedule
		task.Enabled = cfg.Enabled
	}
	task.KeepCount = cfg.KeepCount
	task.OSSStorageID = cfg.OSSStorageID
	task.RemoteID = cfg.RemoteID
	if task.ID == 0 {
		if err := s.db.Create(&task).Error; err != nil {
			return nil, err
		}
	} else if err := s.db.Save(&task).Error; err != nil {
		return nil, err
	}
	return s.GetPanelBackupConfig()
}

func (s *Service) ListPanelBackupHistory(limit int) ([]models.PanelBackupRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	var list []models.PanelBackupRecord
	err := s.db.Order("id desc").Limit(limit).Find(&list).Error
	return list, err
}

func (s *Service) RunPanelBackup(opts PanelBackupOptions) (*models.PanelBackupRecord, error) {
	if opts.KeepCount <= 0 {
		opts.KeepCount = 5
	}
	mig := migration.NewService(s.db, s.dataDir, s.settings)
	exp, err := mig.Export(migration.ExportOptions{
		IncludeSecrets: true,
		IncludeLogs:    opts.IncludeLogs,
	})
	if err != nil {
		return nil, err
	}

	rec := &models.PanelBackupRecord{
		Filename:     exp.Filename,
		LocalPath:    exp.Path,
		Size:         exp.Size,
		Status:       "done",
		OSSStorageID: opts.OSSStorageID,
		RemoteID:     opts.RemoteID,
	}
	if err := s.db.Create(rec).Error; err != nil {
		return nil, err
	}

	var errs []string
	if opts.RemoteID != nil && *opts.RemoteID > 0 {
		if err := s.uploadToRemote(*opts.RemoteID, exp.Path, exp.Filename); err != nil {
			errs = append(errs, "remote: "+err.Error())
		}
	}
	if opts.OSSStorageID != nil && *opts.OSSStorageID > 0 {
		key := "backups/panel/" + exp.Filename
		if err := s.uploadToOSSKey(*opts.OSSStorageID, exp.Path, key); err != nil {
			errs = append(errs, "oss: "+err.Error())
		} else {
			rec.RemoteKey = key
		}
	}
	if len(errs) > 0 {
		rec.Status = "partial"
		rec.ErrorMsg = strings.Join(errs, "; ")
		_ = s.db.Save(rec).Error
	}
	s.prunePanelBackups(opts.KeepCount)
	return rec, nil
}

func (s *Service) RestorePanelFromRecord(recordID uint, mode string) (*migration.ImportResult, error) {
	var rec models.PanelBackupRecord
	if err := s.db.First(&rec, recordID).Error; err != nil {
		return nil, err
	}
	bundlePath := rec.LocalPath
	if _, err := os.Stat(bundlePath); err != nil {
		if rec.RemoteKey == "" || rec.OSSStorageID == nil || *rec.OSSStorageID == 0 {
			return nil, fmt.Errorf("local bundle missing and no remote key")
		}
		tmp := filepath.Join(os.TempDir(), rec.Filename)
		if err := s.downloadFromOSS(*rec.OSSStorageID, rec.RemoteKey, tmp); err != nil {
			return nil, err
		}
		defer os.Remove(tmp)
		bundlePath = tmp
	}
	mig := migration.NewService(s.db, s.dataDir, s.settings)
	return mig.ImportBundle(bundlePath, migration.ImportOptions{Mode: mode})
}

func (s *Service) runPanelTask(task *models.BackupTask) (string, error) {
	keep := task.KeepCount
	if keep <= 0 {
		keep = 5
	}
	rec, err := s.RunPanelBackup(PanelBackupOptions{
		OSSStorageID: task.OSSStorageID,
		RemoteID:     task.RemoteID,
		IncludeLogs:  strings.EqualFold(strings.TrimSpace(task.Target), "logs"),
		KeepCount:    keep,
	})
	if err != nil {
		return "", err
	}
	return rec.LocalPath, nil
}

func (s *Service) prunePanelBackups(keep int) {
	if keep <= 0 {
		keep = 5
	}
	var list []models.PanelBackupRecord
	s.db.Order("id desc").Find(&list)
	if len(list) <= keep {
		return
	}
	for _, old := range list[keep:] {
		_ = s.deletePanelBackupRecord(&old)
	}
}

func (s *Service) deletePanelBackupRecord(rec *models.PanelBackupRecord) error {
	if rec.LocalPath != "" {
		_ = os.Remove(rec.LocalPath)
	}
	if rec.RemoteKey != "" && rec.OSSStorageID != nil && *rec.OSSStorageID > 0 {
		s.deleteOSSObject(*rec.OSSStorageID, rec.RemoteKey)
	}
	return s.db.Delete(rec).Error
}
