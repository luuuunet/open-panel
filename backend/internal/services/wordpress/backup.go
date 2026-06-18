package wordpress

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	dbsvc "github.com/luuuunet/owpanel/internal/services/database"
)

type BackupManifest struct {
	Domain      string    `json:"domain"`
	RootPath    string    `json:"root_path"`
	WPVersion   string    `json:"wp_version"`
	PhpVersion  string    `json:"php_version"`
	DbName      string    `json:"db_name,omitempty"`
	HasDatabase bool      `json:"has_database"`
	CreatedAt   time.Time `json:"created_at"`
	Panel       string    `json:"panel"`
}

type WPBackupConfig struct {
	BackupDir string `json:"backup_dir"`
}

func (s *Service) BackupConfig() WPBackupConfig {
	return WPBackupConfig{BackupDir: s.backupDir()}
}

func (s *Service) backupDir() string {
	return filepath.Join(s.dataDir, "backup", "wordpress")
}

func (s *Service) ListBackups(siteID uint) ([]models.WordPressBackup, error) {
	var list []models.WordPressBackup
	err := s.db.Where("site_id = ?", siteID).Order("id desc").Find(&list).Error
	return list, err
}

func (s *Service) RunBackup(siteID uint) (*models.WordPressBackup, error) {
	site, err := s.Get(siteID)
	if err != nil {
		return nil, err
	}
	if site.RootPath == "" {
		return nil, fmt.Errorf("站点根目录为空")
	}
	if _, err := os.Stat(site.RootPath); err != nil {
		return nil, fmt.Errorf("站点目录不存在: %s", site.RootPath)
	}

	dir := filepath.Join(s.backupDir(), sanitizeBackupName(site.Domain))
	_ = os.MkdirAll(dir, 0755)
	ts := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%s-wp-full.zip", sanitizeBackupName(site.Domain), ts)
	dest := filepath.Join(dir, filename)

	rec := &models.WordPressBackup{
		SiteID: site.ID,
		Domain: site.Domain,
		FilePath: dest,
		Status:   "running",
	}
	if err := s.db.Create(rec).Error; err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "owpanel-wp-backup-*")
	if err != nil {
		rec.Status = "failed"
		rec.ErrorMsg = err.Error()
		_ = s.db.Save(rec).Error
		return rec, err
	}
	defer os.RemoveAll(tmpDir)

	sqlPath := filepath.Join(tmpDir, "database.sql")
	hasDB := false
	dbName := ""
	dbErr := ""
	if conn, err := s.resolveDBCreds(site); err == nil {
		dbName = conn.DBName
		if err := dbsvc.DumpMySQLConn(*conn, sqlPath); err != nil {
			dbErr = err.Error()
		} else {
			hasDB = true
			rec.HasDatabase = true
			rec.DbName = dbName
		}
	} else {
		dbErr = err.Error()
	}

	manifest := BackupManifest{
		Domain:      site.Domain,
		RootPath:    site.RootPath,
		WPVersion:   site.Version,
		PhpVersion:  site.PhpVersion,
		DbName:      dbName,
		HasDatabase: hasDB,
		CreatedAt:   time.Now(),
		Panel:       "OWPanel WordPress Full Backup",
	}
	if dbErr != "" && !hasDB {
		manifest.DbName = "missing: " + dbErr
	}

	if err := createFullBackupZip(site.RootPath, sqlPath, hasDB, manifest, dest); err != nil {
		rec.Status = "failed"
		rec.ErrorMsg = err.Error()
		_ = os.Remove(dest)
		_ = s.db.Save(rec).Error
		return rec, err
	}

	st, err := os.Stat(dest)
	if err != nil {
		rec.Status = "failed"
		rec.ErrorMsg = err.Error()
		_ = s.db.Save(rec).Error
		return rec, err
	}
	rec.Size = st.Size()
	rec.Status = "done"
	rec.HasDatabase = hasDB
	rec.DbName = dbName
	_ = s.db.Save(rec).Error
	s.refreshBackupStatus(site)
	return rec, nil
}

func (s *Service) DeleteBackup(siteID, backupID uint) error {
	var rec models.WordPressBackup
	if err := s.db.Where("id = ? AND site_id = ?", backupID, siteID).First(&rec).Error; err != nil {
		return err
	}
	if rec.FilePath != "" {
		_ = os.Remove(rec.FilePath)
	}
	if err := s.db.Delete(&rec).Error; err != nil {
		return err
	}
	if site, err := s.Get(siteID); err == nil {
		s.refreshBackupStatus(site)
	}
	return nil
}

func (s *Service) GetBackupFile(siteID, backupID uint) (string, error) {
	var rec models.WordPressBackup
	if err := s.db.Where("id = ? AND site_id = ?", backupID, siteID).First(&rec).Error; err != nil {
		return "", err
	}
	if _, err := os.Stat(rec.FilePath); err != nil {
		return "", fmt.Errorf("备份文件已丢失")
	}
	return rec.FilePath, nil
}

func (s *Service) refreshBackupStatus(site *models.WordPressSite) {
	var count int64
	s.db.Model(&models.WordPressBackup{}).Where("site_id = ? AND status = ?", site.ID, "done").Count(&count)
	status := "none"
	if count > 0 {
		status = fmt.Sprintf("%d份", count)
	}
	_ = s.db.Model(site).Update("backup_status", status).Error
}

func (s *Service) resolveDBCreds(site *models.WordPressSite) (*dbsvc.ConnInfo, error) {
	cfgPath := filepath.Join(site.RootPath, "wp-config.php")
	if conn, err := parseWPConfig(cfgPath); err == nil && conn.DBName != "" {
		return conn, nil
	}
	if site.WebsiteID > 0 {
		var w models.Website
		if s.db.First(&w, site.WebsiteID).Error == nil && w.DbName != "" {
			var inst models.DatabaseInstance
			if s.db.Where("name = ?", w.DbName).First(&inst).Error == nil {
				if inst.Password == "" {
					return nil, fmt.Errorf("数据库 %s 未配置密码，请在数据库页面设置凭据", w.DbName)
				}
				host, port := splitDBHost(inst.Host, inst.Port)
				return &dbsvc.ConnInfo{
					Host:     host,
					Port:     port,
					Username: inst.Username,
					Password: inst.Password,
					DBName:   w.DbName,
				}, nil
			}
		}
	}
	return nil, fmt.Errorf("未找到 wp-config.php 或关联数据库，无法导出数据库")
}

func parseWPConfig(path string) (*dbsvc.ConnInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(data)
	get := func(key string) string {
		re := regexp.MustCompile(`define\s*\(\s*['"]` + key + `['"]\s*,\s*['"]([^'"]*)['"]\s*\)`)
		m := re.FindStringSubmatch(content)
		if len(m) >= 2 {
			return m[1]
		}
		return ""
	}
	name := get("DB_NAME")
	user := get("DB_USER")
	pass := get("DB_PASSWORD")
	hostRaw := get("DB_HOST")
	if name == "" || user == "" {
		return nil, fmt.Errorf("wp-config.php 缺少 DB_NAME 或 DB_USER")
	}
	host, port := splitDBHost(hostRaw, 3306)
	return &dbsvc.ConnInfo{
		Host: host, Port: port, Username: user, Password: pass, DBName: name,
	}, nil
}

func splitDBHost(host string, defaultPort int) (string, int) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "127.0.0.1", defaultPort
	}
	if strings.Contains(host, ":") {
		parts := strings.SplitN(host, ":", 2)
		port, err := strconv.Atoi(parts[1])
		if err != nil || port == 0 {
			port = defaultPort
		}
		return parts[0], port
	}
	return host, defaultPort
}

func createFullBackupZip(rootDir, sqlPath string, includeSQL bool, manifest BackupManifest, destZip string) error {
	if err := os.MkdirAll(filepath.Dir(destZip), 0755); err != nil {
		return err
	}
	f, err := os.Create(destZip)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	if err := addZipBytes(zw, "owpanel-backup.json", manifestBytes); err != nil {
		return err
	}

	if includeSQL {
		if err := addZipFile(zw, "database.sql", sqlPath); err != nil {
			return err
		}
	}

	rootDir = filepath.Clean(rootDir)
	err = filepath.Walk(rootDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if shouldSkipBackupDir(path, rootDir) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldSkipBackupFile(path, rootDir) {
			return nil
		}
		rel, err := filepath.Rel(rootDir, path)
		if err != nil {
			return err
		}
		return addZipFile(zw, filepath.ToSlash(rel), path)
	})
	if err != nil {
		_ = os.Remove(destZip)
		return err
	}
	return zw.Close()
}

func shouldSkipBackupDir(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(strings.ToLower(rel))
	skip := []string{
		"wp-content/cache", "wp-content/upgrade", "wp-content/backups",
		".git", "node_modules",
	}
	for _, s := range skip {
		if rel == s || strings.HasPrefix(rel, s+"/") {
			return true
		}
	}
	return false
}

func shouldSkipBackupFile(path, root string) bool {
	base := strings.ToLower(filepath.Base(path))
	if base == ".ds_store" || base == "error_log" || base == "debug.log" {
		return true
	}
	return false
}

func addZipFile(zw *zip.Writer, name, path string) error {
	rf, err := os.Open(path)
	if err != nil {
		return err
	}
	defer rf.Close()
	st, err := rf.Stat()
	if err != nil {
		return err
	}
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	if st.Size() > 0 {
		_, err = io.Copy(w, rf)
	}
	return err
}

func addZipBytes(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func sanitizeBackupName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return "wordpress"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), ".-")
	if out == "" {
		return "wordpress"
	}
	return out
}
