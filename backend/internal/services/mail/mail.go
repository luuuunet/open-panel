package mail

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"gorm.io/gorm"
)

type Service struct {
	db       *gorm.DB
	dataDir  string
	settings *settings.Service
	ws       WebServerHooks
}

func NewService(db *gorm.DB, dataDir string) *Service {
	initWebmailInstallLogs(dataDir)
	return &Service{db: db, dataDir: dataDir}
}

func (s *Service) SetSettings(settingsSvc *settings.Service) {
	s.settings = settingsSvc
}

func (s *Service) ListDomains() ([]models.MailDomain, error) {
	var list []models.MailDomain
	return list, s.db.Order("id desc").Find(&list).Error
}

func (s *Service) CreateDomain(d *models.MailDomain) error {
	d.Domain = strings.TrimSpace(strings.ToLower(d.Domain))
	if d.Domain == "" {
		return fmt.Errorf("域名不能为空")
	}
	d.Status = "active"
	if err := s.db.Create(d).Error; err != nil {
		return err
	}
	if err := s.syncMaps(); err != nil {
		return err
	}
	s.refreshWebmailDomains()
	return nil
}

func (s *Service) DeleteDomain(id uint) error {
	var d models.MailDomain
	if err := s.db.First(&d, id).Error; err != nil {
		return err
	}
	s.db.Where("domain = ?", d.Domain).Delete(&models.MailBox{})
	if err := s.db.Delete(&models.MailDomain{}, id).Error; err != nil {
		return err
	}
	return s.syncMaps()
}

func (s *Service) ListMailboxes(domain string) ([]models.MailBox, error) {
	var list []models.MailBox
	q := s.db.Order("id desc")
	if domain != "" {
		q = q.Where("domain = ?", domain)
	}
	return list, q.Find(&list).Error
}

func (s *Service) CreateMailbox(m *models.MailBox, password string) error {
	m.Address = strings.TrimSpace(strings.ToLower(m.Address))
	m.Domain = strings.TrimSpace(strings.ToLower(m.Domain))
	if m.Address == "" || m.Domain == "" {
		return fmt.Errorf("邮箱地址与域名不能为空")
	}
	if !strings.Contains(m.Address, "@") {
		m.Address = m.Address + "@" + m.Domain
	}
	if password == "" {
		return fmt.Errorf("密码不能为空")
	}
	m.Status = "active"
	if m.Maildir == "" {
		local := strings.Split(m.Address, "@")[0]
		m.Maildir = filepath.Join(s.vmailBase(), m.Domain, local)
	}
	hashed, err := HashPassword(password)
	if err != nil {
		return err
	}
	m.Password = hashed
	if err := s.db.Create(m).Error; err != nil {
		return err
	}
	_ = os.MkdirAll(m.Maildir, 0750)
	_ = exec.Command("chown", "-R", "vmail:vmail", m.Maildir).Run()
	s.savePassSecret(m.Address, password)
	s.refreshDomainCount(m.Domain)
	syncErr := s.syncMaps()
	errMsg := ""
	if syncErr != nil {
		errMsg = syncErr.Error()
	}
	s.db.Model(m).Updates(map[string]interface{}{"synced": syncErr == nil, "sync_error": errMsg})
	return syncErr
}

func (s *Service) UpdateMailboxPassword(id uint, password string) error {
	password = strings.TrimSpace(password)
	if password == "" {
		return fmt.Errorf("密码不能为空")
	}
	var m models.MailBox
	if err := s.db.First(&m, id).Error; err != nil {
		return err
	}
	hashed, err := HashPassword(password)
	if err != nil {
		return err
	}
	syncErr := s.syncMaps()
	if syncErr != nil {
		return syncErr
	}
	s.savePassSecret(m.Address, password)
	return s.db.Model(&m).Updates(map[string]interface{}{
		"password": hashed, "synced": true, "sync_error": "",
	}).Error
}

func (s *Service) DeleteMailbox(id uint) error {
	var m models.MailBox
	if err := s.db.First(&m, id).Error; err != nil {
		return err
	}
	domain := m.Domain
	if err := s.db.Delete(&models.MailBox{}, id).Error; err != nil {
		return err
	}
	s.removePassSecret(m.Address)
	s.refreshDomainCount(domain)
	return s.syncMaps()
}

func (s *Service) SyncAll() error {
	if err := s.syncMaps(); err != nil {
		return err
	}
	s.refreshWebmailDomains()
	return nil
}

func (s *Service) refreshWebmailDomains() {
	if !hasSnappyMailFiles(s.webmailRoot()) {
		return
	}
	_, mailDomain := s.webmailHost()
	_ = s.writeSnappyMailDomainConfigs(mailDomain)
}

func (s *Service) refreshDomainCount(domain string) {
	var count int64
	s.db.Model(&models.MailBox{}).Where("domain = ?", domain).Count(&count)
	s.db.Model(&models.MailDomain{}).Where("domain = ?", domain).Update("mailboxes", count)
}

func (s *Service) secretsDir() string {
	return filepath.Join(s.mailRoot(), "secrets")
}

func (s *Service) savePassSecret(address, password string) {
	dir := s.secretsDir()
	_ = os.MkdirAll(dir, 0700)
	safe := strings.ReplaceAll(address, "@", "_at_")
	_ = os.WriteFile(filepath.Join(dir, safe+".pass"), []byte(password), 0600)
}

func (s *Service) removePassSecret(address string) {
	safe := strings.ReplaceAll(address, "@", "_at_")
	_ = os.Remove(filepath.Join(s.secretsDir(), safe+".pass"))
}

func (s *Service) readPassSecret(address string) string {
	safe := strings.ReplaceAll(address, "@", "_at_")
	data, err := os.ReadFile(filepath.Join(s.secretsDir(), safe+".pass"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (s *Service) syncMaps() error {
	var domains []models.MailDomain
	if err := s.db.Find(&domains).Error; err != nil {
		return err
	}
	var boxes []models.MailBox
	if err := s.db.Find(&boxes).Error; err != nil {
		return err
	}

	dir := s.mailRoot()
	_ = os.MkdirAll(dir, 0755)
	_ = os.MkdirAll(s.vmailBase(), 0750)

	virtualPath := filepath.Join(dir, "postfix-virtual")
	domainsPath := filepath.Join(dir, "postfix-domains")
	passwdPath := filepath.Join(dir, "dovecot-passwd")

	var virtual, domainsMap, passwd strings.Builder
	for _, d := range domains {
		domainsMap.WriteString(d.Domain + " OK\n")
	}
	for _, box := range boxes {
		local := strings.Split(box.Address, "@")[0]
		maildir := box.Maildir
		if maildir == "" {
			maildir = filepath.Join(s.vmailBase(), box.Domain, local)
		}
		virtual.WriteString(fmt.Sprintf("%s %s/%s/\n", box.Address, box.Domain, local))
		if box.Password != "" {
			pass := box.Password
			if !IsHashed(pass) {
				if hashed, err := HashPassword(pass); err == nil && hashed != "" {
					pass = hashed
					s.db.Model(&box).Update("password", hashed)
				}
			}
			passwd.WriteString(fmt.Sprintf("%s:%s:%d:%d::%s:\n", box.Address, pass, vmailUID, vmailGID, maildir))
		}
		_ = os.MkdirAll(maildir, 0750)
		_ = exec.Command("chown", "-R", "vmail:vmail", maildir).Run()
	}

	if err := os.WriteFile(virtualPath, []byte(virtual.String()), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(domainsPath, []byte(domainsMap.String()), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(passwdPath, []byte(passwd.String()), 0600); err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		return nil
	}

	if err := s.ensureVMailUser(); err != nil {
		return err
	}
	if commandExists("postconf") {
		_ = s.applyPostfixMain()
	}
	if err := s.applyDovecotConf(passwdPath); err != nil {
		return err
	}

	if _, err := exec.LookPath("postmap"); err == nil {
		for _, src := range []struct{ src, dst string }{
			{virtualPath, postfixVirtualMap()},
			{domainsPath, postfixDomainMap()},
		} {
			_ = os.WriteFile(src.dst, mustRead(src.src), 0644)
			if out, err := exec.Command("postmap", src.dst).CombinedOutput(); err != nil {
				return fmt.Errorf("postmap %s: %s", src.dst, strings.TrimSpace(string(out)))
			}
		}
		_ = exec.Command("systemctl", "reload", "postfix").Run()
	}
	_ = exec.Command("systemctl", "reload", "dovecot").Run()

	for i := range boxes {
		s.db.Model(&boxes[i]).Updates(map[string]interface{}{"synced": true, "sync_error": ""})
	}
	return nil
}

func mustRead(path string) []byte {
	data, _ := os.ReadFile(path)
	return data
}
