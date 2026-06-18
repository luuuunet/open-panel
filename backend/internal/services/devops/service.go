package devops

import (
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"github.com/luuuunet/owpanel/internal/services/compose"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"github.com/luuuunet/owpanel/internal/services/webserver"
	"gorm.io/gorm"
)

type Service struct {
	db        *gorm.DB
	dataDir   string
	compose   *compose.Service
	webserver *webserver.Manager
	appstore  *appstore.Service
	settings  *settings.Service
}

func NewService(
	db *gorm.DB,
	dataDir string,
	composeSvc *compose.Service,
	ws *webserver.Manager,
	appSvc *appstore.Service,
	settingsSvc *settings.Service,
) *Service {
	return &Service{
		db:        db,
		dataDir:   dataDir,
		compose:   composeSvc,
		webserver: ws,
		appstore:  appSvc,
		settings:  settingsSvc,
	}
}
