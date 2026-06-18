package edgeworker

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

type Service struct {
	db            *gorm.DB
	dataDir       string
	confDir       string
	regen         func() error
	reload        func() error
	nginxConfPath func() string
	getActiveWS   func() string
	panelPort     int
	apiPrefix     func() string
}

func NewService(db *gorm.DB, dataDir string) *Service {
	confDir := filepath.Join(dataDir, "nginx")
	_ = os.MkdirAll(confDir, 0755)
	_ = os.MkdirAll(filepath.Join(confDir, "scripts"), 0755)
	_ = os.MkdirAll(filepath.Join(confDir, "lua"), 0755)
	return &Service{db: db, dataDir: dataDir, confDir: confDir, panelPort: 8888}
}

func (s *Service) SetHooks(regen, reload func() error, nginxConfPath, getActiveWS func() string) {
	s.regen = regen
	s.reload = reload
	s.nginxConfPath = nginxConfPath
	s.getActiveWS = getActiveWS
}

func (s *Service) SetPanelInfo(port int, apiPrefix func() string) {
	if port > 0 {
		s.panelPort = port
	}
	s.apiPrefix = apiPrefix
}

func (s *Service) ConfPath() string {
	return filepath.Join(s.confDir, "edgeworkers.conf")
}

func (s *Service) GlobalServerConfPath() string {
	return filepath.Join(s.confDir, "edgeworkers-global.conf")
}

func (s *Service) SiteServerConfPath(siteID uint) string {
	return filepath.Join(s.confDir, "edgeworkers-site-"+itoa(siteID)+".conf")
}

func (s *Service) List() ([]models.EdgeWorker, error) {
	var list []models.EdgeWorker
	err := s.db.Preload("Bindings").Order("priority asc, id asc").Find(&list).Error
	return list, err
}

func (s *Service) ListEnabled() ([]models.EdgeWorker, error) {
	var list []models.EdgeWorker
	err := s.db.Where("enabled = ?", true).Order("priority asc, id asc").Find(&list).Error
	return list, err
}

func (s *Service) Get(id uint) (*models.EdgeWorker, error) {
	var w models.EdgeWorker
	if err := s.db.Preload("Bindings").First(&w, id).Error; err != nil {
		return nil, err
	}
	return &w, nil
}

func (s *Service) Create(w *models.EdgeWorker) error {
	if err := s.normalizeWorker(w); err != nil {
		return err
	}
	return s.db.Create(w).Error
}

func (s *Service) Update(id uint, patch *models.EdgeWorker) error {
	w, err := s.Get(id)
	if err != nil {
		return err
	}
	if patch.Name != "" {
		w.Name = patch.Name
	}
	w.Description = patch.Description
	if patch.RoutePattern != "" {
		w.RoutePattern = patch.RoutePattern
	}
	if patch.ScriptType != "" {
		w.ScriptType = patch.ScriptType
	}
	w.Script = patch.Script
	w.WebsiteID = patch.WebsiteID
	w.Domains = patch.Domains
	w.Enabled = patch.Enabled
	if patch.Priority > 0 {
		w.Priority = patch.Priority
	}
	if patch.Triggers != "" {
		w.Triggers = patch.Triggers
	}
	if err := s.normalizeWorker(w); err != nil {
		return err
	}
	return s.db.Save(w).Error
}

func (s *Service) Delete(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("worker_id = ?", id).Delete(&models.EdgeWorkerBinding{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.EdgeWorker{}, id).Error
	})
}

func (s *Service) Toggle(id uint, enabled bool) error {
	return s.db.Model(&models.EdgeWorker{}).Where("id = ?", id).Update("enabled", enabled).Error
}

func (s *Service) normalizeWorker(w *models.EdgeWorker) error {
	if w.ScriptType == "" {
		w.ScriptType = "lua"
	}
	if w.Triggers == "" {
		w.Triggers = "request"
	}
	if w.RoutePattern == "" {
		w.RoutePattern = "/"
	}
	if err := s.prepareDomains(w); err != nil {
		return err
	}
	return ValidateScript(w.ScriptType, w.Script)
}

func (s *Service) WorkersForSite(siteID uint) ([]models.EdgeWorker, error) {
	all, err := s.ListEnabled()
	if err != nil {
		return nil, err
	}
	var site models.Website
	if err := s.db.Preload("Aliases").First(&site, siteID).Error; err != nil {
		return nil, err
	}
	aliases := make([]string, len(site.Aliases))
	for i, a := range site.Aliases {
		aliases[i] = a.Domain
	}
	out := WorkersForSite(all, siteID, site.Domain, aliases)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func itoa(n uint) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
