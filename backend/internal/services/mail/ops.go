package mail

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type DNSHint struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Value    string `json:"value"`
	Priority int    `json:"priority,omitempty"`
	Purpose  string `json:"purpose"`
	Required bool   `json:"required"`
}

func (s *Service) DNSHints(domain string) []DNSHint {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return nil
	}
	ip := s.serverIP()
	host := s.mailHostname(domain)
	hints := []DNSHint{
		{Type: "MX", Name: domain, Value: host, Priority: 10, Purpose: "mail_routing", Required: true},
		{Type: "A", Name: host, Value: ip, Purpose: "mail_host", Required: true},
		{Type: "TXT", Name: domain, Value: "v=spf1 mx a ip4:" + ip + " ~all", Purpose: "spf", Required: true},
		{Type: "TXT", Name: "_dmarc." + domain, Value: "v=DMARC1; p=none; rua=mailto:postmaster@" + domain, Purpose: "dmarc", Required: false},
	}
	if ip == "" {
		for i := range hints {
			if hints[i].Type == "A" {
				hints[i].Value = "<服务器公网 IP>"
			}
			if hints[i].Type == "TXT" && hints[i].Purpose == "spf" {
				hints[i].Value = "v=spf1 mx a ~all"
			}
		}
	}
	return hints
}

func (s *Service) mailHostname(domain string) string {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return "mail"
	}
	return "mail." + domain
}

func (s *Service) RestartServices() error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("Windows 不支持")
	}
	for _, svc := range []string{"postfix", "dovecot"} {
		if out, err := exec.Command("systemctl", "restart", svc).CombinedOutput(); err != nil {
			return fmt.Errorf("restart %s: %s", svc, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func (s *Service) SendTestMail(from, to, subject, body string) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("Windows 不支持发送测试邮件")
	}
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" || to == "" {
		return fmt.Errorf("发件人与收件人不能为空")
	}
	if subject == "" {
		subject = "OWPanel Mail Test"
	}
	if body == "" {
		body = "This is a test message from OWPanel mail server."
	}
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n", from, to, subject, body)
	cmd := exec.Command("sendmail", "-f", from, to)
	cmd.Stdin = strings.NewReader(msg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sendmail: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *Service) ApplySSL(domain string) error {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return fmt.Errorf("域名不能为空")
	}
	if runtime.GOOS == "windows" {
		return fmt.Errorf("Windows 不支持")
	}
	cert, key := "", ""
	for _, base := range []string{
		fmt.Sprintf("/etc/letsencrypt/live/%s", domain),
		fmt.Sprintf("%s/ssl/%s", s.dataDir, domain),
	} {
		c := base + "/fullchain.pem"
		k := base + "/privkey.pem"
		if fileExists(c) && fileExists(k) {
			cert, key = c, k
			break
		}
	}
	if cert == "" {
		return fmt.Errorf("未找到域名 %s 的 SSL 证书，请先在 SSL 管理中申请或上传", domain)
	}
	confPath := dovecotInclude()
	data, err := os.ReadFile(confPath)
	if err != nil {
		return s.syncMaps()
	}
	content := string(data)
	content = replaceSSLLine(content, "ssl_cert", cert)
	content = replaceSSLLine(content, "ssl_key", key)
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		return err
	}
	_ = exec.Command("postconf", "-e", "smtpd_tls_cert_file="+cert).Run()
	_ = exec.Command("postconf", "-e", "smtpd_tls_key_file="+key).Run()
	_ = exec.Command("systemctl", "reload", "postfix").Run()
	if out, err := exec.Command("systemctl", "reload", "dovecot").CombinedOutput(); err != nil {
		return fmt.Errorf("reload dovecot: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func replaceSSLLine(content, key, path string) string {
	prefix := key + " = <"
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			lines[i] = key + " = <" + path
			return strings.Join(lines, "\n")
		}
	}
	return content
}
