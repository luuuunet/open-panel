package wordpress

import (
	"fmt"
	"net"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/domaincheck"
	"github.com/luuuunet/owpanel/internal/services/ssl"
)

func domainCanUseLetsEncrypt(host string) bool {
	host = domaincheck.HostOnly(host)
	if host == "" {
		return false
	}
	return net.ParseIP(host) == nil
}

func (s *Service) collectSANDomains(site *models.WordPressSite) string {
	s.loadDomains(site)
	var extras []string
	for _, d := range site.Domains {
		if d.Enabled && !strings.EqualFold(d.Domain, site.Domain) {
			extras = append(extras, d.Domain)
		}
	}
	return strings.Join(extras, ",")
}

func (s *Service) issueSSL(site *models.WordPressSite, logger *DeployLogger, email string) error {
	host := domaincheck.HostOnly(site.Domain)
	if !domainCanUseLetsEncrypt(host) {
		msg := "IP 地址无法申请 Let's Encrypt 证书，请绑定域名后再申请"
		_ = s.db.Model(site).Update("ssl_status", "skipped").Error
		if logger != nil {
			logger.Warn(msg)
		}
		return fmt.Errorf("%s", msg)
	}
	if email == "" {
		email = strings.TrimSpace(site.SSLEmail)
	}
	autoRenew := true
	sslSvc := ssl.NewService(s.db, s.dataDir)
	sslSvc.SetDeployHook(s.DeploySSLForDomain)
	cert, err := sslSvc.Issue(&ssl.IssueRequest{
		Domain:     host,
		SanDomains: s.collectSANDomains(site),
		Webroot:    site.RootPath,
		Email:      email,
		AutoRenew:  &autoRenew,
		Deploy:     true,
	})
	if err != nil {
		_ = s.db.Model(site).Updates(map[string]interface{}{
			"ssl": false, "ssl_status": "failed",
		}).Error
		return err
	}
	if cert.Status != "active" {
		_ = s.db.Model(site).Update("ssl_status", "failed").Error
		return fmt.Errorf("证书申请未完成")
	}
	_ = s.db.Model(site).Updates(map[string]interface{}{
		"ssl": true, "ssl_status": "active", "auto_ssl": true,
	}).Error
	if logger != nil {
		logger.Info("✓ SSL 证书已申请并部署 (Let's Encrypt)")
	}
	return nil
}

// DeploySSLForDomain enables HTTPS vhost for a WordPress site after cert issuance.
func (s *Service) DeploySSLForDomain(domain string) error {
	host := domaincheck.HostOnly(domain)
	var site models.WordPressSite
	if err := s.db.Where("domain = ?", host).First(&site).Error; err != nil {
		return fmt.Errorf("not a wordpress site")
	}
	if _, _, ok := ssl.CertPaths(s.dataDir, host); !ok {
		return fmt.Errorf("certificate files missing for %s", host)
	}
	if site.WebsiteID > 0 {
		forceHTTPS := !site.CloudflareCDN
		_ = s.db.Model(&models.Website{}).Where("id = ?", site.WebsiteID).Updates(map[string]interface{}{
			"ssl": true, "force_https": forceHTTPS,
		}).Error
	}
	_ = s.db.Model(&site).Updates(map[string]interface{}{
		"ssl": true, "force_https": !site.CloudflareCDN, "ssl_status": "active",
	}).Error
	_ = s.patchWPSiteURL(site.RootPath, host, true)
	if err := s.regenerateVhost(site.ID); err != nil {
		return err
	}
	return reloadNginxIfAvailable()
}

func (s *Service) IssueSSLForSite(id uint, email string) error {
	site, err := s.Get(id)
	if err != nil {
		return err
	}
	if email != "" {
		_ = s.db.Model(site).Update("ssl_email", email).Error
		site.SSLEmail = email
	}
	return s.issueSSL(site, nil, email)
}

func (s *Service) syncSSLStatus(site *models.WordPressSite) {
	host := domaincheck.HostOnly(site.Domain)
	if _, _, ok := ssl.CertPaths(s.dataDir, host); ok {
		site.SSL = true
		if site.SSLStatus == "" || site.SSLStatus == "none" {
			site.SSLStatus = "active"
		}
	}
}
