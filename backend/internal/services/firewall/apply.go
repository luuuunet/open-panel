package firewall

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

type Service struct {
	db *gorm.DB
}

type Status struct {
	Backend   string `json:"backend"`
	Active    bool   `json:"active"`
	Supported bool   `json:"supported"`
	Message   string `json:"message,omitempty"`
	RuleCount int    `json:"rule_count"`
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) List() ([]models.FirewallRule, error) {
	var rules []models.FirewallRule
	err := s.db.Order("id desc").Find(&rules).Error
	return rules, err
}

func (s *Service) Create(rule *models.FirewallRule) error {
	if rule.Protocol == "" {
		rule.Protocol = "tcp"
	}
	if rule.Action == "" {
		rule.Action = "allow"
	}
	if err := s.db.Create(rule).Error; err != nil {
		return err
	}
	return s.applyRule(rule)
}

func (s *Service) Delete(id uint) error {
	var rule models.FirewallRule
	if err := s.db.First(&rule, id).Error; err != nil {
		return err
	}
	_ = s.removeRule(&rule)
	return s.db.Delete(&models.FirewallRule{}, id).Error
}

func (s *Service) SyncAll() error {
	var rules []models.FirewallRule
	if err := s.db.Order("id asc").Find(&rules).Error; err != nil {
		return err
	}
	for i := range rules {
		if err := s.applyRule(&rules[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Status() Status {
	st := Status{Backend: "none", Supported: runtime.GOOS != "windows"}
	var count int64
	s.db.Model(&models.FirewallRule{}).Count(&count)
	st.RuleCount = int(count)
	if runtime.GOOS == "windows" {
		st.Message = "Windows 下规则仅保存在面板，不写入系统防火墙"
		return st
	}
	if backend, active, msg := detectBackend(); backend != "" {
		st.Backend = backend
		st.Active = active
		st.Message = msg
		return st
	}
	st.Message = "未检测到 ufw 或 firewalld，规则仅保存在面板"
	return st
}

func detectBackend() (backend string, active bool, message string) {
	if _, err := exec.LookPath("ufw"); err == nil {
		out, _ := exec.Command("ufw", "status").CombinedOutput()
		text := strings.ToLower(string(out))
		active = strings.Contains(text, "active")
		return "ufw", active, strings.TrimSpace(string(out))
	}
	if _, err := exec.LookPath("firewall-cmd"); err == nil {
		out, _ := exec.Command("firewall-cmd", "--state").CombinedOutput()
		active = strings.Contains(strings.ToLower(string(out)), "running")
		return "firewalld", active, strings.TrimSpace(string(out))
	}
	return "", false, ""
}

func (s *Service) applyRule(rule *models.FirewallRule) error {
	err := applySystemRule(rule, true)
	updates := map[string]interface{}{
		"applied": err == nil,
	}
	if err != nil {
		updates["apply_error"] = err.Error()
	} else {
		updates["apply_error"] = ""
	}
	s.db.Model(rule).Updates(updates)
	return err
}

func (s *Service) removeRule(rule *models.FirewallRule) error {
	return applySystemRule(rule, false)
}

func applySystemRule(rule *models.FirewallRule, add bool) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	backend, _, _ := detectBackend()
	proto := strings.ToLower(rule.Protocol)
	if proto == "" {
		proto = "tcp"
	}
	portSpec := fmt.Sprintf("%d/%s", rule.Port, proto)
	action := strings.ToLower(rule.Action)
	if action == "" {
		action = "allow"
	}

	switch backend {
	case "ufw":
		var args []string
		if add {
			if action == "deny" {
				args = []string{"deny", portSpec}
			} else {
				args = []string{"allow", portSpec}
			}
			if ip := strings.TrimSpace(rule.SourceIP); ip != "" {
				args = append(args, "from", ip)
			}
		} else {
			if action == "deny" {
				args = []string{"delete", "deny", portSpec}
			} else {
				args = []string{"delete", "allow", portSpec}
			}
		}
		out, err := exec.Command("ufw", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("ufw: %s", strings.TrimSpace(string(out)))
		}
		return nil
	case "firewalld":
		if add {
			var out []byte
			var err error
			if action == "deny" {
				out, err = exec.Command("firewall-cmd", "--permanent", "--add-rich-rule",
					fmt.Sprintf(`rule family="ipv4" port port="%d" protocol="%s" reject`, rule.Port, proto)).CombinedOutput()
			} else {
				out, err = exec.Command("firewall-cmd", "--permanent", fmt.Sprintf("--add-port=%s", portSpec)).CombinedOutput()
			}
			if err != nil {
				return fmt.Errorf("firewalld: %s", strings.TrimSpace(string(out)))
			}
			out, err = exec.Command("firewall-cmd", "--reload").CombinedOutput()
			if err != nil {
				return fmt.Errorf("firewalld reload: %s", strings.TrimSpace(string(out)))
			}
			return nil
		}
		out, err := exec.Command("firewall-cmd", "--permanent", fmt.Sprintf("--remove-port=%s", portSpec)).CombinedOutput()
		if err != nil {
			return fmt.Errorf("firewalld: %s", strings.TrimSpace(string(out)))
		}
		_, _ = exec.Command("firewall-cmd", "--reload").CombinedOutput()
		return nil
	default:
		return nil
	}
}
