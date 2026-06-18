package website

import (
	"github.com/luuuunet/owpanel/internal/models"
	dbsvc "github.com/luuuunet/owpanel/internal/services/database"
	"github.com/luuuunet/owpanel/internal/services/dns"
	"github.com/luuuunet/owpanel/internal/services/ftp"
	"github.com/luuuunet/owpanel/internal/services/php"
	"github.com/luuuunet/owpanel/internal/services/cache"
	"github.com/luuuunet/owpanel/internal/services/webserver"
	"gorm.io/gorm"
)

type edgeWorkerInclude interface {
	ServerBlockDirectives(site *models.Website) string
}

type Service struct {
	db         *gorm.DB
	dataDir    string
	ftp        *ftp.Service
	database   *dbsvc.Service
	dns        *dns.Service
	ws         *webserver.Manager
	cache      *cache.Service
	edgeWorker edgeWorkerInclude
}

func NewService(db *gorm.DB, dataDir string, ftpSvc *ftp.Service, dbSvc *dbsvc.Service, dnsSvc *dns.Service, ws *webserver.Manager, cacheSvc *cache.Service) *Service {
	s := &Service{db: db, dataDir: dataDir, ftp: ftpSvc, database: dbSvc, dns: dnsSvc, ws: ws, cache: cacheSvc}
	s.ensureCategories()
	return s
}

func (s *Service) SetEdgeWorker(ew edgeWorkerInclude) {
	s.edgeWorker = ew
}

func phpPort(version string) int {
	if version == "" || version == "static" {
		return 0
	}
	return php.PortForVersion(version)
}
