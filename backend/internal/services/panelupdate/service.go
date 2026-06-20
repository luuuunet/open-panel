package panelupdate

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/scheduleutil"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"github.com/luuuunet/owpanel/internal/version"
	"gorm.io/gorm"
)

const defaultRepo = "luuuunet/owpanel"

type Service struct {
	db       *gorm.DB
	dataDir  string
	settings *settings.Service
	client   *http.Client
}

type CheckResult struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	ReleaseName     string `json:"release_name,omitempty"`
	ReleaseNotes    string `json:"release_notes,omitempty"`
	ReleaseURL      string `json:"release_url,omitempty"`
	PublishedAt     string `json:"published_at,omitempty"`
	CanApply        bool   `json:"can_apply"`
	ApplyReason     string `json:"apply_reason,omitempty"`
}

type Config struct {
	Enabled    bool   `json:"enabled"`
	Schedule   string `json:"schedule"`
	AutoApply  bool   `json:"auto_apply"`
	Repo       string `json:"repo"`
}

type Status struct {
	Version     map[string]string      `json:"version"`
	Check       *CheckResult           `json:"check,omitempty"`
	Config      Config                 `json:"config"`
	LastCheckAt *time.Time             `json:"last_check_at,omitempty"`
	History     []models.PanelUpdateRecord `json:"history"`
}

func NewService(db *gorm.DB, dataDir string, settingsSvc *settings.Service) *Service {
	return &Service{
		db:       db,
		dataDir:  dataDir,
		settings: settingsSvc,
		client: &http.Client{
			Timeout: 15 * time.Minute,
		},
	}
}

func (s *Service) Status(ctx context.Context) (*Status, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	st := &Status{
		Version: version.Info(),
		Config:  cfg,
	}
	if t, err := s.parseSettingTime("panel_update_last_check"); err == nil {
		st.LastCheckAt = t
	}
	var history []models.PanelUpdateRecord
	_ = s.db.Order("id desc").Limit(10).Find(&history).Error
	st.History = history
	if st.LastCheckAt != nil && time.Since(*st.LastCheckAt) < 15*time.Minute {
		if cached, err := s.cachedCheck(); err == nil {
			st.Check = cached
			return st, nil
		}
	}
	check, err := s.Check(ctx)
	if err == nil {
		st.Check = check
	}
	return st, nil
}

func (s *Service) Check(ctx context.Context) (*CheckResult, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	release, err := s.fetchLatestRelease(ctx, cfg.Repo)
	if err != nil {
		return nil, err
	}
	current := normalizeTag(version.Version)
	latest := normalizeTag(release.TagName)
	result := &CheckResult{
		CurrentVersion:  current,
		LatestVersion:   latest,
		UpdateAvailable: CompareVersions(current, latest) < 0,
		ReleaseName:     release.Name,
		ReleaseNotes:    release.Body,
		ReleaseURL:      release.HTMLURL,
		PublishedAt:     release.PublishedAt,
	}
	if reason := s.applyBlockReason(); reason != "" {
		result.CanApply = false
		result.ApplyReason = reason
	} else {
		result.CanApply = true
	}
	_ = s.settings.Update(map[string]string{
		"panel_update_last_check":    time.Now().UTC().Format(time.RFC3339),
		"panel_update_cached_latest": latest,
		"panel_update_cached_notes":  truncate(release.Body, 4000),
		"panel_update_cached_url":    release.HTMLURL,
	})
	return result, nil
}

func (s *Service) SaveConfig(cfg Config) error {
	if strings.TrimSpace(cfg.Schedule) == "" {
		cfg.Schedule = "0 4 * * 0"
	}
	if strings.TrimSpace(cfg.Repo) == "" {
		cfg.Repo = defaultRepo
	}
	return s.settings.Update(map[string]string{
		"panel_auto_update_enabled":  boolStr(cfg.Enabled),
		"panel_auto_update_schedule": strings.TrimSpace(cfg.Schedule),
		"panel_auto_update_auto_apply": boolStr(cfg.AutoApply),
		"panel_update_repo":          strings.TrimSpace(cfg.Repo),
	})
}

func (s *Service) Apply(ctx context.Context, targetVersion, trigger string) (*models.PanelUpdateRecord, error) {
	if trigger == "" {
		trigger = "manual"
	}
	if reason := s.applyBlockReason(); reason != "" {
		return nil, fmt.Errorf(reason)
	}
	check, err := s.Check(ctx)
	if err != nil {
		return nil, err
	}
	versionTag := normalizeTag(check.LatestVersion)
	if targetVersion != "" {
		versionTag = normalizeTag(targetVersion)
	}
	if CompareVersions(normalizeTag(version.Version), versionTag) >= 0 {
		return nil, fmt.Errorf("already on version %s", normalizeTag(version.Version))
	}

	record := &models.PanelUpdateRecord{
		FromVersion: normalizeTag(version.Version),
		ToVersion:   versionTag,
		Status:      "downloading",
		Trigger:     trigger,
	}
	if err := s.db.Create(record).Error; err != nil {
		return nil, err
	}

	stagingDir, cleanup, err := s.downloadAndExtract(ctx, versionTag)
	if err != nil {
		record.Status = "failed"
		record.ErrorMsg = truncate(err.Error(), 500)
		_ = s.db.Save(record).Error
		cleanup()
		return record, err
	}

	installDir := resolveInstallDir(s.dataDir)
	serviceName := resolveServiceName()
	scriptPath, err := s.writeApplyScript(installDir, stagingDir, serviceName, record.ID)
	if err != nil {
		record.Status = "failed"
		record.ErrorMsg = truncate(err.Error(), 500)
		_ = s.db.Save(record).Error
		cleanup()
		return record, err
	}

	if err := spawnDetached(scriptPath); err != nil {
		record.Status = "failed"
		record.ErrorMsg = truncate(err.Error(), 500)
		_ = s.db.Save(record).Error
		cleanup()
		return record, err
	}

	record.Status = "applying"
	_ = s.db.Save(record).Error
	_ = s.settings.Update(map[string]string{
		"panel_update_last_run": time.Now().UTC().Format(time.RFC3339),
	})
	return record, nil
}

func (s *Service) RunAutoUpdateIfDue() {
	cfg, err := s.loadConfig()
	if err != nil || !cfg.Enabled {
		return
	}
	lastRun, _ := s.parseSettingTime("panel_update_last_run")
	if !scheduleutil.DueNow(cfg.Schedule, lastRun, time.Now()) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	check, err := s.Check(ctx)
	if err != nil {
		return
	}
	_ = s.settings.Update(map[string]string{
		"panel_update_last_run": time.Now().UTC().Format(time.RFC3339),
	})
	if !check.UpdateAvailable {
		return
	}
	if !cfg.AutoApply {
		return
	}
	_, _ = s.Apply(ctx, "", "auto")
}

func (s *Service) loadConfig() (Config, error) {
	all, err := s.settings.GetAll()
	if err != nil {
		return Config{}, err
	}
	repo := strings.TrimSpace(all["panel_update_repo"])
	if repo == "" {
		repo = defaultRepo
	}
	schedule := strings.TrimSpace(all["panel_auto_update_schedule"])
	if schedule == "" {
		schedule = "0 4 * * 0"
	}
	return Config{
		Enabled:   all["panel_auto_update_enabled"] == "true",
		Schedule:  schedule,
		AutoApply: all["panel_auto_update_auto_apply"] == "true",
		Repo:      repo,
	}, nil
}

func (s *Service) cachedCheck() (*CheckResult, error) {
	all, err := s.settings.GetAll()
	if err != nil {
		return nil, err
	}
	latest := strings.TrimSpace(all["panel_update_cached_latest"])
	if latest == "" {
		return nil, fmt.Errorf("no cache")
	}
	current := normalizeTag(version.Version)
	result := &CheckResult{
		CurrentVersion:  current,
		LatestVersion:   latest,
		UpdateAvailable: CompareVersions(current, latest) < 0,
		ReleaseNotes:    all["panel_update_cached_notes"],
		ReleaseURL:      all["panel_update_cached_url"],
	}
	if reason := s.applyBlockReason(); reason != "" {
		result.CanApply = false
		result.ApplyReason = reason
	} else {
		result.CanApply = true
	}
	return result, nil
}

func (s *Service) parseSettingTime(key string) (*time.Time, error) {
	all, err := s.settings.GetAll()
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(all[key])
	if raw == "" {
		return nil, fmt.Errorf("empty")
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Service) applyBlockReason() string {
	if runtime.GOOS != "linux" {
		return "panel self-update is only supported on Linux"
	}
	if _, err := releasePackageName(); err != nil {
		return err.Error()
	}
	installDir := resolveInstallDir(s.dataDir)
	if st, err := os.Stat(filepath.Join(installDir, "owpanel")); err != nil || st.IsDir() {
		return "install directory not found (expected owpanel binary beside data dir)"
	}
	return ""
}

type ghRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

func (s *Service) fetchLatestRelease(ctx context.Context, repo string) (*ghRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", strings.Trim(repo, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "OWPanel/"+version.Version)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("github API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var release ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	if strings.TrimSpace(release.TagName) == "" {
		return nil, fmt.Errorf("release has no tag")
	}
	return &release, nil
}

func (s *Service) downloadAndExtract(ctx context.Context, versionTag string) (stagingDir string, cleanup func(), err error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return "", func() {}, err
	}
	pkg, err := releasePackageName()
	if err != nil {
		return "", func() {}, err
	}
	updateDir := filepath.Join(s.dataDir, "update")
	if err := os.MkdirAll(updateDir, 0755); err != nil {
		return "", func() {}, err
	}
	tgzPath := filepath.Join(updateDir, pkg+"-"+versionTag+".tar.gz")
	extractDir := filepath.Join(updateDir, "extract-"+versionTag)
	cleanup = func() {
		_ = os.Remove(tgzPath)
		_ = os.RemoveAll(extractDir)
	}
	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s.tar.gz", cfg.Repo, versionTag, pkg)
	if err := s.downloadFile(ctx, url, tgzPath); err != nil {
		return "", cleanup, err
	}
	if err := os.RemoveAll(extractDir); err != nil {
		return "", cleanup, err
	}
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return "", cleanup, err
	}
	if err := extractTarGz(tgzPath, extractDir); err != nil {
		return "", cleanup, err
	}
	root := filepath.Join(extractDir, pkg)
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		root = extractDir
	}
	if st, err := os.Stat(filepath.Join(root, "owpanel")); err != nil || st.IsDir() {
		return "", cleanup, fmt.Errorf("release package missing owpanel binary")
	}
	if st, err := os.Stat(filepath.Join(root, "web", "index.html")); err != nil || st.IsDir() {
		return "", cleanup, fmt.Errorf("release package missing web/index.html")
	}
	return root, cleanup, nil
}

func (s *Service) downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "OWPanel/"+version.Version)
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("download HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	tmp := dest + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

func (s *Service) writeApplyScript(installDir, stagingDir, serviceName string, recordID uint) (string, error) {
	scriptPath := filepath.Join(s.dataDir, "update", fmt.Sprintf("apply-%d.sh", recordID))
	logPath := filepath.Join(s.dataDir, "update", fmt.Sprintf("apply-%d.log", recordID))
	content := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
LOG=%q
exec >>"$LOG" 2>&1
echo "[owpanel-update] starting apply record %d at $(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)"
sleep 3
if command -v systemctl >/dev/null 2>&1; then
  systemctl stop %q || true
fi
install -m 0755 %q/owpanel %q/owpanel
if [[ -f %q/op ]]; then
  install -m 0755 %q/op %q/op
fi
rm -rf %q/web
cp -a %q/web %q/web
if command -v systemctl >/dev/null 2>&1; then
  systemctl start %q || systemctl restart %q
fi
echo "[owpanel-update] done at $(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)"
rm -f %q
`, logPath, recordID, serviceName,
		stagingDir, installDir,
		stagingDir, stagingDir, installDir,
		installDir, stagingDir, installDir,
		serviceName, serviceName,
		scriptPath)
	if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		return "", err
	}
	return scriptPath, nil
}

func spawnDetached(scriptPath string) error {
	cmd := exec.Command("nohup", "bash", scriptPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start update script: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func extractTarGz(src, dest string) error {
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
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, hdr.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)+string(os.PathSeparator)) && filepath.Clean(target) != filepath.Clean(dest) {
			return fmt.Errorf("invalid tar path: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
}

func resolveInstallDir(dataDir string) string {
	for _, key := range []string{"OWPANEL_HOME", "OPEN_PANEL_HOME"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if st, err := os.Stat(filepath.Join(dir, "owpanel")); err == nil && !st.IsDir() {
			return dir
		}
	}
	return filepath.Dir(dataDir)
}

func resolveServiceName() string {
	for _, key := range []string{"OWPANEL_SERVICE", "OPEN_PANEL_SERVICE"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return "owpanel"
}

func releasePackageName() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return "owpanel-linux-amd64", nil
	case "arm64":
		return "owpanel-linux-arm64", nil
	default:
		return "", fmt.Errorf("unsupported CPU architecture: %s", runtime.GOARCH)
	}
}

func normalizeTag(v string) string {
	v = strings.TrimSpace(v)
	return strings.TrimPrefix(strings.ToLower(v), "v")
}

func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
