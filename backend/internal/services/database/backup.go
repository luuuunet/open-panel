package database

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/settings"
)

type BackupConfig struct {
	BackupDir string `json:"backup_dir"`
}

func (s *Service) BackupConfig() BackupConfig {
	return BackupConfig{BackupDir: s.dbBackupDir()}
}

func (s *Service) ListBackups(databaseID uint) ([]models.DatabaseBackup, error) {
	var list []models.DatabaseBackup
	err := s.db.Where("database_id = ?", databaseID).Order("id desc").Find(&list).Error
	return list, err
}

func (s *Service) RunBackup(databaseID uint, opts ...BackupOptions) (*models.DatabaseBackup, error) {
	var opt BackupOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	inst, err := s.Get(databaseID)
	if err != nil {
		return nil, err
	}
	if inst.Password == "" {
		return nil, fmt.Errorf("请先配置数据库连接密码")
	}

	dir := filepath.Join(s.dbBackupDir(), sanitizeFileName(inst.Name))
	_ = os.MkdirAll(dir, 0755)
	ts := time.Now().Format("20060102-150405")
	ext := backupExt(inst.Type)
	filename := fmt.Sprintf("%s-%s%s", sanitizeFileName(inst.Name), ts, ext)
	dest := filepath.Join(dir, filename)

	rec := &models.DatabaseBackup{
		DatabaseID: inst.ID,
		DbName:     inst.Name,
		DbType:     inst.Type,
		FilePath:   dest,
		Status:     "running",
	}
	if err := s.db.Create(rec).Error; err != nil {
		return nil, err
	}

	var runErr error
	switch strings.ToLower(inst.Type) {
	case "mysql", "mariadb":
		runErr = dumpMySQL(inst, dest)
	case "postgresql", "postgres":
		runErr = dumpPostgreSQL(inst, dest)
	case "redis":
		runErr = dumpRedis(inst, dest)
	default:
		runErr = fmt.Errorf("暂不支持 %s 类型备份", inst.Type)
	}

	if runErr != nil {
		rec.Status = "failed"
		rec.ErrorMsg = runErr.Error()
		_ = os.Remove(dest)
		_ = s.db.Save(rec).Error
		return rec, runErr
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
	useOSS := opt.OSSStorageID
	if useOSS == nil {
		useOSS = inst.BackupOSSStorageID
	}
	rec.OSSStorageID = useOSS
	if useOSS != nil && *useOSS > 0 {
		if key, err := s.uploadBackupOSS(*useOSS, dest); err != nil {
			rec.RemoteStatus = "failed"
			rec.RemoteError = "oss: " + err.Error()
		} else {
			rec.RemoteKey = key
			if rec.RemoteStatus == "" || rec.RemoteStatus == "none" {
				rec.RemoteStatus = "synced"
			}
		}
	}
	useRemote := opt.RemoteID
	if useRemote == nil {
		useRemote = inst.BackupRemoteID
	}
	rec.RemoteID = useRemote
	if useRemote != nil && *useRemote > 0 && s.remote != nil {
		if err := s.remote.UploadToRemote(*useRemote, dest, filename); err != nil {
			if rec.RemoteStatus == "synced" {
				rec.RemoteError = strings.TrimSpace(rec.RemoteError + "; remote: " + err.Error())
			} else {
				rec.RemoteStatus = "failed"
				rec.RemoteError = err.Error()
			}
		} else {
			rec.RemoteStatus = "synced"
		}
	}
	_ = s.db.Save(rec).Error
	s.pruneBackups(inst.ID, inst.BackupKeepCount)
	s.refreshBackupStatus(inst)
	return rec, nil
}

func (s *Service) pruneBackups(databaseID uint, keep int) {
	if keep <= 0 {
		keep = 5
	}
	var list []models.DatabaseBackup
	s.db.Where("database_id = ? AND status = ?", databaseID, "done").Order("id desc").Find(&list)
	if len(list) <= keep {
		return
	}
	for _, old := range list[keep:] {
		_ = s.DeleteBackup(databaseID, old.ID)
	}
}

// PruneBackups removes old backup files beyond keep count.
func (s *Service) PruneBackups(databaseID uint, keep int) {
	s.pruneBackups(databaseID, keep)
}

func (s *Service) ImportSQL(databaseID uint, sqlPath string) error {
	inst, err := s.Get(databaseID)
	if err != nil {
		return err
	}
	if inst.Password == "" {
		return fmt.Errorf("请先配置数据库连接密码")
	}
	st, err := os.Stat(sqlPath)
	if err != nil {
		return fmt.Errorf("SQL 文件不存在")
	}
	if st.Size() == 0 {
		return fmt.Errorf("SQL 文件为空")
	}

	switch strings.ToLower(inst.Type) {
	case "mysql", "mariadb":
		return importMySQL(inst, sqlPath)
	case "postgresql", "postgres":
		return importPostgreSQL(inst, sqlPath)
	default:
		return fmt.Errorf("暂不支持 %s 类型导入", inst.Type)
	}
}

func (s *Service) DeleteBackup(databaseID, backupID uint) error {
	var rec models.DatabaseBackup
	if err := s.db.Where("id = ? AND database_id = ?", backupID, databaseID).First(&rec).Error; err != nil {
		return err
	}
	if rec.FilePath != "" {
		_ = os.Remove(rec.FilePath)
	}
	if rec.RemoteKey != "" && rec.OSSStorageID != nil && *rec.OSSStorageID > 0 && s.oss != nil {
		_ = s.oss.DeleteObject(*rec.OSSStorageID, rec.RemoteKey)
	}
	if err := s.db.Delete(&rec).Error; err != nil {
		return err
	}
	if inst, err := s.Get(databaseID); err == nil {
		s.refreshBackupStatus(inst)
	}
	return nil
}

func (s *Service) GetBackupFile(databaseID, backupID uint) (string, error) {
	var rec models.DatabaseBackup
	if err := s.db.Where("id = ? AND database_id = ?", backupID, databaseID).First(&rec).Error; err != nil {
		return "", err
	}
	if rec.FilePath == "" {
		return "", fmt.Errorf("备份文件不存在")
	}
	if _, err := os.Stat(rec.FilePath); err != nil {
		return "", fmt.Errorf("备份文件已丢失")
	}
	return rec.FilePath, nil
}

func (s *Service) refreshBackupStatus(inst *models.DatabaseInstance) {
	var count int64
	s.db.Model(&models.DatabaseBackup{}).Where("database_id = ? AND status = ?", inst.ID, "done").Count(&count)
	status := "none"
	if count > 0 {
		status = fmt.Sprintf("%d份", count)
	}
	_ = s.db.Model(inst).Update("backup_status", status).Error
}

func (s *Service) dbBackupDir() string {
	all, _ := s.settings.GetAll()
	base := all["backup_path"]
	if base == "" {
		base = settings.DefaultBackupPath(s.dataDir)
	}
	dir := resolvePath(s.dataDir, base)
	return filepath.Join(dir, "databases")
}

func backupExt(dbType string) string {
	switch strings.ToLower(dbType) {
	case "redis":
		return ".rdb"
	default:
		return ".sql"
	}
}

func resolvePath(dataDir, p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return filepath.Join(dataDir, "backup")
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(dataDir, p))
}

func sanitizeFileName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return "db"
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
		return "db"
	}
	return out
}

func findBinary(names ...string) (string, error) {
	for _, name := range names {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
		if runtime.GOOS == "windows" {
			if p, err := exec.LookPath(name + ".exe"); err == nil {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("未找到命令: %s（请先在软件商店安装对应数据库）", strings.Join(names, "/"))
}

func dumpMySQL(inst *models.DatabaseInstance, dest string) error {
	bin, err := findBinary("mysqldump", "mariadb-dump")
	if err != nil {
		return err
	}
	args := []string{
		"-h", inst.Host,
		"-P", fmt.Sprintf("%d", inst.Port),
		"-u", inst.Username,
		fmt.Sprintf("-p%s", inst.Password),
		"--single-transaction",
		"--routines",
		"--events",
	}
	args = append(args, mysqldumpExtraArgs(bin)...)
	args = append(args, inst.Name)
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	cmd := exec.Command(bin, args...)
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mysqldump 失败: %w", err)
	}
	return nil
}

func importMySQL(inst *models.DatabaseInstance, sqlPath string) error {
	bin, err := findBinary("mysql", "mariadb")
	if err != nil {
		return err
	}
	args := []string{
		"-h", inst.Host,
		"-P", fmt.Sprintf("%d", inst.Port),
		"-u", inst.Username,
		fmt.Sprintf("-p%s", inst.Password),
		inst.Name,
	}
	in, err := os.Open(sqlPath)
	if err != nil {
		return err
	}
	defer in.Close()
	cmd := exec.Command(bin, args...)
	cmd.Stdin = in
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mysql 导入失败: %w", err)
	}
	return nil
}

func dumpPostgreSQL(inst *models.DatabaseInstance, dest string) error {
	bin, err := findBinary("pg_dump")
	if err != nil {
		return err
	}
	args := []string{
		"-h", inst.Host,
		"-p", fmt.Sprintf("%d", inst.Port),
		"-U", inst.Username,
		"-d", inst.Name,
		"-f", dest,
		"--no-owner",
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+inst.Password)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_dump 失败: %w", err)
	}
	return nil
}

func importPostgreSQL(inst *models.DatabaseInstance, sqlPath string) error {
	bin, err := findBinary("psql")
	if err != nil {
		return err
	}
	args := []string{
		"-h", inst.Host,
		"-p", fmt.Sprintf("%d", inst.Port),
		"-U", inst.Username,
		"-d", inst.Name,
		"-f", sqlPath,
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+inst.Password)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psql 导入失败: %w", err)
	}
	return nil
}

func dumpRedis(inst *models.DatabaseInstance, dest string) error {
	bin, err := findBinary("redis-cli")
	if err != nil {
		return err
	}
	args := []string{"-h", inst.Host, "-p", fmt.Sprintf("%d", inst.Port)}
	if inst.Password != "" {
		args = append(args, "-a", inst.Password)
	}
	args = append(args, "--rdb", dest)
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("redis-cli 备份失败: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func saveUploadFile(src io.Reader, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, src)
	return err
}

type ConnInfo struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	DBName   string `json:"db_name"`
}

func DumpMySQLConn(c ConnInfo, dest string) error {
	port := c.Port
	if port == 0 {
		port = 3306
	}
	inst := &models.DatabaseInstance{
		Host:     c.Host,
		Port:     port,
		Username: c.Username,
		Password: c.Password,
		Name:     c.DBName,
	}
	return dumpMySQL(inst, dest)
}
