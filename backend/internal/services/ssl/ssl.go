package ssl

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"gorm.io/gorm"
)

type Service struct {
	db       *gorm.DB
	dataDir  string
	onDeploy func(domain string) error
}

func NewService(db *gorm.DB, dataDir string) *Service {
	return &Service{db: db, dataDir: dataDir}
}

func (s *Service) SetDeployHook(fn func(domain string) error) {
	s.onDeploy = fn
}

type StatusSummary struct {
	Total            int  `json:"total"`
	Active           int  `json:"active"`
	ExpiringSoon     int  `json:"expiring_soon"`
	Expired          int  `json:"expired"`
	Failed           int  `json:"failed"`
	CertbotInstalled bool `json:"certbot_installed"`
}

type CertDetail struct {
	models.SSLCertificate
	DaysLeft  int    `json:"days_left"`
	Fullchain string `json:"fullchain_path,omitempty"`
	Privkey   string `json:"privkey_path,omitempty"`
	HasCert   bool   `json:"has_cert"`
}

func (s *Service) List() ([]CertDetail, error) {
	var certs []models.SSLCertificate
	if err := s.db.Order("id desc").Find(&certs).Error; err != nil {
		return nil, err
	}
	out := make([]CertDetail, 0, len(certs))
	for i := range certs {
		out = append(out, s.enrichCert(&certs[i]))
	}
	return out, nil
}

func (s *Service) Get(id uint) (*CertDetail, error) {
	var cert models.SSLCertificate
	if err := s.db.First(&cert, id).Error; err != nil {
		return nil, err
	}
	d := s.enrichCert(&cert)
	return &d, nil
}

func (s *Service) StatusSummary() (*StatusSummary, error) {
	list, err := s.List()
	if err != nil {
		return nil, err
	}
	st := &StatusSummary{
		Total:            len(list),
		CertbotInstalled: appstore.CertbotInstalled(s.dataDir),
	}
	for _, c := range list {
		switch c.Status {
		case "active":
			st.Active++
			if c.DaysLeft >= 0 && c.DaysLeft <= 30 {
				st.ExpiringSoon++
			}
			if c.DaysLeft < 0 {
				st.Expired++
			}
		case "failed":
			st.Failed++
		case "simulated":
			st.Failed++
		case "expired":
			st.Expired++
		}
	}
	return st, nil
}

func (s *Service) enrichCert(cert *models.SSLCertificate) CertDetail {
	s.syncExpiryFromDisk(cert)
	fc, pk, ok := CertPaths(s.dataDir, cert.Domain)
	d := CertDetail{
		SSLCertificate: *cert,
		Fullchain:      fc,
		Privkey:        pk,
		HasCert:        ok,
	}
	if cert.ExpiresAt != nil {
		d.DaysLeft = int(time.Until(*cert.ExpiresAt).Hours() / 24)
		if d.DaysLeft < 0 && cert.Status == "active" {
			_ = s.db.Model(cert).Update("status", "expired").Error
			d.Status = "expired"
		}
	}
	return d
}

func (s *Service) Request(domain string) (*models.SSLCertificate, error) {
	return s.Issue(&IssueRequest{Domain: domain})
}

type IssueRequest struct {
	Domain     string `json:"domain"`
	SanDomains string `json:"san_domains"`
	Webroot    string `json:"webroot"`
	Email      string `json:"email"`
	AutoRenew  *bool  `json:"auto_renew"`
	Deploy     bool   `json:"deploy"`
}

func (s *Service) Issue(req *IssueRequest) (*models.SSLCertificate, error) {
	domain := strings.TrimSpace(strings.ToLower(req.Domain))
	if domain == "" {
		return nil, fmt.Errorf("域名不能为空")
	}
	autoRenew := true
	if req.AutoRenew != nil {
		autoRenew = *req.AutoRenew
	}

	var existing models.SSLCertificate
	err := s.db.Where("domain = ?", domain).First(&existing).Error
	if err == nil {
		return s.reissueCert(&existing, req)
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	cert := &models.SSLCertificate{
		Domain:     domain,
		SanDomains: normalizeSAN(req.SanDomains),
		Email:      strings.TrimSpace(req.Email),
		Webroot:    strings.TrimSpace(req.Webroot),
		Provider:   "letsencrypt",
		AutoRenew:  autoRenew,
		Status:     "pending",
	}
	if err := s.db.Create(cert).Error; err != nil {
		return nil, err
	}
	if err := s.runCertbotIssue(cert, false); err != nil {
		return cert, err
	}
	if req.Deploy {
		_ = s.DeployToWebsite(domain)
	}
	return cert, nil
}

func (s *Service) reissueCert(cert *models.SSLCertificate, req *IssueRequest) (*models.SSLCertificate, error) {
	if req.Email != "" {
		cert.Email = strings.TrimSpace(req.Email)
	}
	if req.Webroot != "" {
		cert.Webroot = strings.TrimSpace(req.Webroot)
	}
	if req.SanDomains != "" {
		cert.SanDomains = normalizeSAN(req.SanDomains)
	}
	if req.AutoRenew != nil {
		cert.AutoRenew = *req.AutoRenew
	}
	if err := s.runCertbotIssue(cert, true); err != nil {
		return cert, err
	}
	if req.Deploy {
		_ = s.DeployToWebsite(cert.Domain)
	}
	return cert, nil
}

func (s *Service) Renew(id uint) (*models.SSLCertificate, error) {
	var cert models.SSLCertificate
	if err := s.db.First(&cert, id).Error; err != nil {
		return nil, err
	}
	if err := s.runCertbotIssue(&cert, true); err != nil {
		return &cert, err
	}
	_ = s.DeployToWebsite(cert.Domain)
	return &cert, nil
}

func (s *Service) RenewAll() (int, []string, error) {
	var certs []models.SSLCertificate
	if err := s.db.Where("auto_renew = ? AND status IN ?", true, []string{"active", "expired"}).Find(&certs).Error; err != nil {
		return 0, nil, err
	}
	var failed []string
	n := 0
	for i := range certs {
		if err := s.runCertbotIssue(&certs[i], true); err != nil {
			failed = append(failed, certs[i].Domain+": "+err.Error())
			continue
		}
		n++
		_ = s.DeployToWebsite(certs[i].Domain)
	}
	return n, failed, nil
}

type UploadRequest struct {
	Domain    string `json:"domain"`
	Fullchain string `json:"fullchain"`
	Privkey   string `json:"privkey"`
	Email     string `json:"email"`
	Deploy    bool   `json:"deploy"`
}

func (s *Service) Upload(req *UploadRequest) (*models.SSLCertificate, error) {
	domain := strings.TrimSpace(strings.ToLower(req.Domain))
	if domain == "" || strings.TrimSpace(req.Fullchain) == "" || strings.TrimSpace(req.Privkey) == "" {
		return nil, fmt.Errorf("域名、证书与私钥不能为空")
	}
	dir := filepath.Join(s.dataDir, "ssl", domain)
	_ = os.MkdirAll(dir, 0755)
	if err := os.WriteFile(filepath.Join(dir, "fullchain.pem"), []byte(req.Fullchain), 0644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "privkey.pem"), []byte(req.Privkey), 0600); err != nil {
		return nil, err
	}
	exp, issuer := parseCertMeta(req.Fullchain)

	var cert models.SSLCertificate
	if s.db.Where("domain = ?", domain).First(&cert).Error == nil {
		_ = s.db.Model(&cert).Updates(map[string]interface{}{
			"provider": "custom", "status": "active", "issuer": issuer,
			"expires_at": exp, "error_msg": "", "auto_renew": false,
			"email": strings.TrimSpace(req.Email),
		}).Error
	} else {
		cert = models.SSLCertificate{
			Domain: domain, Email: strings.TrimSpace(req.Email),
			Provider: "custom", AutoRenew: false, Status: "active",
			Issuer: issuer, ExpiresAt: exp,
		}
		if err := s.db.Create(&cert).Error; err != nil {
			return nil, err
		}
	}
	if req.Deploy {
		_ = s.DeployToWebsite(domain)
	}
	return &cert, nil
}

func (s *Service) DeployToWebsite(domain string) error {
	if s.onDeploy != nil {
		return s.onDeploy(domain)
	}
	return nil
}

func (s *Service) Delete(id uint) error {
	var cert models.SSLCertificate
	if err := s.db.First(&cert, id).Error; err != nil {
		return err
	}
	_ = os.RemoveAll(filepath.Join(s.dataDir, "ssl", cert.Domain))
	return s.db.Delete(&cert).Error
}

func (s *Service) runCertbotIssue(cert *models.SSLCertificate, forceRenew bool) error {
	domain := cert.Domain
	webroot := strings.TrimSpace(cert.Webroot)
	if webroot == "" {
		webroot = filepath.Join(s.dataDir, "wwwroot", domain)
		var site models.Website
		if s.db.Where("domain = ?", domain).First(&site).Error == nil && site.RootPath != "" {
			webroot = site.RootPath
		}
	}
	_ = os.MkdirAll(filepath.Join(webroot, ".well-known", "acme-challenge"), 0755)
	cert.Webroot = webroot

	if appstore.CertbotBinary() == "" {
		msg := "未安装 Certbot，请先在软件商店安装 Certbot 或运行 LNMP 一键环境后再申请证书"
		_ = s.db.Model(cert).Updates(map[string]interface{}{"status": "failed", "error_msg": msg}).Error
		cert.Status = "failed"
		cert.ErrorMsg = msg
		return fmt.Errorf("%s", msg)
	}

	domains := []string{domain}
	for _, d := range strings.Split(cert.SanDomains, ",") {
		d = strings.TrimSpace(strings.ToLower(d))
		if d != "" && d != domain {
			domains = append(domains, d)
		}
	}

	args := []string{"certonly", "--webroot", "-w", webroot, "--non-interactive", "--agree-tos"}
	if forceRenew {
		args = append(args, "--force-renewal")
	}
	for _, d := range domains {
		args = append(args, "-d", d)
	}
	if cert.Email != "" {
		args = append(args, "--email", cert.Email)
	} else {
		args = append(args, "--register-unsafely-without-email")
	}

	out, err := exec.Command("certbot", args...).CombinedOutput()
	outStr := strings.TrimSpace(string(out))
	if err != nil {
		_ = s.db.Model(cert).Updates(map[string]interface{}{"status": "failed", "error_msg": outStr}).Error
		cert.Status = "failed"
		cert.ErrorMsg = outStr
		return fmt.Errorf("certbot 失败: %s", outStr)
	}

	s.syncExpiryFromDisk(cert)
	s.copyFromLetsEncrypt(domain)
	_ = s.db.Model(cert).Updates(map[string]interface{}{
		"status": "active", "error_msg": "", "webroot": webroot,
		"expires_at": cert.ExpiresAt, "issuer": cert.Issuer,
	}).Error
	cert.Status = "active"
	return nil
}

func (s *Service) copyFromLetsEncrypt(domain string) {
	le := filepath.Join("/etc/letsencrypt/live", domain)
	fcSrc := filepath.Join(le, "fullchain.pem")
	if _, err := os.Stat(fcSrc); err != nil {
		return
	}
	dir := filepath.Join(s.dataDir, "ssl", domain)
	_ = os.MkdirAll(dir, 0755)
	if data, err := os.ReadFile(fcSrc); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "fullchain.pem"), data, 0644)
	}
	if data, err := os.ReadFile(filepath.Join(le, "privkey.pem")); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "privkey.pem"), data, 0600)
	}
}

func (s *Service) markSimulated(cert *models.SSLCertificate, domain string) error {
	dir := filepath.Join(s.dataDir, "ssl", domain)
	_ = os.MkdirAll(dir, 0755)
	_ = os.WriteFile(filepath.Join(dir, "fullchain.pem"), []byte("# simulated cert\n"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "privkey.pem"), []byte("# simulated key\n"), 0644)
	exp := time.Now().Add(90 * 24 * time.Hour)
	cert.Status = "simulated"
	cert.ExpiresAt = &exp
	cert.Issuer = "simulated"
	return s.db.Model(cert).Updates(map[string]interface{}{
		"status": "simulated", "expires_at": exp, "issuer": "simulated", "error_msg": "",
	}).Error
}

func (s *Service) syncExpiryFromDisk(cert *models.SSLCertificate) {
	fc, _, ok := CertPaths(s.dataDir, cert.Domain)
	if !ok {
		return
	}
	exp, issuer := parseCertFile(fc)
	if exp != nil {
		cert.ExpiresAt = exp
	}
	if issuer != "" {
		cert.Issuer = issuer
	}
}

func parseCertFile(path string) (*time.Time, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ""
	}
	return parseCertMeta(string(data))
}

func parseCertMeta(pemData string) (*time.Time, string) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, ""
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, ""
	}
	return &c.NotAfter, c.Issuer.CommonName
}

func normalizeSAN(s string) string {
	s = strings.ReplaceAll(s, "\n", ",")
	var out []string
	seen := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(strings.ToLower(p))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return strings.Join(out, ",")
}

func CertPaths(dataDir, domain string) (fullchain, privkey string, ok bool) {
	domain = strings.TrimSpace(domain)
	le := filepath.Join("/etc/letsencrypt/live", domain)
	if f, err := os.Stat(filepath.Join(le, "fullchain.pem")); err == nil && !f.IsDir() {
		return filepath.Join(le, "fullchain.pem"), filepath.Join(le, "privkey.pem"), true
	}
	local := filepath.Join(dataDir, "ssl", domain)
	if f, err := os.Stat(filepath.Join(local, "fullchain.pem")); err == nil && !f.IsDir() {
		return filepath.Join(local, "fullchain.pem"), filepath.Join(local, "privkey.pem"), true
	}
	return "", "", false
}
