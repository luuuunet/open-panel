package backup

import (
	"fmt"

	"github.com/luuuunet/owpanel/internal/models"
)

type PresetResult struct {
	Created int    `json:"created"`
	Skipped int    `json:"skipped"`
	Preset  string `json:"preset"`
}

// ApplyPreset creates scheduled backup tasks for all websites or databases that do not already have a task.
// websiteIDs limits website preset to specific sites when non-empty.
// ossStorageID attaches remote OSS upload when set on newly created tasks.
func (s *Service) ApplyPreset(preset, schedule string, websiteIDs []uint, ossStorageID *uint) (*PresetResult, error) {
	if schedule == "" {
		schedule = "0 2 * * *"
	}
	res := &PresetResult{Preset: preset}
	switch preset {
	case "websites":
		var sites []models.Website
		q := s.db
		if len(websiteIDs) > 0 {
			q = q.Where("id IN ?", websiteIDs)
		}
		if err := q.Find(&sites).Error; err != nil {
			return nil, err
		}
		for _, site := range sites {
			var count int64
			s.db.Model(&models.BackupTask{}).Where("website_id = ?", site.ID).Count(&count)
			if count > 0 {
				res.Skipped++
				continue
			}
			wid := site.ID
			task := &models.BackupTask{
				Name:      fmt.Sprintf("每日备份-%s", site.Domain),
				Type:      "website",
				Target:    site.Domain,
				Schedule:  schedule,
				Enabled:   true,
				WebsiteID: &wid,
			}
			if ossStorageID != nil && *ossStorageID > 0 {
				task.OSSStorageID = ossStorageID
			}
			if err := s.Create(task); err != nil {
				return res, err
			}
			res.Created++
		}
	case "databases":
		var dbs []models.DatabaseInstance
		if err := s.db.Find(&dbs).Error; err != nil {
			return nil, err
		}
		for _, db := range dbs {
			var count int64
			s.db.Model(&models.BackupTask{}).Where("database_id = ?", db.ID).Count(&count)
			if count > 0 {
				res.Skipped++
				continue
			}
			did := db.ID
			task := &models.BackupTask{
				Name:       fmt.Sprintf("每日备份-%s", db.Name),
				Type:       "database",
				Target:     db.Name,
				Schedule:   schedule,
				Enabled:    true,
				DatabaseID: &did,
			}
			if ossStorageID != nil && *ossStorageID > 0 {
				task.OSSStorageID = ossStorageID
			}
			if err := s.Create(task); err != nil {
				return res, err
			}
			res.Created++
		}
	case "panel":
		var count int64
		s.db.Model(&models.BackupTask{}).Where("type = ?", "panel").Count(&count)
		if count > 0 {
			res.Skipped++
			break
		}
		task := &models.BackupTask{
			Name:         "面板云备份",
			Type:         "panel",
			Schedule:     schedule,
			Enabled:      true,
			KeepCount:    5,
			OSSStorageID: ossStorageID,
		}
		if err := s.Create(task); err != nil {
			return res, err
		}
		res.Created++
	default:
		return nil, fmt.Errorf("unknown preset: %s", preset)
	}
	return res, nil
}
