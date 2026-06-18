package database

import (
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/ossstorage"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"gorm.io/gorm"
)

type Service struct {
	db       *gorm.DB
	dataDir  string
	settings *settings.Service
	oss      *ossstorage.Service
	remote   RemoteUploader
}

type RemoteUploader interface {
	UploadToRemote(remoteID uint, localFile, remoteName string) error
}

func NewService(db *gorm.DB, dataDir string, settingsSvc *settings.Service) *Service {
	return &Service{db: db, dataDir: dataDir, settings: settingsSvc}
}

func (s *Service) SetRemoteUploader(u RemoteUploader) {
	s.remote = u
}

type InstanceDetail struct {
	models.DatabaseInstance
	HasPassword  bool `json:"has_password"`
	BackupCount  int  `json:"backup_count"`
}

func (s *Service) List() ([]InstanceDetail, error) {
	_ = s.SyncFromServer()
	return s.listInstances()
}

func (s *Service) listInstances() ([]InstanceDetail, error) {
	var instances []models.DatabaseInstance
	if err := s.db.Order("id desc").Find(&instances).Error; err != nil {
		return nil, err
	}
	out := make([]InstanceDetail, 0, len(instances))
	for i := range instances {
		var count int64
		s.db.Model(&models.DatabaseBackup{}).Where("database_id = ?", instances[i].ID).Count(&count)
		out = append(out, InstanceDetail{
			DatabaseInstance: instances[i],
			HasPassword:      instances[i].Password != "",
			BackupCount:      int(count),
		})
	}
	return out, nil
}

func (s *Service) Get(id uint) (*models.DatabaseInstance, error) {
	var inst models.DatabaseInstance
	if err := s.db.First(&inst, id).Error; err != nil {
		return nil, err
	}
	return &inst, nil
}

func (s *Service) Create(instance *models.DatabaseInstance) error {
	if instance.Status == "" {
		instance.Status = "running"
	}
	if instance.Host == "" {
		instance.Host = "127.0.0.1"
	}
	if instance.Port == 0 {
		switch instance.Type {
		case "postgresql":
			instance.Port = 5432
		case "redis":
			instance.Port = 6379
		case "mongodb":
			instance.Port = 27017
		default:
			instance.Port = 3306
		}
	}
	return s.db.Create(instance).Error
}

func (s *Service) UpdateCredentials(id uint, username, password string) error {
	return s.UpdateInstance(id, UpdateRequest{Username: username, Password: password})
}

type UpdateRequest struct {
	Host        string
	Port        int
	Username    string
	Password    string
	Remark      *string
	AllowRemote *bool
	AccessMode  *string
	ForceSSL    *bool
}

func (s *Service) UpdateInstance(id uint, req UpdateRequest) error {
	var inst models.DatabaseInstance
	if err := s.db.First(&inst, id).Error; err != nil {
		return err
	}
	prevMode := AccessModeFromInstance(inst.AccessMode, inst.AllowRemote)
	prevSSL := inst.ForceSSL
	updates := map[string]interface{}{}
	if req.Host != "" {
		updates["host"] = req.Host
	}
	if req.Port > 0 {
		updates["port"] = req.Port
	}
	if req.Username != "" {
		updates["username"] = req.Username
	}
	if req.Password != "" {
		updates["password"] = req.Password
	}
	if req.Remark != nil {
		updates["remark"] = *req.Remark
	}
	if req.AccessMode != nil {
		mode := NormalizeAccessMode(*req.AccessMode)
		updates["access_mode"] = mode
		updates["allow_remote"] = AllowRemoteFromAccessMode(mode)
	} else if req.AllowRemote != nil {
		mode := AccessModeLocal
		if *req.AllowRemote {
			mode = AccessModeBoth
		}
		updates["access_mode"] = mode
		updates["allow_remote"] = *req.AllowRemote
	}
	if req.ForceSSL != nil {
		updates["force_ssl"] = *req.ForceSSL
	}
	if len(updates) == 0 {
		return nil
	}
	if err := s.db.Model(&inst).Updates(updates).Error; err != nil {
		return err
	}
	if err := s.db.First(&inst, id).Error; err != nil {
		return err
	}
	if req.Password != "" {
		inst.Password = req.Password
	}
	if isMySQLType(inst.Type) {
		newMode := AccessModeFromInstance(inst.AccessMode, inst.AllowRemote)
		if req.AccessMode != nil || req.AllowRemote != nil {
			if newMode != prevMode {
				if err := s.applyMySQLAccessMode(&inst, newMode); err != nil {
					return err
				}
			}
		}
		if req.ForceSSL != nil && *req.ForceSSL != prevSSL {
			if err := s.applyMySQLForceSSL(&inst, *req.ForceSSL); err != nil {
				return err
			}
		}
	}
	return nil
}

func isMySQLType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "mysql", "mariadb", "":
		return true
	default:
		return false
	}
}

func (s *Service) Delete(id uint) error {
	var backups []models.DatabaseBackup
	s.db.Where("database_id = ?", id).Find(&backups)
	for _, b := range backups {
		_ = s.DeleteBackup(id, b.ID)
	}
	return s.db.Delete(&models.DatabaseInstance{}, id).Error
}
