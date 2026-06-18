package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	dbsvc "github.com/luuuunet/owpanel/internal/services/database"
	"github.com/luuuunet/owpanel/internal/services/ossstorage"
)

func (s *Service) SetDeps(database *dbsvc.Service, oss *ossstorage.Service) {
	s.database = database
	s.oss = oss
}

func (s *Service) RunTask(id uint) error {
	var task models.BackupTask
	if err := s.db.First(&task, id).Error; err != nil {
		return err
	}
	return s.runTask(&task)
}

func (s *Service) RunDueBackupTasks() int {
	var tasks []models.BackupTask
	if err := s.db.Where("enabled = ?", true).Find(&tasks).Error; err != nil {
		return 0
	}
	now := time.Now()
	n := 0
	for i := range tasks {
		if !cronDueNow(tasks[i].Schedule, tasks[i].LastRun, now) {
			continue
		}
		if err := s.runTask(&tasks[i]); err == nil {
			n++
		}
	}
	return n
}

func (s *Service) runTask(task *models.BackupTask) error {
	now := time.Now()
	s.db.Model(task).Updates(map[string]interface{}{
		"last_run": now, "last_status": "running", "last_error": "",
	})
	var localFile string
	var err error
	switch strings.ToLower(strings.TrimSpace(task.Type)) {
	case "website":
		localFile, err = s.runWebsiteTask(task)
	case "database":
		localFile, err = s.runDatabaseTask(task)
	case "directory":
		localFile, err = s.runDirectoryTask(task)
	default:
		err = fmt.Errorf("unsupported backup type: %s", task.Type)
	}
	if err != nil {
		s.db.Model(task).Updates(map[string]interface{}{
			"last_status": "failed",
			"last_error":  err.Error(),
		})
		return err
	}
	if err := s.uploadTaskOutputs(task, localFile); err != nil {
		s.db.Model(task).Updates(map[string]interface{}{
			"last_status": "partial",
			"last_error":  err.Error(),
		})
		return err
	}
	s.db.Model(task).Updates(map[string]interface{}{
		"last_status": "success",
		"last_error":  "",
	})
	return nil
}

func (s *Service) runWebsiteTask(task *models.BackupTask) (string, error) {
	var websiteID uint
	if task.WebsiteID != nil && *task.WebsiteID > 0 {
		websiteID = *task.WebsiteID
	} else if task.Target != "" {
		if id, err := strconv.ParseUint(task.Target, 10, 64); err == nil {
			websiteID = uint(id)
		} else {
			var site models.Website
			if err := s.db.Where("domain = ?", task.Target).First(&site).Error; err != nil {
				return "", fmt.Errorf("website not found: %s", task.Target)
			}
			websiteID = site.ID
		}
	} else {
		return "", fmt.Errorf("website_id or target required")
	}
	rec, err := s.RunWebsiteBackup(websiteID, task.RemoteID)
	if err != nil {
		return "", err
	}
	return rec.FilePath, nil
}

func (s *Service) runDatabaseTask(task *models.BackupTask) (string, error) {
	if s.database == nil {
		return "", fmt.Errorf("database service not configured")
	}
	var dbID uint
	if task.DatabaseID != nil && *task.DatabaseID > 0 {
		dbID = *task.DatabaseID
	} else if task.Target != "" {
		id, err := strconv.ParseUint(task.Target, 10, 64)
		if err != nil {
			var inst models.DatabaseInstance
			if err := s.db.Where("name = ?", task.Target).First(&inst).Error; err != nil {
				return "", fmt.Errorf("database not found: %s", task.Target)
			}
			dbID = inst.ID
		} else {
			dbID = uint(id)
		}
	} else {
		return "", fmt.Errorf("database_id or target required")
	}
	rec, err := s.database.RunBackup(dbID)
	if err != nil {
		return "", err
	}
	return rec.FilePath, nil
}

func (s *Service) runDirectoryTask(task *models.BackupTask) (string, error) {
	src := strings.TrimSpace(task.Target)
	if src == "" {
		return "", fmt.Errorf("directory target required")
	}
	if !filepath.IsAbs(src) {
		src = filepath.Join(s.dataDir, src)
	}
	src = filepath.Clean(src)
	info, err := os.Stat(src)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("directory not found: %s", src)
	}
	ts := time.Now().Format("20060102-150405")
	dir := filepath.Join(s.backupBaseDir(), "directories")
	_ = os.MkdirAll(dir, 0755)
	name := sanitizeName(filepath.Base(src))
	if name == "" || name == "." {
		name = "dir"
	}
	dest := filepath.Join(dir, fmt.Sprintf("%s-%s.zip", name, ts))
	if _, err := zipDirectory(src, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func (s *Service) uploadTaskOutputs(task *models.BackupTask, localFile string) error {
	if localFile == "" {
		return nil
	}
	var errs []string
	if task.RemoteID != nil && *task.RemoteID > 0 {
		if err := s.uploadToRemote(*task.RemoteID, localFile, filepath.Base(localFile)); err != nil {
			errs = append(errs, "remote: "+err.Error())
		}
	}
	if task.OSSStorageID != nil && *task.OSSStorageID > 0 {
		if err := s.uploadToOSS(*task.OSSStorageID, localFile, filepath.Base(localFile)); err != nil {
			errs = append(errs, "oss: "+err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, "; "))
	}
	return nil
}

func (s *Service) uploadToOSS(storageID uint, localFile, remoteName string) error {
	if s.oss == nil {
		return fmt.Errorf("oss service not configured")
	}
	return s.oss.UploadFile(storageID, localFile, "backups/"+remoteName)
}
