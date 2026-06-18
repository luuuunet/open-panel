package edgekv

import (
	"fmt"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) ListNamespaces() ([]models.EdgeKVNamespace, error) {
	var list []models.EdgeKVNamespace
	return list, s.db.Order("id asc").Find(&list).Error
}

func (s *Service) GetNamespace(id uint) (*models.EdgeKVNamespace, error) {
	var ns models.EdgeKVNamespace
	if err := s.db.First(&ns, id).Error; err != nil {
		return nil, err
	}
	return &ns, nil
}

func (s *Service) CreateNamespace(ns *models.EdgeKVNamespace) error {
	ns.Name = strings.TrimSpace(ns.Name)
	if ns.Name == "" {
		return fmt.Errorf("namespace name is required")
	}
	return s.db.Create(ns).Error
}

func (s *Service) UpdateNamespace(id uint, patch *models.EdgeKVNamespace) error {
	ns, err := s.GetNamespace(id)
	if err != nil {
		return err
	}
	if patch.Name != "" {
		ns.Name = strings.TrimSpace(patch.Name)
	}
	ns.Description = patch.Description
	return s.db.Save(ns).Error
}

func (s *Service) DeleteNamespace(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("namespace_id = ?", id).Delete(&models.EdgeKVEntry{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.EdgeKVNamespace{}, id).Error
	})
}

type KeyItem struct {
	Key       string     `json:"key"`
	Value     string     `json:"value"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func (s *Service) ListKeys(namespaceID uint, prefix string) ([]KeyItem, error) {
	if _, err := s.GetNamespace(namespaceID); err != nil {
		return nil, err
	}
	q := s.db.Model(&models.EdgeKVEntry{}).Where("namespace_id = ?", namespaceID)
	if prefix != "" {
		q = q.Where("key LIKE ?", prefix+"%")
	}
	var rows []models.EdgeKVEntry
	if err := q.Order("key asc").Limit(1000).Find(&rows).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]KeyItem, 0, len(rows))
	for _, r := range rows {
		if r.ExpiresAt != nil && r.ExpiresAt.Before(now) {
			continue
		}
		out = append(out, KeyItem{Key: r.Key, Value: r.Value, ExpiresAt: r.ExpiresAt, UpdatedAt: r.UpdatedAt})
	}
	return out, nil
}

func (s *Service) GetKey(namespaceID uint, key string) (*KeyItem, error) {
	var row models.EdgeKVEntry
	err := s.db.Where("namespace_id = ? AND key = ?", namespaceID, key).First(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ExpiresAt != nil && row.ExpiresAt.Before(time.Now()) {
		_ = s.db.Delete(&row).Error
		return nil, gorm.ErrRecordNotFound
	}
	return &KeyItem{Key: row.Key, Value: row.Value, ExpiresAt: row.ExpiresAt, UpdatedAt: row.UpdatedAt}, nil
}

func (s *Service) PutKey(namespaceID uint, key, value string, expiresAt *time.Time) error {
	if _, err := s.GetNamespace(namespaceID); err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("key is required")
	}
	var row models.EdgeKVEntry
	err := s.db.Where("namespace_id = ? AND key = ?", namespaceID, key).First(&row).Error
	now := time.Now()
	if err == gorm.ErrRecordNotFound {
		row = models.EdgeKVEntry{
			NamespaceID: namespaceID,
			Key:         key,
			Value:       value,
			ExpiresAt:   expiresAt,
			UpdatedAt:   now,
		}
		return s.db.Create(&row).Error
	}
	if err != nil {
		return err
	}
	row.Value = value
	row.ExpiresAt = expiresAt
	row.UpdatedAt = now
	return s.db.Save(&row).Error
}

func (s *Service) DeleteKey(namespaceID uint, key string) error {
	return s.db.Where("namespace_id = ? AND key = ?", namespaceID, key).Delete(&models.EdgeKVEntry{}).Error
}

func (s *Service) ExportNamespace(namespaceID uint) ([]KeyItem, error) {
	return s.ListKeys(namespaceID, "")
}
