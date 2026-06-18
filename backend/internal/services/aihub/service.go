package aihub

import (
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"github.com/luuuunet/owpanel/internal/services/settings"
)

type Service struct {
	dataDir   string
	appstore  *appstore.Service
	settings  *settings.Service
}

func NewService(dataDir string, appSvc *appstore.Service, settingsSvc *settings.Service) *Service {
	return &Service{
		dataDir:  dataDir,
		appstore: appSvc,
		settings: settingsSvc,
	}
}
