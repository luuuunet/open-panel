package appstore

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/settings"
)

func tryMySQLInstall(key, version, installPath, dataDir string) (bool, error) {
	catalogVersion, ok := MySQLVersionFromKey(key)
	if !ok {
		return false, nil
	}
	if version == "" {
		version = catalogVersion
	}
	switch runtime.GOOS {
	case "linux":
		return true, installMySQLLinux(version, installPath, dataDir)
	case "windows":
		return true, installMySQLWindows(version)
	default:
		return false, nil
	}
}

func installMySQLWindows(version string) error {
	spec := packageSpecs["mysql"]
	_ = version // winget Oracle.MySQL does not pin minor series reliably
	if err := installWindowsPackages(spec); err != nil {
		return fmt.Errorf("Windows 安装 MySQL %s 失败（winget 版本选择有限）: %w", version, err)
	}
	return nil
}

func installMySQLLinux(version, installPath, dataDir string) error {
	if err := ensureMySQLDataDir(version, installPath, dataDir); err != nil {
		return err
	}
	mgr := detectLinuxPkgMgr()
	switch mgr {
	case "apt":
		return installMySQLApt(version, installPath, dataDir)
	case "dnf", "yum":
		return installMySQLRpm(version)
	default:
		return fmt.Errorf("unsupported linux package manager for MySQL (need apt/dnf/yum)")
	}
}

func ensureMySQLDataDir(version, installPath, dataDir string) error {
	base := filepath.Join(dataDir, "server", "mysql")
	if resolved := settings.ResolvePanelPath(dataDir, installPath); resolved != "" {
		base = resolved
	}
	verDir := filepath.Join(base, "data")
	if fileExists(verDir) {
		logInstallLine(fmt.Sprintf("MySQL %s data directory exists: %s", version, verDir))
		return nil
	}
	logInstallLine(fmt.Sprintf("MySQL %s will use data directory: %s", version, verDir))
	return os.MkdirAll(verDir, 0750)
}

func installMySQLApt(version, installPath, dataDir string) error {
	pkgs := mysqlAptPackages(version)
	if len(pkgs) == 0 {
		return fmt.Errorf("不支持的 MySQL 版本: %s", version)
	}

	if version == "5.6" || version == "5.5" {
		logInstallLine(fmt.Sprintf("MySQL %s 在较新 Ubuntu/Debian 上通常不可用，尝试归档源或本地目录安装 …", version))
		if err := installMySQLLegacyTarball(version, installPath, dataDir); err == nil {
			return nil
		}
		logInstallLine("归档/本地安装未成功，继续尝试 Oracle APT 仓库 …")
	}

	if err := setupMySQLAptRepo(version); err != nil {
		if version == "8.0" {
			logInstallLine("Oracle MySQL APT 仓库配置失败，回退到发行版 mysql-server …")
			pkgs = []string{"mysql-server", "mysql-client"}
		} else {
			return fmt.Errorf("配置 MySQL APT 仓库: %w", err)
		}
	}

	if err := aptInstallNonInteractive(pkgs...); err != nil {
		return fmt.Errorf("安装 MySQL %s: %w", version, err)
	}
	return startMySQLService("mysql")
}

func installMySQLRpm(version string) error {
	pkgs := mysqlRpmPackages(version)
	if len(pkgs) == 0 {
		return fmt.Errorf("不支持的 MySQL 版本: %s", version)
	}
	mgr := detectLinuxPkgMgr()
	if err := setupMySQLRpmRepo(version); err != nil {
		logInstallLine("MySQL YUM/DNF 仓库配置失败，尝试发行版默认包 …")
		pkgs = []string{"mysql-server"}
	}
	args := append([]string{"install", "-y"}, pkgs...)
	if err := runCommand(mgr, args...); err != nil {
		return fmt.Errorf("安装 MySQL %s: %w", version, err)
	}
	return startMySQLService("mysqld")
}

func mysqlAptPackages(version string) []string {
	switch version {
	case "8.4", "8.0":
		return []string{"mysql-community-server", "mysql-community-client"}
	case "5.7":
		return []string{"mysql-server-5.7", "mysql-client-5.7"}
	case "5.6":
		return []string{"mysql-server-5.6", "mysql-client-5.6"}
	case "5.5":
		return []string{"mysql-server-5.5", "mysql-client-5.5"}
	default:
		return nil
	}
}

func mysqlRpmPackages(version string) []string {
	switch version {
	case "8.4", "8.0":
		return []string{"mysql-community-server", "mysql-community-client"}
	case "5.7":
		return []string{"mysql-community-server", "mysql-community-client"}
	default:
		return nil
	}
}

func mysqlAptServerSelect(version string) string {
	switch version {
	case "8.4":
		return "mysql-8.4"
	case "8.0":
		return "mysql-8.0"
	case "5.7":
		return "mysql-5.7"
	case "5.6":
		return "mysql-5.6"
	case "5.5":
		return "mysql-5.5"
	default:
		return "mysql-8.0"
	}
}

func setupMySQLAptRepo(version string) error {
	if fileExists("/etc/apt/sources.list.d/mysql.list") || fileExists("/etc/apt/sources.list.d/mysql-community.list") {
		return runCommand("apt-get", "update", "-qq")
	}
	if _, err := exec.LookPath("wget"); err != nil {
		return fmt.Errorf("wget 不可用")
	}
	debPath := filepath.Join(os.TempDir(), "mysql-apt-config.deb")
	if err := runCommand("wget", "-O", debPath, "https://dev.mysql.com/get/mysql-apt-config_0.8.29-1_all.deb"); err != nil {
		return err
	}
	selectServer := mysqlAptServerSelect(version)
	_ = runCommand("debconf-set-selections", fmt.Sprintf("mysql-apt-config mysql-apt-config/select-server select %s", selectServer))
	_ = runCommand("debconf-set-selections", "mysql-apt-config mysql-apt-config/select-product select Ok")
	_ = runCommand("debconf-set-selections", "mysql-apt-config mysql-apt-config/select-tools select Enabled")
	if err := runCommand("dpkg", "-i", debPath); err != nil {
		return err
	}
	return runCommand("apt-get", "update", "-qq")
}

func setupMySQLRpmRepo(version string) error {
	if _, err := exec.LookPath("rpm"); err != nil {
		return fmt.Errorf("rpm 不可用")
	}
	repoURL := "https://dev.mysql.com/get/mysql80-community-release-el9-1.noarch.rpm"
	if version == "5.7" {
		repoURL = "https://dev.mysql.com/get/mysql57-community-release-el7-11.noarch.rpm"
	}
	rpmPath := filepath.Join(os.TempDir(), "mysql-community-release.rpm")
	if err := runCommand("curl", "-fsSL", "-o", rpmPath, repoURL); err != nil {
		if err2 := runCommand("wget", "-O", rpmPath, repoURL); err2 != nil {
			return err
		}
	}
	mgr := detectLinuxPkgMgr()
	if err := runCommand("rpm", "-Uvh", rpmPath); err != nil {
		return err
	}
	return runCommand(mgr, "makecache", "-y")
}

func installMySQLLegacyTarball(version, installPath, dataDir string) error {
	base := filepath.Join(dataDir, "server", "mysql")
	if resolved := settings.ResolvePanelPath(dataDir, installPath); resolved != "" {
		base = resolved
	}
	verDir := filepath.Join(base, strings.ReplaceAll(version, ".", ""))
	if fileExists(filepath.Join(verDir, "bin", "mysqld")) || fileExists(filepath.Join(verDir, "bin", "mysqld.exe")) {
		logInstallLine(fmt.Sprintf("检测到已有 MySQL %s 安装: %s", version, verDir))
		return nil
	}
	return fmt.Errorf("MySQL %s 在当前系统上无可靠包源，请手动安装到 %s 或使用 Docker", version, verDir)
}

func aptInstallNonInteractive(pkgs ...string) error {
	args := append([]string{"install", "-y"}, pkgs...)
	cmd := exec.Command("apt-get", args...)
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	logKey := installLogKeyForGoroutine()
	logInstallLineKey(logKey, fmt.Sprintf("$ DEBIAN_FRONTEND=noninteractive apt-get %s", strings.Join(args, " ")))
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if text != "" {
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				logInstallLineKey(logKey, line)
			}
		}
	}
	if err != nil {
		if text != "" {
			return fmt.Errorf("%v: %s", err, text)
		}
		return err
	}
	return nil
}

func startMySQLService(svc string) error {
	_ = runCommand("systemctl", "enable", svc)
	if err := runCommand("systemctl", "start", svc); err != nil {
		return fmt.Errorf("start service %s: %w", svc, err)
	}
	return nil
}
