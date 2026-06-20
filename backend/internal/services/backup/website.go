package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

type WebsiteBackupConfig struct {
	AutoEnabled bool   `json:"auto_enabled"`
	Schedule    string `json:"schedule"`
	KeepCount   int    `json:"keep_count"`
	RemoteID    *uint  `json:"remote_id"`
	BackupDir   string `json:"backup_dir"`
}

type WebsiteBackupSummary struct {
	Count      int        `json:"count"`
	LastAt     *time.Time `json:"last_at"`
	LastPath   string     `json:"last_path"`
	StatusLabel string    `json:"status_label"`
}

func (s *Service) WebsiteBackupSummary(websiteID uint) WebsiteBackupSummary {
	var count int64
	s.db.Model(&models.WebsiteBackup{}).Where("website_id = ?", websiteID).Count(&count)
	var last models.WebsiteBackup
	err := s.db.Where("website_id = ?", websiteID).Order("id desc").First(&last).Error
	sum := WebsiteBackupSummary{Count: int(count)}
	if err == nil {
		t := last.CreatedAt
		sum.LastAt = &t
		sum.LastPath = last.FilePath
		sum.StatusLabel = fmt.Sprintf("%d份", count)
	} else {
		sum.StatusLabel = "none"
	}
	return sum
}

func (s *Service) ListWebsiteBackups(websiteID uint) ([]models.WebsiteBackup, error) {
	var list []models.WebsiteBackup
	err := s.db.Where("website_id = ?", websiteID).Order("id desc").Find(&list).Error
	return list, err
}

func (s *Service) GetWebsiteBackupConfig(websiteID uint) (*WebsiteBackupConfig, error) {
	var site models.Website
	if err := s.db.First(&site, websiteID).Error; err != nil {
		return nil, err
	}
	dir := filepath.Join(s.backupBaseDir(), "sites", sanitizeName(site.Domain))
	return &WebsiteBackupConfig{
		AutoEnabled: site.BackupAutoEnabled,
		Schedule:    site.BackupSchedule,
		KeepCount:   site.BackupKeepCount,
		RemoteID:    site.BackupRemoteID,
		BackupDir:   dir,
	}, nil
}

func (s *Service) UpdateWebsiteBackupConfig(websiteID uint, cfg WebsiteBackupConfig) error {
	updates := map[string]interface{}{
		"backup_auto_enabled": cfg.AutoEnabled,
		"backup_schedule":     cfg.Schedule,
		"backup_keep_count":   cfg.KeepCount,
		"backup_remote_id":    cfg.RemoteID,
	}
	return s.db.Model(&models.Website{}).Where("id = ?", websiteID).Updates(updates).Error
}

func (s *Service) RunWebsiteBackup(websiteID uint, remoteID *uint) (*models.WebsiteBackup, error) {
	var site models.Website
	if err := s.db.First(&site, websiteID).Error; err != nil {
		return nil, err
	}
	if strings.TrimSpace(site.RootPath) == "" {
		return nil, fmt.Errorf("站点根目录为空")
	}
	root := site.RootPath
	if !filepath.IsAbs(root) {
		root = filepath.Join(s.dataDir, root)
	}
	root = filepath.Clean(root)

	ts := time.Now().Format("20060102-150405")
	dir := filepath.Join(s.backupBaseDir(), "sites", sanitizeName(site.Domain))
	_ = os.MkdirAll(dir, 0755)
	filename := fmt.Sprintf("%s-%s.zip", sanitizeName(site.Domain), ts)
	dest := filepath.Join(dir, filename)

	size, err := zipDirectory(root, dest)
	if err != nil {
		return nil, err
	}

	useRemote := remoteID
	if useRemote == nil {
		useRemote = site.BackupRemoteID
	}

	rec := &models.WebsiteBackup{
		WebsiteID:    site.ID,
		Domain:       site.Domain,
		FilePath:     dest,
		Size:         size,
		Status:       "done",
		RemoteStatus: "none",
		RemoteID:     useRemote,
	}
	if err := s.db.Create(rec).Error; err != nil {
		_ = os.Remove(dest)
		return nil, err
	}

	if useRemote != nil && *useRemote > 0 {
		if err := s.uploadToRemote(*useRemote, dest, filepath.Base(dest)); err != nil {
			rec.RemoteStatus = "failed"
			rec.RemoteError = err.Error()
		} else {
			rec.RemoteStatus = "synced"
		}
		_ = s.db.Save(rec).Error
		var remote models.BackupRemote
		if s.db.First(&remote, *useRemote).Error == nil && remote.OSSStorageID != nil && s.oss != nil {
			key, err := s.uploadToOSSWithKey(*remote.OSSStorageID, dest, filepath.Base(dest), "backups")
			if err != nil {
				rec.RemoteError = strings.TrimSpace(rec.RemoteError + "; oss: " + err.Error())
				_ = s.db.Save(rec).Error
			} else {
				rec.RemoteKey = key
				rec.OSSStorageID = remote.OSSStorageID
				_ = s.db.Save(rec).Error
			}
		}
	}

	s.refreshWebsiteBackupStatus(&site)
	s.pruneOldBackups(site.ID, site.BackupKeepCount)
	return rec, nil
}

func (s *Service) DeleteWebsiteBackup(websiteID, backupID uint) error {
	var rec models.WebsiteBackup
	if err := s.db.Where("id = ? AND website_id = ?", backupID, websiteID).First(&rec).Error; err != nil {
		return err
	}
	if rec.FilePath != "" {
		_ = os.Remove(rec.FilePath)
	}
	if rec.RemoteKey != "" && rec.OSSStorageID != nil && *rec.OSSStorageID > 0 {
		s.deleteOSSObject(*rec.OSSStorageID, rec.RemoteKey)
	}
	if err := s.db.Delete(&rec).Error; err != nil {
		return err
	}
	var site models.Website
	if s.db.First(&site, websiteID).Error == nil {
		s.refreshWebsiteBackupStatus(&site)
	}
	return nil
}

func (s *Service) refreshWebsiteBackupStatus(site *models.Website) {
	sum := s.WebsiteBackupSummary(site.ID)
	status := "none"
	if sum.Count > 0 {
		status = sum.StatusLabel
	}
	_ = s.db.Model(site).Update("backup_status", status).Error
}

func (s *Service) pruneOldBackups(websiteID uint, keep int) {
	if keep <= 0 {
		keep = 5
	}
	var list []models.WebsiteBackup
	s.db.Where("website_id = ?", websiteID).Order("id desc").Find(&list)
	if len(list) <= keep {
		return
	}
	for _, old := range list[keep:] {
		_ = s.DeleteWebsiteBackup(websiteID, old.ID)
	}
}

func (s *Service) RunDueAutoBackups() int {
	var sites []models.Website
	s.db.Where("backup_auto_enabled = ?", true).Find(&sites)
	n := 0
	for i := range sites {
		if !s.shouldAutoBackup(&sites[i]) {
			continue
		}
		if _, err := s.RunWebsiteBackup(sites[i].ID, sites[i].BackupRemoteID); err == nil {
			n++
		}
	}
	return n
}

func (s *Service) shouldAutoBackup(site *models.Website) bool {
	schedule := strings.TrimSpace(site.BackupSchedule)
	if schedule == "" {
		schedule = "0 2 * * *"
	}
	sum := s.WebsiteBackupSummary(site.ID)
	return cronDueNow(schedule, sum.LastAt, time.Now())
}

func sanitizeName(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return "site"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), ".-")
	if out == "" {
		return "site"
	}
	return out
}
