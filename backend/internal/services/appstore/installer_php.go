package appstore

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/php"
)

func tryPHPInstall(key, version, installPath, dataDir string) (bool, error) {
	if !strings.HasPrefix(key, "php") || key == "phpmyadmin" {
		return false, nil
	}
	spec, ok := resolvePackageSpec(key)
	if !ok {
		return false, nil
	}
	ver := php.VersionFromKey(key)
	if ver == "" {
		ver = version
	}
	_ = installPath

	switch runtime.GOOS {
	case "linux":
		if err := installPHPLinux(spec); err != nil {
			return true, err
		}
		if err := configurePHPLinux(ver, key, dataDir); err != nil {
			logInstallLine(fmt.Sprintf("PHP %s 配置提示: %v", ver, err))
		}
		if svc := serviceName(spec); svc != "" {
			_ = runCommand("systemctl", "enable", svc)
			if err := runCommand("systemctl", "start", svc); err != nil {
				return true, fmt.Errorf("start service %s: %w", svc, err)
			}
		}
		return true, nil
	case "windows":
		if err := installWindowsPackages(spec); err != nil {
			return true, fmt.Errorf("Windows 安装 %s 失败，请确认 winget 可用或手动安装: %w", key, err)
		}
		return true, nil
	default:
		return false, nil
	}
}

func configurePHPLinux(version, key, dataDir string) error {
	if version == "" {
		return nil
	}
	port := php.PortForVersion(version)
	socketPath := fmt.Sprintf("/run/php/php%s-fpm-openpanel.sock", strings.ReplaceAll(version, ".", ""))
	poolDir := filepath.Join("/etc/php", version, "fpm", "pool.d")
	poolFile := filepath.Join(poolDir, "owpanel.conf")

	if _, err := os.Stat(poolDir); err != nil {
		return fmt.Errorf("pool dir missing: %w", err)
	}

	poolName := "openpanel_" + strings.ReplaceAll(version, ".", "_")
	content := fmt.Sprintf(`; OWPanel managed pool for PHP %s
[%s]
user = www-data
group = www-data
listen = 127.0.0.1:%d
listen.allowed_clients = 127.0.0.1
pm = dynamic
pm.max_children = 50
pm.start_servers = 5
pm.min_spare_servers = 5
pm.max_spare_servers = 35
`, version, poolName, port)

	if err := os.WriteFile(poolFile, []byte(content), 0644); err != nil {
		logInstallLine(fmt.Sprintf("无法写入 %s，尝试 Unix socket …", poolFile))
		content = fmt.Sprintf(`; OWPanel managed pool for PHP %s
[%s]
user = www-data
group = www-data
listen = %s
listen.owner = www-data
listen.group = www-data
listen.mode = 0660
pm = dynamic
pm.max_children = 50
pm.start_servers = 5
pm.min_spare_servers = 5
pm.max_spare_servers = 35
`, version, poolName, socketPath)
		if err2 := os.WriteFile(poolFile, []byte(content), 0644); err2 != nil {
			return err2
		}
	}

	wwwConf := filepath.Join(poolDir, "www.conf")
	if data, err := os.ReadFile(wwwConf); err == nil {
		text := string(data)
		listenLine := fmt.Sprintf("listen = 127.0.0.1:%d", port+100)
		if !strings.Contains(text, listenLine) && strings.Contains(text, "listen = ") {
			text = strings.Replace(text, "listen = /run/php/php"+version+"-fpm.sock",
				listenLine, 1)
			text = strings.Replace(text, "listen = /var/run/php/php-fpm.sock",
				listenLine, 1)
			_ = os.WriteFile(wwwConf, []byte(text), 0644)
		}
	}

	runtimeDir := filepath.Join(dataDir, "php", key)
	_ = os.MkdirAll(runtimeDir, 0755)
	marker := filepath.Join(runtimeDir, ".owpanel-installed")
	_ = os.WriteFile(marker, []byte(fmt.Sprintf("version=%s\nport=%d\n", version, port)), 0644)

	svc := "php" + version + "-fpm"
	_ = runCommand("systemctl", "restart", svc)
	logInstallLine(fmt.Sprintf("PHP %s configured: pool %s, port %d", version, poolName, port))
	return nil
}

func installPHPLinux(spec packageSpec) error {
	mgr := detectLinuxPkgMgr()
	if err := installLinuxPackages(spec); err == nil {
		return nil
	}

	switch mgr {
	case "apt":
		logInstallLine("标准源 PHP 包不可用，尝试配置 ondrej/php PPA …")
		if err := setupPHPDebianRepo(); err != nil {
		 return fmt.Errorf("配置 PHP PPA 失败: %w", err)
		}
	case "dnf", "yum":
		logInstallLine("标准源 PHP 包不可用，尝试配置 Remi 仓库 …")
		if err := setupPHPRemiRepo(); err != nil {
			return fmt.Errorf("配置 Remi 仓库失败: %w", err)
		}
	default:
		return fmt.Errorf("安装 PHP 失败（Linux 需 apt/dnf/yum）")
	}
	return installLinuxPackages(spec)
}

func setupPHPDebianRepo() error {
	if _, err := exec.LookPath("add-apt-repository"); err != nil {
		if err := runCommand("apt-get", "update", "-qq"); err != nil {
			return err
		}
		if err := runCommand("apt-get", "install", "-y", "software-properties-common", "ca-certificates", "lsb-release", "apt-transport-https"); err != nil {
			return err
		}
	}
	if err := runCommand("add-apt-repository", "-y", "ppa:ondrej/php"); err != nil {
		return err
	}
	return runCommand("apt-get", "update", "-qq")
}

func setupPHPRemiRepo() error {
	mgr := detectLinuxPkgMgr()
	if _, err := exec.LookPath("curl"); err != nil {
		if _, err2 := exec.LookPath("wget"); err2 != nil {
			return fmt.Errorf("需要 curl 或 wget")
		}
	}
	if _, err := exec.LookPath("rpm"); err != nil {
		return fmt.Errorf("rpm 不可用")
	}
	_ = runCommand(mgr, "install", "-y", "epel-release")
	repoURL := "https://rpms.remirepo.net/enterprise/remi-release-9.rpm"
	if err := runCommand("rpm", "-Uvh", repoURL); err != nil {
		repoURL = "https://rpms.remirepo.net/enterprise/remi-release-8.rpm"
		if err2 := runCommand("rpm", "-Uvh", repoURL); err2 != nil {
			return err
		}
	}
	return runCommand(mgr, "makecache", "-y")
}
