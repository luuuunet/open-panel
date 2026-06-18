package auth

import (
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

func RecordLoginEvent(db *gorm.DB, username, ip, userAgent string, success bool, reason string) {
	if db == nil {
		return
	}
	if len(userAgent) > 512 {
		userAgent = userAgent[:512]
	}
	_ = db.Create(&models.LoginEvent{
		Username:  username,
		IP:        ip,
		UserAgent: userAgent,
		Success:   success,
		Reason:    reason,
	}).Error
}

func ListLoginEvents(db *gorm.DB, limit, offset int) ([]models.LoginEvent, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var total int64
	if err := db.Model(&models.LoginEvent{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.LoginEvent
	err := db.Order("created_at desc").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func CleanupLoginEvents(db *gorm.DB, olderThanDays int) (int64, error) {
	if olderThanDays <= 0 {
		olderThanDays = 90
	}
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)
	res := db.Where("created_at < ?", cutoff).Delete(&models.LoginEvent{})
	return res.RowsAffected, res.Error
}
