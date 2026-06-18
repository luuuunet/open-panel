package mail

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/php"
)

const webmailDefaultPort = 889

var webmailDownloadURLs = []string{
	"https://github.com/the-djmaze/snappymail/releases/download/v2.38.2/snappymail-2.38.2.tar.gz",
	"https://ghproxy.com/https://github.com/the-djmaze/snappymail/releases/download/v2.38.2/snappymail-2.38.2.tar.gz",
	"https://snappymail.eu/repository/latest.tar.gz",
	"https://snappymail.eu/repository/snappymail-latest.tar.gz",
}

type WebServerHooks struct {
	GetActive func() string
	Reload    func(key string) error
	EnsureInc func(key string)
	IsRunning func(key string) bool
}

type WebmailStatus struct {
	Installed         bool   `json:"installed"`
	Running           bool   `json:"running"`
	URL               string `json:"url"`
	AdminURL          string `json:"admin_url"`
	Host              string `json:"host"`
	Port              int    `json:"port"`
	MailDomain        string `json:"mail_domain"`
	InstallPath       string `json:"install_path"`
	AdminPasswordFile string `json:"admin_password_file"`
	Hint              string `json:"hint"`
	SetupError        string `json:"setup_error,omitempty"`
}

type WebmailInstallRequest struct {
	MailDomain  string `json:"mail_domain"`
	HostPrefix  string `json:"host_prefix"`
	Port        int    `json:"port"`
	UsePortMode bool   `json:"use_port_mode"`
}

func (s *Service) SetWebServerHooks(h WebServerHooks) {
	s.ws = h
}

func (s *Service) webmailRoot() string {
	return filepath.Join(s.mailRoot(), "webmail")
}

func (s *Service) webmailVhostFile() string {
	return filepath.Join(s.dataDir, "nginx", "vhosts", "owpanel-snappymail.conf")
}

func (s *Service) WebmailStatus() (*WebmailStatus, error) {
	root := s.webmailRoot()
	port := s.webmailPort()
	host, mailDomain := s.webmailHost()
	info := &WebmailStatus{
		Installed:         hasSnappyMailFiles(root),
		Port:              port,
		Host:              host,
		MailDomain:        mailDomain,
		InstallPath:       root,
		AdminPasswordFile: filepath.Join(root, "data", "_data_", "_default_", "admin_password.txt"),
	}
	if !info.Installed {
		info.Hint = "安装 SnappyMail 后，用户可通过浏览器登录邮箱（IMAP/SMTP 已预配置为本地 Dovecot/Postfix）。"
		return info, nil
	}
	info.Running = s.webmailRunning(port)
	info.URL = s.webmailAccessURL(host, port)
	info.AdminURL = info.URL + "?admin"
	if info.SetupError == "" {
		if _, err := s.detectWebmailPHP(); err != nil {
			info.SetupError = err.Error()
		}
	}
	if pwd, err := os.ReadFile(info.AdminPasswordFile); err == nil && strings.TrimSpace(string(pwd)) != "" {
		info.Hint = fmt.Sprintf("SnappyMail 管理员默认密码见 %s；首次登录 ?admin 后请修改。邮箱用户使用面板创建的邮箱账号登录。", info.AdminPasswordFile)
	} else if info.SetupError != "" {
		info.Hint = info.SetupError
	} else if !info.Running {
		info.Hint = "请确保 Nginx/OpenResty 与 PHP-FPM 已启动；可点击「修复配置」重试。"
	} else {
		info.Hint = "使用面板创建的完整邮箱地址与密码登录 Web 邮箱。"
	}
	return info, nil
}

func (s *Service) StartInstallWebmail(req *WebmailInstallRequest) error {
	if webmailInstallLogs != nil && webmailInstallLogs.IsInstalling() {
		return fmt.Errorf("SnappyMail 安装正在进行中")
	}
	if webmailInstallLogs != nil {
		webmailInstallLogs.Begin("SnappyMail")
	}
	go s.runInstallWebmail(req)
	return nil
}

func (s *Service) runInstallWebmail(req *WebmailInstallRequest) {
	var installErr error
	defer func() {
		if webmailInstallLogs != nil {
			webmailInstallLogs.Finish(installErr)
		}
	}()

	log := func(format string, args ...interface{}) {
		s.webmailLog(fmt.Sprintf("[%s] "+format, append([]interface{}{time.Now().Format("15:04:05")}, args...)...))
	}

	log("预检: 检查运行环境")
	if runtime.GOOS == "windows" {
		installErr = fmt.Errorf("SnappyMail 需在 Linux 主机上安装")
		return
	}

	phpBin, err := s.detectWebmailPHP()
	if err != nil {
		installErr = err
		return
	}
	log("检测到 PHP: %s", phpBin)

	log("预检: 检查 PHP-FPM / FastCGI 后端")
	fcgi, err := s.ensureWebmailPhpFpmBackend(log)
	if err != nil {
		installErr = err
		return
	}
	log("FastCGI 后端: %s", fcgi)

	root := s.webmailRoot()
	log("准备安装目录: %s", root)
	if err := os.MkdirAll(root, 0755); err != nil {
		installErr = fmt.Errorf("创建安装目录失败: %w", err)
		return
	}

	if !hasSnappyMailFiles(root) {
		log("下载 SnappyMail 安装包…")
		if err := downloadSnappyMail(root, log); err != nil {
			installErr = err
			return
		}
		log("解压完成")
	} else {
		log("检测到已有 SnappyMail 文件，跳过下载")
	}

	mailDomain := strings.TrimSpace(strings.ToLower(req.MailDomain))
	if mailDomain == "" {
		var d models.MailDomain
		if s.db.Order("id asc").First(&d).Error == nil {
			mailDomain = d.Domain
		}
	}
	if mailDomain != "" {
		s.saveWebmailSetting("mail_webmail_domain", mailDomain)
		log("邮件域名: %s", mailDomain)
	} else {
		log("未指定邮件域名，将使用端口模式")
	}

	prefix := strings.TrimSpace(req.HostPrefix)
	if prefix == "" {
		prefix = "webmail"
	}
	s.saveWebmailSetting("mail_webmail_host_prefix", prefix)

	if req.UsePortMode || mailDomain == "" {
		s.saveWebmailSetting("mail_webmail_port_mode", "1")
		port := req.Port
		if port <= 0 {
			port = webmailDefaultPort
		}
		s.saveWebmailSetting("mail_webmail_port", fmt.Sprintf("%d", port))
		log("访问模式: 端口 %d", port)
	} else {
		s.saveWebmailSetting("mail_webmail_port_mode", "0")
		host := prefix + "." + mailDomain
		s.saveWebmailSetting("mail_webmail_host", host)
		log("访问模式: 域名 %s（需 DNS A 记录指向本机）", host)
	}

	log("写入 SnappyMail 域名/IMAP/SMTP 配置…")
	if err := s.writeSnappyMailDomainConfigs(mailDomain); err != nil {
		installErr = fmt.Errorf("写入域名配置失败: %w", err)
		return
	}

	log("设置 data 目录权限…")
	if err := s.ensureWebmailPermissions(root); err != nil {
		installErr = fmt.Errorf("设置权限失败: %w", err)
		return
	}

	log("生成 Nginx 虚拟主机并重载…")
	if err := s.ensureWebmailVhostWithBackend(root, fcgi); err != nil {
		installErr = err
		return
	}

	s.saveWebmailSetting("mail_webmail_installed", "1")
	log("SnappyMail 安装完成")
}

func (s *Service) UninstallWebmail() error {
	_ = os.Remove(s.webmailVhostFile())
	_ = s.reloadWebServer()
	_ = os.RemoveAll(s.webmailRoot())
	s.saveWebmailSetting("mail_webmail_installed", "0")
	return nil
}

func (s *Service) RepairWebmail() error {
	root := s.webmailRoot()
	if !hasSnappyMailFiles(root) {
		return fmt.Errorf("SnappyMail 未安装")
	}
	_, mailDomain := s.webmailHost()
	if err := s.writeSnappyMailDomainConfigs(mailDomain); err != nil {
		return err
	}
	if err := s.ensureWebmailPermissions(root); err != nil {
		return err
	}
	return s.ensureWebmailVhost(root)
}

func (s *Service) saveWebmailSetting(key, value string) {
	s.db.Where("key = ?", key).Assign(models.PanelSetting{Value: value}).FirstOrCreate(&models.PanelSetting{Key: key})
}

func (s *Service) getWebmailSetting(key string) string {
	var row models.PanelSetting
	if s.db.Where("key = ?", key).First(&row).Error != nil {
		return ""
	}
	return strings.TrimSpace(row.Value)
}

func (s *Service) webmailPort() int {
	if s.getWebmailSetting("mail_webmail_port_mode") != "1" {
		return 0
	}
	p := s.getWebmailSetting("mail_webmail_port")
	if p == "" {
		return webmailDefaultPort
	}
	var port int
	fmt.Sscanf(p, "%d", &port)
	if port <= 0 {
		return webmailDefaultPort
	}
	return port
}

func (s *Service) webmailHost() (host, mailDomain string) {
	mailDomain = s.getWebmailSetting("mail_webmail_domain")
	if s.getWebmailSetting("mail_webmail_port_mode") == "1" {
		return "", mailDomain
	}
	host = s.getWebmailSetting("mail_webmail_host")
	if host == "" && mailDomain != "" {
		prefix := s.getWebmailSetting("mail_webmail_host_prefix")
		if prefix == "" {
			prefix = "webmail"
		}
		host = prefix + "." + mailDomain
	}
	return host, mailDomain
}

func (s *Service) webmailAccessURL(host string, port int) string {
	ip := s.serverIP()
	if host != "" {
		if cert, _ := s.resolveSSLPathsForHost(host); cert != "" {
			return "https://" + host + "/"
		}
		return "http://" + host + "/"
	}
	if ip == "" {
		ip = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%d/", ip, port)
}

func (s *Service) resolveSSLPathsForHost(host string) (cert, key string) {
	for _, base := range []string{
		filepath.Join("/etc/letsencrypt/live", host),
		filepath.Join(s.dataDir, "ssl", host),
	} {
		c := filepath.Join(base, "fullchain.pem")
		k := filepath.Join(base, "privkey.pem")
		if fileExists(c) && fileExists(k) {
			return c, k
		}
	}
	return "", ""
}

func hasSnappyMailFiles(root string) bool {
	if fi, err := os.Stat(filepath.Join(root, "index.php")); err == nil && fi.Size() > 50 {
		return true
	}
	if fileExists(filepath.Join(root, "snappymail", "v")) {
		return true
	}
	if fileExists(filepath.Join(root, "data")) {
		return true
	}
	matches, _ := filepath.Glob(filepath.Join(root, "v", "*", "include.php"))
	return len(matches) > 0
}

// snappyMailDocumentRoot returns the nginx/php document root for SnappyMail.
func snappyMailDocumentRoot(root string) string {
	if fi, err := os.Stat(filepath.Join(root, "index.php")); err == nil && fi.Size() > 50 {
		return root
	}
	if fileExists(filepath.Join(root, "snappymail", "v")) {
		return root
	}
	matches, _ := filepath.Glob(filepath.Join(root, "v", "*", "include.php"))
	if len(matches) > 0 {
		return filepath.Dir(filepath.Dir(matches[0]))
	}
	return ""
}

func downloadSnappyMail(root string, log func(string, ...interface{})) error {
	tmp := filepath.Join(os.TempDir(), "snappymail-latest.tar.gz")
	defer os.Remove(tmp)

	var lastErr error
	for i, url := range webmailDownloadURLs {
		if log != nil {
			log("尝试下载 (%d/%d): %s", i+1, len(webmailDownloadURLs), url)
		}
		if err := downloadWebmailFile(url, tmp); err != nil {
			lastErr = err
			if log != nil {
				log("下载失败: %v", err)
			}
			continue
		}
		if log != nil {
			log("下载成功，开始解压…")
		}
		if err := extractSnappyMailArchive(tmp, root); err != nil {
			lastErr = fmt.Errorf("解压失败: %w", err)
			if log != nil {
				log("解压失败: %v", err)
			}
			continue
		}
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("所有下载源均失败，请检查网络: %w", lastErr)
	}
	return fmt.Errorf("下载 SnappyMail 失败")
}

func downloadWebmailFile(url, dest string) error {
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
	out, err := exec.Command("curl", "-fsSL", "-L", "-o", dest, url).CombinedOutput()
	if err == nil && fileExists(dest) {
		return nil
	}
	return fmt.Errorf("%s (%s)", strings.TrimSpace(string(out)), url)
}

func extractSnappyMailArchive(src, destDir string) error {
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

	var entries []tarEntry
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeDir {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return err
		}
		entries = append(entries, tarEntry{name: hdr.Name, mode: hdr.Mode, typ: hdr.Typeflag, data: data})
	}

	hasRootIndex := false
	hasSnappyPrefix := false
	for _, e := range entries {
		if e.name == "index.php" {
			hasRootIndex = true
		}
		if strings.HasPrefix(e.name, "snappymail/") {
			hasSnappyPrefix = true
		}
	}

	stripPrefix := ""
	if !(hasRootIndex && hasSnappyPrefix) {
		for _, e := range entries {
			if i := strings.Index(e.name, "/"); i > 0 {
				stripPrefix = e.name[:i+1]
				break
			}
		}
	}

	for _, e := range entries {
		name := strings.TrimPrefix(e.name, stripPrefix)
		if name == "" || name == "." {
			continue
		}
		target := filepath.Join(destDir, filepath.FromSlash(name))
		switch e.typ {
		case tar.TypeDir:
			_ = os.MkdirAll(target, 0755)
		case tar.TypeReg:
			_ = os.MkdirAll(filepath.Dir(target), 0755)
			if err := os.WriteFile(target, e.data, os.FileMode(e.mode)); err != nil {
				return err
			}
		}
	}
	return nil
}

type tarEntry struct {
	name string
	mode int64
	typ  byte
	data []byte
}

func extractTarGzToDir(src, destDir string) error {
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

func (s *Service) writeSnappyMailDomainConfigs(primaryDomain string) error {
	root := s.webmailRoot()
	domainsDir := filepath.Join(root, "data", "_data_", "_default_", "domains")
	if err := os.MkdirAll(domainsDir, 0755); err != nil {
		return err
	}
	var domains []models.MailDomain
	if err := s.db.Find(&domains).Error; err != nil {
		return err
	}
	if len(domains) == 0 && primaryDomain != "" {
		domains = []models.MailDomain{{Domain: primaryDomain}}
	}
	for _, d := range domains {
		cfg := snappyDomainConfig(d.Domain)
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		path := filepath.Join(domainsDir, d.Domain+".json")
		if err := os.WriteFile(path, data, 0644); err != nil {
			return err
		}
	}
	return nil
}

func snappyDomainConfig(domain string) map[string]interface{} {
	sslOpts := map[string]interface{}{
		"verify_peer":       false,
		"verify_peer_name":  false,
		"allow_self_signed": true,
		"SNI_enabled":       true,
	}
	return map[string]interface{}{
		"name": domain,
		"IMAP": map[string]interface{}{
			"host":        "127.0.0.1",
			"port":        993,
			"secure":      2,
			"shortLogin":  0,
			"sasl":        []string{"PLAIN", "LOGIN"},
			"ssl":         sslOpts,
			"useAuth":     true,
			"setDisabled": false,
			"usePhpImap":  false,
		},
		"SMTP": map[string]interface{}{
			"host":        "127.0.0.1",
			"port":        587,
			"secure":      1,
			"shortLogin":  0,
			"sasl":        []string{"PLAIN", "LOGIN"},
			"ssl":         sslOpts,
			"useAuth":     true,
			"setDisabled": false,
			"setAsDefault": true,
			"usePhpMail":  false,
		},
		"whiteList": "",
	}
}

func (s *Service) ensureWebmailPermissions(root string) error {
	dataDir := filepath.Join(root, "data")
	_ = os.MkdirAll(dataDir, 0755)
	for _, u := range []string{"www-data", "nginx", "apache"} {
		if out, err := exec.Command("id", "-u", u).Output(); err == nil && strings.TrimSpace(string(out)) != "" {
			_ = exec.Command("chown", "-R", u+":"+u, dataDir).Run()
			break
		}
	}
	return nil
}

func (s *Service) ensureWebmailVhost(root string) error {
	fcgi, err := s.ensureWebmailPhpFpmBackend(nil)
	if err != nil {
		return err
	}
	return s.ensureWebmailVhostWithBackend(root, fcgi)
}

func (s *Service) ensureWebmailVhostWithBackend(root, fcgi string) error {
	host, _ := s.webmailHost()
	port := s.webmailPort()
	docRoot := snappyMailDocumentRoot(root)
	if docRoot == "" {
		docRoot = root
	}
	rootSlash := filepath.ToSlash(docRoot)

	var body string
	if host != "" {
		cert, key := s.resolveSSLPathsForHost(host)
		if cert != "" && key != "" {
			body = fmt.Sprintf(`server {
    listen 80;
    server_name %s;
    return 301 https://$host$request_uri;
}
server {
    listen 443 ssl http2;
    server_name %s;
    root %s;
    index index.php;

    ssl_certificate %s;
    ssl_certificate_key %s;
    client_max_body_size 50M;

    location ~ ^/(data|include)/ {
        deny all;
    }
    location / {
        try_files $uri $uri/ /index.php?$query_string;
    }
    location ~ \.php$ {
        fastcgi_pass %s;
        fastcgi_index index.php;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
    }
}`, host, host, rootSlash, cert, key, fcgi)
		} else {
			body = webmailServerBlock(host, rootSlash, fcgi, false)
		}
	} else {
		if port <= 0 {
			port = webmailDefaultPort
		}
		body = fmt.Sprintf(`server {
    listen %d;
    server_name _;
    root %s;
    index index.php;
    client_max_body_size 50M;

    location ~ ^/(data|include)/ {
        deny all;
    }
    location / {
        try_files $uri $uri/ /index.php?$query_string;
    }
    location ~ \.php$ {
        fastcgi_pass %s;
        fastcgi_index index.php;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
    }
}`, port, rootSlash, fcgi)
	}

	content := "# OWPanel — SnappyMail Webmail\n" + body + "\n"
	if err := os.MkdirAll(filepath.Dir(s.webmailVhostFile()), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(s.webmailVhostFile(), []byte(content), 0644); err != nil {
		return fmt.Errorf("写入 Nginx 配置失败: %w", err)
	}
	if err := s.reloadWebServer(); err != nil {
		return fmt.Errorf("Nginx 重载失败: %w", err)
	}
	return nil
}

func webmailServerBlock(host, root, fcgi string, _ bool) string {
	return fmt.Sprintf(`server {
    listen 80;
    server_name %s;
    root %s;
    index index.php;
    client_max_body_size 50M;

    location ~ ^/(data|include)/ {
        deny all;
    }
    location / {
        try_files $uri $uri/ /index.php?$query_string;
    }
    location ~ \.php$ {
        fastcgi_pass %s;
        fastcgi_index index.php;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
    }
}`, host, root, fcgi)
}

func (s *Service) detectWebmailPHP() (string, error) {
	mgr := php.NewManager(s.dataDir)
	for _, key := range []string{"php83", "php82", "php81", "php74"} {
		st := mgr.Status(key)
		if st.Binary != "" {
			return st.Binary, nil
		}
	}
	for _, name := range []string{"php", "php8.3", "php83", "php8.2", "php82", "php8.1", "php81", "php7.4", "php74"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	for _, ver := range []string{"83", "82", "81", "74"} {
		for _, rel := range []string{
			filepath.Join("server", "php", ver, "bin", "php"),
			filepath.Join("server", "php"+ver, "bin", "php"),
			filepath.Join("server", "php"+ver, "php"),
		} {
			p := filepath.Join(s.dataDir, rel)
			if fileExists(p) {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("未检测到 PHP，请先在软件商店安装 PHP 并启动 PHP-FPM")
}

func (s *Service) ensureWebmailPhpFpmBackend(log func(string, ...interface{})) (string, error) {
	mgr := php.NewManager(s.dataDir)
	keys := []string{"php83", "php82", "php81", "php74"}

	for _, key := range keys {
		st := mgr.Status(key)
		if !st.Running {
			continue
		}
		backend := php.FastCGIBackend(php.VersionFromKey(key))
		if s.verifyFastcgiBackend(backend) {
			if log != nil {
				log("使用运行中的 PHP %s FastCGI: %s", st.Version, backend)
			}
			return backend, nil
		}
		if log != nil {
			log("PHP %s 标记为运行中但 FastCGI 不可达: %s", st.Version, backend)
		}
	}

	for _, key := range keys {
		st := mgr.Status(key)
		if st.Binary == "" && st.Message != "" {
			continue
		}
		ver := php.VersionFromKey(key)
		if log != nil {
			log("尝试启动 PHP %s …", ver)
		}
		if err := mgr.Start(key); err != nil {
			if log != nil {
				log("启动 PHP %s 失败: %v", ver, err)
			}
			continue
		}
		backend := php.FastCGIBackend(ver)
		if s.verifyFastcgiBackend(backend) {
			if log != nil {
				log("PHP %s 已启动，FastCGI: %s", ver, backend)
			}
			return backend, nil
		}
		if log != nil {
			log("PHP %s 已启动但 FastCGI 仍不可达: %s", ver, backend)
		}
	}

	// Try system php-fpm services directly
	if runtime.GOOS == "linux" {
		for _, ver := range []string{"8.3", "8.2", "8.1", "7.4"} {
			svc := "php" + ver + "-fpm"
			if log != nil {
				log("尝试 systemctl start %s …", svc)
			}
			_ = exec.Command("systemctl", "start", svc).Run()
			backend := php.FastCGIBackend(ver)
			if s.verifyFastcgiBackend(backend) {
				if log != nil {
					log("系统 PHP-FPM %s 可用: %s", ver, backend)
				}
				return backend, nil
			}
		}
	}

	backend := php.FastCGIBackend("8.3")
	if !s.verifyFastcgiBackend(backend) {
		return "", fmt.Errorf("PHP-FPM 未运行且无法启动，请先在软件商店安装并启动 PHP（期望 FastCGI: %s）", backend)
	}
	return backend, nil
}

func (s *Service) verifyFastcgiBackend(backend string) bool {
	if strings.HasPrefix(backend, "unix:") {
		sock := strings.TrimPrefix(backend, "unix:")
		_, err := os.Stat(sock)
		return err == nil
	}
	addr := backend
	if !strings.Contains(addr, ":") {
		addr = "127.0.0.1:" + addr
	}
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (s *Service) reloadWebServer() error {
	if s.ws.Reload == nil {
		return nil
	}
	active := "nginx"
	if s.ws.GetActive != nil {
		if v := s.ws.GetActive(); v != "" {
			active = v
		}
	}
	if s.ws.EnsureInc != nil {
		s.ws.EnsureInc(active)
	}
	return s.ws.Reload(active)
}

func (s *Service) webmailRunning(port int) bool {
	if !fileExists(s.webmailVhostFile()) {
		return false
	}
	active := "nginx"
	if s.ws.GetActive != nil {
		if v := s.ws.GetActive(); v != "" {
			active = v
		}
	}
	if s.ws.IsRunning != nil && !s.ws.IsRunning(active) {
		return false
	}
	checkPort := port
	if checkPort <= 0 {
		checkPort = 80
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", checkPort))
	if err == nil {
		_ = ln.Close()
		if port > 0 {
			return false
		}
	}
	if port > 0 {
		return strings.Contains(err.Error(), "bind") || strings.Contains(err.Error(), "address already in use")
	}
	return fileExists(s.webmailVhostFile())
}
