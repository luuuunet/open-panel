package backup

import (
	"fmt"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	dbsvc "github.com/luuuunet/owpanel/internal/services/database"
)

type DatabaseBackupConfig struct {
	AutoEnabled    bool   `json:"auto_enabled"`
	Schedule       string `json:"schedule"`
	KeepCount      int    `json:"keep_count"`
	RemoteID       *uint  `json:"remote_id"`
	OSSStorageID   *uint  `json:"oss_storage_id"`
	BackupDir      string `json:"backup_dir"`
}

type DatabaseBackupSummary struct {
	Count       int        `json:"count"`
	LastAt      *time.Time `json:"last_at"`
	StatusLabel string     `json:"status_label"`
}

func (s *Service) GetDatabaseBackupConfig(databaseID uint) (*DatabaseBackupConfig, DatabaseBackupSummary, error) {
	var inst models.DatabaseInstance
	if err := s.db.First(&inst, databaseID).Error; err != nil {
		return nil, DatabaseBackupSummary{}, err
	}
	cfg := &DatabaseBackupConfig{
		AutoEnabled:  inst.BackupAutoEnabled,
		Schedule:     inst.BackupSchedule,
		KeepCount:    inst.BackupKeepCount,
		RemoteID:     inst.BackupRemoteID,
		OSSStorageID: inst.BackupOSSStorageID,
	}
	if s.database != nil {
		bc := s.database.BackupConfig()
		cfg.BackupDir = bc.BackupDir
	}
	if cfg.KeepCount <= 0 {
		cfg.KeepCount = 5
	}
	return cfg, s.databaseBackupSummary(databaseID), nil
}

func (s *Service) UpdateDatabaseBackupConfig(databaseID uint, cfg DatabaseBackupConfig) error {
	if cfg.KeepCount <= 0 {
		cfg.KeepCount = 5
	}
	return s.db.Model(&models.DatabaseInstance{}).Where("id = ?", databaseID).Updates(map[string]interface{}{
		"backup_auto_enabled":    cfg.AutoEnabled,
		"backup_schedule":        cfg.Schedule,
		"backup_keep_count":      cfg.KeepCount,
		"backup_remote_id":       cfg.RemoteID,
		"backup_oss_storage_id":  cfg.OSSStorageID,
	}).Error
}

func (s *Service) databaseBackupSummary(databaseID uint) DatabaseBackupSummary {
	var count int64
	s.db.Model(&models.DatabaseBackup{}).Where("database_id = ? AND status = ?", databaseID, "done").Count(&count)
	var last models.DatabaseBackup
	sum := DatabaseBackupSummary{Count: int(count)}
	if err := s.db.Where("database_id = ? AND status = ?", databaseID, "done").Order("id desc").First(&last).Error; err == nil {
		t := last.CreatedAt
		sum.LastAt = &t
		sum.StatusLabel = fmt.Sprintf("%d份", count)
	} else {
		sum.StatusLabel = "none"
	}
	return sum
}

func (s *Service) RunDueDatabaseAutoBackups() int {
	if s.database == nil {
		return 0
	}
	var dbs []models.DatabaseInstance
	if err := s.db.Where("backup_auto_enabled = ?", true).Find(&dbs).Error; err != nil {
		return 0
	}
	n := 0
	for i := range dbs {
		if !s.shouldAutoDatabaseBackup(&dbs[i]) {
			continue
		}
		opts := dbsvc.BackupOptions{
			RemoteID:     dbs[i].BackupRemoteID,
			OSSStorageID: dbs[i].BackupOSSStorageID,
		}
		if _, err := s.database.RunBackup(dbs[i].ID, opts); err == nil {
			s.database.PruneBackups(dbs[i].ID, dbs[i].BackupKeepCount)
			n++
		}
	}
	return n
}

func (s *Service) shouldAutoDatabaseBackup(inst *models.DatabaseInstance) bool {
	schedule := inst.BackupSchedule
	if schedule == "" {
		schedule = "0 3 * * *"
	}
	sum := s.databaseBackupSummary(inst.ID)
	return cronDueNow(schedule, sum.LastAt, time.Now())
}
