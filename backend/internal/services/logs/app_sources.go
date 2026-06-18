package logs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/appstore"
)

type logCandidate struct {
	suffix   string
	label    string
	path     string
	virtual  bool
	docker   bool
	container string
}

func (s *Service) discoverInstalledAppSources(add func(Source)) {
	if s.db == nil {
		return
	}
	var apps []models.App
	if err := s.db.Where("installed = ?", true).Find(&apps).Error; err != nil {
		return
	}
	catalog := appstore.CatalogByKey()
	for _, app := range apps {
		meta := catalog[app.Key]
		name := app.Name
		if name == "" {
			name = meta.Name
		}
		installPath := app.InstallPath
		if installPath == "" {
			installPath = meta.InstallPath
		}
		for _, c := range logCandidatesForApp(app.Key, installPath, s.dataDir) {
			id := "app." + app.Key + "." + c.suffix
			label := c.label
			if label == "" {
				label = c.suffix
			}
			add(Source{
				ID:       id,
				Category: "software",
				Name:     name + " · " + label,
				Path:     c.path,
				Virtual:  c.virtual,
				AppKey:   app.Key,
				AppName:  name,
				LogKind:  c.suffix,
			})
		}
	}
}

func logCandidatesForApp(key, installPath, dataDir string) []logCandidate {
	var out []logCandidate
	add := func(suffix, label, path string) {
		if path == "" {
			return
		}
		out = append(out, logCandidate{suffix: suffix, label: label, path: path})
	}
	addVirtual := func(suffix, label, path string) {
		if path == "" {
			return
		}
		out = append(out, logCandidate{suffix: suffix, label: label, path: path, virtual: true})
	}

	serverBase := filepath.Join(dataDir, "server", key)

	switch key {
	case "nginx", "openresty":
		add("access", "Access log", firstExisting(
			filepath.Join(serverBase, "logs", "access.log"),
			filepath.Join(installPath, "logs", "access.log"),
			"/var/log/nginx/access.log",
			"/usr/local/nginx/logs/access.log",
			"/usr/local/openresty/nginx/logs/access.log",
		))
		add("error", "Error log", firstExisting(
			filepath.Join(serverBase, "logs", "error.log"),
			filepath.Join(installPath, "logs", "error.log"),
			"/var/log/nginx/error.log",
			"/usr/local/nginx/logs/error.log",
			"/usr/local/openresty/nginx/logs/error.log",
		))
	case "apache":
		add("error", "Error log", firstExisting(
			filepath.Join(serverBase, "logs", "error_log"),
			filepath.Join(installPath, "logs", "error_log"),
			"/var/log/apache2/error.log",
		))
		add("access", "Access log", firstExisting(
			filepath.Join(serverBase, "logs", "access_log"),
			filepath.Join(installPath, "logs", "access_log"),
			"/var/log/apache2/access.log",
		))
	case "openlitespeed":
		add("error", "Error log", firstExisting(
			filepath.Join(installPath, "logs", "error.log"),
			filepath.Join(installPath, "logs", "stderr.log"),
		))
		add("access", "Access log", filepath.Join(installPath, "logs", "access.log"))
	case "mysql", "mariadb":
		add("error", "Error log", firstExisting(
			filepath.Join(dataDir, "mysql", "error.log"),
			filepath.Join(serverBase, "error.log"),
			filepath.Join(installPath, "data", "error.log"),
			"/var/log/mysql/error.log",
			"/var/log/mysqld.log",
		))
	case "postgresql":
		add("postgres", "postgresql.log", firstExisting(
			filepath.Join(serverBase, "data", "log", "postgresql.log"),
			filepath.Join(installPath, "data", "log", "postgresql.log"),
			"/var/log/postgresql/postgresql.log",
		))
	case "redis":
		add("redis", "redis.log", firstExisting(
			filepath.Join(serverBase, "redis.log"),
			"/var/log/redis/redis-server.log",
		))
	case "mongodb":
		add("mongod", "mongod.log", firstExisting(
			filepath.Join(serverBase, "logs", "mongod.log"),
			filepath.Join(serverBase, "log", "mongod.log"),
			"/var/log/mongodb/mongod.log",
		))
	case "docker":
		addVirtual("daemon", "docker daemon", firstExisting(
			"/var/log/docker.log",
			filepath.Join(dataDir, "docker", "docker.log"),
		))
	case "fail2ban":
		add("fail2ban", "fail2ban.log", firstExisting(
			"/var/log/fail2ban.log",
			filepath.Join(serverBase, "fail2ban.log"),
		))
	case "pureftpd":
		add("ftp", "pure-ftpd.log", firstExisting(
			"/var/log/pure-ftpd.log",
			"/var/log/pureftpd.log",
			filepath.Join(serverBase, "pure-ftpd.log"),
		))
	case "supervisor":
		add("supervisor", "supervisord.log", firstExisting(
			"/var/log/supervisor/supervisord.log",
			filepath.Join(serverBase, "supervisord.log"),
		))
	case "tomcat9", "tomcat10":
		add("catalina", "catalina.out", firstExisting(
			filepath.Join(installPath, "logs", "catalina.out"),
			"/var/log/tomcat9/catalina.out",
			"/var/log/tomcat10/catalina.out",
		))
	case "pm2":
		add("pm2", "pm2.log", firstExisting(
			filepath.Join(serverBase, "logs", "pm2.log"),
			filepath.Join(serverBase, "pm2.log"),
		))
	case "php83", "php82", "php81", "php74":
		out = append(out, logCandidatesForPHP(key, installPath, dataDir)...)
	default:
		if container, ok := appstore.DockerContainerName(key); ok {
			out = append(out, logCandidate{
				suffix: "docker", label: "容器日志", path: container, virtual: true, docker: true, container: container,
			})
		}
	}

	// Panel-managed install dir log files
	out = appendLogDir(out, serverBase)
	if installPath != "" && !strings.HasPrefix(filepath.Clean(installPath), filepath.Clean(serverBase)) {
		out = appendLogDir(out, installPath)
	}

	// De-duplicate by suffix
	seen := map[string]bool{}
	var deduped []logCandidate
	for _, c := range out {
		if seen[c.suffix] {
			continue
		}
		seen[c.suffix] = true
		deduped = append(deduped, c)
	}
	if len(deduped) == 0 {
		// Installed app with no known log path — placeholder runtime log
		deduped = append(deduped, logCandidate{
			suffix: "runtime",
			label:  "运行日志",
			path:   filepath.Join(serverBase, key+".log"),
		})
	}
	return deduped
}

func firstExisting(paths ...string) string {
	for _, p := range paths {
		if p == "" || strings.Contains(p, "*") {
			continue
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		if strings.Contains(p, "*") {
			if m, _ := filepath.Glob(p); len(m) > 0 {
				return m[0]
			}
			continue
		}
		return p
	}
	return ""
}

func logFileKind(name string) (suffix, label string) {
	lower := strings.ToLower(name)
	switch {
	case lower == "error.log" || lower == "error_log" || strings.HasSuffix(lower, ".err"):
		return "error", "Error log"
	case lower == "access.log" || lower == "access_log":
		return "access", "Access log"
	default:
		ext := filepath.Ext(name)
		return "file_" + slug(strings.TrimSuffix(name, ext)), name
	}
}

func appendLogDir(out []logCandidate, dir string) []logCandidate {
	if dir == "" {
		return out
	}
	for _, sub := range []string{dir, filepath.Join(dir, "logs"), filepath.Join(dir, "data")} {
		entries, err := os.ReadDir(sub)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			lower := strings.ToLower(e.Name())
			if !strings.HasSuffix(lower, ".log") && !strings.HasSuffix(lower, ".err") {
				continue
			}
			suffix, label := logFileKind(e.Name())
			out = append(out, logCandidate{
				suffix: suffix,
				label:  label,
				path:   filepath.Join(sub, e.Name()),
			})
		}
	}
	return out
}

func (s *Service) hasInstalledWebServer() bool {
	if s.db == nil {
		return false
	}
	var n int64
	s.db.Model(&models.App{}).Where("installed = ? AND app_key IN ?", true, []string{"nginx", "openresty", "apache", "openlitespeed"}).Count(&n)
	return n > 0
}

func (s *Service) hasInstalledApp(keys ...string) bool {
	if s.db == nil {
		return false
	}
	var n int64
	s.db.Model(&models.App{}).Where("installed = ? AND app_key IN ?", true, keys).Count(&n)
	return n > 0
}

func panelServerLogPath(dataDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(dataDir, "panel.log")
	}
	return filepath.Join(dataDir, "panel.log")
}
