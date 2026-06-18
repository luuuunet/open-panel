package settings

import (
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

type Service struct {
	db      *gorm.DB
	dataDir string
}

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

func NewServiceWithDataDir(db *gorm.DB, dataDir string) *Service {
	return &Service{db: db, dataDir: dataDir}
}

func (s *Service) baseDefaults() map[string]string {
	backup := defaultBackupPath(s.dataDir)
	website := defaultWebsitePath(s.dataDir)
	return map[string]string{
		"panel_name":            "OWPanel",
		"panel_port":            "8888",
		"panel_ssl":             "false",
		"login_captcha":         "false",
		"session_timeout":       "86400",
		"backup_path":           backup,
		"website_path":          website,
		"active_web_server":     "nginx",
		"ai_enabled":            "false",
		"ai_provider":           "openai",
		"ai_api_key":            "",
		"ai_base_url":           "",
		"ai_model":              "gpt-4o-mini",
		"server_public_ip":      "",
		"auto_ops_enabled":      "true",
		"auto_ops_interval":     "30",
		"auto_ops_cooldown":     "300",
		"auto_ops_max_restarts": "5",
		"cluster_agent_token":   "",
		"panel_ip_whitelist_enabled": "false",
		"panel_ip_whitelist":         "",
		"panel_ip_blacklist":         "",
		"password_require_strong":    "true",
		"panel_security_headers":     "true",
		"power_save_enabled":         "false",
	}
}

func (s *Service) GetAll() (map[string]string, error) {
	s.ensureDefaults()
	defaults := s.baseDefaults()
	keys := make([]string, 0, len(defaults))
	for k := range defaults {
		keys = append(keys, k)
	}
	s.EnsureKeys(keys...)
	s.ensureSafePath()
	s.migrateLegacyPaths()
	var rows []models.PanelSetting
	if err := s.db.Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]string, len(rows))
	for _, r := range rows {
		result[r.Key] = r.Value
	}
	return result, nil
}

func (s *Service) Update(data map[string]string) error {
	for k, v := range data {
		if k == "ai_api_key" && strings.TrimSpace(v) == "" {
			continue
		}
		if k == "ai_api_key" {
			v = strings.TrimSpace(v)
		}
		if err := s.set(k, v); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) set(key, value string) error {
	var row models.PanelSetting
	err := s.db.Where("key = ?", key).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return s.db.Create(&models.PanelSetting{Key: key, Value: value}).Error
	}
	if err != nil {
		return err
	}
	return s.db.Model(&row).Update("value", value).Error
}

func (s *Service) EnsureKeys(keys ...string) {
	for _, k := range keys {
		var count int64
		s.db.Model(&models.PanelSetting{}).Where("key = ?", k).Count(&count)
		if count > 0 {
			continue
		}
		if k == "panel_safe_path" {
			s.db.Create(&models.PanelSetting{Key: k, Value: safePathValue()})
			continue
		}
		if v, ok := s.baseDefaults()[k]; ok {
			s.db.Create(&models.PanelSetting{Key: k, Value: v})
		}
	}
}

func (s *Service) ensureSafePath() {
	var row models.PanelSetting
	err := s.db.Where("key = ?", "panel_safe_path").First(&row).Error
	if err == gorm.ErrRecordNotFound {
		s.db.Create(&models.PanelSetting{Key: "panel_safe_path", Value: safePathValue()})
		return
	}
	if err == nil && strings.TrimSpace(row.Value) == "" {
		s.db.Model(&row).Update("value", safePathValue())
	}
}

func (s *Service) ensureDefaults() {
	var count int64
	s.db.Model(&models.PanelSetting{}).Count(&count)
	if count > 0 {
		s.ensureSafePath()
		return
	}
	for k, v := range s.baseDefaults() {
		s.db.Create(&models.PanelSetting{Key: k, Value: v})
	}
	s.db.Create(&models.PanelSetting{Key: "panel_safe_path", Value: safePathValue()})
}

func (s *Service) migrateLegacyPaths() {
	if s.dataDir == "" {
		return
	}
	for _, key := range []string{"backup_path", "website_path"} {
		var row models.PanelSetting
		if s.db.Where("key = ?", key).First(&row).Error != nil {
			continue
		}
		val := strings.TrimSpace(row.Value)
		if val == "" || !strings.HasPrefix(val, "/www/") {
			continue
		}
		migrated := ResolvePanelPath(s.dataDir, val)
		if migrated != val {
			s.db.Model(&row).Update("value", migrated)
		}
	}
}
