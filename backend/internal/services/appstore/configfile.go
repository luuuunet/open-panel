package appstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/php"
)

type ConfigMeta struct {
	ConfigPath         string `json:"config_path"`
	ResolvedConfigPath string `json:"resolved_config_path"`
	HasConfigFile      bool   `json:"has_config_file"`
	IsPHP              bool   `json:"is_php"`
}

func IsPHPKey(key string) bool {
	return strings.HasPrefix(key, "php") && key != "phpmyadmin"
}

func (s *Service) ConfigMeta(key string) (ConfigMeta, error) {
	app, err := s.Get(key)
	if err != nil {
		return ConfigMeta{}, err
	}
	resolved := s.resolveConfigPath(key, app.ConfigPath)
	meta := ConfigMeta{
		ConfigPath:         app.ConfigPath,
		ResolvedConfigPath: resolved,
		HasConfigFile:      resolved != "" && fileExists(resolved),
		IsPHP:              IsPHPKey(key),
	}
	if meta.IsPHP {
		if ini, err := php.NewManager(s.dataDir).IniPath(key); err == nil {
			meta.ResolvedConfigPath = ini
			meta.HasConfigFile = fileExists(ini)
		}
	}
	return meta, nil
}

func (s *Service) ReadConfigRaw(key string) (string, error) {
	app, err := s.Get(key)
	if err != nil {
		return "", err
	}
	if !app.Installed {
		return "", errors.New("software not installed")
	}
	if IsPHPKey(key) {
		return php.NewManager(s.dataDir).ReadIni(key)
	}
	if isWebServerKey(key) {
		return s.readWebServerConfig(key)
	}
	path := s.resolveConfigPath(key, app.ConfigPath)
	if path == "" {
		return "", errors.New("no config file path for this software")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && (detectConfigKind(path, key) == "env" || detectConfigKind(path, key) == "json") {
			return "", nil
		}
		return "", fmt.Errorf("read config: %w", err)
	}
	return string(data), nil
}

func (s *Service) WriteConfigRaw(key, content string) error {
	app, err := s.Get(key)
	if err != nil {
		return err
	}
	if !app.Installed {
		return errors.New("software not installed")
	}
	if IsPHPKey(key) {
		if err := php.NewManager(s.dataDir).WriteIni(key, content); err != nil {
			return err
		}
		return s.ServiceAction(key, "restart")
	}
	if isWebServerKey(key) {
		return s.writeWebServerConfig(key, content)
	}
	path := s.resolveConfigPath(key, app.ConfigPath)
	if path == "" {
		return errors.New("no config file path for this software")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	return s.ServiceAction(key, "reload")
}

func (s *Service) PHPDetail(key string) (php.PHPDetail, error) {
	if !IsPHPKey(key) {
		return php.PHPDetail{}, errors.New("not a PHP runtime")
	}
	app, err := s.Get(key)
	if err != nil {
		return php.PHPDetail{}, err
	}
	if !app.Installed {
		return php.PHPDetail{}, errors.New("software not installed")
	}
	return php.NewManager(s.dataDir).Detail(key)
}

func (s *Service) SetPHPExtension(key, name string, enabled bool) error {
	if !IsPHPKey(key) {
		return errors.New("not a PHP runtime")
	}
	if err := php.NewManager(s.dataDir).SetExtension(key, name, enabled); err != nil {
		return err
	}
	return s.ServiceAction(key, "restart")
}

func (s *Service) InstallPHPExtension(key, name string) error {
	if !IsPHPKey(key) {
		return errors.New("not a PHP runtime")
	}
	if err := php.NewManager(s.dataDir).InstallExtension(key, name); err != nil {
		return err
	}
	return s.ServiceAction(key, "restart")
}

func (s *Service) SetPHPDisableFunctions(key, value string) error {
	if !IsPHPKey(key) {
		return errors.New("not a PHP runtime")
	}
	if err := php.NewManager(s.dataDir).SetDisableFunctions(key, value); err != nil {
		return err
	}
	return s.ServiceAction(key, "restart")
}

func (s *Service) resolveConfigPath(key, fallback string) string {
	candidates := []string{}
	if fallback != "" {
		candidates = append(candidates, fallback)
	}
	switch key {
	case "nginx":
		candidates = append(candidates,
			"/etc/nginx/nginx.conf",
			filepath.Join(s.dataDir, "server", "nginx", "conf", "nginx.conf"),
			filepath.Join(s.dataDir, "nginx", "nginx.conf"),
		)
	case "openresty":
		candidates = append(candidates,
			"/usr/local/openresty/nginx/conf/nginx.conf",
			"/etc/openresty/nginx.conf",
		)
	case "apache":
		candidates = append(candidates,
			"/etc/apache2/apache2.conf",
			"/etc/httpd/conf/httpd.conf",
			filepath.Join(s.dataDir, "apache", "owpanel.conf"),
		)
	case "mysql", "mariadb":
		candidates = append(candidates, "/etc/my.cnf", "/etc/mysql/my.cnf")
	case "redis":
		candidates = append(candidates,
			"/etc/redis/redis.conf",
			filepath.Join(s.dataDir, "server", "redis", "redis.conf"),
			filepath.Join(s.dataDir, "redis", "redis.conf"),
		)
	case "postgresql":
		candidates = append(candidates,
			filepath.Join(s.dataDir, "server", "pgsql", "data", "postgresql.conf"),
			"/var/lib/postgresql/data/postgresql.conf",
		)
	case "mongodb":
		candidates = append(candidates, "/etc/mongod.conf")
	case "memcached":
		candidates = append(candidates, "/etc/memcached.conf")
	case "pureftpd":
		candidates = append(candidates,
			filepath.Join(s.dataDir, "server", "pureftpd", "etc", "pure-ftpd.conf"),
			"/etc/pure-ftpd/pure-ftpd.conf",
		)
	case "phpmyadmin":
		candidates = append(candidates, filepath.Join(s.dataDir, "server", "phpmyadmin", "config.inc.php"))
	case "docker":
		candidates = append(candidates, "/etc/docker/daemon.json", filepath.Join(s.dataDir, "docker", "daemon.json"))
	case "fail2ban":
		candidates = append(candidates, "/etc/fail2ban/jail.local", "/etc/fail2ban/jail.conf")
	case "supervisor":
		candidates = append(candidates, "/etc/supervisor/supervisord.conf")
	case "tomcat9":
		candidates = append(candidates, "/etc/tomcat9/server.xml")
	case "tomcat10":
		candidates = append(candidates, "/etc/tomcat10/server.xml")
	case "localai":
		candidates = append(candidates, filepath.Join(s.dataDir, "ai", "localai", "config.yaml"))
	case "jupyter":
		candidates = append(candidates, filepath.Join(s.dataDir, "ai", "jupyter", "jupyter_lab_config.py"))
	}
	if _, ok := dockerSpec(key); ok {
		candidates = append(candidates, filepath.Join(s.dataDir, "apps", key, ".env"))
	}
	if !IsPHPKey(key) {
		if app, err := s.Get(key); err == nil && app.InstallPath != "" {
			candidates = append(candidates, filepath.Join(app.InstallPath, ".env"))
		}
	}
	if IsPHPKey(key) {
		candidates = append(candidates,
			filepath.Join(s.dataDir, "php", key, "php.ini"),
			filepath.Join(s.dataDir, "server", key, "etc", "php.ini"),
		)
		if mgr := php.NewManager(s.dataDir); mgr != nil {
			if ini, err := mgr.IniPath(key); err == nil {
				candidates = append([]string{ini}, candidates...)
			}
		}
	}
	for _, p := range candidates {
		if p != "" && fileExists(p) {
			return p
		}
	}
	if len(candidates) > 0 && candidates[0] != "" {
		return candidates[0]
	}
	return fallback
}

func isWebServerKey(key string) bool {
	switch key {
	case "nginx", "openresty", "apache":
		return true
	}
	return false
}

func (s *Service) readWebServerConfig(key string) (string, error) {
	path := s.resolveConfigPath(key, "")
	app, _ := s.Get(key)
	if app != nil && app.ConfigPath != "" {
		path = s.resolveConfigPath(key, app.ConfigPath)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		panelConf := filepath.Join(s.dataDir, key, "owpanel.conf")
		if key == "apache" {
			panelConf = filepath.Join(s.dataDir, "apache", "owpanel.conf")
		}
		data, err = os.ReadFile(panelConf)
		if err != nil {
			return "", err
		}
	}
	return string(data), nil
}

func (s *Service) writeWebServerConfig(key, content string) error {
	app, err := s.Get(key)
	if err != nil {
		return err
	}
	path := s.resolveConfigPath(key, app.ConfigPath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	return s.ServiceAction(key, "reload")
}
