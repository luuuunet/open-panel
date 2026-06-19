package backup

import "github.com/luuuunet/owpanel/internal/models"

// LinkOSSToTasks sets oss_storage_id on backup tasks that do not have one yet.
func (s *Service) LinkOSSToTasks(storageID uint) (int, error) {
	if storageID == 0 {
		return 0, nil
	}
	res := s.db.Model(&models.BackupTask{}).
		Where("(oss_storage_id IS NULL OR oss_storage_id = 0) AND enabled = ?", true).
		Update("oss_storage_id", storageID)
	return int(res.RowsAffected), res.Error
}
