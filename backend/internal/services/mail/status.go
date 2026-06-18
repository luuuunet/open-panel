package mail

import (
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

type ServiceStatus struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Enabled   bool   `json:"enabled"`
}

type PortStatus struct {
	Port  int    `json:"port"`
	Label string `json:"label"`
	Open  bool   `json:"open"`
	Proto string `json:"proto"`
}

type StackStatus struct {
	Installed     bool            `json:"installed"`
	Ready         bool            `json:"ready"`
	PlatformNote  string          `json:"platform_note,omitempty"`
	Services      []ServiceStatus `json:"services"`
	Ports         []PortStatus    `json:"ports"`
	DomainCount   int64           `json:"domain_count"`
	MailboxCount  int64           `json:"mailbox_count"`
	VMailBase     string          `json:"vmail_base"`
	Hostname      string          `json:"hostname"`
	ServerIP      string          `json:"server_ip"`
	ConfigSynced  bool            `json:"config_synced"`
	LastSyncError string          `json:"last_sync_error,omitempty"`
}

func (s *Service) Status() (*StackStatus, error) {
	st := &StackStatus{
		VMailBase: s.vmailBase(),
		Hostname:  hostname(),
		ServerIP:  s.serverIP(),
	}
	if runtime.GOOS == "windows" {
		st.PlatformNote = "Windows 主机不支持 Postfix/Dovecot，请在 Linux 服务器上使用邮件功能。"
		return st, nil
	}

	st.Services = []ServiceStatus{
		s.serviceStatus("postfix", "Postfix"),
		s.serviceStatus("dovecot", "Dovecot"),
	}
	st.Installed = st.Services[0].Installed && st.Services[1].Installed
	st.Ready = st.Services[0].Running && st.Services[1].Running

	st.Ports = []PortStatus{
		checkPort(25, "SMTP", "tcp"),
		checkPort(587, "Submission", "tcp"),
		checkPort(993, "IMAPS", "tcp"),
		checkPort(995, "POP3S", "tcp"),
	}

	s.db.Model(&models.MailDomain{}).Count(&st.DomainCount)
	s.db.Model(&models.MailBox{}).Count(&st.MailboxCount)

	var unsynced int64
	s.db.Model(&models.MailBox{}).Where("synced = ?", false).Count(&unsynced)
	st.ConfigSynced = unsynced == 0
	if unsynced > 0 {
		var box models.MailBox
		if s.db.Where("synced = ?", false).First(&box).Error == nil && box.SyncError != "" {
			st.LastSyncError = box.SyncError
		}
	}
	return st, nil
}

func (s *Service) serviceStatus(unit, name string) ServiceStatus {
	st := ServiceStatus{Key: unit, Name: name}
	if runtime.GOOS == "windows" {
		return st
	}
	bin := unit
	if _, err := exec.LookPath(bin); err != nil {
		return st
	}
	st.Installed = systemdUnitExists(unit)
	if out, err := exec.Command("systemctl", "is-active", unit).Output(); err == nil {
		st.Running = strings.TrimSpace(string(out)) == "active"
	}
	if out, err := exec.Command("systemctl", "is-enabled", unit).Output(); err == nil {
		st.Enabled = strings.TrimSpace(string(out)) == "enabled"
	}
	return st
}

func systemdUnitExists(unit string) bool {
	out, err := exec.Command("systemctl", "list-unit-files", unit+".service", "--no-pager", "--no-legend").Output()
	if err != nil {
		return commandExists(unit)
	}
	return strings.Contains(string(out), unit)
}

func checkPort(port int, label, proto string) PortStatus {
	ps := PortStatus{Port: port, Label: label, Proto: proto}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	conn, err := net.DialTimeout(proto, addr, 2*time.Second)
	if err == nil {
		ps.Open = true
		_ = conn.Close()
	}
	return ps
}

func hostname() string {
	if out, err := exec.Command("hostname", "-f").Output(); err == nil {
		if h := strings.TrimSpace(string(out)); h != "" {
			return h
		}
	}
	h, _ := os.Hostname()
	return strings.TrimSpace(h)
}

func (s *Service) serverIP() string {
	if s.settings != nil {
		if all, err := s.settings.GetAll(); err == nil {
			if ip := strings.TrimSpace(all["server_public_ip"]); ip != "" {
				return ip
			}
		}
	}
	if out, err := exec.Command("hostname", "-I").Output(); err == nil {
		parts := strings.Fields(string(out))
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return ""
}
