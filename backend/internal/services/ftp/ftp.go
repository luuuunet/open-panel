package ftp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Service struct {
	db      *gorm.DB
	dataDir string
}

func NewService(db *gorm.DB, dataDir string) *Service {
	return &Service{db: db, dataDir: dataDir}
}

func (s *Service) List() ([]models.FTPAccount, error) {
	var list []models.FTPAccount
	return list, s.db.Order("id desc").Find(&list).Error
}

func (s *Service) Create(acc *models.FTPAccount, password string) error {
	if strings.TrimSpace(acc.Path) == "" {
		return fmt.Errorf("根目录不能为空")
	}
	syncErr := syncPureFTP(acc.Username, password, acc.Path, true)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	acc.Password = string(hash)
	acc.Status = "enabled"
	acc.Synced = syncErr == nil
	if syncErr != nil {
		acc.SyncError = syncErr.Error()
	}
	if err := s.db.Create(acc).Error; err != nil {
		if syncErr == nil {
			_ = syncPureFTP(acc.Username, password, acc.Path, false)
		}
		return err
	}
	s.savePassFile(acc.Username, password)
	return syncErr
}

func (s *Service) Delete(id uint) error {
	var acc models.FTPAccount
	if err := s.db.First(&acc, id).Error; err != nil {
		return err
	}
	_ = syncPureFTP(acc.Username, "", acc.Path, false)
	s.removePassFile(acc.Username)
	return s.db.Delete(&models.FTPAccount{}, id).Error
}

func (s *Service) SyncAll() error {
	var list []models.FTPAccount
	if err := s.db.Find(&list).Error; err != nil {
		return err
	}
	for i := range list {
		pass := s.readPassFile(list[i].Username)
		if pass == "" {
			continue
		}
		err := syncPureFTP(list[i].Username, pass, list[i].Path, true)
		updates := map[string]interface{}{"synced": err == nil}
		if err != nil {
			updates["sync_error"] = err.Error()
		} else {
			updates["sync_error"] = ""
		}
		s.db.Model(&list[i]).Updates(updates)
	}
	return nil
}

func (s *Service) passDir() string {
	return filepath.Join(s.dataDir, "ftp", "secrets")
}

func (s *Service) savePassFile(username, password string) {
	dir := s.passDir()
	_ = os.MkdirAll(dir, 0700)
	path := filepath.Join(dir, username+".pass")
	_ = os.WriteFile(path, []byte(password), 0600)
}

func (s *Service) readPassFile(username string) string {
	data, err := os.ReadFile(filepath.Join(s.passDir(), username+".pass"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (s *Service) GetPassword(username string) string {
	return s.readPassFile(username)
}

func (s *Service) SetPassword(username, password string) error {
	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
		return fmt.Errorf("username and password required")
	}
	var acc models.FTPAccount
	if err := s.db.Unscoped().Where("username = ?", username).First(&acc).Error; err != nil {
		return fmt.Errorf("FTP 账号不存在")
	}
	if acc.DeletedAt.Valid {
		_ = s.db.Unscoped().Model(&acc).Update("deleted_at", nil).Error
	}
	if err := syncPureFTPPassword(username, password); err != nil {
		if err2 := syncPureFTP(username, password, acc.Path, true); err2 != nil {
			return err
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	s.db.Model(&acc).Updates(map[string]interface{}{
		"password": string(hash), "synced": true, "sync_error": "",
	})
	s.savePassFile(username, password)
	return nil
}

func (s *Service) removePassFile(username string) {
	_ = os.Remove(filepath.Join(s.passDir(), username+".pass"))
}
