package wordpress

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

type UpdateRequest struct {
	RootPath      string `json:"root_path"`
	Path          string `json:"path"`
	PhpVersion    string `json:"php_version"`
	Version       string `json:"version"`
	Remark        string `json:"remark"`
	AutoSSL       *bool  `json:"auto_ssl"`
	SSLEmail      string `json:"ssl_email"`
	CloudflareCDN *bool  `json:"cloudflare_cdn"`
}

func (s *Service) Update(id uint, req *UpdateRequest) (*models.WordPressSite, error) {
	site, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	updates := map[string]interface{}{}

	if rp := strings.TrimSpace(req.RootPath); rp != "" {
		rp = filepath.Clean(strings.ReplaceAll(rp, "/", string(filepath.Separator)))
		if !filepath.IsAbs(rp) {
			rp = filepath.Join(s.dataDir, rp)
		}
		rp = filepath.Clean(rp)
		if err := os.MkdirAll(rp, 0755); err != nil {
			return nil, fmt.Errorf("创建目录失败: %w", err)
		}
		updates["root_path"] = rp
		site.RootPath = rp
	}

	if pv := strings.TrimSpace(req.PhpVersion); pv != "" {
		updates["php_version"] = pv
		site.PhpVersion = pv
	}
	if v := strings.TrimSpace(req.Version); v != "" {
		updates["version"] = v
		site.Version = v
	}
	if req.Path != "" {
		updates["path"] = strings.TrimSpace(req.Path)
	}
	updates["remark"] = strings.TrimSpace(req.Remark)
	if req.AutoSSL != nil {
		updates["auto_ssl"] = *req.AutoSSL
		site.AutoSSL = *req.AutoSSL
	}
	if req.SSLEmail != "" {
		updates["ssl_email"] = strings.TrimSpace(req.SSLEmail)
		site.SSLEmail = strings.TrimSpace(req.SSLEmail)
	}
	cdnChanged := false
	if req.CloudflareCDN != nil {
		updates["cloudflare_cdn"] = *req.CloudflareCDN
		site.CloudflareCDN = *req.CloudflareCDN
		cdnChanged = true
		if *req.CloudflareCDN {
			updates["auto_ssl"] = false
			site.AutoSSL = false
		}
	}

	if len(updates) == 0 {
		return site, nil
	}

	if err := s.db.Model(site).Updates(updates).Error; err != nil {
		return nil, err
	}

	if site.WebsiteID > 0 {
		webUpdates := map[string]interface{}{}
		if _, ok := updates["root_path"]; ok {
			webUpdates["root_path"] = site.RootPath
		}
		if _, ok := updates["php_version"]; ok {
			webUpdates["php_version"] = site.PhpVersion
		}
		if _, ok := updates["remark"]; ok {
			webUpdates["remark"] = strings.TrimSpace(req.Remark)
		}
		if len(webUpdates) > 0 {
			_ = s.db.Model(&models.Website{}).Where("id = ?", site.WebsiteID).Updates(webUpdates).Error
		}
	}

	needVhost := false
	if _, ok := updates["root_path"]; ok {
		needVhost = true
	}
	if _, ok := updates["php_version"]; ok {
		needVhost = true
	}

	if cdnChanged {
		if err := s.applyCDNMode(site); err != nil {
			return nil, err
		}
	} else if needVhost {
		if err := s.regenerateVhost(id); err != nil {
			return nil, err
		}
		_ = reloadNginxIfAvailable()
	}

	return s.Get(id)
}
