package php

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var versionPorts = map[string]int{
	"8.3": 9000, "8.2": 9001, "8.1": 9002, "7.4": 9003,
	"8.4": 9004, "8.0": 9005, "7.3": 9006, "7.2": 9007,
	"7.1": 9008, "7.0": 9009, "5.6": 9010, "5.5": 9011,
	"5.4": 9012, "5.3": 9013,
}

var keyVersions = map[string]string{
	"php83": "8.3", "php82": "8.2", "php81": "8.1", "php74": "7.4",
	"php84": "8.4", "php80": "8.0", "php73": "7.3", "php72": "7.2",
	"php71": "7.1", "php70": "7.0", "php56": "5.6", "php55": "5.5",
	"php54": "5.4", "php53": "5.3",
}

type Status struct {
	Running bool   `json:"running"`
	Version string `json:"version"`
	Port    int    `json:"port"`
	Binary  string `json:"binary"`
	PID     int    `json:"pid"`
	Mode    string `json:"mode"`
	Message string `json:"message,omitempty"`
}

type Manager struct {
	dataDir string
}

func NewManager(dataDir string) *Manager {
	return &Manager{dataDir: dataDir}
}

func VersionFromKey(key string) string {
	if v, ok := keyVersions[key]; ok {
		return v
	}
	if strings.HasPrefix(key, "php") && len(key) > 3 {
		verKey := key[3:]
		if len(verKey) >= 2 {
			return verKey[:1] + "." + verKey[1:]
		}
		return verKey
	}
	return ""
}

func PortForVersion(ver string) int {
	if p, ok := versionPorts[ver]; ok {
		return p
	}
	return 9000
}

// FastCGIBackend returns nginx fastcgi_pass target (unix socket on Linux when available).
func FastCGIBackend(version string) string {
	if runtime.GOOS == "linux" {
		sock := fmt.Sprintf("/run/php/php%s-fpm.sock", version)
		if _, err := os.Stat(sock); err == nil {
			return "unix:" + sock
		}
	}
	return fmt.Sprintf("127.0.0.1:%d", PortForVersion(version))
}

func (m *Manager) Status(key string) Status {
	ver := VersionFromKey(key)
	port := PortForVersion(ver)
	st := Status{Version: ver, Port: port}
	bin, err := m.findBinary(ver)
	if err != nil {
		st.Message = err.Error()
		return st
	}
	st.Binary = bin
	st.Mode = "php-cgi"

	if pid := m.readPID(key); pid > 0 && processAlive(pid) {
		st.Running = true
		st.PID = pid
		return st
	}
	if m.portOpen(port) {
		st.Running = true
		return st
	}
	if runtime.GOOS == "linux" {
		svc := linuxService(ver)
		if out, err := exec.Command("systemctl", "is-active", svc).Output(); err == nil {
			if strings.TrimSpace(string(out)) == "active" {
				st.Running = true
				st.Mode = "systemd:" + svc
				return st
			}
		}
	}
	return st
}

func (m *Manager) Start(key string) error {
	st := m.Status(key)
	if st.Running {
		return nil
	}
	ver := VersionFromKey(key)
	port := PortForVersion(ver)

	if runtime.GOOS == "linux" {
		svc := linuxService(ver)
		if svc != "" {
			if err := exec.Command("systemctl", "start", svc).Run(); err == nil {
				return nil
			}
		}
	}

	bin, err := m.findBinary(ver)
	if err != nil {
		return err
	}
	cgi := phpCgiPath(bin)
	if cgi == "" {
		return fmt.Errorf("未找到 php-cgi，请先在软件商店安装 PHP %s", ver)
	}
	_ = m.ensureRuntimeDir(key, ver)

	logPath := filepath.Join(m.runtimeDir(key), "php-cgi.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	cmd := exec.Command(cgi, "-b", fmt.Sprintf("127.0.0.1:%d", port))
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = detachedProcAttr()
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("启动 php-cgi 失败: %w", err)
	}
	_ = logFile.Close()
	_ = m.writePID(key, cmd.Process.Pid)

	for i := 0; i < 20; i++ {
		if m.portOpen(port) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("php-cgi 已启动但端口 %d 未监听，请查看 %s", port, logPath)
}

func (m *Manager) Stop(key string) error {
	if pid := m.readPID(key); pid > 0 {
		_ = killProcess(pid)
		_ = m.removePID(key)
	}
	if runtime.GOOS == "linux" {
		svc := linuxService(VersionFromKey(key))
		if svc != "" {
			_ = exec.Command("systemctl", "stop", svc).Run()
		}
	}
	return nil
}

func (m *Manager) Restart(key string) error {
	_ = m.Stop(key)
	time.Sleep(500 * time.Millisecond)
	return m.Start(key)
}

func (m *Manager) findBinary(version string) (string, error) {
	if runtime.GOOS == "linux" {
		for _, name := range []string{"php" + version, "php" + strings.ReplaceAll(version, ".", "")} {
			if path, err := exec.LookPath(name); err == nil {
				return path, nil
			}
		}
	}
	patterns := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "PHP", "v"+version, "php.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Microsoft", "WinGet", "Packages",
			fmt.Sprintf("PHP.PHP.%s_Microsoft.Winget.Source_8wekyb3d8bbwe", version), "php.exe"),
	}
	for _, p := range patterns {
		if fileExists(p) {
			return p, nil
		}
	}
	base := filepath.Join(os.Getenv("LocalAppData"), "Microsoft", "WinGet", "Packages")
	if entries, err := os.ReadDir(base); err == nil {
		prefix := "PHP.PHP." + version
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), prefix) {
				p := filepath.Join(base, e.Name(), "php.exe")
				if fileExists(p) {
					return p, nil
				}
			}
		}
	}
	sim := filepath.Join(m.dataDir, "server", "php"+strings.ReplaceAll(version, ".", ""), "php.exe")
	if fileExists(sim) {
		return sim, nil
	}
	if path, err := exec.LookPath("php"); err == nil {
		if out, err := exec.Command(path, "-r", "echo PHP_VERSION;").Output(); err == nil {
			if strings.HasPrefix(strings.TrimSpace(string(out)), version) {
				return path, nil
			}
		}
	}
	return "", fmt.Errorf("未找到 PHP %s，请先在软件商店安装", version)
}

func phpCgiPath(phpBin string) string {
	dir := filepath.Dir(phpBin)
	if runtime.GOOS == "windows" {
		cgi := filepath.Join(dir, "php-cgi.exe")
		if fileExists(cgi) {
			return cgi
		}
	}
	cgi := filepath.Join(dir, "php-cgi")
	if fileExists(cgi) {
		return cgi
	}
	if path, err := exec.LookPath("php-cgi"); err == nil {
		return path
	}
	return ""
}

func linuxService(version string) string {
	if version != "" {
		return "php" + version + "-fpm"
	}
	return "php-fpm"
}

func (m *Manager) runtimeDir(key string) string {
	return filepath.Join(m.dataDir, "php", key)
}

func (m *Manager) systemIniPaths(version string) []string {
	if runtime.GOOS != "linux" || version == "" {
		return nil
	}
	return []string{
		filepath.Join("/etc/php", version, "fpm", "php.ini"),
		filepath.Join("/etc/php", version, "cli", "php.ini"),
		filepath.Join("/etc/php", version, "apache2", "php.ini"),
	}
}

func (m *Manager) accelConfPath(version string) string {
	return filepath.Join("/etc/php", version, "fpm", "conf.d", "99-owpanel-accel.ini")
}

func (m *Manager) ensureRuntimeDir(key, version string) error {
	dir := m.runtimeDir(key)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	ini := filepath.Join(dir, "php.ini")
	if fileExists(ini) {
		return m.ensurePHPErrorLogINI(ini, dir)
	}
	for _, src := range m.systemIniPaths(version) {
		if fileExists(src) {
			data, err := os.ReadFile(src)
			if err != nil {
				continue
			}
			if err := os.WriteFile(ini, data, 0644); err != nil {
				return err
			}
			return m.ensurePHPErrorLogINI(ini, dir)
		}
	}
	bin, err := m.findBinary(version)
	if err != nil {
		return m.writeMinimalIni(ini, dir)
	}
	src := filepath.Join(filepath.Dir(bin), "php.ini-development")
	if !fileExists(src) {
		src = filepath.Join(filepath.Dir(bin), "php.ini-production")
	}
	if fileExists(src) {
		data, _ := os.ReadFile(src)
		errLog := filepath.ToSlash(filepath.Join(dir, "php_errors.log"))
		extra := "\nextension_dir = \"" + filepath.ToSlash(filepath.Join(filepath.Dir(bin), "ext")) + "\"\n"
		extra += "log_errors = On\nerror_log = \"" + errLog + "\"\n"
		_ = os.WriteFile(ini, append(data, []byte(extra)...), 0644)
		return nil
	}
	return m.writeMinimalIni(ini, dir)
}

func (m *Manager) writeMinimalIni(ini, dir string) error {
	if fileExists(ini) {
		return nil
	}
	errLog := filepath.ToSlash(filepath.Join(dir, "php_errors.log"))
	content := "; OWPanel managed PHP config\nlog_errors = On\nerror_log = \"" + errLog + "\"\n"
	return os.WriteFile(ini, []byte(content), 0644)
}

func (m *Manager) ensurePHPErrorLogINI(iniPath, dir string) error {
	data, err := os.ReadFile(iniPath)
	if err != nil {
		return err
	}
	content := string(data)
	if strings.Contains(strings.ToLower(content), "error_log") {
		return nil
	}
	errLog := filepath.ToSlash(filepath.Join(dir, "php_errors.log"))
	patch := "\nlog_errors = On\nerror_log = \"" + errLog + "\"\n"
	return os.WriteFile(iniPath, append(data, []byte(patch)...), 0644)
}

func (m *Manager) pidFile(key string) string {
	return filepath.Join(m.runtimeDir(key), "php-cgi.pid")
}

func (m *Manager) writePID(key string, pid int) error {
	return os.WriteFile(m.pidFile(key), []byte(strconv.Itoa(pid)), 0644)
}

func (m *Manager) readPID(key string) int {
	data, err := os.ReadFile(m.pidFile(key))
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid
}

func (m *Manager) removePID(key string) error {
	return os.Remove(m.pidFile(key))
}

func (m *Manager) portOpen(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func processAlive(pid int) bool {
	if runtime.GOOS == "windows" {
		out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH").Output()
		if err != nil {
			return false
		}
		return strings.Contains(string(out), strconv.Itoa(pid))
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func killProcess(pid int) error {
	if runtime.GOOS == "windows" {
		return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/F").Run()
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
