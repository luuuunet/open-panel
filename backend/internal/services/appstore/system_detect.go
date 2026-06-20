package appstore

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ReconcileInstalledFromSystem syncs catalog install records with the host.
func (s *Service) ReconcileInstalledFromSystem() {
	s.reconcileInstalledFromSystem(false)
}

func (s *Service) reconcileIfDue() {
	s.reconcileInstalledFromSystem(true)
}

func (s *Service) reconcileInstalledFromSystem(throttled bool) {
	if throttled {
		s.reconcileMu.Lock()
		if !s.lastReconcileAt.IsZero() && time.Since(s.lastReconcileAt) < reconcileMinInterval {
			s.reconcileMu.Unlock()
			return
		}
		s.lastReconcileAt = time.Now()
		s.reconcileMu.Unlock()
	}
	s.ensureCatalog()
	s.reconcileDockerInstallRecords()
	s.reconcileNodeInstallRecords()
	s.reconcilePHPInstallRecords()
	for _, item := range mergedCatalog() {
		key := item.Key
		s.ClearSimulatedIfRealPresent(key)
		app, err := s.Get(key)
		if err != nil {
			continue
		}
		if app.Installed && !IsSimulatedInstall(key, s.dataDir) {
			continue
		}
		if !systemPackagePresent(key, s.dataDir) {
			continue
		}
		status := s.detectAppStatus(key)
		version := app.Version
		if version == "" {
			version = item.Version
		}
		_ = s.db.Model(&app).Updates(map[string]interface{}{
			"installed":     true,
			"status":        status,
			"version":       version,
			"install_error": "",
		}).Error
		s.InvalidateLiveStatus(key)
	}
}

func (s *Service) reconcileNodeInstallRecords() {
	for _, item := range mergedCatalog() {
		key := item.Key
		if !strings.HasPrefix(key, "nodejs") {
			continue
		}
		app, err := s.Get(key)
		if err != nil || !app.Installed || IsSimulatedInstall(key, s.dataDir) {
			continue
		}
		if nodePackagePresent(key, s.dataDir) {
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

func (s *Service) reconcileDockerInstallRecords() {
	if !dockerEngineReady() {
		return
	}
	for key := range dockerAppSpecs {
		app, err := s.Get(key)
		if err != nil || !app.Installed || IsSimulatedInstall(key, s.dataDir) {
			continue
		}
		spec, ok := dockerSpec(key)
		if !ok {
			continue
		}
		if dockerContainerExists(spec.Container) {
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

// SystemPackagePresent reports whether key is installed on the host OS.
func SystemPackagePresent(key, dataDir string) bool {
	return systemPackagePresent(key, dataDir)
}

// ClearSimulatedIfRealPresent removes a stale simulated-install marker when the real package exists.
func (s *Service) ClearSimulatedIfRealPresent(key string) bool {
	if !IsSimulatedInstall(key, s.dataDir) {
		return false
	}
	if !systemPackagePresent(key, s.dataDir) {
		return false
	}
	marker := filepath.Join(s.dataDir, "server", key, ".owpanel-installed")
	_ = os.Remove(marker)
	s.InvalidateLiveStatus(key)
	app, err := s.Get(key)
	if err != nil {
		return true
	}
	status := s.detectAppStatus(key)
	_ = s.db.Model(app).Updates(map[string]interface{}{
		"installed":     true,
		"status":        status,
		"install_error": "",
	}).Error
	return true
}

func dotnetPackagePresent(key, dataDir string) bool {
	base := resolveDotnetBase(dataDir, key, filepath.Join("server", "dotnet", dotnetMajorFromKey(key)))
	if isSimulatedAt(base) {
		return false
	}
	return fileExists(filepath.Join(base, "root", "dotnet"))
}

func systemPackagePresent(key, dataDir string) bool {
	if IsSimulatedInstall(key, dataDir) {
		return false
	}
	if strings.HasPrefix(key, "php") && key != "phpmyadmin" {
		return phpPanelInstalled(key, dataDir)
	}
	if key == "pm2" {
		return detectPM2() == "running"
	}
	if key == "composer" {
		return detectComposer(dataDir) == "running"
	}
	if key == "certbot" {
		return detectCertbot() == "running"
	}
	if key == "phpmyadmin" {
		return fileExists(filepath.Join(dataDir, "server", "phpmyadmin", ".owpanel-installed"))
	}
	if strings.HasPrefix(key, "java") {
		return javaPackagePresent(key, dataDir)
	}
	if strings.HasPrefix(key, "nodejs") {
		return nodePackagePresent(key, dataDir)
	}
	if strings.HasPrefix(key, "dotnet") {
		return dotnetPackagePresent(key, dataDir)
	}
	if key == "docker" {
		return dockerEngineReady() || fileExists(filepath.Join(dataDir, "server", "docker", ".owpanel-installed"))
	}

	spec, ok := resolvePackageSpec(key)
	if !ok {
		return false
	}
	if runtime.GOOS == "linux" {
		if svc := serviceName(spec); svc != "" {
			out, err := exec.Command("systemctl", "status", svc).CombinedOutput()
			text := string(out)
			if err == nil || strings.Contains(text, "Loaded:") {
				return true
			}
		}
		if linuxPackageInstalled(spec) {
			return true
		}
	}
	if runtime.GOOS == "windows" && len(spec.WinPackages) > 0 {
		for _, pkg := range spec.WinPackages {
			out, err := exec.Command("winget", "list", "--id", pkg, "-e").CombinedOutput()
			if err == nil && strings.Contains(string(out), pkg) {
				return true
			}
		}
	}
	return false
}
