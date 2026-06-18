package cilium

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/appstore"
	"gorm.io/gorm"
)

const (
	k3sAppKey     = "k3s"
	ciliumAppKey  = "cilium"
)

type Service struct {
	db      *gorm.DB
	apps    *appstore.Service
	dataDir string
}

func NewService(db *gorm.DB, apps *appstore.Service, dataDir string) *Service {
	return &Service{db: db, apps: apps, dataDir: dataDir}
}

func defaultConfig() CiliumConfig {
	return CiliumConfig{
		Scope:               "global",
		HostFirewallEnabled: true,
		HubbleEnabled:       true,
		HubbleUIEnabled:     true,
		AuditMode:           true,
	}
}

func (s *Service) ensureDefaults() {
	var n int64
	s.db.Model(&CiliumConfig{}).Where("scope = ?", "global").Count(&n)
	if n == 0 {
		cfg := defaultConfig()
		_ = s.db.Create(&cfg).Error
	}
}

func (s *Service) GetConfig() (*CiliumConfig, error) {
	s.ensureDefaults()
	var cfg CiliumConfig
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

func (s *Service) UpdateConfig(patch *CiliumConfig) (*CiliumConfig, error) {
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

func (s *Service) linuxHost() bool {
	return runtime.GOOS == "linux"
}

func (s *Service) k3sRunning() bool {
	return appstore.K3sRunning()
}

func (s *Service) appInstalled(key string) bool {
	if s.apps == nil {
		return false
	}
	app, err := s.apps.Get(key)
	return err == nil && app.Installed
}

func (s *Service) kernelOK() bool {
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return false
	}
	ver := strings.TrimSpace(string(data))
	parts := strings.SplitN(ver, ".", 3)
	if len(parts) < 2 {
		return false
	}
	major := 0
	minor := 0
	fmt.Sscanf(parts[0], "%d", &major)
	fmt.Sscanf(parts[1], "%d", &minor)
	if major > 5 {
		return true
	}
	if major == 5 && minor >= 10 {
		return true
	}
	return false
}

type InstallStackResult struct {
	Message string `json:"message"`
	K3s     bool   `json:"k3s"`
	Cilium  bool   `json:"cilium"`
}

func (s *Service) InstallStack(installK3s, installCilium bool) (*InstallStackResult, error) {
	if !s.linuxHost() {
		return nil, fmt.Errorf("Cilium 仅支持 Linux 服务器")
	}
	res := &InstallStackResult{}
	if installK3s && !s.k3sRunning() {
		if err := appstore.RunK3sInstall(s.dataDir); err != nil {
			return nil, fmt.Errorf("k3s 安装失败: %w", err)
		}
		s.markInstalled(k3sAppKey)
		res.K3s = true
	} else {
		res.K3s = s.k3sRunning()
	}
	if installCilium {
		if !s.k3sRunning() {
			return nil, fmt.Errorf("k3s 未就绪，无法安装 Cilium")
		}
		if err := appstore.RunCiliumInstall(s.dataDir); err != nil {
			return nil, err
		}
		s.markInstalled(ciliumAppKey)
		res.Cilium = true
	}
	res.Message = "Cilium 栈安装完成"
	return res, nil
}

func (s *Service) ApplyHostFirewall() (*CiliumConfig, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	if err := helmUpgradeHostFirewall(cfg.HostFirewallEnabled, cfg.HubbleEnabled, cfg.HubbleUIEnabled); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *Service) markInstalled(key string) {
	if s.apps == nil {
		return
	}
	_ = s.apps.MarkInstalled(key, "latest")
}
