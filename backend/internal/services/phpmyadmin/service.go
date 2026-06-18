package phpmyadmin

import (
	"archive/tar"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/php"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"gorm.io/gorm"
)

const appKey = "phpmyadmin"

type AccessInfo struct {
	Installed   bool   `json:"installed"`
	Running     bool   `json:"running"`
	URL         string `json:"url"`
	Port        int    `json:"port"`
	Path        string `json:"path"`
	InstallPath string `json:"install_path"`
	Hint        string `json:"hint"`
	SetupError  string `json:"setup_error,omitempty"`
}

type WebServerHooks struct {
	GetActive func() string
	Reload    func(key string) error
	EnsureInc func(key string)
	IsRunning func(key string) bool
}

type Service struct {
	dataDir string
	db      *gorm.DB
	ws      WebServerHooks
}

func New(dataDir string, db *gorm.DB) *Service {
	return &Service{dataDir: dataDir, db: db}
}

func (s *Service) SetWebServerHooks(h WebServerHooks) {
	s.ws = h
}

func (s *Service) Install(installPath, version string, port int) error {
	root := findInstallRoot(installPath, s.dataDir)
	if root == "" {
		root = preferredInstallPath(installPath, s.dataDir)
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		// dataDir install path is always used when the catalog path is unavailable
		root = filepath.Join(s.dataDir, "server", appKey)
		if err := os.MkdirAll(root, 0755); err != nil {
			return err
		}
	}
	if !hasPhpMyAdminFiles(root) {
		if err := downloadAndExtract(root, version); err != nil {
			return err
		}
	}
	if err := writeConfig(root); err != nil {
		return err
	}
	marker := filepath.Join(root, ".owpanel-installed")
	_ = os.WriteFile(marker, []byte("phpmyadmin\n"), 0644)
	return s.EnsureVhost(root, port)
}

func (s *Service) Uninstall(installPath string) error {
	root := findInstallRoot(installPath, s.dataDir)
	if root == "" {
		root = preferredInstallPath(installPath, s.dataDir)
	}
	_ = os.Remove(vhostFile(s.dataDir))
	s.reloadActive()
	return os.RemoveAll(root)
}

func findInstallRoot(installPath, dataDir string) string {
	candidates := []string{
		filepath.Join(dataDir, "server", appKey),
	}
	if installPath != "" {
		candidates = append(candidates, installPath)
	}
	candidates = append(candidates,
		"/usr/share/phpmyadmin",
		"/usr/share/phpMyAdmin",
	)
	seen := map[string]bool{}
	for _, c := range candidates {
		c = filepath.Clean(c)
		if c == "" || c == "." || seen[c] {
			continue
		}
		seen[c] = true
		if hasPhpMyAdminFiles(c) || fileExists(filepath.Join(c, ".owpanel-installed")) {
			return c
		}
	}
	return ""
}

func preferredInstallPath(installPath, dataDir string) string {
	if resolved := settings.ResolvePanelPath(dataDir, installPath); resolved != "" {
		return resolved
	}
	return filepath.Join(dataDir, "server", appKey)
}

func (s *Service) EnsureVhost(root string, port int) error {
	if port <= 0 {
		port = 888
	}
	fcgiBackend := detectPhpFpmBackend()
	conf := fmt.Sprintf(`# OWPanel — phpMyAdmin
server {
    listen %d;
    server_name _;
    root %s;
    index index.php index.html;

    location / {
        try_files $uri $uri/ /index.php?$query_string;
    }

    location ~ \.php$ {
        fastcgi_pass %s;
        fastcgi_index index.php;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
    }

    location ~ /\.ht {
        deny all;
    }
}
`, port, filepath.ToSlash(root), fcgiBackend)
	if err := os.MkdirAll(filepath.Dir(vhostFile(s.dataDir)), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(vhostFile(s.dataDir), []byte(conf), 0644); err != nil {
		return err
	}
	if err := s.reloadActive(); err != nil {
		// Vhost file is written; reload may fail when nginx/openresty is not installed yet.
		return nil
	}
	return nil
}

func (s *Service) AccessInfo(installPath string, port int, dbInstalled bool) (*AccessInfo, error) {
	if port <= 0 {
		port = 888
	}
	root := findInstallRoot(installPath, s.dataDir)
	if root == "" {
		root = preferredInstallPath(installPath, s.dataDir)
	}
	installed := dbInstalled ||
		hasPhpMyAdminFiles(root) ||
		fileExists(filepath.Join(root, ".owpanel-installed"))
	info := &AccessInfo{
		Installed:   installed,
		Port:        port,
		Path:        "/",
		InstallPath: root,
	}
	if !installed {
		info.Hint = "请先在软件商店安装 phpMyAdmin"
		return info, nil
	}
	if !hasPhpMyAdminFiles(root) {
		if err := s.Install(installPath, "5.2", port); err != nil {
			info.SetupError = err.Error()
		}
		root = findInstallRoot(installPath, s.dataDir)
		if root == "" {
			root = preferredInstallPath(installPath, s.dataDir)
		}
		info.InstallPath = root
	}
	if !fileExists(vhostFile(s.dataDir)) {
		if err := s.EnsureVhost(root, port); err != nil && info.SetupError == "" {
			info.SetupError = err.Error()
		}
	}
	host := serverHost()
	info.URL = fmt.Sprintf("http://%s:%d/", host, port)
	info.Running = s.isRunning(port)
	if info.SetupError != "" {
		info.Hint = "部署未完成：" + info.SetupError + "。可点击「修复访问配置」重试，并确认 Nginx/OpenResty、PHP-FPM 已启动。"
	} else if !info.Running {
		info.Hint = "请确保 Nginx/OpenResty 与 PHP-FPM 已启动；若仍无法访问，可点击「修复访问配置」"
	} else {
		info.Hint = "使用 MySQL root 或站点数据库账号登录（Server: 127.0.0.1；root 密码见面板「数据库」页）"
	}
	return info, nil
}

func (s *Service) Status(port int) string {
	if port <= 0 {
		port = 888
	}
	root := findInstallRoot("", s.dataDir)
	if root == "" {
		root = filepath.Join(s.dataDir, "server", appKey)
	}
	if !fileExists(filepath.Join(root, ".owpanel-installed")) && !hasPhpMyAdminFiles(root) {
		if s.db != nil && AppInstalled(s.db) {
			return "stopped"
		}
		return "stopped"
	}
	if s.isRunning(port) {
		return "running"
	}
	if fileExists(vhostFile(s.dataDir)) {
		return "stopped"
	}
	return "stopped"
}

func (s *Service) Start(installPath string, port int) error {
	root := findInstallRoot(installPath, s.dataDir)
	if root == "" {
		root = preferredInstallPath(installPath, s.dataDir)
	}
	if !hasPhpMyAdminFiles(root) {
		if err := s.Install(installPath, "5.2", port); err != nil {
			return err
		}
		root = findInstallRoot(installPath, s.dataDir)
		if root == "" {
			return fmt.Errorf("phpMyAdmin 文件不存在，请重新安装")
		}
	}
	return s.EnsureVhost(root, port)
}

func (s *Service) Stop() error {
	_ = os.Remove(vhostFile(s.dataDir))
	return s.reloadActive()
}

func (s *Service) isRunning(port int) bool {
	if !fileExists(vhostFile(s.dataDir)) {
		return false
	}
	active := s.activeWebServer()
	if s.ws.IsRunning != nil && !s.ws.IsRunning(active) {
		return false
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err == nil {
		_ = ln.Close()
		return false
	}
	return strings.Contains(err.Error(), "bind") || strings.Contains(err.Error(), "address already in use")
}

func (s *Service) reloadActive() error {
	if s.ws.Reload == nil {
		return nil
	}
	active := s.activeWebServer()
	if s.ws.EnsureInc != nil {
		s.ws.EnsureInc(active)
	}
	return s.ws.Reload(active)
}

func (s *Service) activeWebServer() string {
	if s.ws.GetActive != nil {
		if v := s.ws.GetActive(); v != "" {
			return v
		}
	}
	var row models.PanelSetting
	if s.db != nil && s.db.Where("key = ?", "active_web_server").First(&row).Error == nil && row.Value != "" {
		return row.Value
	}
	return "nginx"
}

func resolveInstallPath(installPath, dataDir string) string {
	if root := findInstallRoot(installPath, dataDir); root != "" {
		return root
	}
	return preferredInstallPath(installPath, dataDir)
}

func vhostFile(dataDir string) string {
	return filepath.Join(dataDir, "nginx", "vhosts", "owpanel-phpmyadmin.conf")
}

func hasPhpMyAdminFiles(root string) bool {
	return fileExists(filepath.Join(root, "index.php")) && fileExists(filepath.Join(root, "libraries", "common.inc.php"))
}

func downloadAndExtract(root, version string) error {
	ver := normalizeVersion(version)
	url := fmt.Sprintf("https://files.phpmyadmin.net/phpMyAdmin/%s/phpMyAdmin-%s-all-languages.tar.gz", ver, ver)
	tmp := filepath.Join(os.TempDir(), "phpmyadmin-"+ver+".tar.gz")
	if err := downloadFile(url, tmp); err != nil {
		return fmt.Errorf("下载 phpMyAdmin 失败: %w", err)
	}
	defer os.Remove(tmp)
	return extractTarGz(tmp, root)
}

func normalizeVersion(version string) string {
	v := strings.TrimSpace(version)
	if v == "" || v == "latest" {
		return "5.2.1"
	}
	if strings.Count(v, ".") == 1 {
		return v + ".1"
	}
	return v
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			f, err := os.Create(dest)
			if err == nil {
				_, copyErr := io.Copy(f, resp.Body)
				f.Close()
				if copyErr == nil {
					return nil
				}
			}
		}
	}
	out, err := exec.Command("curl", "-fsSL", "-o", dest, url).CombinedOutput()
	if err == nil && fileExists(dest) {
		return nil
	}
	return fmt.Errorf("%s (%s)", strings.TrimSpace(string(out)), url)
}

func extractTarGz(src, destDir string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var stripPrefix string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := hdr.Name
		if stripPrefix == "" {
			if i := strings.Index(name, "/"); i > 0 {
				stripPrefix = name[:i+1]
			}
		}
		name = strings.TrimPrefix(name, stripPrefix)
		if name == "" || name == "." {
			continue
		}
		target := filepath.Join(destDir, filepath.FromSlash(name))
		switch hdr.Typeflag {
		case tar.TypeDir:
			_ = os.MkdirAll(target, 0755)
		case tar.TypeReg:
			_ = os.MkdirAll(filepath.Dir(target), 0755)
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

func writeConfig(root string) error {
	cfgPath := filepath.Join(root, "config.inc.php")
	if fileExists(cfgPath) {
		return nil
	}
	secret, _ := randomHex(16)
	content := fmt.Sprintf(`<?php
/**
 * OWPanel generated phpMyAdmin config
 */
declare(strict_types=1);

$cfg['blowfish_secret'] = '%s';
$i = 0;
$i++;
$cfg['Servers'][$i]['auth_type'] = 'cookie';
$cfg['Servers'][$i]['host'] = '127.0.0.1';
$cfg['Servers'][$i]['compress'] = false;
$cfg['Servers'][$i]['AllowNoPassword'] = false;
`, secret)
	return os.WriteFile(cfgPath, []byte(content), 0644)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func detectPhpFpmBackend() string {
	mgr := php.NewManager("")
	for _, key := range []string{"php83", "php82", "php81", "php74"} {
		st := mgr.Status(key)
		if st.Running {
			return php.FastCGIBackend(php.VersionFromKey(key))
		}
	}
	return php.FastCGIBackend("8.3")
}

func serverHost() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if v4 := ip.To4(); v4 != nil {
				return v4.String()
			}
		}
	}
	return "127.0.0.1"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func AppInstalled(db *gorm.DB) bool {
	var app models.App
	if db.Where("app_key = ? AND installed = ?", appKey, true).First(&app).Error != nil {
		return false
	}
	return true
}
