package wordpress

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/appstore"
	dbsvc "github.com/luuuunet/owpanel/internal/services/database"
	"github.com/luuuunet/owpanel/internal/services/domaincheck"
	ftpsvc "github.com/luuuunet/owpanel/internal/services/ftp"
	"github.com/luuuunet/owpanel/internal/services/sitepurge"
	"gorm.io/gorm"
)

type Service struct {
	db       *gorm.DB
	dataDir  string
	appstore *appstore.Service
	database *dbsvc.Service
	ftp      *ftpsvc.Service
}

func NewService(db *gorm.DB, dataDir string, appstore *appstore.Service, database *dbsvc.Service, ftp *ftpsvc.Service) *Service {
	return &Service{db: db, dataDir: dataDir, appstore: appstore, database: database, ftp: ftp}
}

func (s *Service) List() ([]models.WordPressSite, error) {
	var list []models.WordPressSite
	if err := s.db.Order("id desc").Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		s.fillRuntime(&list[i])
		s.loadDomains(&list[i])
	}
	return list, nil
}

func (s *Service) Get(id uint) (*models.WordPressSite, error) {
	var site models.WordPressSite
	if err := s.db.First(&site, id).Error; err != nil {
		return nil, err
	}
	s.fillRuntime(&site)
	s.loadDomains(&site)
	return &site, nil
}

type CreateRequest struct {
	models.WordPressSite
	ExtraDomains []string `json:"extra_domains"`
	DomainsText  string   `json:"domains_text"`
	DatabaseMode string   `json:"database_mode"` // auto | custom | existing | skip
	DatabaseID   uint     `json:"database_id"`
	DbName       string   `json:"db_name"`
	DbUser       string   `json:"db_user"`
	DbPassword   string   `json:"db_password"`
	DbHost       string   `json:"db_host"`
	DbPort       int      `json:"db_port"`
}

func (s *Service) CreateWithDomains(req *CreateRequest) error {
	site, extras, err := s.prepareSite(req)
	if err != nil {
		return err
	}
	if err := domaincheck.AssertAvailable(s.db, collectAllDomains(site.Domain, extras), domaincheck.Scope{}); err != nil {
		return err
	}
	site.Status = "pending"
	if !domainCanUseLetsEncrypt(site.Domain) {
		site.AutoSSL = false
	}
	if err := s.db.Create(site).Error; err != nil {
		return err
	}
	req.ID = site.ID
	s.ensurePrimaryDomain(site)

	for _, d := range extras {
		d = normalizeDomain(d)
		if d == "" || strings.EqualFold(d, site.Domain) {
			continue
		}
		_ = s.db.Create(&models.WordPressDomain{
			SiteID: site.ID, Domain: d, Type: "alias", Enabled: true,
		}).Error
	}

	if _, err := s.provision(site, databaseOptionsFromRequest(req)); err != nil {
		s.db.Model(site).Update("status", "error")
		return err
	}
	for _, d := range extras {
		_ = s.syncWebsiteAlias(site, normalizeDomain(d))
	}
	return s.regenerateVhost(site.ID)
}

func (s *Service) prepareSite(req *CreateRequest) (*models.WordPressSite, []string, error) {
	site := &req.WordPressSite
	if strings.TrimSpace(site.Domain) == "" {
		return nil, nil, fmt.Errorf("domain is required")
	}
	site.Domain = normalizeDomain(site.Domain)
	if site.Version == "" {
		site.Version = "6.7"
	}
	if site.PhpVersion == "" {
		site.PhpVersion = s.defaultPHP()
	}
	site.NginxVersion = s.detectNginxVersion()
	site.RootPath = s.resolveRoot(site.Path, site.Domain)
	if site.CloudflareCDN {
		site.AutoSSL = false
	}

	extras := req.ExtraDomains
	if len(extras) == 0 && req.DomainsText != "" {
		extras = parseDomainList(req.DomainsText)
	}
	return site, extras, nil
}

func (s *Service) provisionWithLog(site *models.WordPressSite, dbOpts DatabaseOptions, logger *DeployLogger) (*ProvisionResult, error) {
	if logger == nil {
		logger = &DeployLogger{}
	}
	result := &ProvisionResult{}
	root := site.RootPath
	logger.Info("正在创建网站目录...")
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("create site dir: %w", err)
	}
	logger.Info("✓ 目录已就绪: " + root)

	empty, _ := dirEmpty(root)
	if empty {
		logger.Info(fmt.Sprintf("正在从 wordpress.org 下载 WordPress %s ...", site.Version))
		if err := s.downloadWordPress(root, site.Version); err != nil {
			logger.Warn(fmt.Sprintf("官方包下载失败: %v", err))
			logger.Info("正在使用本地脚手架初始化站点文件...")
			if err2 := s.scaffoldWordPress(root, site); err2 != nil {
				return nil, fmt.Errorf("download wp: %v; scaffold: %w", err, err2)
			}
			logger.Info("✓ 已生成本地脚手架（可稍后手动上传完整 WordPress）")
		} else {
			logger.Info("✓ WordPress 核心文件解压完成")
		}
	} else {
		logger.Info("目录非空，跳过 WordPress 下载")
	}

	logger.Info("正在配置数据库…")
	creds, err := s.setupDatabase(site, dbOpts, logger)
	if err != nil {
		return nil, err
	}
	if creds != nil {
		result.DbName = creds.Name
		result.DbUser = creds.User
		result.DbPassword = creds.Password
	}
	if err := s.writeWPConfig(root, site.Domain, creds, logger); err != nil {
		return nil, err
	}
	if site.CloudflareCDN {
		_ = s.patchWPSiteURL(root, site.Domain, true)
	}

	logger.Info("正在生成 Nginx 虚拟主机配置...")
	nginxConf, err := s.writeNginxVhost(site)
	if err != nil {
		return nil, err
	}
	site.NginxConf = nginxConf
	logger.Info("✓ Nginx 配置: " + nginxConf)

	logger.Info("正在注册网站记录到面板...")
	websiteID, err := s.ensureWebsite(site)
	if err != nil {
		return nil, err
	}
	site.WebsiteID = websiteID
	logger.Info(fmt.Sprintf("✓ 网站 ID: %d", websiteID))

	logger.Info("正在创建 FTP 账号…")
	ftpUser, ftpPass, err := s.ensureFTP(site, logger)
	if err != nil {
		logger.Warn("FTP 创建失败: " + err.Error())
	} else {
		result.FtpUser = ftpUser
		result.FtpPassword = ftpPass
	}

	_ = s.ensureWPSiteOwnership(root, logger)
	if err := s.writeWPFilesystemConfig(root, logger); err != nil {
		logger.Warn("配置 WordPress 文件写入失败: " + err.Error())
	}

	updates := map[string]interface{}{
		"root_path":     root,
		"nginx_conf":    nginxConf,
		"nginx_version": site.NginxVersion,
		"php_version":   site.PhpVersion,
		"website_id":    websiteID,
		"status":        "running",
	}
	if creds != nil {
		updates["database_id"] = creds.InstanceID
		updates["db_name"] = creds.Name
		updates["db_user"] = creds.User
		updates["db_host"] = creds.Host
		updates["db_port"] = creds.Port
	}
	if err := s.db.Model(site).Updates(updates).Error; err != nil {
		return nil, err
	}
	logger.Info("✓ 站点状态已更新为 running")
	return result, nil
}

func (s *Service) Create(site *models.WordPressSite) error {
	return s.CreateWithDomains(&CreateRequest{WordPressSite: *site})
}

func (s *Service) Repair(id uint) error {
	site, err := s.Get(id)
	if err != nil {
		return err
	}
	if site.RootPath == "" {
		site.RootPath = s.resolveRoot(site.Path, site.Domain)
	}
	if site.PhpVersion == "" {
		site.PhpVersion = s.defaultPHP()
	}
	site.NginxVersion = s.detectNginxVersion()
	s.db.Model(site).Updates(map[string]interface{}{
		"root_path": site.RootPath, "php_version": site.PhpVersion, "nginx_version": site.NginxVersion,
	})
	s.ensurePrimaryDomain(site)
	_, err = s.provision(site, defaultDatabaseOptions(site))
	if err != nil {
		return err
	}
	if _, ftpErr := s.EnsureFTPForSite(id); ftpErr != nil {
		// non-fatal: repair may still succeed without FTP
	}
	if site.RootPath != "" {
		_ = s.FixWPConfig(site.RootPath)
	}
	site, err = s.Get(id)
	if err != nil {
		return err
	}
	return s.applyCDNMode(site)
}

func (s *Service) EnsureFTPForSite(id uint) (*ProvisionResult, error) {
	site, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if site.WebsiteID == 0 {
		websiteID, err := s.ensureWebsite(site)
		if err != nil {
			return nil, err
		}
		site.WebsiteID = websiteID
		_ = s.db.Model(site).Update("website_id", websiteID).Error
	}
	user, pass, err := s.ensureFTP(site, nil)
	if err != nil {
		return nil, err
	}
	_ = s.ensureWPSiteOwnership(site.RootPath, nil)
	_ = s.writeWPFilesystemConfig(site.RootPath, nil)
	return &ProvisionResult{FtpUser: user, FtpPassword: pass}, nil
}

func (s *Service) ApplyDomains(siteID uint) error {
	return s.regenerateVhost(siteID)
}

func (s *Service) Delete(id uint) error {
	var site models.WordPressSite
	if err := s.db.First(&site, id).Error; err != nil {
		return err
	}
	s.loadDomains(&site)
	hosts := []string{site.Domain}
	for _, d := range site.Domains {
		hosts = append(hosts, d.Domain)
	}
	s.cleanupWPSiteResources(&site)
	sitepurge.Domains(s.db, hosts, sitepurge.Options{
		DataDir:       s.dataDir,
		RemoveWWWRoot: site.RootPath,
	})
	if site.WebsiteID > 0 {
		sitepurge.PurgeWebsiteID(s.db, site.WebsiteID)
	}
	return s.db.Delete(&models.WordPressSite{}, id).Error
}

func (s *Service) cleanupWPSiteResources(site *models.WordPressSite) {
	if site.DbName != "" {
		var inst models.DatabaseInstance
		if s.db.Where("name = ?", site.DbName).First(&inst).Error == nil {
			_ = s.database.Delete(inst.ID)
		}
	}
	if site.RootPath != "" {
		var acc models.FTPAccount
		if s.db.Where("path = ?", site.RootPath).First(&acc).Error == nil {
			_ = s.ftp.Delete(acc.ID)
		}
	}
	if site.NginxConf != "" {
		_ = os.Remove(site.NginxConf)
	}
}

func (s *Service) Backup(id uint) (*models.WordPressBackup, error) {
	return s.RunBackup(id)
}

func (s *Service) provision(site *models.WordPressSite, dbOpts DatabaseOptions) (*ProvisionResult, error) {
	return s.provisionWithLog(site, dbOpts, nil)
}

func (s *Service) ensureWebsite(site *models.WordPressSite) (uint, error) {
	if site.WebsiteID > 0 {
		s.syncWebsiteWordPressFields(site.WebsiteID, site)
		return site.WebsiteID, nil
	}
	host := domaincheck.HostOnly(site.Domain)
	var existing models.Website
	if s.db.Where("domain = ?", host).First(&existing).Error == nil {
		s.syncWebsiteWordPressFields(existing.ID, site)
		var prim models.WebsiteAlias
		if s.db.Where("website_id = ? AND type = ?", existing.ID, "primary").First(&prim).Error != nil {
			_ = s.db.Create(&models.WebsiteAlias{
				WebsiteID: existing.ID, Domain: host, Port: 80, Type: "primary",
			}).Error
		}
		return existing.ID, nil
	}
	if err := domaincheck.AssertAvailable(s.db, []string{host}, domaincheck.Scope{IgnoreWPSiteID: site.ID}); err != nil {
		return 0, err
	}
	phpVer := site.PhpVersion
	if phpVer == "" {
		phpVer = s.defaultPHP()
	}
	w := models.Website{
		Domain:       host,
		RootPath:     site.RootPath,
		ProjectType:  "wordpress",
		PHP:          true,
		PhpVersion:   phpVer,
		RewriteRules: WordPressRewriteRules,
		SSL:          false,
		Status:       "running",
		Remark:       "WordPress: " + site.Version,
	}
	if err := s.db.Create(&w).Error; err != nil {
		return 0, err
	}
	_ = s.db.Create(&models.WebsiteAlias{
		WebsiteID: w.ID, Domain: host, Port: 80, Type: "primary",
	}).Error
	return w.ID, nil
}

func (s *Service) syncWebsiteWordPressFields(websiteID uint, site *models.WordPressSite) {
	phpVer := site.PhpVersion
	if phpVer == "" {
		phpVer = s.defaultPHP()
	}
	var w models.Website
	if s.db.First(&w, websiteID).Error != nil {
		return
	}
	updates := map[string]interface{}{}
	if w.PhpVersion == "" || w.PhpVersion == "static" {
		updates["php_version"] = phpVer
		updates["php"] = true
	}
	if strings.TrimSpace(w.RewriteRules) == "" {
		updates["rewrite_rules"] = WordPressRewriteRules
	}
	if w.ProjectType == "" || w.ProjectType == "php" {
		updates["project_type"] = "wordpress"
	}
	if w.RootPath == "" && site.RootPath != "" {
		updates["root_path"] = site.RootPath
	}
	if len(updates) > 0 {
		_ = s.db.Model(&w).Updates(updates).Error
	}
}

func (s *Service) resolveRoot(basePath, domain string) string {
	domain = strings.TrimSpace(domain)
	basePath = strings.TrimSpace(basePath)
	basePath = strings.ReplaceAll(basePath, "/", string(filepath.Separator))

	isLegacy := basePath == "" || basePath == string(filepath.Separator) ||
		strings.EqualFold(strings.TrimSuffix(basePath, string(filepath.Separator)), "wwwroot") ||
		strings.EqualFold(basePath, filepath.Join(string(filepath.Separator), "www", "wwwroot")) ||
		strings.EqualFold(basePath, filepath.Join("www", "wwwroot"))

	if isLegacy {
		basePath = filepath.Join(s.dataDir, "wwwroot")
	} else if !filepath.IsAbs(basePath) {
		basePath = filepath.Join(s.dataDir, basePath)
	}

	if !strings.EqualFold(filepath.Base(basePath), domain) {
		basePath = filepath.Join(basePath, domain)
	}
	return filepath.Clean(basePath)
}

func (s *Service) fillRuntime(site *models.WordPressSite) {
	if site.RootPath == "" {
		site.RootPath = s.resolveRoot(site.Path, site.Domain)
	}
	if abs, err := filepath.Abs(site.RootPath); err == nil {
		site.RootPath = abs
	}
	if site.PhpVersion == "" {
		site.PhpVersion = s.defaultPHP()
	}
	if site.NginxVersion == "" {
		site.NginxVersion = s.detectNginxVersion()
	}
	if site.Status == "" {
		site.Status = "pending"
	}
	s.syncSSLStatus(site)
	if _, err := os.Stat(site.RootPath); err != nil && site.Status == "running" {
		site.Status = "error"
	}
}

func (s *Service) defaultPHP() string {
	apps, _ := s.appstore.ListInstalled()
	for _, a := range apps {
		if strings.HasPrefix(a.Key, "php") && a.Installed {
			return a.Version
		}
	}
	return "8.3"
}

func (s *Service) detectNginxVersion() string {
	apps, _ := s.appstore.ListInstalled()
	for _, a := range apps {
		if a.Key == "nginx" && a.Installed {
			if a.Version != "" {
				return a.Version
			}
			return "installed"
		}
	}
	if out, err := execNginxVersion(); err == nil {
		return out
	}
	return "未安装"
}

func (s *Service) phpFastCGIPort(version string) int {
	m := map[string]int{"8.3": 9000, "8.2": 9001, "8.1": 9002, "7.4": 9003}
	if p, ok := m[version]; ok {
		return p
	}
	return 9000
}

func dirEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func (s *Service) downloadWordPress(root, version string) error {
	url := fmt.Sprintf("https://wordpress.org/wordpress-%s.zip", version)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wordpress download HTTP %d", resp.StatusCode)
	}
	tmpZip := filepath.Join(os.TempDir(), fmt.Sprintf("wp-%s.zip", version))
	f, err := os.Create(tmpZip)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	f.Close()
	defer os.Remove(tmpZip)

	r, err := zip.OpenReader(tmpZip)
	if err != nil {
		return err
	}
	defer r.Close()

	prefix := "wordpress/"
	for _, zf := range r.File {
		name := strings.TrimPrefix(zf.Name, prefix)
		if name == "" {
			continue
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		if zf.FileInfo().IsDir() {
			_ = os.MkdirAll(target, 0755)
			continue
		}
		_ = os.MkdirAll(filepath.Dir(target), 0755)
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) scaffoldWordPress(root string, site *models.WordPressSite) error {
	index := fmt.Sprintf(`<?php
/**
 * OWPanel WordPress site — %s
 * Complete install: visit http://%s/wp-admin/install.php
 */
define('WP_USE_THEMES', true);
require __DIR__ . '/wp-load.php';
`, site.Domain, site.Domain)
	_ = os.WriteFile(filepath.Join(root, "index.php"), []byte(index), 0644)
	_ = os.MkdirAll(filepath.Join(root, "wp-content", "uploads"), 0755)
	readme := fmt.Sprintf("WordPress site %s\nRoot: %s\nPHP: %s\nNginx: %s\n", site.Domain, root, site.PhpVersion, site.NginxVersion)
	return os.WriteFile(filepath.Join(root, "owpanel-readme.txt"), []byte(readme), 0644)
}
