package website

import (
	"crypto/md5"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

func (s *Service) ListSubdirs(siteID uint) ([]models.WebsiteSubdir, error) {
	var list []models.WebsiteSubdir
	return list, s.db.Where("website_id = ?", siteID).Order("prefix").Find(&list).Error
}

type SubdirRequest struct {
	Prefix   string `json:"prefix"`
	RootPath string `json:"root_path"`
	Remark   string `json:"remark"`
}

func (s *Service) AddSubdir(siteID uint, req *SubdirRequest) (*models.WebsiteSubdir, error) {
	prefix := normalizeSubdirPrefix(req.Prefix)
	root := strings.TrimSpace(req.RootPath)
	if prefix == "" || root == "" {
		return nil, fmt.Errorf("子目录与目录路径不能为空")
	}
	sub := models.WebsiteSubdir{
		WebsiteID: siteID,
		Prefix:    prefix,
		RootPath:  root,
		Remark:    strings.TrimSpace(req.Remark),
	}
	if err := s.db.Create(&sub).Error; err != nil {
		return nil, err
	}
	if _, err := s.regenAfterFeature(siteID); err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Service) UpdateSubdir(siteID, subID uint, req *SubdirRequest) (*models.WebsiteSubdir, error) {
	var sub models.WebsiteSubdir
	if err := s.db.Where("id = ? AND website_id = ?", subID, siteID).First(&sub).Error; err != nil {
		return nil, err
	}
	prefix := normalizeSubdirPrefix(req.Prefix)
	root := strings.TrimSpace(req.RootPath)
	if prefix == "" || root == "" {
		return nil, fmt.Errorf("子目录与目录路径不能为空")
	}
	sub.Prefix = prefix
	sub.RootPath = root
	sub.Remark = strings.TrimSpace(req.Remark)
	if err := s.db.Save(&sub).Error; err != nil {
		return nil, err
	}
	if _, err := s.regenAfterFeature(siteID); err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Service) DeleteSubdir(siteID, subID uint) error {
	if err := s.db.Where("id = ? AND website_id = ?", subID, siteID).Delete(&models.WebsiteSubdir{}).Error; err != nil {
		return err
	}
	_, err := s.regenAfterFeature(siteID)
	return err
}

func normalizeSubdirPrefix(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, "/")
	if p == "" {
		return ""
	}
	return "/" + p
}

func (s *Service) regenAfterFeature(siteID uint) (*models.Website, error) {
	site, err := s.Get(siteID)
	if err != nil {
		return nil, err
	}
	if err := s.regenerateLimitZones(); err != nil {
		return nil, err
	}
	if err := s.regenerateVhost(site); err != nil {
		return nil, err
	}
	return s.Get(siteID)
}

func (s *Service) htpasswdPath(domain string) string {
	dir := filepath.Join(s.dataDir, "nginx", "auth")
	_ = os.MkdirAll(dir, 0755)
	safe := strings.ReplaceAll(domain, ":", "_")
	return filepath.Join(dir, safe+".htpasswd")
}

func (s *Service) ensureHtpasswd(site *models.Website) (string, error) {
	if !site.AccessAuthEnabled {
		return "", nil
	}
	user := strings.TrimSpace(site.AccessAuthUser)
	pass := strings.TrimSpace(site.AccessAuthPass)
	if user == "" || pass == "" {
		return "", fmt.Errorf("启用认证时需填写用户名和密码")
	}
	path := s.htpasswdPath(site.Domain)
	hash, err := apr1Hash(pass)
	if err != nil {
		return "", err
	}
	line := user + ":" + hash + "\n"
	if err := os.WriteFile(path, []byte(line), 0600); err != nil {
		return "", err
	}
	return filepath.ToSlash(path), nil
}

func apr1Hash(password string) (string, error) {
	if out, err := exec.Command("openssl", "passwd", "-apr1", password).Output(); err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	// fallback: md5-crypt style placeholder when openssl unavailable
	sum := md5.Sum([]byte(password))
	return fmt.Sprintf("$apr1$%x$%x", sum[:4], sum), nil
}

func siteLimitZone(domain string) string {
	sum := md5.Sum([]byte(strings.ToLower(domain)))
	return fmt.Sprintf("op_lr_%x", sum[:4])
}

func (s *Service) limitsConfPath() string {
	return filepath.Join(s.dataDir, "nginx", "owpanel-limits.conf")
}

func (s *Service) regenerateLimitZones() error {
	var sites []models.Website
	if err := s.db.Where("traffic_limit_enabled = ?", true).Find(&sites).Error; err != nil {
		return err
	}
	dir := filepath.Join(s.dataDir, "nginx")
	_ = os.MkdirAll(dir, 0755)
	var b strings.Builder
	b.WriteString("# OWPanel site rate limits — include in nginx http {}\n")
	if len(sites) == 0 {
		b.WriteString("# (no sites with traffic limit enabled)\n")
	} else {
		for _, site := range sites {
			rate := strings.TrimSpace(site.TrafficRate)
			if rate == "" {
				rate = "10r/s"
			}
			zone := siteLimitZone(site.Domain)
			b.WriteString(fmt.Sprintf("limit_req_zone $binary_remote_addr zone=%s:10m rate=%s;\n", zone, rate))
		}
	}
	return os.WriteFile(s.limitsConfPath(), []byte(b.String()), 0644)
}

func (s *Service) loadSubdirs(siteID uint) []models.WebsiteSubdir {
	var subs []models.WebsiteSubdir
	_ = s.db.Where("website_id = ?", siteID).Order("prefix").Find(&subs).Error
	return subs
}
