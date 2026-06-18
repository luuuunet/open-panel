package enterprise

import (
	"github.com/luuuunet/owpanel/internal/services/audit"
	"github.com/luuuunet/owpanel/internal/services/cluster"
	"github.com/luuuunet/owpanel/internal/services/dashboard"
	"github.com/luuuunet/owpanel/internal/services/security"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"github.com/luuuunet/owpanel/internal/services/uptime"
	"gorm.io/gorm"
)

type Service struct {
	db        *gorm.DB
	settings  *settings.Service
	cluster   *cluster.Service
	dashboard *dashboard.Service
	security  *security.Service
	uptime    *uptime.Service
	syslog    *audit.Syslog
}

func NewService(
	db *gorm.DB,
	settingsSvc *settings.Service,
	clusterSvc *cluster.Service,
	dashSvc *dashboard.Service,
	securitySvc *security.Service,
	uptimeSvc *uptime.Service,
	syslogSvc *audit.Syslog,
) *Service {
	s := &Service{
		db:        db,
		settings:  settingsSvc,
		cluster:   clusterSvc,
		dashboard: dashSvc,
		security:  securitySvc,
		uptime:    uptimeSvc,
		syslog:    syslogSvc,
	}
	s.settings.EnsureKeys("audit_retention_days", "audit_syslog_forward")
	return s
}
