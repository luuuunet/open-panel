package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/pm2"
	"gorm.io/gorm"
)

var versionOptions = map[string][]string{
	"php": {"8.4", "8.3", "8.2", "8.1", "8.0", "7.4", "7.3", "7.2", "7.1", "7.0", "5.6", "5.5", "5.4", "5.3"},
	"java":   {"21", "17", "11", "8"},
	"nodejs": {"22", "20", "18"},
	"go":     {"1.23", "1.22", "1.21"},
	"rust":   {"1.84", "1.83", "1.82"},
	"python": {"3.12", "3.11", "3.10"},
	"dotnet": {"10.0", "9.0", "8.0"},
}

type Service struct {
	db      *gorm.DB
	dataDir string
	pm2     *pm2.Manager
}

func NewService(db *gorm.DB, dataDir string) *Service {
	return &Service{db: db, dataDir: dataDir, pm2: pm2.NewManager(dataDir)}
}

func (s *Service) Versions(kind string) []string {
	if v, ok := versionOptions[kind]; ok {
		return v
	}
	return []string{}
}

func (s *Service) List(kind string) ([]models.RuntimeProject, error) {
	kind = strings.TrimSpace(strings.ToLower(kind))
	var list []models.RuntimeProject
	q := s.db.Order("id desc")
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	if kind == "" || kind == "nodejs" {
		list = s.mergeLegacyNode(list, kind == "nodejs")
	}
	if kind == "" || kind == "java" {
		list = s.mergeLegacyJava(list, kind == "java")
	}
	return list, nil
}

func (s *Service) mergeLegacyNode(list []models.RuntimeProject, _ bool) []models.RuntimeProject {
	var nodes []models.NodeProject
	if err := s.db.Order("id desc").Find(&nodes).Error; err != nil {
		return list
	}
	for _, n := range nodes {
		skip := false
		for _, r := range list {
			if r.LegacySource == "node" && r.LegacyID == n.ID {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		ports, _ := json.Marshal([]models.RuntimePort{{
			HostPort: n.Port, ContainerPort: n.Port, Protocol: "tcp",
		}})
		list = append(list, models.RuntimeProject{
			ID: n.ID, Kind: "nodejs", Name: n.Name, Path: n.Path,
			Version: n.NodeVer, ExternalPort: n.Port, Status: n.Status,
			Remark: n.Remark, Ports: string(ports),
			LegacySource: "node", LegacyID: n.ID,
			CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt,
		})
	}
	return list
}

func (s *Service) mergeLegacyJava(list []models.RuntimeProject, _ bool) []models.RuntimeProject {
	var items []models.JavaProject
	if err := s.db.Order("id desc").Find(&items).Error; err != nil {
		return list
	}
	for _, j := range items {
		skip := false
		for _, r := range list {
			if r.LegacySource == "java" && r.LegacyID == j.ID {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		ports, _ := json.Marshal([]models.RuntimePort{{
			HostPort: j.Port, ContainerPort: j.Port, Protocol: "tcp",
		}})
		name := j.Name
		if name == "" {
			name = j.Domain
		}
		list = append(list, models.RuntimeProject{
			ID: j.ID, Kind: "java", Name: name, Path: j.Path,
			Version: j.JavaVer, ExternalPort: j.Port, Status: j.Status,
			Remark: j.Remark, Ports: string(ports),
			RunScript: fmt.Sprintf("java -jar app.jar --server.port=%d", j.Port),
			LegacySource: "java", LegacyID: j.ID,
			CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt,
		})
	}
	return list
}

func (s *Service) Create(p *models.RuntimeProject) error {
	p.Kind = strings.TrimSpace(strings.ToLower(p.Kind))
	if p.Kind == "" {
		return fmt.Errorf("运行环境类型不能为空")
	}
	if p.Name == "" {
		return fmt.Errorf("名称不能为空")
	}
	if p.Path == "" {
		p.Path = filepath.Join(s.dataDir, "wwwroot", p.Name)
	}
	if p.Version == "" {
		if vers := s.Versions(p.Kind); len(vers) > 0 {
			p.Version = vers[0]
		}
	}
	if p.ContainerName == "" {
		p.ContainerName = "op-" + sanitizeName(p.Name)
	}
	if p.Status == "" {
		p.Status = "stopped"
	}
	s.normalizePorts(p)
	if err := s.db.Create(p).Error; err != nil {
		return err
	}
	return nil
}

func (s *Service) Update(id uint, patch *models.RuntimeProject) error {
	var p models.RuntimeProject
	if err := s.db.First(&p, id).Error; err != nil {
		return err
	}
	if p.LegacySource != "" {
		return fmt.Errorf("旧版项目请在网站或 Node.js 页面管理")
	}
	if patch.Name != "" {
		p.Name = patch.Name
	}
	if patch.Path != "" {
		p.Path = patch.Path
	}
	if patch.Version != "" {
		p.Version = patch.Version
	}
	if patch.RunScript != "" {
		p.RunScript = patch.RunScript
	}
	if patch.ContainerName != "" {
		p.ContainerName = patch.ContainerName
	}
	p.Remark = patch.Remark
	if patch.Ports != "" {
		p.Ports = patch.Ports
	}
	if patch.EnvVars != "" {
		p.EnvVars = patch.EnvVars
	}
	if patch.Mounts != "" {
		p.Mounts = patch.Mounts
	}
	if patch.HostMappings != "" {
		p.HostMappings = patch.HostMappings
	}
	s.normalizePorts(&p)
	return s.db.Save(&p).Error
}

func (s *Service) Delete(id uint, legacySource string, legacyID uint) error {
	if legacySource == "node" && legacyID > 0 {
		var p models.NodeProject
		if err := s.db.First(&p, legacyID).Error; err != nil {
			return err
		}
		_ = s.pm2.Stop("op-" + p.Name)
		return s.db.Delete(&p).Error
	}
	if legacySource == "java" && legacyID > 0 {
		var p models.JavaProject
		if err := s.db.First(&p, legacyID).Error; err != nil {
			return err
		}
		return s.db.Delete(&p).Error
	}
	var p models.RuntimeProject
	if err := s.db.First(&p, id).Error; err != nil {
		return err
	}
	_ = s.stop(&p)
	return s.db.Delete(&p).Error
}

func (s *Service) Toggle(id uint, status string, legacySource string, legacyID uint) error {
	status = strings.TrimSpace(status)
	if status != "running" && status != "stopped" {
		return fmt.Errorf("invalid status")
	}
	if legacySource == "node" && legacyID > 0 {
		var p models.NodeProject
		if err := s.db.First(&p, legacyID).Error; err != nil {
			return err
		}
		if status == "running" {
			cwd := p.Path
			if cwd == "" {
				cwd = filepath.Join(s.dataDir, "wwwroot", p.Name)
			}
			script := pm2.DefaultScript(cwd)
			_, err := s.pm2.Start("op-"+p.Name, cwd, script, p.Port)
			if err != nil {
				return err
			}
		} else {
			_ = s.pm2.Stop("op-" + p.Name)
		}
		return s.db.Model(&p).Update("status", status).Error
	}
	if legacySource == "java" && legacyID > 0 {
		return fmt.Errorf("Java 项目请在网站页面启停")
	}
	var p models.RuntimeProject
	if err := s.db.First(&p, id).Error; err != nil {
		return err
	}
	if status == "running" {
		if err := s.start(&p); err != nil {
			return err
		}
	} else {
		_ = s.stop(&p)
	}
	return s.db.Model(&p).Updates(map[string]interface{}{
		"status":       status,
		"container_id": p.ContainerID,
	}).Error
}

func (s *Service) normalizePorts(p *models.RuntimeProject) {
	ports := parsePorts(p.Ports)
	if len(ports) == 0 && p.ExternalPort > 0 {
		ports = []models.RuntimePort{{HostPort: p.ExternalPort, ContainerPort: p.ExternalPort, Protocol: "tcp"}}
	}
	if len(ports) > 0 && p.ExternalPort == 0 {
		p.ExternalPort = ports[0].HostPort
	}
	if b, err := json.Marshal(ports); err == nil {
		p.Ports = string(b)
	}
}

func (s *Service) start(p *models.RuntimeProject) error {
	if _, err := os.Stat(p.Path); err != nil {
		return fmt.Errorf("代码目录不存在: %s", p.Path)
	}
	if s.useDocker(p) {
		return s.startDocker(p)
	}
	return s.startPM2(p)
}

func (s *Service) useDocker(p *models.RuntimeProject) bool {
	if !s.dockerAvailable() {
		return false
	}
	switch p.Kind {
	case "dotnet", "python", "go", "rust":
		return true
	case "java":
		return strings.TrimSpace(p.RunScript) != ""
	default:
		return false
	}
}

func (s *Service) startPM2(p *models.RuntimeProject) error {
	if !s.pm2.Installed() {
		return fmt.Errorf("PM2 未安装，请先在软件商店安装 PM2")
	}
	script := strings.TrimSpace(p.RunScript)
	shell := script != "" && (strings.Contains(script, " ") || p.Kind == "dotnet" || p.Kind == "python" || p.Kind == "go" || p.Kind == "rust")
	env := parseEnv(p.EnvVars)
	if p.Kind == "dotnet" {
		for k, v := range s.dotnetRuntimeEnv() {
			if env == nil {
				env = map[string]string{}
			}
			if _, ok := env[k]; !ok {
				env[k] = v
			}
		}
	}
	if p.ExternalPort > 0 {
		if env == nil {
			env = map[string]string{}
		}
		if _, ok := env["PORT"]; !ok {
			env["PORT"] = fmt.Sprintf("%d", p.ExternalPort)
		}
	}
	if script == "" && p.Kind == "nodejs" {
		script = pm2.DefaultScript(p.Path)
	}
	if script == "" {
		return fmt.Errorf("请填写启动命令")
	}
	name := s.pm2Name(p)
	_, err := s.pm2.StartWithOptions(pm2.StartOptions{
		Name: name, Cwd: p.Path, Script: script,
		Port: p.ExternalPort, Env: env, Shell: shell,
	})
	return err
}

func (s *Service) startDocker(p *models.RuntimeProject) error {
	_ = s.stopDocker(p.ContainerName)
	image := s.dockerImage(p.Kind, p.Version)
	args := []string{"run", "-d", "--name", p.ContainerName, "--restart", "unless-stopped"}
	for _, pt := range parsePorts(p.Ports) {
		proto := pt.Protocol
		if proto == "" {
			proto = "tcp"
		}
		if pt.HostPort > 0 && pt.ContainerPort > 0 {
			args = append(args, "-p", fmt.Sprintf("%d:%d/%s", pt.HostPort, pt.ContainerPort, proto))
		}
	}
	for _, e := range parseEnvList(p.EnvVars) {
		args = append(args, "-e", fmt.Sprintf("%s=%s", e.Key, e.Value))
	}
	codeMount := false
	for _, m := range parseMounts(p.Mounts) {
		if m.Host == "" || m.Container == "" {
			continue
		}
		opt := "rw"
		if m.ReadOnly {
			opt = "ro"
		}
		args = append(args, "-v", fmt.Sprintf("%s:%s:%s", m.Host, m.Container, opt))
		if m.Host == p.Path {
			codeMount = true
		}
	}
	if !codeMount && p.Path != "" {
		args = append(args, "-v", fmt.Sprintf("%s:%s", p.Path, p.Path), "-w", p.Path)
	}
	for _, hm := range parseHostMappings(p.HostMappings) {
		if hm.Host != "" && hm.IP != "" {
			args = append(args, "--add-host", fmt.Sprintf("%s:%s", hm.Host, hm.IP))
		}
	}
	args = append(args, image)
	script := strings.TrimSpace(p.RunScript)
	if script == "" {
		switch p.Kind {
		case "dotnet":
			script = "dotnet MyWebApp.dll"
		case "python":
			script = "python app.py"
		case "go":
			script = "./app"
		case "rust":
			script = "cargo build --release && ./target/release/app"
		default:
			script = "sh"
		}
	}
	args = append(args, "sh", "-c", script)
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Docker 启动失败: %s", strings.TrimSpace(string(out)))
	}
	p.ContainerID = strings.TrimSpace(string(out))
	if len(p.ContainerID) > 12 {
		p.ContainerID = p.ContainerID[:12]
	}
	return nil
}

func (s *Service) stop(p *models.RuntimeProject) error {
	if p.ContainerID != "" || s.containerExists(p.ContainerName) {
		return s.stopDocker(p.ContainerName)
	}
	_ = s.pm2.Stop(s.pm2Name(p))
	return nil
}

func (s *Service) stopDocker(name string) error {
	if name == "" {
		return nil
	}
	_ = exec.Command("docker", "stop", name).Run()
	_ = exec.Command("docker", "rm", "-f", name).Run()
	return nil
}

func (s *Service) containerExists(name string) bool {
	if name == "" || !s.dockerAvailable() {
		return false
	}
	out, err := exec.Command("docker", "ps", "-a", "--filter", "name=^/"+name+"$", "--format", "{{.Names}}").Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func (s *Service) dockerAvailable() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

func (s *Service) dockerImage(kind, version string) string {
	version = strings.TrimSpace(version)
	switch kind {
	case "dotnet":
		if version == "" {
			version = "8.0"
		}
		return "mcr.microsoft.com/dotnet/aspnet:" + version
	case "python":
		if version == "" {
			version = "3.12"
		}
		return "python:" + version
	case "go":
		if version == "" {
			version = "1.23"
		}
		return "golang:" + version
	case "rust":
		if version == "" {
			version = "1.84"
		}
		return "rust:" + version
	case "java":
		if version == "" {
			version = "17"
		}
		return "eclipse-temurin:" + version + "-jdk"
	case "nodejs":
		if version == "" {
			version = "20"
		}
		return "node:" + version
	default:
		return "alpine:latest"
	}
}

func (s *Service) dotnetRuntimeEnv() map[string]string {
	for _, major := range []string{"10", "8", "9"} {
		root := filepath.Join(s.dataDir, "server", "dotnet", major, "root")
		if st, err := os.Stat(filepath.Join(root, "dotnet")); err == nil && !st.IsDir() {
			path := root
			if cur := os.Getenv("PATH"); cur != "" {
				path = root + string(os.PathListSeparator) + cur
			}
			return map[string]string{"DOTNET_ROOT": root, "PATH": path}
		}
	}
	return nil
}

func (s *Service) pm2Name(p *models.RuntimeProject) string {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = fmt.Sprintf("rt-%d", p.ID)
	}
	return "op-rt-" + sanitizeName(name)
}

func sanitizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else if r == ' ' || r == '.' {
			b.WriteRune('-')
		}
	}
	out := b.String()
	if out == "" {
		return "runtime"
	}
	return out
}

func parsePorts(raw string) []models.RuntimePort {
	var list []models.RuntimePort
	_ = json.Unmarshal([]byte(raw), &list)
	return list
}

func parseEnvList(raw string) []models.RuntimeEnvVar {
	var list []models.RuntimeEnvVar
	_ = json.Unmarshal([]byte(raw), &list)
	return list
}

func parseEnv(raw string) map[string]string {
	list := parseEnvList(raw)
	if len(list) == 0 {
		return nil
	}
	m := make(map[string]string, len(list))
	for _, e := range list {
		if e.Key != "" {
			m[e.Key] = e.Value
		}
	}
	return m
}

func parseMounts(raw string) []models.RuntimeMount {
	var list []models.RuntimeMount
	_ = json.Unmarshal([]byte(raw), &list)
	return list
}

func parseHostMappings(raw string) []models.RuntimeHostMapping {
	var list []models.RuntimeHostMapping
	_ = json.Unmarshal([]byte(raw), &list)
	return list
}
