package config

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/luuuunet/owpanel/internal/secrets"
)

type Config struct {
	Port      int
	DataDir   string
	WebDir    string
	JWTSecret string
}

func Load() *Config {
	port := 8888
	if v := envFirst("OWPANEL_PORT", "OPEN_PANEL_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	dataDir := resolveDataDir()

	webDir := envFirst("OWPANEL_WEB", "OPEN_PANEL_WEB")
	if webDir == "" {
		webDir = resolveWebDir()
	}

	jwtSecret := envFirst("OWPANEL_JWT_SECRET", "OPEN_PANEL_JWT_SECRET")

	cfg := &Config{
		Port:      port,
		DataDir:   dataDir,
		WebDir:    webDir,
		JWTSecret: jwtSecret,
	}
	cfg.ResolveSecrets()
	return cfg
}

func (c *Config) ResolveSecrets() {
	if c.JWTSecret == "" {
		c.JWTSecret = secrets.LoadOrCreateJWTSecret(c.DataDir)
	}
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

func resolveDataDir() string {
	if v := envFirst("OWPANEL_DATA", "OPEN_PANEL_DATA"); v != "" {
		return v
	}
	for _, dir := range dataDirCandidates() {
		if hasPanelDB(dir) {
			abs, err := filepath.Abs(dir)
			if err == nil {
				return abs
			}
			return dir
		}
	}
	if abs, err := filepath.Abs("./data"); err == nil {
		return abs
	}
	return "./data"
}

func resolveWebDir() string {
	for _, dir := range []string{"./web", "../backend/web", "../web"} {
		if st, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !st.IsDir() {
			abs, err := filepath.Abs(dir)
			if err == nil {
				return abs
			}
			return dir
		}
	}
	return "./web"
}

func dataDirCandidates() []string {
	candidates := []string{
		"./data",
		"../data",
		"/opt/owpanel/data",
		"/opt/open-panel/data",
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "data"))
	}
	return candidates
}

func hasPanelDB(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "panel.db"))
	return err == nil && !st.IsDir()
}
