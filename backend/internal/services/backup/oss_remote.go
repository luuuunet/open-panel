package backup

import (
	"fmt"
	"strings"
)

func (s *Service) uploadToOSSKey(storageID uint, localFile, key string) error {
	if s.oss == nil {
		return nil
	}
	return s.oss.UploadFile(storageID, localFile, strings.TrimLeft(key, "/"))
}

func (s *Service) uploadToOSSWithKey(storageID uint, localFile, remoteName, prefix string) (string, error) {
	prefix = strings.Trim(prefix, "/")
	key := remoteName
	if prefix != "" {
		key = prefix + "/" + remoteName
	}
	if err := s.uploadToOSSKey(storageID, localFile, key); err != nil {
		return "", err
	}
	return key, nil
}

func (s *Service) deleteOSSObject(storageID uint, key string) {
	if s.oss == nil || storageID == 0 || strings.TrimSpace(key) == "" {
		return
	}
	_ = s.oss.DeleteObject(storageID, key)
}

func (s *Service) downloadFromOSS(storageID uint, key, localPath string) error {
	if s.oss == nil {
		return fmt.Errorf("oss service not configured")
	}
	return s.oss.DownloadFile(storageID, key, localPath)
}
