package website

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/ssl"
)

func (s *Service) DeploySSLForDomain(domain string) error {
	domain = strings.TrimSpace(strings.ToLower(domain))
	var site models.Website
	if err := s.db.Where("domain = ?", domain).First(&site).Error; err != nil {
		return fmt.Errorf("未找到域名 %s 对应的网站", domain)
	}
	fc, _, ok := ssl.CertPaths(s.dataDir, domain)
	if !ok {
		return fmt.Errorf("证书文件不存在: %s", domain)
	}
	_ = fc
	if err := s.db.Model(&site).Updates(map[string]interface{}{
		"ssl": true, "force_https": true,
	}).Error; err != nil {
		return err
	}
	site.SSL = true
	site.ForceHTTPS = true
	return s.applyVhost(&site)
}

func (s *Service) IssueSSL(siteID uint, email string, sanDomains string, deploy bool) error {
	site, err := s.Get(siteID)
	if err != nil {
		return err
	}
	autoRenew := true
	sslSvc := ssl.NewService(s.db, s.dataDir)
	sslSvc.SetDeployHook(s.DeploySSLForDomain)
	cert, err := sslSvc.Issue(&ssl.IssueRequest{
		Domain:     site.Domain,
		SanDomains: sanDomains,
		Webroot:    site.RootPath,
		Email:      email,
		AutoRenew:  &autoRenew,
		Deploy:     deploy,
	})
	if err != nil {
		return err
	}
	if cert.Status != "active" {
		return fmt.Errorf("证书申请未完成")
	}
	if !deploy {
		if err := s.db.Model(site).Update("ssl", true).Error; err != nil {
			return err
		}
		site.SSL = true
		return s.applyVhost(site)
	}
	return nil
}
