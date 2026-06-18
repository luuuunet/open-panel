package database

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/platform"
	"github.com/luuuunet/owpanel/internal/services/appstore"
)

type PostgreSQLStatus struct {
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
	Running   bool   `json:"running"`
}

type PgExtensionInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Available   bool   `json:"available"`
	Installed   bool   `json:"installed"`
	Contrib     bool   `json:"contrib"`
	CanInstall  bool   `json:"can_install"`
}

type PgExtensionDetail struct {
	Extensions   []PgExtensionInfo `json:"extensions"`
	CanInstall   bool              `json:"can_install"`
	ServerVersion string           `json:"server_version"`
	Database     string            `json:"database,omitempty"`
}

func (s *Service) PostgreSQLStatus() PostgreSQLStatus {
	st := PostgreSQLStatus{}
	if _, err := findBinary("psql"); err != nil {
		return st
	}
	st.Installed = true
	st.Running = true
	if ver, err := s.pgServerVersion(); err == nil && ver != "" {
		st.Version = ver
	}
	return st
}

func (s *Service) ListPgExtensionCatalog(databaseName string) (*PgExtensionDetail, error) {
	if _, err := findBinary("psql"); err != nil {
		return nil, fmt.Errorf("PostgreSQL 客户端未安装")
	}
	ver, _ := s.pgServerVersion()
	available, err := s.pgAvailableExtensions()
	if err != nil {
		return nil, err
	}
	installed := map[string]bool{}
	if databaseName != "" {
		installed, err = s.pgInstalledExtensions(databaseName)
		if err != nil {
			return nil, err
		}
	}
	canInstall := runtime.GOOS == "linux"
	out := make([]PgExtensionInfo, 0, len(appstore.PostgreSQLExtensionCatalog))
	for _, item := range appstore.PostgreSQLExtensionCatalog {
		out = append(out, PgExtensionInfo{
			Name:        item.Name,
			Description: item.Description,
			Available:   available[item.Name],
			Installed:   installed[item.Name],
			Contrib:     item.Contrib,
			CanInstall:  canInstall && !available[item.Name],
		})
	}
	return &PgExtensionDetail{
		Extensions:    out,
		CanInstall:    canInstall,
		ServerVersion: ver,
		Database:      databaseName,
	}, nil
}

func (s *Service) InstallPgExtensionPackage(name string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("扩展包安装目前仅支持 Linux")
	}
	catalog := appstore.PostgreSQLExtensionCatalogMap()
	item, ok := catalog[name]
	if !ok {
		return fmt.Errorf("未知扩展: %s", name)
	}
	ver, err := s.pgMajorVersion()
	if err != nil {
		return err
	}
	pkgs := pgExtensionPackages(item, ver)
	if len(pkgs) == 0 {
		if item.Contrib {
			pkgs = []string{"postgresql-contrib"}
		} else {
			return fmt.Errorf("未配置扩展 %s 的安装包", name)
		}
	}
	return installLinuxPackages(pkgs)
}

func (s *Service) SetPgDatabaseExtension(inst *models.DatabaseInstance, name string, enabled bool) error {
	tpe := strings.ToLower(inst.Type)
	if tpe != "postgresql" && tpe != "postgres" {
		return fmt.Errorf("仅 PostgreSQL 数据库支持扩展")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("扩展名不能为空")
	}
	if _, ok := appstore.PostgreSQLExtensionCatalogMap()[name]; !ok {
		return fmt.Errorf("未知扩展: %s", name)
	}
	escDb := inst.Name
	if enabled {
		available, err := s.pgAvailableExtensions()
		if err != nil {
			return err
		}
		if !available[name] {
			return fmt.Errorf("扩展 %s 尚未在服务器安装，请先安装扩展包", name)
		}
		q := fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %s;", pgQuoteIdent(name))
		_, err = s.pgExecOnDatabase(inst, "-d", escDb, "-c", q)
		return err
	}
	q := fmt.Sprintf("DROP EXTENSION IF EXISTS %s;", pgQuoteIdent(name))
	_, err := s.pgExecOnDatabase(inst, "-d", escDb, "-c", q)
	return err
}

func (s *Service) pgServerVersion() (string, error) {
	out, err := s.pgSuperExec("-tAc", "SHOW server_version;")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (s *Service) pgMajorVersion() (string, error) {
	ver, err := s.pgServerVersion()
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.TrimSpace(ver), ".")
	if len(parts) == 0 || parts[0] == "" {
		return "", fmt.Errorf("无法解析 PostgreSQL 版本: %s", ver)
	}
	return parts[0], nil
}

func (s *Service) pgAvailableExtensions() (map[string]bool, error) {
	out, err := s.pgSuperExec("-tAc", "SELECT name FROM pg_available_extensions ORDER BY name;")
	if err != nil {
		return nil, fmt.Errorf("查询可用扩展失败: %s", strings.TrimSpace(string(out)))
	}
	m := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		n := strings.TrimSpace(line)
		if n != "" {
			m[n] = true
		}
	}
	return m, nil
}

func (s *Service) pgInstalledExtensions(dbName string) (map[string]bool, error) {
	out, err := s.pgSuperExec("-d", dbName, "-tAc", "SELECT extname FROM pg_extension;")
	if err != nil {
		return nil, fmt.Errorf("查询已安装扩展失败: %s", strings.TrimSpace(string(out)))
	}
	m := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		n := strings.TrimSpace(line)
		if n != "" && n != "plpgsql" {
			m[n] = true
		}
	}
	return m, nil
}

func pgQuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (s *Service) pgExecOnDatabase(inst *models.DatabaseInstance, args ...string) ([]byte, error) {
	bin, err := findBinary("psql")
	if err != nil {
		return nil, err
	}
	host := inst.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := inst.Port
	if port == 0 {
		port = 5432
	}
	user := inst.Username
	if user == "" {
		user = "postgres"
	}
	base := []string{"-h", host, "-p", fmt.Sprintf("%d", port), "-U", user, "-v", "ON_ERROR_STOP=1"}
	base = append(base, args...)
	if runtime.GOOS == "linux" && isLocalHost(host) && user == "postgres" && inst.Password == "" {
		cmd := exec.Command("sudo", append([]string{"-u", "postgres", bin}, base...)...)
		out, err := cmd.CombinedOutput()
		if err == nil && !strings.Contains(string(out), "ERROR") {
			return out, nil
		}
	}
	cmd := exec.Command(bin, base...)
	if inst.Password != "" {
		cmd.Env = append(os.Environ(), "PGPASSWORD="+inst.Password)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	if strings.Contains(string(out), "ERROR") {
		return out, fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return out, nil
}

func isLocalHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	return h == "" || h == "127.0.0.1" || h == "localhost" || h == "::1"
}

func pgExtensionPackages(item appstore.PgExtensionCatalogItem, majorVer string) []string {
	mgr := platform.PackageManager()
	switch mgr {
	case "apt":
		if item.AptPkgFmt != "" {
			return []string{fmt.Sprintf(item.AptPkgFmt, majorVer)}
		}
	case "dnf", "yum":
		if item.DnfPkgFmt != "" {
			return []string{fmt.Sprintf(item.DnfPkgFmt, majorVer)}
		}
	}
	return nil
}

func installLinuxPackages(pkgs []string) error {
	mgr := platform.PackageManager()
	switch mgr {
	case "apt":
		if err := runPgCommand("apt-get", "update", "-qq"); err != nil {
			return fmt.Errorf("apt update: %w", err)
		}
		args := append([]string{"install", "-y"}, pkgs...)
		if err := runPgCommand("apt-get", args...); err != nil {
			return fmt.Errorf("apt install: %w", err)
		}
		return nil
	case "dnf":
		args := append([]string{"install", "-y"}, pkgs...)
		return runPgCommand("dnf", args...)
	case "yum":
		args := append([]string{"install", "-y"}, pkgs...)
		return runPgCommand("yum", args...)
	default:
		return fmt.Errorf("unsupported package manager (need apt/dnf/yum)")
	}
}

func runPgCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}
