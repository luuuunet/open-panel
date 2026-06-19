package appstore

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/luuuunet/owpanel/internal/platform"
	"github.com/luuuunet/owpanel/internal/services/settings"
)

type packageSpec struct {
	Apt         []string
	Dnf         []string
	Service     string
	ServiceRpm  string
	WinPackages []string // winget package ids
}

var packageSpecs = map[string]packageSpec{
	"nginx":         {Apt: []string{"nginx"}, Dnf: []string{"nginx"}, Service: "nginx", WinPackages: []string{"nginxinc.nginx"}},
	"openresty":     {Apt: []string{"openresty"}, Dnf: []string{"openresty"}, Service: "openresty"},
	"apache":        {Apt: []string{"apache2"}, Dnf: []string{"httpd"}, Service: "apache2", ServiceRpm: "httpd", WinPackages: []string{"ApacheLounge.httpd"}},
	"openlitespeed": {Apt: []string{"openlitespeed"}, Dnf: []string{"openlitespeed"}, Service: "lsws"},
	"mysql":         {Apt: []string{"mysql-server"}, Dnf: []string{"mysql-server"}, Service: "mysql", WinPackages: []string{"Oracle.MySQL"}},
	"mariadb":       {Apt: []string{"mariadb-server"}, Dnf: []string{"mariadb-server"}, Service: "mariadb"},
	"postgresql":    {Apt: []string{"postgresql", "postgresql-contrib"}, Dnf: []string{"postgresql-server", "postgresql"}, Service: "postgresql"},
	"redis":         {Apt: []string{"redis-server"}, Dnf: []string{"redis"}, Service: "redis-server", ServiceRpm: "redis"},
	"mongodb":       {Apt: []string{"mongodb"}, Dnf: []string{"mongodb-org"}, Service: "mongod"},
	"php83":         {Apt: []string{"php8.3-fpm", "php8.3-mysql", "php8.3-cli", "php8.3-common", "php8.3-xml", "php8.3-curl", "php8.3-mbstring"}, Dnf: []string{"php-fpm", "php-mysqlnd", "php-cli", "php-xml", "php-mbstring"}, Service: "php8.3-fpm", ServiceRpm: "php-fpm", WinPackages: []string{"PHP.PHP.8.3"}},
	"php82":         {Apt: []string{"php8.2-fpm", "php8.2-mysql", "php8.2-cli", "php8.2-common", "php8.2-xml", "php8.2-curl", "php8.2-mbstring"}, Service: "php8.2-fpm"},
	"php81":         {Apt: []string{"php8.1-fpm", "php8.1-mysql", "php8.1-cli", "php8.1-common", "php8.1-xml", "php8.1-curl", "php8.1-mbstring"}, Service: "php8.1-fpm"},
	"php80":         {Apt: []string{"php8.0-fpm", "php8.0-mysql", "php8.0-cli", "php8.0-common", "php8.0-xml", "php8.0-curl", "php8.0-mbstring"}, Service: "php8.0-fpm"},
	"php74":         {Apt: []string{"php7.4-fpm", "php7.4-mysql", "php7.4-cli", "php7.4-common", "php7.4-xml", "php7.4-curl", "php7.4-mbstring"}, Service: "php7.4-fpm"},
	"nodejs20":      {Apt: []string{"nodejs", "npm"}, Dnf: []string{"nodejs", "npm"}, WinPackages: []string{"OpenJS.NodeJS.LTS"}},
	"nodejs18":      {Apt: []string{"nodejs", "npm"}, Dnf: []string{"nodejs", "npm"}, WinPackages: []string{"OpenJS.NodeJS.18"}},
	"python":        {Apt: []string{"python3", "python3-pip", "python3-venv"}, Dnf: []string{"python3", "python3-pip"}, WinPackages: []string{"Python.Python.3.12"}},
	"java21":        {Apt: []string{"openjdk-21-jdk"}, Dnf: []string{"java-21-openjdk", "java-21-openjdk-devel"}, WinPackages: []string{"Microsoft.OpenJDK.21"}},
	"java17":        {Apt: []string{"openjdk-17-jdk"}, Dnf: []string{"java-17-openjdk", "java-17-openjdk-devel"}, WinPackages: []string{"EclipseAdoptium.Temurin.17.JDK"}},
	"java11":        {Apt: []string{"openjdk-11-jdk"}, Dnf: []string{"java-11-openjdk", "java-11-openjdk-devel"}, WinPackages: []string{"EclipseAdoptium.Temurin.11.JDK"}},
	"java8":         {Apt: []string{"openjdk-8-jdk"}, Dnf: []string{"java-1.8.0-openjdk", "java-1.8.0-openjdk-devel"}, WinPackages: []string{"EclipseAdoptium.Temurin.8.JDK"}},
	"pureftpd":      {Apt: []string{"pure-ftpd"}, Dnf: []string{"pure-ftpd"}, Service: "pure-ftpd"},
	"postfix":       {Apt: []string{"postfix"}, Dnf: []string{"postfix"}, Service: "postfix"},
	"dovecot":       {Apt: []string{"dovecot-core", "dovecot-imapd", "dovecot-pop3d"}, Dnf: []string{"dovecot"}, Service: "dovecot"},
	"memcached":     {Apt: []string{"memcached"}, Dnf: []string{"memcached"}, Service: "memcached"},
	"docker":        {Apt: []string{"docker.io"}, Dnf: []string{"docker"}, Service: "docker", WinPackages: []string{"Docker.DockerDesktop"}},
	"fail2ban":      {Apt: []string{"fail2ban"}, Dnf: []string{"fail2ban"}, Service: "fail2ban"},
	"supervisor":    {Apt: []string{"supervisor"}, Dnf: []string{"supervisor"}, Service: "supervisor"},
	"pm2":           {},
	"composer":      {Apt: []string{"composer"}, Dnf: []string{"composer"}, WinPackages: []string{"Composer.Composer"}},
	"certbot":       {Apt: []string{"certbot", "python3-certbot-nginx"}, Dnf: []string{"certbot", "python3-certbot-nginx"}, WinPackages: []string{"Certify.CertifySSLManager"}},
	"tomcat9":       {Apt: []string{"tomcat9"}, Dnf: []string{"tomcat"}, Service: "tomcat9", ServiceRpm: "tomcat"},
	"tomcat10":      {Apt: []string{"tomcat10"}, Dnf: []string{"tomcat"}, Service: "tomcat10", ServiceRpm: "tomcat"},
}

var criticalPackageKeys = map[string]bool{
	"nginx": true, "openresty": true, "apache": true, "openlitespeed": true,
	"mysql": true, "mariadb": true, "postgresql": true, "redis": true, "mongodb": true,
	"php83": true, "php82": true, "php81": true, "php80": true, "php74": true,
	"docker": true, "certbot": true, "pureftpd": true, "postfix": true, "dovecot": true,
	"fail2ban": true, "memcached": true, "supervisor": true,
}

func isCriticalPackage(key string) bool {
	if strings.HasPrefix(key, "php") && key != "phpmyadmin" {
		return true
	}
	if strings.HasPrefix(key, "mysql") {
		return true
	}
	return criticalPackageKeys[key]
}

func phpPackageSpec(ver string) packageSpec {
	pkg := "php" + ver
	return packageSpec{
		Apt: []string{
			pkg + "-fpm", pkg + "-mysql", pkg + "-cli", pkg + "-common",
			pkg + "-xml", pkg + "-curl", pkg + "-mbstring",
		},
		Dnf:         []string{"php-fpm", "php-mysqlnd", "php-cli", "php-xml", "php-mbstring"},
		Service:     pkg + "-fpm",
		ServiceRpm:  "php-fpm",
		WinPackages: []string{"PHP.PHP." + ver},
	}
}

func resolvePackageSpec(key string) (packageSpec, bool) {
	if spec, ok := packageSpecs[key]; ok {
		return spec, true
	}
	if strings.HasPrefix(key, "php") && key != "phpmyadmin" {
		verKey := strings.TrimPrefix(key, "php")
		if len(verKey) >= 2 {
			ver := verKey[:1] + "." + verKey[1:]
			return phpPackageSpec(ver), true
		}
	}
	return packageSpec{}, false
}

func runSystemInstall(key, version, installPath, dataDir string) error {
	if ok, err := tryMailStackInstall(key, version, installPath, dataDir); ok {
		return err
	}
	if ok, err := tryPhpMyAdminInstall(key, version, installPath, dataDir); ok {
		return err
	}
	if ok, err := tryAIInstall(key, version, installPath, dataDir); ok {
		return err
	}
	if ok, err := tryRuntimeInstall(key, version, installPath, dataDir); ok {
		return err
	}
	if ok, err := tryKafkaInstall(key, version, installPath, dataDir); ok {
		return err
	}
	if ok, err := tryK3sInstall(key, version, installPath, dataDir); ok {
		return err
	}
	if ok, err := tryCiliumInstall(key, version, installPath, dataDir); ok {
		return err
	}
	if ok, err := tryDockerInstall(key, version, installPath, dataDir); ok {
		return err
	}
	if ok, err := tryMySQLInstall(key, version, installPath, dataDir); ok {
		return err
	}
	if ok, err := tryPHPInstall(key, version, installPath, dataDir); ok {
		return err
	}

	spec, ok := resolvePackageSpec(key)
	if !ok {
		if isCriticalPackage(key) {
			return fmt.Errorf("未知的关键组件: %s", key)
		}
		return simulateInstall(key, version, installPath, dataDir)
	}

	switch runtime.GOOS {
	case "linux":
		if err := installLinuxPackagesWithFallback(key, spec); err != nil {
			if isCriticalPackage(key) {
				return fmt.Errorf("安装 %s 失败（Linux 需 apt/dnf/yum）: %w", key, err)
			}
			return err
		}
		if svc := serviceName(spec); svc != "" {
			_ = runCommand("systemctl", "enable", svc)
			if err := runCommand("systemctl", "start", svc); err != nil {
				return fmt.Errorf("start service %s: %w", svc, err)
			}
		}
		return nil
	case "windows":
		if err := installWindowsPackages(spec); err == nil {
			return nil
		}
		if isCriticalPackage(key) {
			return fmt.Errorf("Windows 安装 %s 失败，请确认 winget 可用或手动安装", key)
		}
		return simulateInstall(key, version, installPath, dataDir)
	default:
		if isCriticalPackage(key) {
			return fmt.Errorf("当前系统不支持安装 %s", key)
		}
		return simulateInstall(key, version, installPath, dataDir)
	}
}

func runSystemUninstall(key, dataDir string) error {
	if ok, err := tryMailStackUninstall(key, dataDir); ok {
		return err
	}
	if ok, err := tryPhpMyAdminUninstall(key, dataDir); ok {
		return err
	}
	if ok, err := tryAIUninstall(key, dataDir); ok {
		return err
	}
	if ok, err := tryDockerUninstall(key, dataDir); ok {
		return err
	}

	spec, ok := resolvePackageSpec(key)
	if !ok {
		return removeSimulatedInstall(key, dataDir)
	}

	switch runtime.GOOS {
	case "linux":
		if svc := serviceName(spec); svc != "" {
			_ = runCommand("systemctl", "stop", svc)
			_ = runCommand("systemctl", "disable", svc)
		}
		pkgs := linuxPackages(spec)
		if len(pkgs) == 0 {
			return nil
		}
		mgr := detectLinuxPkgMgr()
		switch mgr {
		case "apt":
			args := append([]string{"remove", "-y"}, pkgs...)
			return runCommand("apt-get", args...)
		case "dnf", "yum":
			args := append([]string{"remove", "-y"}, pkgs...)
			return runCommand(mgr, args...)
		}
		return removeSimulatedInstall(key, dataDir)
	case "windows":
		_ = removeSimulatedInstall(key, dataDir)
		return nil
	default:
		return removeSimulatedInstall(key, dataDir)
	}
}

func runServiceAction(key, action, dataDir string) error {
	if ok, err := tryOpenpanelServiceAction(key, action, dataDir); ok {
		return err
	}
	if ok, err := tryDockerServiceAction(key, action); ok {
		return err
	}
	if ok, err := tryAIServiceAction(key, action); ok {
		return err
	}
	if ok, err := tryK3sServiceAction(key, action); ok {
		return err
	}

	spec, ok := resolvePackageSpec(key)
	if !ok {
		return nil
	}
	svc := serviceName(spec)
	if svc == "" {
		return nil
	}

	switch runtime.GOOS {
	case "linux":
		switch action {
		case "start":
			return runCommand("systemctl", "start", svc)
		case "stop":
			return runCommand("systemctl", "stop", svc)
		case "restart":
			return runCommand("systemctl", "restart", svc)
		case "reload":
			return runCommand("systemctl", "reload", svc)
		}
	case "windows":
		switch action {
		case "start":
			return runCommand("sc", "start", svc)
		case "stop":
			return runCommand("sc", "stop", svc)
		case "restart":
			_ = runCommand("sc", "stop", svc)
			time.Sleep(time.Second)
			return runCommand("sc", "start", svc)
		}
	}
	return nil
}

func detectServiceStatus(key string) string {
	if ok, status := tryDockerStatus(key); ok {
		return status
	}
	if ok, status := tryAIStatus(key); ok {
		return status
	}
	if ok, status := tryK3sStatus(key); ok {
		return status
	}
	if strings.HasPrefix(key, "java") {
		return detectJavaStatus(key)
	}
	if key == "pm2" {
		return detectPM2()
	}
	if key == "certbot" {
		return detectCertbot()
	}

	spec, ok := resolvePackageSpec(key)
	if !ok {
		return "stopped"
	}
	svc := serviceName(spec)
	if svc == "" {
		return "stopped"
	}

	if runtime.GOOS == "linux" {
		out, err := exec.Command("systemctl", "is-active", svc).Output()
		if err == nil && strings.TrimSpace(string(out)) == "active" {
			return "running"
		}
		return "stopped"
	}
	if runtime.GOOS == "windows" {
		return detectWindowsServiceStatus(key, svc)
	}
	return "stopped"
}

func installLinuxPackages(spec packageSpec) error {
	pkgs := linuxPackages(spec)
	if len(pkgs) == 0 {
		return fmt.Errorf("no packages defined for this software")
	}
	mgr := detectLinuxPkgMgr()
	switch mgr {
	case "apt":
		if err := runAptGet("update", "-qq"); err != nil {
			return fmt.Errorf("apt update: %w", err)
		}
		if err := runAptInstall(pkgs...); err != nil {
			return fmt.Errorf("apt install: %w", err)
		}
		return nil
	case "dnf":
		args := append([]string{"install", "-y"}, pkgs...)
		return runCommand("dnf", args...)
	case "yum":
		args := append([]string{"install", "-y"}, pkgs...)
		return runCommand("yum", args...)
	default:
		return fmt.Errorf("unsupported linux package manager (need apt/dnf/yum)")
	}
}

func runAptGet(args ...string) error {
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

func runAptInstall(pkgs ...string) error {
	args := append([]string{
		"install", "-y",
		"-o", "Dpkg::Options::=--force-confdef",
		"-o", "Dpkg::Options::=--force-confold",
	}, pkgs...)
	return runAptGet(args...)
}

func installWindowsPackages(spec packageSpec) error {
	if len(spec.WinPackages) == 0 {
		return fmt.Errorf("no winget package configured")
	}
	if _, err := exec.LookPath("winget"); err != nil {
		return fmt.Errorf("winget not found")
	}
	for _, pkg := range spec.WinPackages {
		if err := runCommand("winget", "install", "-e", "--id", pkg, "--accept-package-agreements", "--accept-source-agreements"); err != nil {
			return err
		}
	}
	return nil
}

func linuxPackages(spec packageSpec) []string {
	mgr := detectLinuxPkgMgr()
	if mgr == "apt" {
		return spec.Apt
	}
	if len(spec.Dnf) > 0 {
		return spec.Dnf
	}
	return spec.Apt
}

func serviceName(spec packageSpec) string {
	mgr := detectLinuxPkgMgr()
	if mgr != "apt" && spec.ServiceRpm != "" {
		return spec.ServiceRpm
	}
	return spec.Service
}

func detectLinuxPkgMgr() string {
	return platform.PackageManager()
}

func simulateInstall(key, version, installPath, dataDir string) error {
	logInstallLine(fmt.Sprintf("开发/模拟模式：正在安装 %s …", key))
	base := filepath.Join(dataDir, "server", key)
	if resolved := settings.ResolvePanelPath(dataDir, installPath); resolved != "" {
		base = resolved
	}
	if err := os.MkdirAll(base, 0755); err != nil {
		return err
	}
	marker := filepath.Join(base, ".owpanel-installed")
	content := fmt.Sprintf("key=%s\nversion=%s\ninstalled_at=%s\nmode=simulated\n", key, version, time.Now().Format(time.RFC3339))
	if err := os.WriteFile(marker, []byte(content), 0644); err != nil {
		return err
	}
	logInstallLine(fmt.Sprintf("已写入安装标记: %s", marker))
	logInstallLine("注意：此为模拟安装，未安装真实软件包")
	return nil
}

func IsSimulatedInstall(key, dataDir string) bool {
	marker := filepath.Join(dataDir, "server", key, ".owpanel-installed")
	b, err := os.ReadFile(marker)
	if err != nil {
		return false
	}
	return strings.Contains(string(b), "mode=simulated")
}

func removeSimulatedInstall(key, dataDir string) error {
	base := filepath.Join(dataDir, "server", key)
	return os.RemoveAll(base)
}

func runCommand(name string, args ...string) error {
	logKey := installLogKeyForGoroutine()
	cmdLine := fmt.Sprintf("$ %s %s", name, strings.Join(args, " "))
	logInstallLineKey(logKey, cmdLine)

	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logInstallLineKey(logKey, "ERROR: "+err.Error())
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		logInstallLineKey(logKey, "ERROR: "+err.Error())
		return err
	}

	if err := cmd.Start(); err != nil {
		logInstallLineKey(logKey, "ERROR: "+err.Error())
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		streamCommandOutput(stdout, "", logKey)
	}()
	go func() {
		defer wg.Done()
		streamCommandOutput(stderr, "[stderr] ", logKey)
	}()
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		msg := err.Error()
		logInstallLineKey(logKey, "ERROR: "+msg)
		return fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), msg)
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func detectJavaStatus(key string) string {
	ver := strings.TrimPrefix(key, "java")
	if javaBinaryForKey(key) != "" {
		return "running"
	}
	if hostJavaProcessMatchesVersion(ver) {
		return "running"
	}
	return "stopped"
}

func detectJavaStatusForInstall(key, dataDir string) string {
	if fileExists(filepath.Join(dataDir, "server", key, ".owpanel-installed")) {
		return "running"
	}
	return detectJavaStatus(key)
}

func javaVersionMatch(output, ver string) bool {
	output = strings.ToLower(output)
	switch ver {
	case "8":
		return strings.Contains(output, "1.8") || strings.Contains(output, `"8"`)
	case "21":
		return strings.Contains(output, "21.")
	case "17":
		return strings.Contains(output, "17.")
	case "11":
		return strings.Contains(output, "11.")
	}
	return false
}
