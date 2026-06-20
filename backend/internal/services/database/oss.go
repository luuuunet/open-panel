package database

import (
	"fmt"
	"path/filepath"

	"github.com/luuuunet/owpanel/internal/services/ossstorage"
)

type BackupOptions struct {
	OSSStorageID *uint
	RemoteID     *uint
}

func (s *Service) SetOSS(oss *ossstorage.Service) {
	s.oss = oss
}

func (s *Service) uploadBackupOSS(storageID uint, localFile string) (string, error) {
	if s.oss == nil || storageID == 0 {
		return "", fmt.Errorf("OSS 未配置")
	}
	key := "backups/databases/" + filepath.Base(localFile)
	return key, s.oss.UploadFile(storageID, localFile, key)
}
