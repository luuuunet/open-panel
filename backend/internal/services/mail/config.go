package mail

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

const (
	vmailUID = 5000
	vmailGID = 5000
)

func (s *Service) mailRoot() string {
	return filepath.Join(s.dataDir, "mail")
}

func (s *Service) vmailBase() string {
	return filepath.Join(s.mailRoot(), "vmail")
}

func postfixDomainMap() string  { return "/etc/postfix/owpanel-domains" }
func postfixVirtualMap() string { return "/etc/postfix/owpanel-virtual" }
func dovecotInclude() string    { return "/etc/dovecot/conf.d/99-owpanel.conf" }

func (s *Service) ensureVMailUser() error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if idOut, err := exec.Command("id", "-u", "vmail").Output(); err == nil && strings.TrimSpace(string(idOut)) != "" {
		return nil
	}
	out, err := exec.Command("groupadd", "-g", fmt.Sprintf("%d", vmailGID), "vmail").CombinedOutput()
	if err != nil && !strings.Contains(string(out), "exists") {
		return fmt.Errorf("groupadd vmail: %s", strings.TrimSpace(string(out)))
	}
	out, err = exec.Command("useradd", "-u", fmt.Sprintf("%d", vmailUID), "-g", "vmail", "-d", s.vmailBase(), "-s", "/usr/sbin/nologin", "-M", "vmail").CombinedOutput()
	if err != nil && !strings.Contains(string(out), "exists") {
		return fmt.Errorf("useradd vmail: %s", strings.TrimSpace(string(out)))
	}
	return os.MkdirAll(s.vmailBase(), 0750)
}

func (s *Service) applyPostfixMain() error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if _, err := exec.LookPath("postconf"); err != nil {
		return fmt.Errorf("postfix 未安装")
	}
	hostname, _ := os.Hostname()
	mailHost := "mail." + strings.TrimSpace(hostname)
	if mailHost == "mail." {
		mailHost = hostname
	}

	settings := map[string]string{
		"myhostname":               mailHost,
		"mydestination":            "",
		"local_recipient_maps":     "",
		"inet_interfaces":          "all",
		"inet_protocols":           "all",
		"virtual_mailbox_domains":  "hash:" + postfixDomainMap(),
		"virtual_mailbox_maps":     "hash:" + postfixVirtualMap(),
		"virtual_mailbox_base":     s.vmailBase(),
		"virtual_minimum_uid":      "100",
		"virtual_uid_maps":         "static:5000",
		"virtual_gid_maps":         "static:5000",
		"virtual_transport":        "virtual",
		"home_mailbox":             "Maildir/",
		"smtpd_tls_security_level": "may",
		"smtp_tls_security_level":  "may",
	}
	for k, v := range settings {
		out, err := exec.Command("postconf", "-e", k+"="+v).CombinedOutput()
		if err != nil {
			return fmt.Errorf("postconf %s: %s", k, strings.TrimSpace(string(out)))
		}
	}
	s.ensurePostfixMasterSubmission()
	return nil
}

func (s *Service) ensurePostfixMasterSubmission() {
	master := "/etc/postfix/master.cf"
	data, err := os.ReadFile(master)
	if err != nil {
		return
	}
	content := string(data)
	if strings.Contains(content, "submission inet") && !strings.Contains(content, "#submission inet") {
		return
	}
	block := `
submission inet n       -       y       -       -       smtpd
  -o syslog_name=postfix/submission
  -o smtpd_tls_security_level=encrypt
  -o smtpd_sasl_auth_enable=yes
  -o smtpd_client_restrictions=permit_sasl_authenticated,reject
  -o milter_macro_daemon_name=ORIGINATING
`
	if strings.Contains(content, "#submission inet") {
		content = strings.Replace(content, "#submission inet", "submission inet", 1)
		_ = os.WriteFile(master, []byte(content), 0644)
		return
	}
	_ = os.WriteFile(master, []byte(content+block), 0644)
}

func (s *Service) applyDovecotConf(passwdPath string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	cert, key := s.resolveSSLPaths()
	conf := fmt.Sprintf(`# Managed by OWPanel — do not edit manually
protocols = imap pop3
mail_location = maildir:%%h
first_valid_uid = %d
last_valid_uid = %d
passdb {
  driver = passwd-file
  args = username_format=%%u scheme=SHA512-CRYPT %s
}
userdb {
  driver = passwd-file
  args = username_format=%%u %s
}
service imap-login {
  inet_listener imap {
    port = 143
  }
  inet_listener imaps {
    port = 993
    ssl = yes
  }
}
service pop3-login {
  inet_listener pop3 {
    port = 110
  }
  inet_listener pop3s {
    port = 995
    ssl = yes
  }
}
ssl = required
ssl_cert = <%s
ssl_key = <%s
`, vmailUID, vmailUID, passwdPath, passwdPath, cert, key)
	if err := os.WriteFile(dovecotInclude(), []byte(conf), 0644); err != nil {
		return err
	}
	return nil
}

func (s *Service) resolveSSLPaths() (cert, key string) {
	cert = "/etc/ssl/certs/ssl-cert-snakeoil.pem"
	key = "/etc/ssl/private/ssl-cert-snakeoil.key"
	if runtime.GOOS == "windows" {
		return cert, key
	}
	var domains []string
	s.db.Model(&models.MailDomain{}).Order("id asc").Pluck("domain", &domains)
	for _, d := range domains {
		for _, base := range []string{
			filepath.Join("/etc/letsencrypt/live", d),
			filepath.Join(s.dataDir, "ssl", d),
		} {
			c := filepath.Join(base, "fullchain.pem")
			k := filepath.Join(base, "privkey.pem")
			if fileExists(c) && fileExists(k) {
				return c, k
			}
		}
	}
	return cert, key
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
