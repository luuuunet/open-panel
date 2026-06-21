package appstore

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/platform"
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
	platform.SanitizeBrokenAptRepos()
	osInfo := platform.Detect()
	if osInfo.OSFamily == "debian" || osInfo.OSFamily == "ubuntu" {
		logInstallLine("Debian/Ubuntu 优先安装 MariaDB（MySQL 兼容，官方仓库稳定可用）…")
		if err := runStackFallback("mariadb"); err == nil {
			return startMySQLService("mariadb")
		}
		logInstallLine(fmt.Sprintf("MariaDB stack 安装未成功，尝试 Oracle MySQL %s …", version))
	}

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
		if fbErr := installMariaDBFallback(version, err); fbErr == nil {
			return nil
		}
		return fmt.Errorf("配置 Oracle MySQL 官方源失败（Debian/Ubuntu 已移除 mysql-server 包）: %w。建议改用软件商店中的 MariaDB，或检查网络能否访问 dev.mysql.com", err)
	}

	presetMySQLCommunityDebconf()
	if err := runAptInstall(pkgs...); err != nil {
		if fbErr := installMariaDBFallback(version, err); fbErr == nil {
			return nil
		}
		return fmt.Errorf("安装 MySQL %s: %w", version, err)
	}
	return startMySQLServiceLinux()
}

// installMariaDBFallback installs MariaDB when Oracle MySQL is unavailable on Debian-family systems.
func installMariaDBFallback(version string, cause error) error {
	if platform.Detect().OSFamily != "debian" && platform.Detect().OSFamily != "ubuntu" {
		return cause
	}
	platform.SanitizeBrokenAptRepos()
	logInstallLine(fmt.Sprintf("MySQL %s 官方包安装失败 (%v)，尝试 MariaDB …", version, cause))
	if err := runStackFallback("mariadb"); err == nil {
		logInstallLine("已通过 stack 脚本安装 MariaDB（MySQL 兼容，Debian 推荐）")
		return startMySQLService("mariadb")
	}
	if err := platform.AptGetUpdate("-qq"); err != nil {
		return fmt.Errorf("%w; apt update after MariaDB stack: %v", cause, err)
	}
	if err := runAptInstall("mariadb-server"); err != nil {
		return cause
	}
	logInstallLine("已安装 MariaDB（MySQL 兼容，Debian 推荐）")
	return startMySQLService("mariadb")
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
	if mysqlAptRepoConfigured() {
		return runAptGet("update", "-qq")
	}
	if err := installMySQLAptPrereqs(); err != nil {
		return fmt.Errorf("安装依赖: %w", err)
	}
	debPath := filepath.Join(os.TempDir(), "mysql-apt-config.deb")
	if err := downloadMySQLAptConfig(debPath); err != nil {
		return err
	}
	presetMySQLAptConfigDebconf(version)
	if err := dpkgInstallNonInteractive(debPath); err != nil {
		return err
	}
	return runAptGet("update", "-qq")
}

func mysqlAptRepoConfigured() bool {
	if fileExists("/etc/apt/sources.list.d/mysql.list") || fileExists("/etc/apt/sources.list.d/mysql-community.list") {
		return true
	}
	dir := "/etc/apt/sources.list.d"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := strings.ToLower(e.Name())
		if strings.Contains(name, "mysql") {
			return true
		}
	}
	return false
}

func installMySQLAptPrereqs() error {
	if err := runAptGet("update", "-qq"); err != nil {
		return err
	}
	return runAptInstall(
		"ca-certificates", "curl", "wget", "gnupg", "lsb-release",
		"debconf", "debconf-utils", "apt-transport-https",
	)
}

func downloadMySQLAptConfig(dest string) error {
	urls := []string{
		"https://dev.mysql.com/get/mysql-apt-config",
		"https://repo.mysql.com/mysql-apt-config_0.8.29-1_all.deb",
	}
	var lastErr error
	for _, u := range urls {
		if _, err := exec.LookPath("curl"); err == nil {
			if err := runCommand("curl", "-fsSL", "-o", dest, u); err == nil {
				return nil
			} else {
				lastErr = err
			}
		}
		if _, err := exec.LookPath("wget"); err == nil {
			if err := runCommand("wget", "-O", dest, u); err == nil {
				return nil
			} else {
				lastErr = err
			}
		}
	}
	if lastErr != nil {
		return fmt.Errorf("下载 mysql-apt-config: %w", lastErr)
	}
	return fmt.Errorf("curl/wget 不可用，无法下载 MySQL 官方源配置包")
}

func presetMySQLAptConfigDebconf(version string) {
	selectServer := mysqlAptServerSelect(version)
	selections := []string{
		fmt.Sprintf("mysql-apt-config mysql-apt-config/select-server select %s", selectServer),
		"mysql-apt-config mysql-apt-config/select-product select Ok",
		"mysql-apt-config mysql-apt-config/select-tools select Enabled",
	}
	for _, line := range selections {
		debconfSet(line)
	}
}

func presetMySQLCommunityDebconf() {
	selections := []string{
		"mysql-community-server mysql-community-server/root-pass password owpanel",
		"mysql-community-server mysql-community-server/re-root-pass password owpanel",
	}
	for _, line := range selections {
		debconfSet(line)
	}
}

func debconfSet(selection string) {
	cmd := exec.Command("debconf-set-selections")
	cmd.Stdin = strings.NewReader(selection + "\n")
	_ = cmd.Run()
}

func dpkgInstallNonInteractive(debPath string) error {
	cmd := exec.Command("dpkg", "-i", debPath)
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	logKey := installLogKeyForGoroutine()
	logInstallLineKey(logKey, fmt.Sprintf("$ DEBIAN_FRONTEND=noninteractive dpkg -i %s", debPath))
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

func startMySQLServiceLinux() error {
	for _, svc := range []string{"mysql", "mysqld"} {
		if err := startMySQLService(svc); err == nil {
			return nil
		}
	}
	return fmt.Errorf("MySQL 已安装但无法启动服务（尝试了 mysql / mysqld）")
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
	return runAptInstall(pkgs...)
}

func startMySQLService(svc string) error {
	_ = runCommand("systemctl", "enable", svc)
	if err := runCommand("systemctl", "start", svc); err != nil {
		return fmt.Errorf("start service %s: %w", svc, err)
	}
	return nil
}
