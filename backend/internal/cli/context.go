package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/luuuunet/owpanel/internal/auth"
	"github.com/luuuunet/owpanel/internal/config"
	"github.com/luuuunet/owpanel/internal/database"
	"github.com/luuuunet/owpanel/internal/services/settings"
)

const systemdUnit = "owpanel"

type Context struct {
	Cfg      *config.Config
	DataDir  string
	Install  string
	Auth     *auth.Service
	Settings *settings.Service
}

func NewContext() (*Context, error) {
	ensurePanelEnv()
	cfg := config.Load()
	dataDir := cfg.DataDir
	if !filepath.IsAbs(dataDir) {
		if wd, err := os.Getwd(); err == nil {
			dataDir = filepath.Join(wd, dataDir)
		}
	}

	install := envFirst("OWPANEL_HOME", "OPEN_PANEL_HOME")
	if install == "" {
		install = filepath.Dir(dataDir)
	}

	db, err := database.Init(dataDir)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}

	return &Context{
		Cfg:      cfg,
		DataDir:  dataDir,
		Install:  install,
		Auth:     auth.NewService(db, cfg.JWTSecret),
		Settings: settings.NewServiceWithDataDir(db, dataDir),
	}, nil
}

func serviceName() string {
	if v := envFirst("OWPANEL_SERVICE", "OPEN_PANEL_SERVICE"); v != "" {
		return v
	}
	return systemdUnit
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

var envLineRe = regexp.MustCompile(`^Environment=((?:OWPANEL|OPEN_PANEL)_[A-Z_]+)=(.*)$`)

func ensurePanelEnv() {
	if os.Getenv("OWPANEL_DATA") != "" || os.Getenv("OPEN_PANEL_DATA") != "" {
		return
	}
	unit := filepath.Join("/etc/systemd/system", serviceName()+".service")
	data, err := os.ReadFile(unit)
	if err != nil {
		for _, legacy := range []struct{ home, data string }{
			{"/opt/owpanel", "/opt/owpanel/data"},
			{"/opt/open-panel", "/opt/open-panel/data"},
		} {
			if _, err := os.Stat(legacy.data); err == nil {
				os.Setenv("OWPANEL_DATA", legacy.data)
				os.Setenv("OWPANEL_HOME", legacy.home)
				return
			}
		}
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		m := envLineRe.FindStringSubmatch(strings.TrimSpace(line))
		if len(m) == 3 {
			key := m[1]
			if strings.HasPrefix(key, "OPEN_PANEL_") {
				key = "OWPANEL_" + strings.TrimPrefix(key, "OPEN_PANEL_")
			}
			os.Setenv(key, strings.Trim(m[2], `"`))
		}
	}
	if os.Getenv("OWPANEL_DATA") == "" {
		if exe, err := os.Executable(); err == nil {
			home := filepath.Dir(exe)
			if st, err := os.Stat(filepath.Join(home, "data", "panel.db")); err == nil && !st.IsDir() {
				os.Setenv("OWPANEL_HOME", home)
				os.Setenv("OWPANEL_DATA", filepath.Join(home, "data"))
				if os.Getenv("OWPANEL_WEB") == "" {
					if st, err := os.Stat(filepath.Join(home, "web", "index.html")); err == nil && !st.IsDir() {
						os.Setenv("OWPANEL_WEB", filepath.Join(home, "web"))
					}
				}
			}
		}
	}
}

func readInt(prompt string, def int) int {
	s := readLine(prompt)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
