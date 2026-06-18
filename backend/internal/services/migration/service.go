package migration

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"gorm.io/gorm"
)

const bundleVersion = 1

type ExportOptions struct {
	IncludeLogs    bool `json:"include_logs"`
	IncludeSecrets bool `json:"include_secrets"`
}

type ImportOptions struct {
	Mode string `json:"mode"` // replace (default) or merge
}

type Manifest struct {
	Version        int               `json:"version"`
	ExportedAt     string            `json:"exported_at"`
	Hostname       string            `json:"hostname"`
	DataDir        string            `json:"data_dir"`
	IncludeLogs    bool              `json:"include_logs"`
	IncludeSecrets bool              `json:"include_secrets"`
	Counts         map[string]int64  `json:"counts"`
	IncludedPaths  []string          `json:"included_paths"`
	Settings       map[string]string `json:"settings,omitempty"`
	Warnings       []string          `json:"warnings,omitempty"`
}

type ExportResult struct {
	Filename string `json:"filename"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Manifest Manifest `json:"manifest"`
}

type ImportResult struct {
	Mode           string   `json:"mode"`
	RestoredPaths  []string `json:"restored_paths"`
	RequiresRestart bool    `json:"requires_restart"`
	Warnings       []string `json:"warnings,omitempty"`
}

type PreviewResult struct {
	Manifest Manifest `json:"manifest"`
}

type Service struct {
	db       *gorm.DB
	dataDir  string
	settings *settings.Service
}

func NewService(db *gorm.DB, dataDir string, settingsSvc *settings.Service) *Service {
	return &Service{db: db, dataDir: dataDir, settings: settingsSvc}
}

func (s *Service) exportDir() string {
	return filepath.Join(s.dataDir, "panel-migration")
}

func (s *Service) Preview() (*PreviewResult, error) {
	manifest, err := s.buildManifest(ExportOptions{IncludeSecrets: false})
	if err != nil {
		return nil, err
	}
	manifest.IncludedPaths = s.listPlannedPaths(ExportOptions{})
	return &PreviewResult{Manifest: *manifest}, nil
}

func (s *Service) Export(opts ExportOptions) (*ExportResult, error) {
	if err := os.MkdirAll(s.exportDir(), 0755); err != nil {
		return nil, err
	}
	manifest, err := s.buildManifest(opts)
	if err != nil {
		return nil, err
	}
	planned := s.listPlannedPaths(opts)
	manifest.IncludedPaths = planned

	staging := filepath.Join(s.exportDir(), fmt.Sprintf("staging-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(staging, 0755); err != nil {
		return nil, err
	}
	defer os.RemoveAll(staging)

	if err := s.checkpointAndCopyDB(staging); err != nil {
		return nil, fmt.Errorf("copy panel database: %w", err)
	}
	if opts.IncludeSecrets {
		for _, name := range []string{".jwt_secret", ".edge_worker_secret"} {
			src := filepath.Join(s.dataDir, name)
			if st, err := os.Stat(src); err == nil && !st.IsDir() {
				if err := copyFile(src, filepath.Join(staging, name)); err != nil {
					manifest.Warnings = append(manifest.Warnings, fmt.Sprintf("skip %s: %v", name, err))
				}
			}
		}
	}
	for _, rel := range planned {
		src := filepath.Join(s.dataDir, rel)
		dst := filepath.Join(staging, "data", rel)
		if err := copyPathRecursive(src, dst); err != nil {
			manifest.Warnings = append(manifest.Warnings, fmt.Sprintf("skip %s: %v", rel, err))
		}
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(staging, "manifest.json"), manifestData, 0644); err != nil {
		return nil, err
	}

	ts := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("owpanel-migration-%s.tar.gz", ts)
	dest := filepath.Join(s.exportDir(), filename)
	if err := createTarGz(staging, dest); err != nil {
		return nil, err
	}
	st, err := os.Stat(dest)
	if err != nil {
		return nil, err
	}
	return &ExportResult{
		Filename: filename,
		Path:     dest,
		Size:     st.Size(),
		Manifest: *manifest,
	}, nil
}

func (s *Service) ResolveExportPath(filename string) (string, error) {
	name := filepath.Base(strings.TrimSpace(filename))
	if name == "" || name == "." || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid filename")
	}
	path := filepath.Join(s.exportDir(), name)
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		return "", fmt.Errorf("export bundle not found")
	}
	return path, nil
}

func (s *Service) ImportBundle(bundlePath string, opts ImportOptions) (*ImportResult, error) {
	mode := strings.ToLower(strings.TrimSpace(opts.Mode))
	if mode == "" {
		mode = "replace"
	}
	if mode != "replace" && mode != "merge" {
		return nil, fmt.Errorf("unsupported import mode: %s", mode)
	}

	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("owpanel-import-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(tmp, 0755); err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if err := extractTarGz(bundlePath, tmp); err != nil {
		return nil, fmt.Errorf("extract bundle: %w", err)
	}

	manifestPath := filepath.Join(tmp, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("missing manifest.json")
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}
	if manifest.Version != bundleVersion {
		return nil, fmt.Errorf("unsupported bundle version %d", manifest.Version)
	}

	res := &ImportResult{Mode: mode, RequiresRestart: true}
	if mode == "replace" {
		if err := s.backupCurrentState(); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("pre-import backup: %v", err))
		}
	}
	if err := s.restoreDBFromBundle(tmp, mode, res); err != nil {
		return nil, err
	}
	if err := s.restoreSecretsFromBundle(tmp, mode, res); err != nil {
		res.Warnings = append(res.Warnings, err.Error())
	}
	if err := s.restoreDataFromBundle(tmp, mode, res); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *Service) buildManifest(opts ExportOptions) (*Manifest, error) {
	host, _ := os.Hostname()
	all, _ := s.settings.GetAll()
	publicSettings := map[string]string{}
	for k, v := range all {
		switch k {
		case "ai_api_key", "hf_token", "cluster_agent_token":
			if v != "" {
				publicSettings[k] = "[redacted]"
			}
		default:
			publicSettings[k] = v
		}
	}
	return &Manifest{
		Version:        bundleVersion,
		ExportedAt:     time.Now().UTC().Format(time.RFC3339),
		Hostname:       host,
		DataDir:        s.dataDir,
		IncludeLogs:    opts.IncludeLogs,
		IncludeSecrets: opts.IncludeSecrets,
		Counts:         s.countEntities(),
		Settings:       publicSettings,
	}, nil
}

func (s *Service) countEntities() map[string]int64 {
	modelsToCount := []struct {
		key   string
		model any
	}{
		{"users", &models.User{}},
		{"websites", &models.Website{}},
		{"databases", &models.DatabaseInstance{}},
		{"ssl_certificates", &models.SSLCertificate{}},
		{"apps", &models.App{}},
		{"ftp_accounts", &models.FTPAccount{}},
		{"cron_jobs", &models.CronJob{}},
		{"mail_domains", &models.MailDomain{}},
		{"mailboxes", &models.MailBox{}},
		{"wordpress_sites", &models.WordPressSite{}},
		{"docker_bindings", &models.DockerContainerBinding{}},
		{"extensions", &models.PanelSetting{}}, // placeholder replaced below
	}
	out := make(map[string]int64, len(modelsToCount))
	for _, item := range modelsToCount {
		if item.key == "extensions" {
			continue
		}
		var n int64
		_ = s.db.Model(item.model).Count(&n).Error
		out[item.key] = n
	}
	extDir := filepath.Join(s.dataDir, "extensions")
	if entries, err := os.ReadDir(extDir); err == nil {
		var n int64
		for _, e := range entries {
			if e.IsDir() {
				n++
			}
		}
		out["extensions"] = n
	}
	return out
}

func (s *Service) defaultDataSubdirs(includeLogs bool) []string {
	dirs := []string{
		"wwwroot", "backup", "server", "nginx", "apache", "ssl", "security",
		"extensions", "apps", "ai", "mail", "ftp", "docker", "geoip", "waf",
	}
	if includeLogs {
		dirs = append(dirs, "logs")
	}
	return dirs
}

func (s *Service) listPlannedPaths(opts ExportOptions) []string {
	seen := map[string]bool{}
	var out []string
	add := func(rel string) {
		rel = filepath.ToSlash(strings.Trim(rel, "/\\"))
		if rel == "" || seen[rel] {
			return
		}
		seen[rel] = true
		out = append(out, rel)
	}
	for _, d := range s.defaultDataSubdirs(opts.IncludeLogs) {
		add(d)
	}
	all, _ := s.settings.GetAll()
	for _, key := range []string{"website_path", "backup_path"} {
		p := settings.ResolvePanelPath(s.dataDir, all[key])
		if p == "" {
			continue
		}
		if rel, err := filepath.Rel(s.dataDir, p); err == nil && !strings.HasPrefix(rel, "..") {
			add(rel)
		}
	}
	return out
}

func (s *Service) checkpointAndCopyDB(destDir string) error {
	_ = s.db.Exec("PRAGMA wal_checkpoint(FULL)").Error
	dbPath := filepath.Join(s.dataDir, "panel.db")
	if err := copyFile(dbPath, filepath.Join(destDir, "panel.db")); err != nil {
		return err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		src := dbPath + suffix
		if st, err := os.Stat(src); err == nil && !st.IsDir() {
			_ = copyFile(src, filepath.Join(destDir, "panel.db"+suffix))
		}
	}
	return nil
}

func (s *Service) backupCurrentState() error {
	ts := time.Now().Format("20060102-150405")
	backupDir := filepath.Join(s.exportDir(), "pre-import-"+ts)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}
	_ = s.db.Exec("PRAGMA wal_checkpoint(FULL)").Error
	dbPath := filepath.Join(s.dataDir, "panel.db")
	_ = copyFile(dbPath, filepath.Join(backupDir, "panel.db"))
	for _, rel := range s.listPlannedPaths(ExportOptions{IncludeLogs: true}) {
		src := filepath.Join(s.dataDir, rel)
		dst := filepath.Join(backupDir, "data", rel)
		_ = copyPathRecursive(src, dst)
	}
	return nil
}

func (s *Service) restoreDBFromBundle(tmp, mode string, res *ImportResult) error {
	src := filepath.Join(tmp, "panel.db")
	if st, err := os.Stat(src); err != nil || st.IsDir() {
		return fmt.Errorf("bundle missing panel.db")
	}
	if mode == "merge" {
		res.Warnings = append(res.Warnings, "merge mode keeps existing panel.db; only data directories are merged")
		return nil
	}
	dest := filepath.Join(s.dataDir, "panel.db")
	if err := copyFile(src, dest); err != nil {
		return fmt.Errorf("restore panel.db: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		bundleFile := filepath.Join(tmp, "panel.db"+suffix)
		destFile := dest + suffix
		if st, err := os.Stat(bundleFile); err == nil && !st.IsDir() {
			_ = copyFile(bundleFile, destFile)
		} else {
			_ = os.Remove(destFile)
		}
	}
	res.RestoredPaths = append(res.RestoredPaths, "panel.db")
	return nil
}

func (s *Service) restoreSecretsFromBundle(tmp, mode string, res *ImportResult) error {
	if mode == "merge" {
		return nil
	}
	for _, name := range []string{".jwt_secret", ".edge_worker_secret"} {
		src := filepath.Join(tmp, name)
		if st, err := os.Stat(src); err != nil || st.IsDir() {
			continue
		}
		dest := filepath.Join(s.dataDir, name)
		if err := copyFile(src, dest); err != nil {
			return fmt.Errorf("restore %s: %w", name, err)
		}
		res.RestoredPaths = append(res.RestoredPaths, name)
	}
	return nil
}

func (s *Service) restoreDataFromBundle(tmp, mode string, res *ImportResult) error {
	srcRoot := filepath.Join(tmp, "data")
	if st, err := os.Stat(srcRoot); err != nil || !st.IsDir() {
		res.Warnings = append(res.Warnings, "bundle has no data/ directory")
		return nil
	}
	return filepath.Walk(srcRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		dest := filepath.Join(s.dataDir, rel)
		if info.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		if mode == "merge" {
			if _, err := os.Stat(dest); err == nil {
				return nil
			}
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		if err := copyFile(path, dest); err != nil {
			return err
		}
		res.RestoredPaths = append(res.RestoredPaths, rel)
		return nil
	})
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func copyPathRecursive(src, dest string) error {
	st, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !st.IsDir() {
		return copyFile(src, dest)
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
}
