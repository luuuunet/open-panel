package wordpress

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

type ProvisionResult struct {
	FtpUser     string `json:"ftp_user,omitempty"`
	FtpPassword string `json:"ftp_password,omitempty"`
	DbName      string `json:"db_name,omitempty"`
	DbUser      string `json:"db_user,omitempty"`
	DbPassword  string `json:"db_password,omitempty"`
}

func (s *Service) ensureFTP(site *models.WordPressSite, logger *DeployLogger) (user, pass string, err error) {
	if s.ftp == nil {
		return "", "", fmt.Errorf("FTP 服务未就绪")
	}
	root := strings.TrimSpace(site.RootPath)
	if root == "" {
		return "", "", fmt.Errorf("网站根目录为空")
	}
	root = filepath.Clean(root)

	if site.WebsiteID > 0 {
		var w models.Website
		if s.db.First(&w, site.WebsiteID).Error == nil && w.FtpUser != "" {
			user = w.FtpUser
			pass = s.ftp.GetPassword(user)
			if pass == "" {
				pass = randomPassword(16)
				if err := s.ftp.SetPassword(user, pass); err != nil {
					return "", "", err
				}
			}
			if logger != nil {
				logger.Info("✓ FTP 账号已就绪: " + user)
			}
			return user, pass, nil
		}
	}

	var acc models.FTPAccount
	if err := s.findFTPAccount(root, sanitizeFtpName(site.Domain), &acc); err == nil {
		pass = s.ftp.GetPassword(acc.Username)
		if pass == "" {
			pass = randomPassword(16)
			if err := s.ftp.SetPassword(acc.Username, pass); err != nil {
				return "", "", err
			}
		}
		if site.WebsiteID > 0 {
			_ = s.db.Model(&models.Website{}).Where("id = ?", site.WebsiteID).Update("ftp_user", acc.Username).Error
		}
		if logger != nil {
			logger.Info("✓ FTP 账号已绑定: " + acc.Username)
		}
		return acc.Username, pass, nil
	}

	user = sanitizeFtpName(site.Domain)
	pass = randomPassword(16)
	acc = models.FTPAccount{Username: user, Path: root}
	if err := s.ftp.Create(&acc, pass); err != nil {
		if s.findFTPAccount(root, user, &acc) == nil {
			pass = s.ftp.GetPassword(acc.Username)
			if pass == "" {
				pass = randomPassword(16)
				if err := s.ftp.SetPassword(acc.Username, pass); err != nil {
					return "", "", err
				}
			}
		} else {
			return "", "", err
		}
	}
	if site.WebsiteID > 0 {
		_ = s.db.Model(&models.Website{}).Where("id = ?", site.WebsiteID).Update("ftp_user", user).Error
	}
	if logger != nil {
		logger.Info("✓ FTP 账号已创建: " + user)
	}
	return user, pass, nil
}

func (s *Service) findFTPAccount(root, username string, acc *models.FTPAccount) error {
	if err := s.db.Where("path = ?", root).First(acc).Error; err == nil {
		return nil
	}
	if err := s.db.Unscoped().Where("username = ?", username).First(acc).Error; err != nil {
		return err
	}
	updates := map[string]interface{}{"path": root, "deleted_at": nil}
	return s.db.Unscoped().Model(acc).Updates(updates).Error
}

func sanitizeFtpName(domain string) string {
	s := strings.NewReplacer(".", "_", "-", "_", ":", "_", "*", "w").Replace(domain)
	if len(s) > 48 {
		s = s[:48]
	}
	if s == "" {
		s = "site"
	}
	return s
}

func randomPassword(n int) string {
	b := make([]byte, (n+1)/2)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}
