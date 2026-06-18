package aisite

import (
	"github.com/luuuunet/owpanel/internal/services/aichat"
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"github.com/luuuunet/owpanel/internal/services/cron"
	"github.com/luuuunet/owpanel/internal/services/devops"
	"github.com/luuuunet/owpanel/internal/services/nodejs"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"github.com/luuuunet/owpanel/internal/services/website"
	"gorm.io/gorm"
)

type Service struct {
	db       *gorm.DB
	dataDir  string
	website  *website.Service
	devops   *devops.Service
	aichat   *aichat.Service
	appstore *appstore.Service
	settings *settings.Service
	nodejs   *nodejs.Service
	cron     *cron.Service
}

func NewService(
	db *gorm.DB,
	dataDir string,
	websiteSvc *website.Service,
	devopsSvc *devops.Service,
	aichatSvc *aichat.Service,
	appSvc *appstore.Service,
	settingsSvc *settings.Service,
	nodejsSvc *nodejs.Service,
	cronSvc *cron.Service,
) *Service {
	return &Service{
		db:       db,
		dataDir:  dataDir,
		website:  websiteSvc,
		devops:   devopsSvc,
		aichat:   aichatSvc,
		appstore: appSvc,
		settings: settingsSvc,
		nodejs:   nodejsSvc,
		cron:     cronSvc,
	}
}
