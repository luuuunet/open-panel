package waf

import (
	"os"
	"path/filepath"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"gorm.io/gorm"
)

type Service struct {
	db      *gorm.DB
	dataDir string
	confDir string
}

func NewService(db *gorm.DB, dataDir string) *Service {
	confDir := filepath.Join(dataDir, "security")
	_ = os.MkdirAll(confDir, 0755)
	return &Service{db: db, dataDir: dataDir, confDir: confDir}
}

func (s *Service) ConfPath() string {
	return filepath.Join(s.confDir, "owpanel-waf.conf")
}

func (s *Service) BlacklistMapPath() string {
	return filepath.Join(s.confDir, "ip_blacklist.map")
}

func (s *Service) WhitelistMapPath() string {
	return filepath.Join(s.confDir, "ip_whitelist.map")
}

func (s *Service) LogFormatPath() string {
	return filepath.Join(s.confDir, "log_format.conf")
}

func (s *Service) Fail2BanFilterPath() string {
	return filepath.Join(s.confDir, "fail2ban-nginx-security.conf")
}

func defaultConfig(dataDir string) models.SecurityConfig {
	return models.SecurityConfig{
		Scope:            "global",
		RateLimitEnabled: true,
		RateLimitRate:    "10r/s",
		RateLimitBurst:   20,
		RateLimitNodelay: true,
		ConnLimitEnabled: true,
		ConnLimitPerIP:   50,
		BlacklistEnabled: true,
		AllowSearchBots:  true,
		BlockHeadlessBots: true,
		BlockHttpMethods: "TRACE,TRACK,DEBUG,CONNECT",
		SlowAttackEnabled:      true,
		ClientBodyTimeoutSec:   12,
		ClientHeaderTimeoutSec: 12,
		ApiRateLimitRate:    "30r/s",
		ApiRateLimitBurst:   60,
		HotlinkAllowEmpty:   true,
		HeaderPreset:        "custom",
		FilterEnabled:    true,
		BlockBadUserAgent: true,
		BlockScannerUA:   true,
		HeadersEnabled:   true,
		CSP:              "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'",
		XFrameOptions:    "SAMEORIGIN",
		HSTSEnabled:      true,
		HSTSMaxAge:       31536000,
		XContentTypeOpts: true,
		ReferrerPolicy:   "strict-origin-when-cross-origin",
		LogFormatEnabled: true,
		SecurityLogPath:  settings.DefaultSecurityLogPath(dataDir),
		GeoMode:          "block",
	}
}

func (s *Service) GetConfig() (*models.SecurityConfig, error) {
	s.ensureDefaults()
	var cfg models.SecurityConfig
	err := s.db.Where("scope = ?", "global").First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		cfg = defaultConfig(s.dataDir)
		if err := s.db.Create(&cfg).Error; err != nil {
			return nil, err
		}
		return &cfg, nil
	}
	return &cfg, err
}

func (s *Service) UpdateConfig(patch *models.SecurityConfig) (*models.SecurityConfig, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	patch.ID = cfg.ID
	patch.Scope = "global"
	if err := s.db.Save(patch).Error; err != nil {
		return nil, err
	}
	return s.GetConfig()
}

func (s *Service) ensureDefaults() {
	s.SeedDefaults()
	s.SeedFilterRules()
	var count int64
	s.db.Model(&models.SecurityConfig{}).Count(&count)
	if count == 0 {
		cfg := defaultConfig(s.dataDir)
		s.db.Create(&cfg)
	}
}
