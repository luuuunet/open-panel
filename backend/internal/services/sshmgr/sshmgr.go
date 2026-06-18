package sshmgr

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

type Service struct{ db *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

func (s *Service) List() ([]models.SSHKey, error) {
	var list []models.SSHKey
	if err := s.db.Order("id desc").Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		list[i].HasPrivate = strings.TrimSpace(list[i].PrivateKey) != ""
	}
	return list, nil
}

func (s *Service) Get(id uint) (*models.SSHKey, error) {
	var k models.SSHKey
	if err := s.db.First(&k, id).Error; err != nil {
		return nil, err
	}
	k.HasPrivate = strings.TrimSpace(k.PrivateKey) != ""
	return &k, nil
}

func (s *Service) PrivateKey(id uint) (string, error) {
	k, err := s.Get(id)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(k.PrivateKey) == "" {
		return "", fmt.Errorf("该密钥未保存私钥")
	}
	return k.PrivateKey, nil
}

func (s *Service) Create(key *models.SSHKey) error {
	return s.db.Create(key).Error
}

func (s *Service) Delete(id uint) error {
	return s.db.Delete(&models.SSHKey{}, id).Error
}
