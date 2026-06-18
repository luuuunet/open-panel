package cache

import (
	"os"
	"path/filepath"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"github.com/luuuunet/owpanel/internal/services/webserver"
	"gorm.io/gorm"
)

type Service struct {
	db            *gorm.DB
	dataDir       string
	confDir       string
	regen         func() error
	reload        func() error
	nginxConfPath func() string
	apps          *appstore.Service
	webserver     *webserver.Manager
	applyVhost    func(uint) error
}

func NewService(db *gorm.DB, dataDir string) *Service {
	confDir := filepath.Join(dataDir, "cache")
	_ = os.MkdirAll(confDir, 0755)
	_ = os.MkdirAll(filepath.Join(confDir, "proxy"), 0755)
	_ = os.MkdirAll(filepath.Join(confDir, "fastcgi"), 0755)
	return &Service{db: db, dataDir: dataDir, confDir: confDir}
}

func (s *Service) SetHooks(regen, reload func() error) {
	s.regen = regen
	s.reload = reload
}

func (s *Service) ConfPath() string {
	return filepath.Join(s.confDir, "owpanel-cache.conf")
}

func (s *Service) ProxyCacheDir() string {
	return filepath.Join(s.confDir, "proxy")
}

func (s *Service) FastCGICacheDir() string {
	return filepath.Join(s.confDir, "fastcgi")
}

func defaultConfig() models.CacheConfig {
	return models.CacheConfig{
		Scope:            "global",
		Enabled:          false,
		DevMode:          false,
		AutoSiteEnable:   true,
		ProxyMaxSize:     "5g",
		ProxyInactive:    "60m",
		FastcgiMaxSize:   "2g",
		FastcgiInactive:  "30m",
		ZoneMemory:       "100m",
		HtmlTTLMinutes:   5,
		StaticTTLHours:   168,
		BypassCookies:    "PHPSESSID|wordpress_logged_in|session|auth_token",
		BypassPaths:      "/admin|/wp-admin|/api/|/login",
		StaleEnabled:     true,
		HonorOrigin:      false,
		CacheQueryString: true,
	}
}

func (s *Service) ensureDefaults() {
	var n int64
	s.db.Model(&models.CacheConfig{}).Where("scope = ?", "global").Count(&n)
	if n == 0 {
		cfg := defaultConfig()
		_ = s.db.Create(&cfg).Error
	}
}

func (s *Service) GetConfig() (*models.CacheConfig, error) {
	s.ensureDefaults()
	var cfg models.CacheConfig
	err := s.db.Where("scope = ?", "global").First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		cfg = defaultConfig()
		if err := s.db.Create(&cfg).Error; err != nil {
			return nil, err
		}
		return &cfg, nil
	}
	return &cfg, err
}

func (s *Service) UpdateConfig(patch *models.CacheConfig) (*models.CacheConfig, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	if err := s.db.Model(cfg).Updates(patch).Error; err != nil {
		return nil, err
	}
	return s.GetConfig()
}

func (s *Service) GlobalEnabled() bool {
	cfg, err := s.GetConfig()
	return err == nil && cfg != nil && cfg.Enabled
}

func (s *Service) ShouldEnableNewSite() bool {
	cfg, err := s.GetConfig()
	return err == nil && cfg != nil && cfg.Enabled && cfg.AutoSiteEnable
}

func (s *Service) SiteEnabled(site *models.Website) bool {
	if site == nil || !site.CacheEnabled {
		return false
	}
	return s.GlobalEnabled()
}

func (s *Service) htmlTTL(site *models.Website, cfg *models.CacheConfig) int {
	if site != nil && site.CacheHtmlTTL > 0 {
		return site.CacheHtmlTTL
	}
	if cfg != nil && cfg.HtmlTTLMinutes > 0 {
		return cfg.HtmlTTLMinutes
	}
	return 5
}

func (s *Service) staticTTL(site *models.Website, cfg *models.CacheConfig) int {
	if site != nil && site.CacheStaticTTL > 0 {
		return site.CacheStaticTTL
	}
	if cfg != nil && cfg.StaticTTLHours > 0 {
		return cfg.StaticTTLHours
	}
	return 168
}
