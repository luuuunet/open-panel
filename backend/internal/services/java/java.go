package java

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"gorm.io/gorm"
)

type Service struct {
	db      *gorm.DB
	dataDir string
}

func NewService(db *gorm.DB, dataDir string) *Service {
	return &Service{db: db, dataDir: dataDir}
}

func (s *Service) List() ([]models.JavaProject, error) {
	var list []models.JavaProject
	return list, s.db.Order("id desc").Find(&list).Error
}

func (s *Service) Create(p *models.JavaProject) error {
	if p.Name == "" && p.Domain != "" {
		p.Name = p.Domain
	}
	if p.Domain == "" {
		p.Domain = p.Name
	}
	if p.Port == 0 {
		p.Port = 8080
	}
	if p.TomcatKey == "" {
		p.TomcatKey = "tomcat9"
	}
	if p.JavaVer == "" {
		p.JavaVer = "17"
	}
	if p.Path == "" {
		p.Path = filepath.Join(s.dataDir, "wwwroot", p.Domain)
	}
	_ = os.MkdirAll(p.Path, 0755)
	if p.Status == "" {
		p.Status = "stopped"
	}
	if err := s.db.Create(p).Error; err != nil {
		return err
	}
	return s.writeProxyVhost(p)
}

func (s *Service) Delete(id uint) error {
	var p models.JavaProject
	if err := s.db.First(&p, id).Error; err != nil {
		return err
	}
	s.removeVhost(p.Domain)
	return s.db.Delete(&p).Error
}

func (s *Service) UpdateRemark(id uint, remark string) (*models.JavaProject, error) {
	var p models.JavaProject
	if err := s.db.First(&p, id).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&p).Update("remark", strings.TrimSpace(remark)).Error; err != nil {
		return nil, err
	}
	p.Remark = strings.TrimSpace(remark)
	return &p, nil
}

func (s *Service) Toggle(id uint, status string) error {
	var p models.JavaProject
	if err := s.db.First(&p, id).Error; err != nil {
		return err
	}
	status = strings.TrimSpace(status)
	if status != "running" && status != "stopped" {
		return fmt.Errorf("invalid status")
	}

	if status == "running" {
		if err := s.ensureTomcat(&p); err != nil {
			return err
		}
	} else {
		_ = s.stopTomcat(&p)
	}

	if err := s.db.Model(&p).Update("status", status).Error; err != nil {
		return err
	}
	p.Status = status
	return s.writeProxyVhost(&p)
}

func (s *Service) ensureTomcat(p *models.JavaProject) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	svc := appstore.TomcatServiceName(p.TomcatKey)
	if svc == "" {
		return nil
	}
	return exec.Command("systemctl", "start", svc).Run()
}

func (s *Service) stopTomcat(p *models.JavaProject) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	svc := appstore.TomcatServiceName(p.TomcatKey)
	if svc == "" {
		return nil
	}
	return exec.Command("systemctl", "stop", svc).Run()
}

func (s *Service) writeProxyVhost(p *models.JavaProject) error {
	confDir := filepath.Join(s.dataDir, "nginx", "vhosts")
	_ = os.MkdirAll(confDir, 0755)
	confPath := filepath.Join(confDir, p.Domain+".conf")

	upstream := fmt.Sprintf("http://127.0.0.1:%d", p.Port)
	if cp := strings.TrimSpace(p.ContextPath); cp != "" && cp != "/" {
		if !strings.HasPrefix(cp, "/") {
			cp = "/" + cp
		}
		upstream = fmt.Sprintf("http://127.0.0.1:%d%s", p.Port, cp)
	}

	var body string
	if p.Status == "stopped" {
		body = fmt.Sprintf(`server {
    listen 80;
    server_name %s;
    return 503;
}`, p.Domain)
	} else {
		body = fmt.Sprintf(`server {
    listen 80;
    server_name %s;

    location / {
        proxy_pass %s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}`, p.Domain, upstream)
	}
	content := fmt.Sprintf("# OWPanel Java — %s\n%s\n", p.Domain, body)
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		return err
	}
	reloadNginx()
	return nil
}

func reloadNginx() {
	if runtime.GOOS != "linux" {
		return
	}
	_ = exec.Command("nginx", "-t").Run()
	_ = exec.Command("nginx", "-s", "reload").Run()
	_ = exec.Command("systemctl", "reload", "nginx").Run()
	_ = exec.Command("systemctl", "reload", "openresty").Run()
}

func (s *Service) removeVhost(domain string) {
	_ = os.Remove(filepath.Join(s.dataDir, "nginx", "vhosts", domain+".conf"))
}
