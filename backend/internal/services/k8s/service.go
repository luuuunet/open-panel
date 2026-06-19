package k8s

import (
	"runtime"

	"github.com/luuuunet/owpanel/internal/bootstrap"
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"github.com/luuuunet/owpanel/internal/services/settings"
)

const k3sAppKey = "k3s"

type Service struct {
	apps     *appstore.Service
	settings *settings.Service
	dataDir  string
}

func NewService(apps *appstore.Service, settingsSvc *settings.Service, dataDir string) *Service {
	return &Service{apps: apps, settings: settingsSvc, dataDir: dataDir}
}

func (s *Service) linuxHost() bool {
	return runtime.GOOS == "linux"
}

func (s *Service) totalRAMMB() uint64 {
	return bootstrap.TotalRAMMB()
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

func (s *Service) markInstalled(key string) {
	if s.apps == nil {
		return
	}
	_ = s.apps.MarkInstalled(key, "latest")
}
