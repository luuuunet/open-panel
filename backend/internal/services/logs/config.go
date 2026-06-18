package logs

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type viewConfig struct {
	Enabled        map[string]bool `json:"enabled"`
	RetentionDays  int             `json:"retention_days"`
	AutoCleanup    bool            `json:"auto_cleanup"`
	LoggingEnabled *bool           `json:"logging_enabled,omitempty"`
}

type RetentionSettings struct {
	RetentionDays  int  `json:"retention_days"`
	AutoCleanup    bool `json:"auto_cleanup"`
	LoggingEnabled bool `json:"logging_enabled"`
}

func (s *Service) configPath() string {
	return filepath.Join(s.dataDir, "logs-view.json")
}

func (s *Service) loadConfig() viewConfig {
	cfg := viewConfig{
		Enabled:       map[string]bool{},
		RetentionDays: 7,
	}
	data, err := os.ReadFile(s.configPath())
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	if cfg.Enabled == nil {
		cfg.Enabled = map[string]bool{}
	}
	return cfg
}

func (s *Service) IsLoggingEnabled() bool {
	return s.isLoggingEnabled(s.loadConfig())
}

func (s *Service) isLoggingEnabled(cfg viewConfig) bool {
	if cfg.LoggingEnabled == nil {
		return true
	}
	return *cfg.LoggingEnabled
}

func (s *Service) GetRetentionSettings() RetentionSettings {
	cfg := s.loadConfig()
	return RetentionSettings{
		RetentionDays:  cfg.RetentionDays,
		AutoCleanup:    cfg.AutoCleanup,
		LoggingEnabled: s.isLoggingEnabled(cfg),
	}
}

func (s *Service) SetRetentionSettings(days int, auto bool, loggingEnabled *bool) error {
	if days < 0 {
		days = 0
	}
	cfg := s.loadConfig()
	cfg.RetentionDays = days
	cfg.AutoCleanup = auto
	if loggingEnabled != nil {
		cfg.LoggingEnabled = loggingEnabled
	}
	return s.saveConfig(cfg)
}

func (s *Service) saveConfig(cfg viewConfig) error {
	if cfg.Enabled == nil {
		cfg.Enabled = map[string]bool{}
	}
	_ = os.MkdirAll(s.dataDir, 0755)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.configPath(), data, 0644)
}

func (s *Service) isEnabled(id string, cfg viewConfig) bool {
	if !s.isLoggingEnabled(cfg) {
		return false
	}
	if v, ok := cfg.Enabled[id]; ok {
		return v
	}
	return true
}
