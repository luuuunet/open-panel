package bastion

import (
	"fmt"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

type PermissionInput struct {
	UserID     uint       `json:"user_id"`
	AssetID    uint       `json:"asset_id"`
	Permission string     `json:"permission"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

func (s *Service) ListPermissions() ([]models.BastionPermission, error) {
	var list []models.BastionPermission
	if err := s.db.Order("id desc").Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		var u models.User
		if s.db.First(&u, list[i].UserID).Error == nil {
			list[i].Username = u.Username
		}
		var a models.BastionAsset
		if s.db.First(&a, list[i].AssetID).Error == nil {
			list[i].AssetName = a.Name
		}
	}
	return list, nil
}

func (s *Service) CreatePermission(in PermissionInput, createdBy uint) (*models.BastionPermission, error) {
	if in.UserID == 0 || in.AssetID == 0 {
		return nil, fmt.Errorf("用户和资产不能为空")
	}
	perm := strings.TrimSpace(in.Permission)
	if perm == "" {
		perm = "connect"
	}
	var existing models.BastionPermission
	err := s.db.Where("user_id = ? AND asset_id = ?", in.UserID, in.AssetID).First(&existing).Error
	if err == nil {
		existing.Permission = perm
		existing.ExpiresAt = in.ExpiresAt
		existing.CreatedBy = createdBy
		if err := s.db.Save(&existing).Error; err != nil {
			return nil, err
		}
		return &existing, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}
	p := models.BastionPermission{
		UserID: in.UserID, AssetID: in.AssetID,
		Permission: perm, ExpiresAt: in.ExpiresAt, CreatedBy: createdBy,
	}
	if err := s.db.Create(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Service) DeletePermission(id uint) error {
	return s.db.Delete(&models.BastionPermission{}, id).Error
}

func (s *Service) permittedAssetIDs(userID uint) ([]uint, error) {
	var perms []models.BastionPermission
	now := time.Now()
	if err := s.db.Where("user_id = ?", userID).Find(&perms).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(perms))
	for _, p := range perms {
		if p.ExpiresAt != nil && p.ExpiresAt.Before(now) {
			continue
		}
		ids = append(ids, p.AssetID)
	}
	return ids, nil
}

func (s *Service) CheckPermission(userID, assetID uint) (bool, string, error) {
	perm, err := s.GetUserAssetPermission(userID, assetID)
	if err != nil {
		return false, "", err
	}
	if perm == "" {
		return false, "", nil
	}
	return true, perm, nil
}
