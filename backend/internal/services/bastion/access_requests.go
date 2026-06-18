package bastion

import (
	"fmt"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

type AccessRequestInput struct {
	AssetID       uint   `json:"asset_id"`
	AccountID     *uint  `json:"account_id"`
	Reason        string `json:"reason"`
	DurationHours int    `json:"duration_hours"`
}

func (s *Service) enrichAccessRequest(r *models.BastionAccessRequest) {
	var u models.User
	if s.db.First(&u, r.UserID).Error == nil {
		r.Username = u.Username
	}
	if a, err := s.GetAsset(r.AssetID); err == nil {
		r.AssetName = a.Name
	}
	if r.ApprovedBy != nil && *r.ApprovedBy > 0 {
		var approver models.User
		if s.db.First(&approver, *r.ApprovedBy).Error == nil {
			r.ApproverName = approver.Username
		}
	}
}

func (s *Service) ListAccessRequests(userID uint, role string) ([]models.BastionAccessRequest, error) {
	var list []models.BastionAccessRequest
	q := s.db.Order("created_at desc").Limit(200)
	if role != "admin" {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		s.enrichAccessRequest(&list[i])
	}
	return list, nil
}

func (s *Service) CreateAccessRequest(userID uint, in AccessRequestInput) (*models.BastionAccessRequest, error) {
	if in.AssetID == 0 {
		return nil, fmt.Errorf("请选择资产")
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return nil, fmt.Errorf("请填写申请理由")
	}
	hours := in.DurationHours
	if hours <= 0 {
		hours = 4
	}
	if hours > 72 {
		hours = 72
	}
	if _, err := s.GetAsset(in.AssetID); err != nil {
		return nil, fmt.Errorf("资产不存在")
	}
	req := models.BastionAccessRequest{
		UserID: userID, AssetID: in.AssetID, AccountID: in.AccountID,
		Reason: reason, DurationHours: hours, Status: "pending",
	}
	if err := s.db.Create(&req).Error; err != nil {
		return nil, err
	}
	s.enrichAccessRequest(&req)
	return &req, nil
}

func (s *Service) ApproveAccessRequest(id, approverID uint, onApproved func(*models.BastionAccessRequest)) (*models.BastionAccessRequest, error) {
	var req models.BastionAccessRequest
	if err := s.db.First(&req, id).Error; err != nil {
		return nil, err
	}
	if req.Status != "pending" {
		return nil, fmt.Errorf("申请状态不可审批: %s", req.Status)
	}
	expires := time.Now().Add(time.Duration(req.DurationHours) * time.Hour)
	updates := map[string]interface{}{
		"status": "approved", "approved_by": approverID, "expires_at": expires,
	}
	if err := s.db.Model(&req).Updates(updates).Error; err != nil {
		return nil, err
	}
	perm := PermissionInput{
		UserID: req.UserID, AssetID: req.AssetID,
		Permission: "connect", ExpiresAt: &expires,
	}
	if _, err := s.CreatePermission(perm, approverID); err != nil {
		return nil, err
	}
	if err := s.db.First(&req, id).Error; err != nil {
		return nil, err
	}
	s.enrichAccessRequest(&req)
	if onApproved != nil {
		onApproved(&req)
	}
	return &req, nil
}

func (s *Service) RejectAccessRequest(id, approverID uint) (*models.BastionAccessRequest, error) {
	var req models.BastionAccessRequest
	if err := s.db.First(&req, id).Error; err != nil {
		return nil, err
	}
	if req.Status != "pending" {
		return nil, fmt.Errorf("申请状态不可拒绝: %s", req.Status)
	}
	if err := s.db.Model(&req).Updates(map[string]interface{}{
		"status": "rejected", "approved_by": approverID,
	}).Error; err != nil {
		return nil, err
	}
	s.enrichAccessRequest(&req)
	return &req, nil
}

func (s *Service) initAccessRequests() {
	go s.accessRequestExpiryLoop()
}

func (s *Service) accessRequestExpiryLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.expireAccessRequestsAndPermissions()
	}
}

func (s *Service) expireAccessRequestsAndPermissions() {
	now := time.Now()
	var reqs []models.BastionAccessRequest
	s.db.Where("status = ? AND expires_at IS NOT NULL AND expires_at < ?", "approved", now).Find(&reqs)
	for _, r := range reqs {
		_ = s.db.Model(&models.BastionAccessRequest{}).Where("id = ?", r.ID).Update("status", "expired").Error
	}
	_ = s.db.Where("expires_at IS NOT NULL AND expires_at < ?", now).Delete(&models.BastionPermission{}).Error
}

func (s *Service) ListAccessRequestsForExport(from, to time.Time) ([]models.BastionAccessRequest, error) {
	var list []models.BastionAccessRequest
	q := s.db.Where("created_at >= ? AND created_at <= ?", from, to).Order("created_at asc")
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		s.enrichAccessRequest(&list[i])
	}
	return list, nil
}

func (s *Service) CountJITPermissions() (jit, standing int64) {
	now := time.Now()
	s.db.Model(&models.BastionPermission{}).Where("expires_at IS NOT NULL AND expires_at > ?", now).Count(&jit)
	s.db.Model(&models.BastionPermission{}).Where("expires_at IS NULL OR expires_at <= ?", now).Count(&standing)
	return
}

func (s *Service) GetAccessRequest(id uint) (*models.BastionAccessRequest, error) {
	var req models.BastionAccessRequest
	if err := s.db.First(&req, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("申请不存在")
		}
		return nil, err
	}
	s.enrichAccessRequest(&req)
	return &req, nil
}
