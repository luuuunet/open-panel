package appstore

import (
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/php"
)

// phpPanelInstalled reports whether the panel installed/managed this PHP version.
func phpPanelInstalled(key, dataDir string) bool {
	if key == "phpmyadmin" || !strings.HasPrefix(key, "php") {
		return false
	}
	if fileExists(filepath.Join(dataDir, "php", key, ".owpanel-installed")) {
		return true
	}
	if fileExists(filepath.Join(dataDir, "server", key, ".owpanel-installed")) {
		return true
	}
	return false
}

func (s *Service) reconcilePHPInstallRecords() {
	for _, item := range mergedCatalog() {
		key := item.Key
		if !strings.HasPrefix(key, "php") || key == "phpmyadmin" {
			continue
		}
		app, err := s.Get(key)
		if err != nil || !app.Installed || IsSimulatedInstall(key, s.dataDir) {
			continue
		}
		if phpPanelInstalled(key, s.dataDir) {
			continue
		}
		_ = s.db.Model(&app).Updates(map[string]interface{}{
			"installed":     false,
			"status":        "stopped",
			"install_error": "",
		}).Error
		s.InvalidateLiveStatus(key)
	}
}

// phpVersionInstalledForListing is true when the panel should show this PHP in runtime/management UI.
func (s *Service) phpVersionInstalledForListing(key string) bool {
	if phpPanelInstalled(key, s.dataDir) {
		return true
	}
	app, err := s.Get(key)
	return err == nil && app.Installed && !IsSimulatedInstall(key, s.dataDir)
}

func phpBinaryForKey(key, dataDir string) string {
	st := php.NewManager(dataDir).Status(key)
	return st.Binary
}
