package website

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/domaincheck"
	"github.com/luuuunet/owpanel/internal/services/sitepurge"
	"gorm.io/gorm"
)

type CreateRequest struct {
	DomainsText string   `json:"domains_text"`
	Domains     []string `json:"domains"`
	Description string   `json:"description"`
	RootPath    string   `json:"root_path"`
	Ftp         string   `json:"ftp"`      // none | create
	Database    string   `json:"database"` // none | mysql
	PhpVersion  string   `json:"php_version"`
	Category    string   `json:"category"`
	DnsMode     string   `json:"dns_mode"` // manual | auto
	SSL         bool     `json:"ssl"`
	ExpiresAt   string   `json:"expires_at"` // optional YYYY-MM-DD, empty = permanent
}

type CreateResult struct {
	Site        models.Website `json:"site"`
	FtpUser     string         `json:"ftp_user,omitempty"`
	FtpPassword string         `json:"ftp_password,omitempty"`
	DbName      string         `json:"db_name,omitempty"`
	DbUser      string         `json:"db_user,omitempty"`
	DbPassword  string         `json:"db_password,omitempty"`
}

type BatchCreateRequest struct {
	CreateRequest
	BatchText string `json:"batch_text"` // 管道格式：域名|根目录|FTP|数据库|PHP
}

type BatchCreateResult struct {
	Created []CreateResult `json:"created"`
	Failed  []string       `json:"failed"`
}

func (s *Service) List() ([]models.Website, error) {
	var sites []models.Website
	if err := s.db.Preload("Aliases").Order("id desc").Find(&sites).Error; err != nil {
		return nil, err
	}
	return sites, nil
}

func (s *Service) Get(id uint) (*models.Website, error) {
	var site models.Website
	if err := s.db.Preload("Aliases").Preload("Subdirs").First(&site, id).Error; err != nil {
		return nil, err
	}
	s.ensurePrimaryAlias(&site)
	s.reloadAliases(&site)
	return &site, nil
}

func (s *Service) Create(req *CreateRequest) (*CreateResult, error) {
	entries := s.parseRequestDomains(req)
	if len(entries) == 0 {
		return nil, fmt.Errorf("请输入域名")
	}
	primary := entries[0]
	aliases := entries[1:]

	allHosts := make([]string, 0, len(entries))
	for _, e := range entries {
		allHosts = append(allHosts, e.Host)
	}
	if err := domaincheck.AssertAvailable(s.db, allHosts, domaincheck.Scope{}); err != nil {
		return nil, err
	}

	root := s.resolveRoot(req.RootPath, primary.Host)
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("创建网站目录失败: %w", err)
	}
	s.ensureIndexHTML(root, req.PhpVersion)

	phpVer := strings.TrimSpace(req.PhpVersion)
	usePHP := phpVer != "" && phpVer != "static"
	if phpVer == "" {
		phpVer = "static"
	}
	projectType := "php"
	if phpVer == "static" {
		projectType = "static"
	}
	webServer := s.activeWebServer()

	site := models.Website{
		Domain:      primary.Host,
		RootPath:    root,
		ProjectType: projectType,
		WebServer:   webServer,
		PHP:         usePHP,
		PhpVersion: phpVer,
		SSL:        req.SSL,
		Port:       primary.Port,
		Status:     "running",
		Remark:     strings.TrimSpace(req.Description),
		Category:   s.normalizeCategory(req.Category),
		DnsMode:    req.DnsMode,
	}
	if site.DnsMode == "" {
		site.DnsMode = "manual"
	}
	if exp, err := ParseExpiresDate(req.ExpiresAt); err != nil {
		return nil, err
	} else if exp != nil {
		site.ExpiresAt = exp
	}
	if s.cache != nil && s.cache.ShouldEnableNewSite() {
		site.CacheEnabled = true
	}

	result := &CreateResult{}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&site).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.WebsiteAlias{
			WebsiteID: site.ID, Domain: primary.Host, Port: primary.Port, Type: "primary",
		}).Error; err != nil {
			return err
		}
		for _, a := range aliases {
			if err := tx.Create(&models.WebsiteAlias{
				WebsiteID: site.ID, Domain: a.Host, Port: a.Port, Type: "alias",
			}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	site.Aliases = s.loadAliases(site.ID)
	if err := s.applyVhost(&site); err != nil {
		log.Printf("[website] apply vhost %s: %v", site.Domain, err)
	}
	if refreshed, err := s.Get(site.ID); err == nil {
		site = *refreshed
	}

	if req.Ftp == "" {
		req.Ftp = "create"
	}
	if req.Ftp == "create" {
		user, pass, err := s.createFTP(&site)
		if err != nil {
			log.Printf("[website] create ftp %s: %v", site.Domain, err)
		} else {
			result.FtpUser = user
			result.FtpPassword = pass
			site.FtpUser = user
			_ = s.db.Model(&site).Update("ftp_user", user).Error
		}
	}

	if req.Database == "mysql" {
		dbName, dbUser, dbPass, err := s.createDatabase(&site)
		if err != nil {
			return nil, err
		}
		result.DbName = dbName
		result.DbUser = dbUser
		result.DbPassword = dbPass
		site.DbName = dbName
		_ = s.db.Model(&site).Update("db_name", dbName).Error
	}

	if req.DnsMode == "auto" {
		_ = s.autoDNS(site.Domain, aliases, site.ID)
	}

	result.Site = site
	return result, nil
}

func (s *Service) BatchCreate(req *BatchCreateRequest) (*BatchCreateResult, error) {
	text := strings.TrimSpace(req.BatchText)
	if text == "" {
		text = strings.TrimSpace(req.DomainsText)
	}

	out := &BatchCreateResult{}

	if isPipeBatchFormat(text) {
		lines, err := parsePipeBatchText(text)
		if err != nil {
			return nil, err
		}
		for _, line := range lines {
			singleReq, err := line.toCreateRequest(req.CreateRequest, s.DefaultRootBase())
			if err != nil {
				out.Failed = append(out.Failed, err.Error())
				continue
			}
			res, err := s.Create(singleReq)
			if err != nil {
				out.Failed = append(out.Failed, fmt.Sprintf("第 %d 行 (%s): %v", line.lineNum, line.domainsRaw, err))
				continue
			}
			out.Created = append(out.Created, *res)
		}
	} else {
		domainLines := parseDomainList(text)
		if len(domainLines) == 0 {
			return nil, fmt.Errorf("请输入域名，每行一个")
		}
		for _, line := range domainLines {
			singleReq := req.CreateRequest
			singleReq.DomainsText = line.Host
			if line.Port != 80 {
				singleReq.DomainsText = fmt.Sprintf("%s:%d", line.Host, line.Port)
			}
			res, err := s.Create(&singleReq)
			if err != nil {
				out.Failed = append(out.Failed, fmt.Sprintf("%s: %v", line.Host, err))
				continue
			}
			out.Created = append(out.Created, *res)
		}
	}

	if len(out.Created) == 0 && len(out.Failed) > 0 {
		return out, fmt.Errorf("批量创建失败")
	}
	return out, nil
}

func (s *Service) Delete(id uint) error {
	site, err := s.Get(id)
	if err != nil {
		return err
	}
	hosts := s.collectSiteHosts(site)
	s.cleanupSiteResources(site)
	sitepurge.Domains(s.db, hosts, sitepurge.Options{
		DataDir:       s.dataDir,
		RemoveWWWRoot: site.RootPath,
	})
	sitepurge.PurgeWebsiteID(s.db, site.ID)
	return nil
}

func (s *Service) collectSiteHosts(site *models.Website) []string {
	if site == nil {
		return nil
	}
	hosts := []string{site.Domain}
	for _, a := range site.Aliases {
		hosts = append(hosts, a.Domain)
	}
	return sitepurge.UniqueHosts(hosts)
}

func (s *Service) cleanupSiteResources(site *models.Website) {
	if site.FtpUser != "" {
		var acc models.FTPAccount
		if s.db.Where("username = ?", site.FtpUser).First(&acc).Error == nil {
			_ = s.ftp.Delete(acc.ID)
		}
	} else if site.RootPath != "" {
		var acc models.FTPAccount
		if s.db.Where("path = ?", site.RootPath).First(&acc).Error == nil {
			_ = s.ftp.Delete(acc.ID)
		}
	}
	if site.DbName != "" {
		var inst models.DatabaseInstance
		if s.db.Where("name = ?", site.DbName).First(&inst).Error == nil {
			_ = s.database.Delete(inst.ID)
		}
	}
	if s.dns != nil {
		_ = s.dns.DeleteByWebsiteID(site.ID)
	}
	host := domaincheck.HostOnly(site.Domain)
	var wpSites []models.WordPressSite
	s.db.Where("website_id = ? OR domain = ?", site.ID, host).Find(&wpSites)
	for _, wp := range wpSites {
		s.db.Where("site_id = ?", wp.ID).Delete(&models.WordPressDomain{})
		s.db.Where("site_id = ?", wp.ID).Delete(&models.WordPressBackup{})
		_ = s.db.Delete(&wp).Error
	}
	sitepurge.PurgeWebsiteID(s.db, site.ID)
}

func (s *Service) ListCategories() ([]models.SiteCategory, error) {
	s.ensureCategories()
	var list []models.SiteCategory
	return list, s.db.Order("sort asc, id asc").Find(&list).Error
}

func (s *Service) DefaultRootBase() string {
	return filepath.Join(s.dataDir, "wwwroot")
}

func (s *Service) parseRequestDomains(req *CreateRequest) []domainEntry {
	if len(req.Domains) > 0 {
		var entries []domainEntry
		for _, d := range req.Domains {
			if e := parseDomainLine(d); e.Host != "" {
				entries = append(entries, e)
			}
		}
		return entries
	}
	return parseDomainList(req.DomainsText)
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

func (s *Service) ensureIndexHTML(root, phpVersion string) {
	index := filepath.Join(root, "index.html")
	if _, err := os.Stat(index); err == nil {
		return
	}
	content := `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Welcome</title></head>
<body><h1>It works!</h1><p>OWPanel site is ready.</p></body></html>`
	if phpVersion != "" && phpVersion != "static" {
		phpIndex := filepath.Join(root, "index.php")
		_ = os.WriteFile(phpIndex, []byte(`<?php echo "PHP " . PHP_VERSION . " is working";`), 0644)
	}
	_ = os.WriteFile(index, []byte(content), 0644)
}

func (s *Service) createFTP(site *models.Website) (string, string, error) {
	user := sanitizeName(site.Domain)
	pass := randomPassword(12)
	acc := &models.FTPAccount{Username: user, Path: site.RootPath}
	if err := s.ftp.Create(acc, pass); err != nil {
		return "", "", err
	}
	return user, pass, nil
}

func (s *Service) createDatabase(site *models.Website) (string, string, string, error) {
	name := sanitizeName(site.Domain) + "_db"
	user := sanitizeName(site.Domain)
	pass := randomPassword(16)
	inst := &models.DatabaseInstance{
		Name: name, Type: "mysql", Host: "127.0.0.1", Port: 3306,
		Username: user, Password: pass, Status: "running",
	}
	if err := s.database.Create(inst); err != nil {
		return "", "", "", err
	}
	if inst.Type == "mysql" || inst.Type == "mariadb" || inst.Type == "" {
		if err := s.database.ProvisionMySQL(name, user, pass); err != nil {
			_ = s.database.Delete(inst.ID)
			return "", "", "", err
		}
	}
	return name, user, pass, nil
}

func (s *Service) autoDNS(primary string, aliases []domainEntry, websiteID uint) error {
	var hosts []string
	for _, a := range aliases {
		hosts = append(hosts, a.Host)
	}
	return s.dns.AutoDNSForWebsite(primary, hosts, websiteID)
}

func (s *Service) loadAliases(websiteID uint) []models.WebsiteAlias {
	var aliases []models.WebsiteAlias
	s.db.Where("website_id = ?", websiteID).Order("type desc, id asc").Find(&aliases)
	return aliases
}

func (s *Service) allDomainEntries(site *models.Website) []domainEntry {
	if len(site.Aliases) == 0 {
		s.db.Where("website_id = ?", site.ID).Find(&site.Aliases)
	}
	var entries []domainEntry
	for _, a := range site.Aliases {
		entries = append(entries, domainEntry{Host: a.Domain, Port: a.Port})
	}
	return entries
}

func (s *Service) ensureCategories() {
	var count int64
	s.db.Model(&models.SiteCategory{}).Count(&count)
	if count > 0 {
		return
	}
	defaults := []models.SiteCategory{
		{Name: "默认类别", Sort: 0},
		{Name: "PHP项目", Sort: 1},
		{Name: "静态站点", Sort: 2},
	}
	for _, c := range defaults {
		_ = s.db.Create(&c).Error
	}
}

func (s *Service) normalizeCategory(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "默认类别"
	}
	return name
}

func randomPassword(n int) string {
	b := make([]byte, (n+1)/2)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}
