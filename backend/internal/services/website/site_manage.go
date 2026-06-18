package website

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/domaincheck"
)

type UpdateRequest struct {
	RootPath       *string `json:"root_path"`
	PhpVersion     *string `json:"php_version"`
	SSL            *bool   `json:"ssl"`
	ForceHTTPS     *bool   `json:"force_https"`
	Remark         *string `json:"remark"`
	Category       *string `json:"category"`
	IndexFiles     *string `json:"index_files"`
	RewriteRules   *string `json:"rewrite_rules"`
	ExtraNginxConf *string `json:"extra_nginx_conf"`
	RedirectURL    *string `json:"redirect_url"`
	ProxyPass      *string `json:"proxy_pass"`
	CacheEnabled   *bool   `json:"cache_enabled"`
	CacheDevMode   *bool   `json:"cache_dev_mode"`
	CacheHtmlTTL   *int    `json:"cache_html_ttl"`
	CacheStaticTTL *int    `json:"cache_static_ttl"`
	AccessAuthEnabled *bool   `json:"access_auth_enabled"`
	AccessAuthUser    *string `json:"access_auth_user"`
	AccessAuthPass    *string `json:"access_auth_pass"`
	AccessAllowIPs    *string `json:"access_allow_ips"`
	AccessDenyIPs     *string `json:"access_deny_ips"`
	TrafficLimitEnabled *bool   `json:"traffic_limit_enabled"`
	TrafficRate         *string `json:"traffic_rate"`
	TrafficBurst        *int    `json:"traffic_burst"`
	HotlinkEnabled      *bool   `json:"hotlink_enabled"`
	HotlinkAllowEmpty   *bool   `json:"hotlink_allow_empty"`
	HotlinkAllowDomains *string `json:"hotlink_allow_domains"`
	CrossSiteProtectEnabled *bool `json:"cross_site_protect_enabled"`
	ExpiresAt           *string `json:"expires_at"` // YYYY-MM-DD, empty string = permanent
}

func (s *Service) ListDomains(siteID uint) ([]models.WebsiteAlias, error) {
	site, err := s.Get(siteID)
	if err != nil {
		return nil, err
	}
	return site.Aliases, nil
}

func (s *Service) AddDomains(siteID uint, text string) ([]models.WebsiteAlias, error) {
	site, err := s.Get(siteID)
	if err != nil {
		return nil, err
	}
	entries := parseDomainList(text)
	if len(entries) == 0 {
		return nil, fmt.Errorf("请输入域名")
	}

	existing := map[string]bool{}
	for _, a := range site.Aliases {
		existing[a.Domain] = true
	}

	var toCheck []string
	for _, e := range entries {
		if !existing[e.Host] {
			toCheck = append(toCheck, e.Host)
		}
	}
	if len(toCheck) > 0 {
		if err := domaincheck.AssertAvailable(s.db, toCheck, domaincheck.Scope{IgnoreWebsiteID: siteID}); err != nil {
			return nil, err
		}
	}

	var added []models.WebsiteAlias
	for _, e := range entries {
		if existing[e.Host] {
			continue
		}
		alias := models.WebsiteAlias{
			WebsiteID: siteID, Domain: e.Host, Port: e.Port, Type: "alias",
		}
		if err := s.db.Create(&alias).Error; err != nil {
			return nil, err
		}
		added = append(added, alias)
	}
	if len(added) == 0 {
		return nil, fmt.Errorf("没有新域名可添加（可能已存在）")
	}

	site, err = s.Get(siteID)
	if err != nil {
		return nil, err
	}
	if err := s.regenerateVhost(site); err != nil {
		log.Printf("[website] regenerate vhost after add domains: %v", err)
	}
	if site.DnsMode == "auto" {
		var entries []domainEntry
		for _, a := range added {
			entries = append(entries, domainEntry{Host: a.Domain, Port: a.Port})
		}
		_ = s.autoDNS(site.Domain, entries, siteID)
	}
	return added, nil
}

func (s *Service) RemoveDomain(siteID, aliasID uint) error {
	var alias models.WebsiteAlias
	if err := s.db.Where("id = ? AND website_id = ?", aliasID, siteID).First(&alias).Error; err != nil {
		return err
	}
	if alias.Type == "primary" {
		return fmt.Errorf("主域名不可删除")
	}
	if err := s.db.Delete(&alias).Error; err != nil {
		return err
	}
	site, err := s.Get(siteID)
	if err != nil {
		return err
	}
	return s.regenerateVhost(site)
}

func (s *Service) BatchRemoveDomains(siteID uint, aliasIDs []uint) error {
	if len(aliasIDs) == 0 {
		return fmt.Errorf("请选择要删除的域名")
	}
	for _, id := range aliasIDs {
		var alias models.WebsiteAlias
		if err := s.db.Where("id = ? AND website_id = ?", id, siteID).First(&alias).Error; err != nil {
			continue
		}
		if alias.Type == "primary" {
			continue
		}
		_ = s.db.Delete(&alias).Error
	}
	site, err := s.Get(siteID)
	if err != nil {
		return err
	}
	return s.regenerateVhost(site)
}

func (s *Service) ApplyVhost(siteID uint) error {
	site, err := s.Get(siteID)
	if err != nil {
		return err
	}
	return s.regenerateVhost(site)
}

func (s *Service) UpdateSite(siteID uint, req *UpdateRequest) (*models.Website, error) {
	site, err := s.Get(siteID)
	if err != nil {
		return nil, err
	}
	updates := map[string]interface{}{}
	if req.RootPath != nil {
		root := strings.TrimSpace(*req.RootPath)
		if root == "" {
			return nil, fmt.Errorf("网站目录不能为空")
		}
		updates["root_path"] = root
	}
	if req.PhpVersion != nil {
		v := strings.TrimSpace(*req.PhpVersion)
		usePHP := v != "" && v != "static"
		updates["php_version"] = v
		updates["php"] = usePHP
	}
	if req.SSL != nil {
		updates["ssl"] = *req.SSL
	}
	if req.ForceHTTPS != nil {
		updates["force_https"] = *req.ForceHTTPS
	}
	if req.Remark != nil {
		updates["remark"] = strings.TrimSpace(*req.Remark)
	}
	if req.Category != nil {
		updates["category"] = s.normalizeCategory(*req.Category)
	}
	if req.IndexFiles != nil {
		updates["index_files"] = strings.TrimSpace(*req.IndexFiles)
	}
	if req.RewriteRules != nil {
		updates["rewrite_rules"] = strings.TrimSpace(*req.RewriteRules)
	}
	if req.ExtraNginxConf != nil {
		updates["extra_nginx_conf"] = *req.ExtraNginxConf
	}
	if req.RedirectURL != nil {
		updates["redirect_url"] = strings.TrimSpace(*req.RedirectURL)
	}
	if req.ProxyPass != nil {
		updates["proxy_pass"] = strings.TrimSpace(*req.ProxyPass)
	}
	if req.CacheEnabled != nil {
		updates["cache_enabled"] = *req.CacheEnabled
	}
	if req.CacheDevMode != nil {
		updates["cache_dev_mode"] = *req.CacheDevMode
	}
	if req.CacheHtmlTTL != nil {
		updates["cache_html_ttl"] = *req.CacheHtmlTTL
	}
	if req.CacheStaticTTL != nil {
		updates["cache_static_ttl"] = *req.CacheStaticTTL
	}
	if req.AccessAuthEnabled != nil {
		updates["access_auth_enabled"] = *req.AccessAuthEnabled
	}
	if req.AccessAuthUser != nil {
		updates["access_auth_user"] = strings.TrimSpace(*req.AccessAuthUser)
	}
	if req.AccessAuthPass != nil && strings.TrimSpace(*req.AccessAuthPass) != "" {
		updates["access_auth_pass"] = *req.AccessAuthPass
	}
	if req.AccessAllowIPs != nil {
		updates["access_allow_ips"] = strings.TrimSpace(*req.AccessAllowIPs)
	}
	if req.AccessDenyIPs != nil {
		updates["access_deny_ips"] = strings.TrimSpace(*req.AccessDenyIPs)
	}
	if req.TrafficLimitEnabled != nil {
		updates["traffic_limit_enabled"] = *req.TrafficLimitEnabled
	}
	if req.TrafficRate != nil {
		updates["traffic_rate"] = strings.TrimSpace(*req.TrafficRate)
	}
	if req.TrafficBurst != nil {
		updates["traffic_burst"] = *req.TrafficBurst
	}
	if req.HotlinkEnabled != nil {
		updates["hotlink_enabled"] = *req.HotlinkEnabled
	}
	if req.HotlinkAllowEmpty != nil {
		updates["hotlink_allow_empty"] = *req.HotlinkAllowEmpty
	}
	if req.HotlinkAllowDomains != nil {
		updates["hotlink_allow_domains"] = strings.TrimSpace(*req.HotlinkAllowDomains)
	}
	if req.CrossSiteProtectEnabled != nil {
		updates["cross_site_protect_enabled"] = *req.CrossSiteProtectEnabled
		if *req.CrossSiteProtectEnabled {
			updates["hotlink_enabled"] = true
		}
	}
	if req.ExpiresAt != nil {
		exp, err := ParseExpiresDate(*req.ExpiresAt)
		if err != nil {
			return nil, err
		}
		updates["expires_at"] = exp
	}
	if len(updates) == 0 {
		return site, nil
	}
	if err := s.db.Model(site).Updates(updates).Error; err != nil {
		return nil, err
	}
	site, err = s.Get(siteID)
	if err != nil {
		return nil, err
	}
	if err := s.regenerateLimitZones(); err != nil {
		return nil, err
	}
	if err := s.regenerateVhost(site); err != nil {
		return nil, err
	}
	return site, nil
}

func (s *Service) ReadNginxConf(siteID uint) (string, error) {
	site, err := s.Get(siteID)
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(site.NginxConf)
	if path == "" {
		conf, err := s.writeNginxVhost(site)
		if err != nil {
			return "", err
		}
		path = conf
		_ = s.db.Model(site).Update("nginx_conf", conf).Error
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取配置失败: %w", err)
	}
	return string(data), nil
}

func (s *Service) SaveNginxConf(siteID uint, content string) error {
	site, err := s.Get(siteID)
	if err != nil {
		return err
	}
	path := strings.TrimSpace(site.NginxConf)
	if path == "" {
		path, err = s.writeNginxVhost(site)
		if err != nil {
			return err
		}
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	return s.db.Model(site).Update("nginx_conf", path).Error
}

func (s *Service) SiteLogs(siteID uint, lines int) (map[string]interface{}, error) {
	site, err := s.Get(siteID)
	if err != nil {
		return nil, err
	}
	if lines <= 0 {
		lines = 100
	}
	if lines > 500 {
		lines = 500
	}
	logDir := filepath.Join(s.dataDir, "logs")
	accessPath := filepath.Join(logDir, site.Domain+"_access.log")
	errorPath := filepath.Join(logDir, site.Domain+"_error.log")

	return map[string]interface{}{
		"access_log":  accessPath,
		"error_log":   errorPath,
		"access_tail": tailFile(accessPath, lines),
		"error_tail":  tailFile(errorPath, lines),
	}, nil
}

func (s *Service) regenerateVhost(site *models.Website) error {
	return s.applyVhost(site)
}

func (s *Service) RegenerateAll() error {
	if err := s.regenerateLimitZones(); err != nil {
		return err
	}
	var sites []models.Website
	if err := s.db.Preload("Aliases").Find(&sites).Error; err != nil {
		return err
	}
	reloadWS := map[string]struct{}{}
	for i := range sites {
		if sites[i].Status == "stopped" {
			continue
		}
		ws, err := s.writeVhostOnly(&sites[i])
		if err != nil {
			return fmt.Errorf("%s: %w", sites[i].Domain, err)
		}
		if ws != "" {
			reloadWS[ws] = struct{}{}
		}
	}
	for ws := range reloadWS {
		if err := s.reloadWebServer(ws); err != nil {
			return err
		}
	}
	return nil
}

func tailFile(path string, maxLines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}
