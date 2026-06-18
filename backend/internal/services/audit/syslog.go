package audit

import (
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/luuuunet/owpanel/internal/services/settings"
)

type Syslog struct {
	settings *settings.Service
	mu       sync.Mutex
}

func NewSyslog(settingsSvc *settings.Service) *Syslog {
	return &Syslog{settings: settingsSvc}
}

type syslogConfig struct {
	Enabled  bool
	Host     string
	Port     string
	Protocol string
}

func (s *Syslog) loadConfig() syslogConfig {
	cfg := syslogConfig{}
	if s.settings == nil {
		return cfg
	}
	all, err := s.settings.GetAll()
	if err != nil {
		return cfg
	}
	cfg.Enabled = all["syslog_enabled"] == "true"
	cfg.Host = strings.TrimSpace(all["syslog_host"])
	cfg.Port = strings.TrimSpace(all["syslog_port"])
	if cfg.Port == "" {
		cfg.Port = "514"
	}
	cfg.Protocol = strings.ToLower(strings.TrimSpace(all["syslog_protocol"]))
	if cfg.Protocol == "" {
		cfg.Protocol = "udp"
	}
	return cfg
}

func (s *Syslog) Emit(eventType, message string) {
	cfg := s.loadConfig()
	if !cfg.Enabled || cfg.Host == "" {
		return
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	line := fmt.Sprintf("<134>1 %s owpanel pam - - [%s] %s", ts, eventType, message)
	go func() {
		if err := s.send(cfg, line); err != nil {
			log.Printf("syslog forward failed: %v", err)
		}
	}()
}

func (s *Syslog) send(cfg syslogConfig, line string) error {
	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	s.mu.Lock()
	defer s.mu.Unlock()
	if cfg.Protocol == "tcp" {
		conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			return err
		}
		defer conn.Close()
		_, err = conn.Write([]byte(line + "\n"))
		return err
	}
	conn, err := net.DialTimeout("udp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write([]byte(line))
	return err
}

func (s *Syslog) LoginSuccess(username, ip string) {
	s.Emit("login_success", fmt.Sprintf("user=%s ip=%s", username, ip))
}

func (s *Syslog) LoginFailure(username, ip, reason string) {
	s.Emit("login_failure", fmt.Sprintf("user=%s ip=%s reason=%s", username, ip, reason))
}

func (s *Syslog) SessionStart(username, asset, host string) {
	s.Emit("session_start", fmt.Sprintf("user=%s asset=%s host=%s", username, asset, host))
}

func (s *Syslog) SessionEnd(username, asset, host, status string) {
	s.Emit("session_end", fmt.Sprintf("user=%s asset=%s host=%s status=%s", username, asset, host, status))
}

func (s *Syslog) CommandBlock(username, command string) {
	s.Emit("command_block", fmt.Sprintf("user=%s command=%q", username, command))
}

func (s *Syslog) AccessRequestApproved(id uint, username, asset string, approver string) {
	s.Emit("access_request_approve", fmt.Sprintf("id=%d user=%s asset=%s approver=%s", id, username, asset, approver))
}

func (s *Syslog) PasswordRotation(accountID uint, username, status string) {
	s.Emit("password_rotation", fmt.Sprintf("account_id=%d user=%s status=%s", accountID, username, status))
}
