package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

type Service struct{ db *gorm.DB }

type ServiceInfo struct {
	Name      string `json:"name"`
	Container string `json:"container"`
	Image     string `json:"image"`
	State     string `json:"state"`
	Status    string `json:"status"`
	Ports     string `json:"ports"`
}

type Detail struct {
	models.ComposeApp
	ComposeFile  string        `json:"compose_file"`
	LiveStatus   string        `json:"live_status"`
	Services     []ServiceInfo `json:"services"`
	ServiceCount int           `json:"service_count"`
}

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

func (s *Service) List() ([]Detail, error) {
	var list []models.ComposeApp
	if err := s.db.Order("id desc").Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]Detail, 0, len(list))
	for i := range list {
		out = append(out, s.enrich(&list[i]))
	}
	return out, nil
}

func (s *Service) Get(id uint) (*Detail, error) {
	var app models.ComposeApp
	if err := s.db.First(&app, id).Error; err != nil {
		return nil, err
	}
	d := s.enrich(&app)
	return &d, nil
}

func (s *Service) enrich(app *models.ComposeApp) Detail {
	d := Detail{ComposeApp: *app, Services: []ServiceInfo{}}
	if app.Path != "" {
		d.ComposeFile = composeFile(app.Path)
		d.LiveStatus = detectComposeStatus(app.Path)
		d.Services = listComposeServices(app.Path)
		d.ServiceCount = len(d.Services)
	}
	return d
}

func (s *Service) Create(app *models.ComposeApp, scaffold bool, templateID string, autoStart bool) error {
	app.Path = strings.TrimSpace(app.Path)
	if app.Path == "" {
		return fmt.Errorf("路径不能为空")
	}
	if !filepath.IsAbs(app.Path) {
		app.Path = filepath.Clean(app.Path)
	}
	if _, err := ensureComposeDir(app.Path, scaffold, templateID); err != nil {
		return err
	}
	if !dockerAvailable() || !autoStart {
		app.Status = "stopped"
		return s.db.Create(app).Error
	}
	if _, err := runCompose(app.Path, "", "up", "-d"); err != nil {
		return err
	}
	app.Status = "running"
	return s.db.Create(app).Error
}

func (s *Service) Restart(id uint) error {
	var app models.ComposeApp
	if err := s.db.First(&app, id).Error; err != nil {
		return err
	}
	if !dockerAvailable() {
		return fmt.Errorf("Docker 不可用")
	}
	_, err := runCompose(app.Path, "", "restart")
	if err != nil {
		return err
	}
	return s.db.Model(&app).Update("status", "running").Error
}

func (s *Service) Delete(id uint) error {
	var app models.ComposeApp
	if err := s.db.First(&app, id).Error; err != nil {
		return err
	}
	if dockerAvailable() && app.Path != "" {
		_, _ = runCompose(app.Path, "", "down")
	}
	return s.db.Delete(&models.ComposeApp{}, id).Error
}

func (s *Service) Toggle(id uint, status string) error {
	var app models.ComposeApp
	if err := s.db.First(&app, id).Error; err != nil {
		return err
	}
	if !dockerAvailable() {
		return s.db.Model(&app).Update("status", status).Error
	}
	var err error
	if status == "running" {
		_, err = runCompose(app.Path, "", "up", "-d")
	} else {
		_, err = runCompose(app.Path, "", "down")
	}
	if err != nil {
		return err
	}
	return s.db.Model(&app).Update("status", status).Error
}

func (s *Service) Logs(id uint, tail int) (string, error) {
	var app models.ComposeApp
	if err := s.db.First(&app, id).Error; err != nil {
		return "", err
	}
	if tail <= 0 {
		tail = 100
	}
	return runCompose(app.Path, "", "logs", "--tail", fmt.Sprintf("%d", tail))
}

func (s *Service) Pull(id uint) error {
	var app models.ComposeApp
	if err := s.db.First(&app, id).Error; err != nil {
		return err
	}
	_, err := runCompose(app.Path, "", "pull")
	return err
}

func (s *Service) SyncStatus(id uint) (*Detail, error) {
	var app models.ComposeApp
	if err := s.db.First(&app, id).Error; err != nil {
		return nil, err
	}
	st := detectComposeStatus(app.Path)
	_ = s.db.Model(&app).Update("status", st).Error
	app.Status = st
	d := s.enrich(&app)
	return &d, nil
}

func (s *Service) ReadComposeFile(id uint) (string, error) {
	var app models.ComposeApp
	if err := s.db.First(&app, id).Error; err != nil {
		return "", err
	}
	cf := composeFile(app.Path)
	data, err := os.ReadFile(cf)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Service) Templates() []Template {
	return ListTemplates()
}

func (s *Service) WriteComposeFile(id uint, content string) error {
	var app models.ComposeApp
	if err := s.db.First(&app, id).Error; err != nil {
		return err
	}
	cf := composeFile(app.Path)
	return os.WriteFile(cf, []byte(content), 0644)
}

type RollingResult struct {
	Strategy string `json:"strategy"`
	Log      string `json:"log"`
	Status   string `json:"status"`
}

func (s *Service) RollingUpdate(id uint) (*RollingResult, error) {
	var app models.ComposeApp
	if err := s.db.First(&app, id).Error; err != nil {
		return nil, err
	}
	if !dockerAvailable() {
		return nil, fmt.Errorf("Docker 不可用")
	}
	if _, err := runCompose(app.Path, "", "pull"); err != nil {
		return &RollingResult{Strategy: "rolling", Status: "partial", Log: "pull: " + err.Error()}, nil
	}
	out, err := runCompose(app.Path, "", "up", "-d", "--remove-orphans")
	if err != nil {
		return nil, fmt.Errorf("rolling update: %w", err)
	}
	_ = s.db.Model(&app).Update("status", "running").Error
	return &RollingResult{Strategy: "rolling", Status: "success", Log: out}, nil
}

func (s *Service) BlueGreenUpdate(id uint) (*RollingResult, error) {
	var app models.ComposeApp
	if err := s.db.First(&app, id).Error; err != nil {
		return nil, err
	}
	if !dockerAvailable() {
		return nil, fmt.Errorf("Docker 不可用")
	}
	project := filepath.Base(app.Path)
	greenProject := project + "_green"
	out1, _ := runCompose(app.Path, "-p", greenProject, "up", "-d", "--scale", "app=1")
	if _, err := runCompose(app.Path, "", "pull"); err != nil {
		return &RollingResult{Strategy: "blue-green", Status: "partial", Log: err.Error()}, nil
	}
	out2, err := runCompose(app.Path, "", "up", "-d")
	if err != nil {
		return nil, err
	}
	_, _ = runCompose(app.Path, "-p", greenProject, "down")
	log := strings.TrimSpace(out1 + "\n" + out2)
	return &RollingResult{Strategy: "blue-green", Status: "success", Log: log}, nil
}
