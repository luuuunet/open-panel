package bastion

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

const policyKey = "bastion_command_policy"

var defaultBlockPatterns = []string{
	`rm\s+-rf\s+/`,
	`rm\s+-rf\s+\*`,
	`\bmkfs\b`,
	`\bdd\s+if=`,
	`:(){ :|:& };:`,
	`\bshutdown\b`,
	`\breboot\b`,
	`\binit\s+0\b`,
	`>\s*/dev/sd`,
	`\bwipefs\b`,
	`\bformat\s+c:`,
}

type CommandPolicy struct {
	Mode        string   `json:"mode"` // block | warn
	Blocklist   []string `json:"blocklist"`
	CustomRules []string `json:"custom_rules"`
}

type CommandFilter struct {
	service *Service
	policy  CommandPolicy
	compiled []*regexp.Regexp
}

func (s *Service) LoadCommandPolicy() CommandPolicy {
	var setting models.PanelSetting
	p := CommandPolicy{Mode: "block", Blocklist: defaultBlockPatterns}
	if s.db.Where("key = ?", policyKey).First(&setting).Error == nil && strings.TrimSpace(setting.Value) != "" {
		_ = json.Unmarshal([]byte(setting.Value), &p)
	}
	if len(p.Blocklist) == 0 {
		p.Blocklist = defaultBlockPatterns
	}
	return p
}

func (s *Service) SaveCommandPolicy(p CommandPolicy) error {
	b, _ := json.Marshal(p)
	var setting models.PanelSetting
	err := s.db.Where("key = ?", policyKey).First(&setting).Error
	if err != nil {
		if err2 := s.db.Create(&models.PanelSetting{Key: policyKey, Value: string(b)}).Error; err2 != nil {
			return err2
		}
		s.initCommands()
		return nil
	}
	setting.Value = string(b)
	if err := s.db.Save(&setting).Error; err != nil {
		return err
	}
	s.initCommands()
	return nil
}

func (s *Service) commandsFilter() *CommandFilter {
	p := s.LoadCommandPolicy()
	cf := &CommandFilter{service: s, policy: p}
	rules := append(append([]string{}, p.Blocklist...), p.CustomRules...)
	for _, r := range rules {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		re, err := regexp.Compile("(?i)" + r)
		if err == nil {
			cf.compiled = append(cf.compiled, re)
		}
	}
	return cf
}

func (s *Service) initCommands() {
	s.commands = s.commandsFilter()
}

// ValidateCommand checks ops/ad-hoc commands against bastion command policy.
func (s *Service) ValidateCommand(command string) error {
	cf := s.commandsFilter()
	lines := strings.Split(command, "\n")
	for _, line := range lines {
		cmdLine := strings.TrimSpace(line)
		if cmdLine == "" || strings.HasPrefix(cmdLine, "#") {
			continue
		}
		for _, re := range cf.compiled {
			if re.MatchString(cmdLine) {
				if cf.policy.Mode == "warn" {
					s.logBlocked(0, "", cmdLine, "warned", 0, "")
					continue
				}
				return fmt.Errorf("命令被安全策略拦截: %s", cmdLine)
			}
		}
	}
	return nil
}

// FilterInput processes stdin chunks; returns filtered data, blocked flag, detected command line.
func (cf *CommandFilter) FilterInput(buf *strings.Builder, data []byte) ([]byte, bool, string) {
	buf.Write(data)
	text := buf.String()
	// detect complete command on enter
	if !strings.Contains(text, "\r") && !strings.Contains(text, "\n") {
		return data, false, ""
	}
	lines := strings.Split(text, "\n")
	last := lines[len(lines)-1]
	if !strings.Contains(text, "\n") && strings.Contains(text, "\r") {
		parts := strings.Split(text, "\r")
		last = parts[len(parts)-1]
	}
	cmdLine := strings.TrimSpace(last)
	buf.Reset()
	for _, re := range cf.compiled {
		if re.MatchString(cmdLine) {
			if cf.policy.Mode == "warn" {
				cf.service.logBlocked(0, "", cmdLine, "warned", 0, "")
				return data, false, cmdLine
			}
			return nil, true, cmdLine
		}
	}
	return data, false, cmdLine
}

func (s *Service) ListCommandAudits(limit int) ([]models.BastionCommandAudit, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var list []models.BastionCommandAudit
	if err := s.db.Order("id desc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
