package backup

import (
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/settings"
	dbsvc "github.com/luuuunet/owpanel/internal/services/database"
	"github.com/luuuunet/owpanel/internal/services/ossstorage"
	"gorm.io/gorm"
)

type Service struct {
	db       *gorm.DB
	dataDir  string
	settings *settings.Service
	database *dbsvc.Service
	oss      *ossstorage.Service
}

func NewService(db *gorm.DB, dataDir string, settingsSvc *settings.Service) *Service {
	return &Service{db: db, dataDir: dataDir, settings: settingsSvc}
}

func (s *Service) List() ([]models.BackupTask, error) {
	var list []models.BackupTask
	return list, s.db.Order("id desc").Find(&list).Error
}

func (s *Service) Create(task *models.BackupTask) error {
	return s.db.Create(task).Error
}

func (s *Service) Delete(id uint) error {
	return s.db.Delete(&models.BackupTask{}, id).Error
}

func (s *Service) Toggle(id uint, enabled bool) error {
	return s.db.Model(&models.BackupTask{}).Where("id = ?", id).Update("enabled", enabled).Error
}

func (s *Service) backupBaseDir() string {
	all, _ := s.settings.GetAll()
	base := all["backup_path"]
	if base == "" {
		base = settings.DefaultBackupPath(s.dataDir)
	}
	return resolvePath(s.dataDir, base)
}
