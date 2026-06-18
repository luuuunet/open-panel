package wordpress

import (
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

func (s *Service) GetSiteCredentials(id uint) (*ProvisionResult, error) {
	site, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	result := &ProvisionResult{}
	s.fillFTPCredentials(site, result)
	s.fillDBCredentials(site, result)
	return result, nil
}

func (s *Service) fillFTPCredentials(site *models.WordPressSite, result *ProvisionResult) {
	if s.ftp == nil {
		return
	}
	var username string
	if site.WebsiteID > 0 {
		var w models.Website
		if s.db.First(&w, site.WebsiteID).Error == nil && w.FtpUser != "" {
			username = w.FtpUser
		}
	}
	if username == "" && strings.TrimSpace(site.RootPath) != "" {
		root := filepath.Clean(site.RootPath)
		var acc models.FTPAccount
		if s.db.Where("path = ?", root).First(&acc).Error == nil {
			username = acc.Username
		}
	}
	if username == "" {
		return
	}
	result.FtpUser = username
	result.FtpPassword = s.ftp.GetPassword(username)
}

func (s *Service) fillDBCredentials(site *models.WordPressSite, result *ProvisionResult) {
	if site.DatabaseID > 0 && s.database != nil {
		user, pass, _, _, err := s.database.GetCredentials(site.DatabaseID)
		if err == nil {
			if inst, err := s.database.Get(site.DatabaseID); err == nil {
				result.DbName = inst.Name
				result.DbUser = user
				result.DbPassword = pass
				return
			}
		}
	}
	if site.DbName != "" {
		var inst models.DatabaseInstance
		if s.db.Where("name = ?", site.DbName).First(&inst).Error == nil {
			result.DbName = inst.Name
			result.DbUser = inst.Username
			if s.database != nil && inst.ID > 0 {
				if user, pass, _, _, err := s.database.GetCredentials(inst.ID); err == nil {
					result.DbUser = user
					result.DbPassword = pass
				}
			} else if inst.Password != "" {
				result.DbPassword = inst.Password
			}
		} else {
			result.DbName = site.DbName
			result.DbUser = site.DbUser
		}
		if result.DbUser == "" {
			result.DbUser = site.DbUser
		}
		return
	}
	cfgPath := filepath.Join(site.RootPath, "wp-config.php")
	if conn, err := parseWPConfig(cfgPath); err == nil {
		result.DbName = conn.DBName
		result.DbUser = conn.Username
		result.DbPassword = conn.Password
	}
}
