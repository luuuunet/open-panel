package database

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/appstore"
)

const mysqlRootPasswordKey = "mysql_root_password"

type MySQLStatus struct {
	Installed      bool   `json:"installed"`
	Version        string `json:"version"`
	ServerVersion  string `json:"server_version,omitempty"`
	Engine         string `json:"engine"`
	LegacyMySQL57  bool   `json:"legacy_mysql57"`
	Running        bool   `json:"running"`
}

func (s *Service) MySQLStatus() MySQLStatus {
	st := MySQLStatus{}
	bin, err := findBinary("mysql", "mariadb")
	if err != nil {
		return st
	}
	st.Installed = true
	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err == nil {
		raw := strings.TrimSpace(string(out))
		st.Engine, st.Version = parseMySQLVersionOutput(raw)
	}
	if st.Engine == "" {
		st.Engine = "mysql"
	}
	if srv := s.queryMySQLServer(); srv.Version != "" {
		st.ServerVersion = srv.Version
		if srv.Engine != "" {
			st.Engine = srv.Engine
		}
		st.LegacyMySQL57 = srv.Engine == "mysql" && srv.Major == 5 && srv.Minor == 7
		if st.Version == "" {
			st.Version = srv.Version
		}
	}
	st.Running = true
	return st
}

// detectMySQLEngine returns "mariadb" or "mysql" from the installed client.
func detectMySQLEngine() string {
	bin, err := findBinary("mysql", "mariadb")
	if err != nil {
		return "mysql"
	}
	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		return "mysql"
	}
	engine, _ := parseMySQLVersionOutput(strings.TrimSpace(string(out)))
	if engine == "" {
		return "mysql"
	}
	return engine
}

func parseMySQLVersionOutput(raw string) (engine, display string) {
	raw = strings.TrimSpace(raw)
	lower := strings.ToLower(raw)
	engine = "mysql"
	if strings.Contains(lower, "mariadb") {
		engine = "mariadb"
	}
	if i := strings.Index(raw, "Distrib "); i >= 0 {
		rest := strings.TrimSpace(raw[i+len("Distrib "):])
		if part := strings.Fields(rest); len(part) > 0 {
			display = strings.TrimSuffix(strings.Split(part[0], "-")[0], ",")
		}
	} else if i := strings.Index(raw, "Ver "); i >= 0 {
		rest := strings.TrimSpace(raw[i+len("Ver "):])
		if part := strings.Fields(rest); len(part) > 0 {
			v := part[0]
			if strings.Contains(strings.ToLower(v), "mariadb") {
				display = strings.Split(v, "-")[0]
			} else {
				display = strings.TrimSuffix(v, ",")
			}
		}
	}
	if display == "" && raw != "" {
		if len(raw) > 48 {
			display = raw[:48] + "…"
		} else {
			display = raw
		}
	}
	return engine, display
}

func (s *Service) GetCredentials(id uint) (username, password, host string, port int, err error) {
	inst, err := s.Get(id)
	if err != nil {
		return "", "", "", 0, err
	}
	return inst.Username, inst.Password, inst.Host, inst.Port, nil
}

func (s *Service) ChangeMySQLRootPassword(newPassword string) error {
	if strings.TrimSpace(newPassword) == "" {
		return fmt.Errorf("密码不能为空")
	}
	if err := s.applyMySQLRootPassword(newPassword); err != nil {
		return err
	}
	s.storeMySQLRootPassword(newPassword)
	return nil
}

// EnsureMySQLRootPasswordAuth enables password login over TCP (127.0.0.1) for phpMyAdmin.
// Ubuntu MySQL defaults to auth_socket for root@localhost, which rejects TCP connections.
func (s *Service) EnsureMySQLRootPasswordAuth() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	if _, err := findBinary("mysql", "mariadb"); err != nil {
		return fmt.Errorf("MySQL 未安装")
	}
	if pass := s.getStoredMySQLRootPassword(); pass != "" && canMySQLRootTCPLogin(pass) {
		return nil
	}
	authSocket, err := s.rootUsesAuthSocket()
	if err != nil {
		return fmt.Errorf("检查 MySQL root 认证方式失败: %w", err)
	}
	if !authSocket {
		stored := s.getStoredMySQLRootPassword()
		if stored != "" {
			// Panel/MySQL drift: re-apply stored password via debian maintenance account.
			if err := s.applyMySQLRootPassword(stored); err != nil {
				return fmt.Errorf("MySQL root 密码无效，请在数据库页面重置 root 密码")
			}
			if canMySQLRootTCPLogin(stored) {
				return nil
			}
			return fmt.Errorf("MySQL root 密码无效，请在数据库页面重置 root 密码")
		}
		return s.bootstrapMySQLRootPassword()
	}
	pass, err := randomMySQLPassword(16)
	if err != nil {
		return err
	}
	if err := s.applyMySQLRootPassword(pass); err != nil {
		return err
	}
	if !canMySQLRootTCPLogin(pass) {
		return fmt.Errorf("MySQL root 密码已设置但 127.0.0.1 TCP 登录验证失败")
	}
	s.storeMySQLRootPassword(pass)
	s.ensureRootDatabaseRecord(pass)
	return nil
}

func (s *Service) bootstrapMySQLRootPassword() error {
	if _, ok := debianMySQLDefaultsFile(); !ok {
		return nil
	}
	pass, err := randomMySQLPassword(16)
	if err != nil {
		return err
	}
	if err := s.applyMySQLRootPassword(pass); err != nil {
		return err
	}
	if !canMySQLRootTCPLogin(pass) {
		return fmt.Errorf("MySQL root 密码已设置但 127.0.0.1 TCP 登录验证失败")
	}
	s.storeMySQLRootPassword(pass)
	s.ensureRootDatabaseRecord(pass)
	return nil
}

func (s *Service) applyMySQLRootPassword(password string) error {
	info := s.queryMySQLServer()
	id := info.identifiedBy(password)
	var sql string
	if info.supportsCreateUserIfNotExists() {
		sql = fmt.Sprintf(
			"ALTER USER 'root'@'localhost' %s; "+
				"CREATE USER IF NOT EXISTS 'root'@'127.0.0.1' %s; "+
				"ALTER USER 'root'@'127.0.0.1' %s; "+
				"GRANT ALL PRIVILEGES ON *.* TO 'root'@'127.0.0.1' WITH GRANT OPTION; "+
				"FLUSH PRIVILEGES;",
			id, id, id,
		)
	} else {
		esc := strings.ReplaceAll(password, "'", "''")
		sql = fmt.Sprintf(
			"SET PASSWORD FOR 'root'@'localhost' = PASSWORD('%s'); "+
				"GRANT ALL PRIVILEGES ON *.* TO 'root'@'127.0.0.1' IDENTIFIED BY '%s' WITH GRANT OPTION; "+
				"FLUSH PRIVILEGES;",
			esc, esc,
		)
	}
	out, err := s.mysqlRootExec("-e", sql)
	if err != nil {
		return fmt.Errorf("修改 root 密码失败: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *Service) getStoredMySQLRootPassword() string {
	var row models.PanelSetting
	if s.db != nil && s.db.Where("key = ?", mysqlRootPasswordKey).First(&row).Error == nil {
		return row.Value
	}
	var inst models.DatabaseInstance
	if s.db != nil && s.db.Where("username = ? AND type IN ?", "root", []string{"mysql", "mariadb"}).First(&inst).Error == nil {
		return inst.Password
	}
	return ""
}

func (s *Service) storeMySQLRootPassword(password string) {
	if s.db == nil {
		return
	}
	s.db.Where(models.PanelSetting{Key: mysqlRootPasswordKey}).
		Assign(models.PanelSetting{Value: password}).
		FirstOrCreate(&models.PanelSetting{Key: mysqlRootPasswordKey})
	s.db.Model(&models.DatabaseInstance{}).
		Where("username = ? AND type IN ?", "root", []string{"mysql", "mariadb"}).
		Update("password", password)
}

func (s *Service) ensureRootDatabaseRecord(password string) {
	if s.db == nil {
		return
	}
	var count int64
	s.db.Model(&models.DatabaseInstance{}).
		Where("username = ? AND type IN ?", "root", []string{"mysql", "mariadb"}).
		Count(&count)
	if count > 0 {
		return
	}
	_ = s.db.Create(&models.DatabaseInstance{
		Name:     "MySQL Root",
		Type:     "mysql",
		Host:     "127.0.0.1",
		Port:     3306,
		Username: "root",
		Password: password,
		Status:   "running",
	}).Error
}

// mysqlRootExec runs mysql as root using stored TCP credentials, debian maintenance
// access, or auth_socket (passwordless local socket) in that order.
func (s *Service) mysqlRootExec(args ...string) ([]byte, error) {
	bin, err := findBinary("mysql", "mariadb")
	if err != nil {
		return nil, err
	}
	if pass := s.getStoredMySQLRootPassword(); pass != "" && canMySQLRootTCPLogin(pass) {
		out, err := mysqlExecWithPassword(bin, append([]string{"-u", "root", "-h", "127.0.0.1", "--batch", "--raw", "--skip-column-names"}, args...), pass)
		if err == nil && !strings.Contains(string(out), "ERROR") {
			return out, nil
		}
	}
	if debian, ok := debianMySQLDefaultsFile(); ok {
		out, err := exec.Command(bin, append([]string{"--defaults-file=" + debian}, args...)...).CombinedOutput()
		if err == nil && !strings.Contains(string(out), "ERROR") {
			return out, nil
		}
	}
	return exec.Command(bin, append([]string{"-u", "root"}, args...)...).CombinedOutput()
}

func debianMySQLDefaultsFile() (string, bool) {
	const path = "/etc/mysql/debian.cnf"
	if runtime.GOOS != "linux" {
		return "", false
	}
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	return path, true
}

func (s *Service) rootUsesAuthSocket() (bool, error) {
	out, err := s.mysqlRootExec("-N", "-e", "SELECT plugin FROM mysql.user WHERE user='root' AND host='localhost' LIMIT 1;")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "auth_socket", nil
}

func canMySQLRootTCPLogin(password string) bool {
	bin, err := findBinary("mysql", "mariadb")
	if err != nil {
		return false
	}
	out, err := mysqlExecWithPassword(bin, []string{"-u", "root", "-h", "127.0.0.1", "-e", "SELECT 1;"}, password)
	return err == nil && !strings.Contains(string(out), "ERROR")
}

func randomMySQLPassword(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// EnsureMySQLInstalled installs and starts MySQL/MariaDB on Linux when missing.
func (s *Service) EnsureMySQLInstalled(logf func(string)) error {
	if _, err := findBinary("mysql", "mariadb"); err == nil {
		return s.EnsureMySQLRootPasswordAuth()
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("未检测到 MySQL，请先在软件商店安装 MySQL/MariaDB")
	}
	if logf != nil {
		logf("正在安装 MariaDB/MySQL …")
	}
	if err := appstore.RunMariaDBStackInstall(); err != nil {
		return fmt.Errorf("安装 MySQL/MariaDB 失败: %w", err)
	}
	for _, svc := range []string{"mariadb", "mysql", "mysqld"} {
		_ = exec.Command("systemctl", "enable", svc).Run()
		_ = exec.Command("systemctl", "start", svc).Run()
	}
	if _, err := findBinary("mysql", "mariadb"); err != nil {
		return fmt.Errorf("MySQL 安装后仍不可用")
	}
	if logf != nil {
		logf("✓ MySQL 已安装并启动")
	}
	return s.EnsureMySQLRootPasswordAuth()
}

func (s *Service) MySQLClientAvailable() bool {
	_, err := findBinary("mysql", "mariadb")
	return err == nil
}

// ProvisionMySQLOptions 创建 MySQL 库与用户时的选项。
type ProvisionMySQLOptions struct {
	Name        string
	Username    string
	Password    string
	Charset     string
	AllowRemote bool
	AccessMode  string
	ForceSSL    bool
}

func mysqlCharsetCollation(charset string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(charset)) {
	case "utf8", "utf-8":
		return "utf8", "utf8_general_ci"
	case "gbk":
		return "gbk", "gbk_chinese_ci"
	case "big5":
		return "big5", "big5_chinese_ci"
	default:
		return "utf8mb4", "utf8mb4_unicode_ci"
	}
}

// CharsetFromInput normalizes user-facing charset labels for storage.
func CharsetFromInput(charset string) string {
	cs, _ := mysqlCharsetCollation(charset)
	return cs
}

// ProvisionMySQL 在 MySQL/MariaDB 上创建真实数据库与用户
func (s *Service) ProvisionMySQL(name, username, password string) error {
	return s.ProvisionMySQLWith(ProvisionMySQLOptions{
		Name: name, Username: username, Password: password, Charset: "utf8mb4",
	})
}

func (s *Service) ProvisionMySQLWith(opts ProvisionMySQLOptions) error {
	name := strings.TrimSpace(opts.Name)
	username := strings.TrimSpace(opts.Username)
	password := opts.Password
	if name == "" || username == "" {
		return fmt.Errorf("数据库名和用户名不能为空")
	}
	if password == "" {
		return fmt.Errorf("密码不能为空")
	}
	cs, coll := mysqlCharsetCollation(opts.Charset)
	info := s.queryMySQLServer()
	esc := mysqlEscapeSQL
	userLocal := mysqlEnsureUserSQL(info, username, "localhost", password)
	userLoop := mysqlEnsureUserSQL(info, username, "127.0.0.1", password)
	sql := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET %s COLLATE %s; "+
			"%s %s "+
			"GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost'; "+
			"GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'127.0.0.1'; FLUSH PRIVILEGES;",
		esc(name), cs, coll,
		userLocal, userLoop,
		esc(name), esc(username), esc(name), esc(username),
	)
	out, err := s.mysqlRootExec("-e", sql)
	if err != nil {
		return fmt.Errorf("创建数据库失败: %s", strings.TrimSpace(string(out)))
	}
	dbType := "mysql"
	if info.Engine == "mariadb" {
		dbType = "mariadb"
	}
	inst := &models.DatabaseInstance{
		Name: name, Type: dbType, Username: username, Password: password,
		Charset: cs, ForceSSL: opts.ForceSSL,
	}
	mode := AccessModeFromInstance(opts.AccessMode, opts.AllowRemote)
	inst.AccessMode = mode
	inst.AllowRemote = AllowRemoteFromAccessMode(mode)
	if err := s.applyMySQLAccessMode(inst, mode); err != nil {
		return err
	}
	if opts.ForceSSL {
		if err := s.applyMySQLForceSSL(inst, true); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) applyMySQLForceSSL(inst *models.DatabaseInstance, require bool) error {
	if inst == nil || !isMySQLType(inst.Type) {
		return nil
	}
	user := strings.TrimSpace(inst.Username)
	if user == "" {
		user = strings.TrimSpace(inst.Name)
	}
	if user == "" {
		return fmt.Errorf("用户名为空")
	}
	req := "NONE"
	if require {
		req = "SSL"
	}
	escUser := mysqlEscapeSQL(user)
	hosts := mysqlHostsForAccessMode(AccessModeFromInstance(inst.AccessMode, inst.AllowRemote))
	parts := make([]string, 0, len(hosts))
	for _, h := range hosts {
		parts = append(parts, fmt.Sprintf("ALTER USER '%s'@'%s' REQUIRE %s;", escUser, h, req))
	}
	parts = append(parts, "FLUSH PRIVILEGES;")
	out, err := s.mysqlRootExec("-e", strings.Join(parts, " "))
	if err != nil {
		return fmt.Errorf("设置 SSL 要求失败: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func mysqlEscapeSQL(v string) string {
	return strings.ReplaceAll(v, "'", "''")
}

func (s *Service) mysqlUserHasHostGrant(username, host string) bool {
	username = strings.TrimSpace(username)
	host = strings.TrimSpace(host)
	if username == "" || host == "" {
		return false
	}
	q := fmt.Sprintf("SELECT COUNT(*) FROM mysql.user WHERE user='%s' AND host='%s';", mysqlEscapeSQL(username), mysqlEscapeSQL(host))
	out, err := s.mysqlRootExec("-N", "-e", q)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != "0"
}

func (s *Service) mysqlUserHasRemoteGrant(username string) bool {
	return s.mysqlUserHasHostGrant(username, "%")
}

func (s *Service) detectMySQLAccessMode(username string) string {
	hasRemote := s.mysqlUserHasHostGrant(username, "%")
	hasLocal := s.mysqlUserHasHostGrant(username, "localhost") || s.mysqlUserHasHostGrant(username, "127.0.0.1")
	if hasRemote && hasLocal {
		return AccessModeBoth
	}
	if hasRemote {
		return AccessModeRemote
	}
	return AccessModeLocal
}

func (s *Service) applyMySQLRemoteAccess(inst *models.DatabaseInstance, allow bool) error {
	mode := AccessModeLocal
	if allow {
		mode = AccessModeBoth
	}
	return s.applyMySQLAccessMode(inst, mode)
}

func (s *Service) applyMySQLAccessMode(inst *models.DatabaseInstance, mode string) error {
	if inst == nil {
		return fmt.Errorf("数据库不存在")
	}
	if !isMySQLType(inst.Type) {
		return nil
	}
	if _, err := findBinary("mysql", "mariadb"); err != nil {
		return fmt.Errorf("未安装 MySQL，无法修改远程访问权限")
	}
	mode = NormalizeAccessMode(mode)
	user := strings.TrimSpace(inst.Username)
	if user == "" {
		user = strings.TrimSpace(inst.Name)
	}
	if user == "" {
		return fmt.Errorf("用户名为空")
	}
	pass := inst.Password
	needsRemote := mode == AccessModeRemote || mode == AccessModeBoth
	needsLocal := mode == AccessModeLocal || mode == AccessModeBoth
	if needsRemote && pass == "" {
		return fmt.Errorf("请先设置数据库密码后再开启远程访问")
	}
	dbName := strings.TrimSpace(inst.Name)
	escUser := mysqlEscapeSQL(user)
	escDB := mysqlEscapeSQL(dbName)
	info := s.queryMySQLServer()

	var parts []string
	if needsLocal {
		if pass != "" {
			parts = append(parts, mysqlEnsureUserSQL(info, user, "localhost", pass))
			parts = append(parts, mysqlEnsureUserSQL(info, user, "127.0.0.1", pass))
			parts = append(parts,
				fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost';", escDB, escUser),
				fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'127.0.0.1';", escDB, escUser),
			)
		}
	} else {
		parts = append(parts,
			fmt.Sprintf("DROP USER IF EXISTS '%s'@'localhost';", escUser),
			fmt.Sprintf("DROP USER IF EXISTS '%s'@'127.0.0.1';", escUser),
		)
	}
	if needsRemote {
		parts = append(parts, mysqlEnsureUserSQL(info, user, "%", pass))
		parts = append(parts, fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%%';", escDB, escUser))
	} else {
		parts = append(parts, fmt.Sprintf("DROP USER IF EXISTS '%s'@'%%';", escUser))
	}
	parts = append(parts, "FLUSH PRIVILEGES;")
	out, err := s.mysqlRootExec("-e", strings.Join(parts, " "))
	if err != nil {
		return fmt.Errorf("修改访问权限失败: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
