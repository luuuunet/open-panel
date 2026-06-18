package nodejs

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"github.com/luuuunet/owpanel/internal/services/pm2"
	"gorm.io/gorm"
)

type Service struct {
	db      *gorm.DB
	dataDir string
	pm2     *pm2.Manager
}

func NewService(db *gorm.DB, dataDir string) *Service {
	return &Service{db: db, dataDir: dataDir, pm2: pm2.NewManager(dataDir)}
}

func (s *Service) List() ([]models.NodeProject, error) {
	var list []models.NodeProject
	return list, s.db.Order("id desc").Find(&list).Error
}

func (s *Service) Create(p *models.NodeProject) error {
	if p.NodeVer == "" {
		p.NodeVer = "20"
	}
	if p.Domain == "" {
		p.Domain = p.Name
	}
	if p.Status == "" {
		p.Status = "stopped"
	}
	return s.db.Create(p).Error
}

func (s *Service) Delete(id uint) error {
	var p models.NodeProject
	if err := s.db.First(&p, id).Error; err != nil {
		return err
	}
	_ = s.pm2.Stop(s.pm2Name(&p))
	return s.db.Delete(&p).Error
}

func (s *Service) UpdateRemark(id uint, remark string) (*models.NodeProject, error) {
	var p models.NodeProject
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
	var p models.NodeProject
	if err := s.db.First(&p, id).Error; err != nil {
		return err
	}
	status = strings.TrimSpace(status)
	if status == "running" {
		if err := s.start(&p); err != nil {
			return err
		}
	} else {
		_ = s.pm2.Stop(s.pm2Name(&p))
	}
	return s.db.Model(&p).Update("status", status).Error
}

func (s *Service) start(p *models.NodeProject) error {
	if !s.pm2.Installed() {
		return fmt.Errorf("PM2 未安装，请先在软件商店安装 PM2")
	}
	cwd := p.Path
	if cwd == "" {
		cwd = filepath.Join(s.dataDir, "wwwroot", p.Name)
	}
	if _, err := os.Stat(cwd); err != nil {
		return fmt.Errorf("项目路径不存在: %s", cwd)
	}
	startCwd := cwd
	script := pm2.DefaultScript(cwd)
	if isNextApp(cwd) && p.Port > 0 {
		if repoRoot := findMonorepoRoot(cwd); repoRoot != "" {
			if filter := appFilterFromPath(repoRoot, cwd); filter != "" {
				startCwd = repoRoot
				script = fmt.Sprintf("pnpm --filter %s exec next start -p %d", filter, p.Port)
			}
		}
		if startCwd == cwd {
			if bin := resolveNextBin(cwd); bin != "" {
				script = fmt.Sprintf("%s start -p %d", shellQuotePath(bin), p.Port)
			} else {
				script = fmt.Sprintf("pnpm exec next start -p %d", p.Port)
				if root := findMonorepoRoot(cwd); root != "" {
					startCwd = root
				}
			}
		}
	}
	env := nodeRuntimeEnv(s.dataDir, p.NodeVer, p.Domain)
	_, err := s.pm2.StartWithOptions(pm2.StartOptions{
		Name:   s.pm2Name(p),
		Cwd:    startCwd,
		Script: script,
		Port:   p.Port,
		Env:    env,
		Shell:  strings.Contains(script, " "),
	})
	return err
}

func findMonorepoRoot(cwd string) string {
	dir := cwd
	for i := 0; i < 6; i++ {
		if fileExists(filepath.Join(dir, "pnpm-workspace.yaml")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func appFilterFromPath(repoRoot, appCwd string) string {
	rel, err := filepath.Rel(repoRoot, appCwd)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) >= 2 && parts[0] == "apps" {
		return parts[1]
	}
	return filepath.Base(appCwd)
}

func resolveNextBin(cwd string) string {
	dir := cwd
	for i := 0; i < 6; i++ {
		bin := filepath.Join(dir, "node_modules", ".bin", "next")
		if fileExists(bin) {
			return bin
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func shellQuotePath(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isNextApp(cwd string) bool {
	pkg, err := os.ReadFile(filepath.Join(cwd, "package.json"))
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(pkg))
	return strings.Contains(lower, `"next"`)
}

func nodeRuntimeEnv(dataDir, nodeVer, domain string) map[string]string {
	major := 20
	if v := strings.TrimSpace(strings.TrimPrefix(nodeVer, "v")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			major = n
		}
	}
	env := map[string]string{}
	if dir := appstore.NodeBinDir(dataDir, major); dir != "" {
		path := dir
		if existing := os.Getenv("PATH"); existing != "" {
			path = dir + string(os.PathListSeparator) + existing
		}
		env["PATH"] = path
	}
	if domain != "" {
		base := "https://" + domain
		env["NODE_ENV"] = "production"
		env["DOCS_ORIGIN"] = "https://docs." + domain
		env["BLOG_ORIGIN"] = "https://blog." + domain
		env["NEXT_DOCS_ORIGIN"] = env["DOCS_ORIGIN"]
		env["NEXT_BLOG_ORIGIN"] = env["BLOG_ORIGIN"]
		env["NEXT_PUBLIC_SITE_URL"] = base
		env["SITE_URL"] = base
	}
	return env
}

func (s *Service) pm2Name(p *models.NodeProject) string {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = fmt.Sprintf("node-%d", p.ID)
	}
	return "op-" + name
}
