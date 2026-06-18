package wordpress

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

type DatabaseOptions struct {
	Mode     string // auto | custom | existing | skip
	ID       uint
	Name     string
	User     string
	Password string
	Host     string
	Port     int
}

type databaseCredentials struct {
	InstanceID uint
	Name       string
	User       string
	Password   string
	Host       string
	Port       int
}

func (s *Service) setupDatabase(site *models.WordPressSite, opts DatabaseOptions, logger *DeployLogger) (*databaseCredentials, error) {
	if opts.Mode == "skip" || opts.Mode == "none" {
		logger.Info("跳过数据库配置（可在 wp-config.php 中手动填写）")
		return nil, nil
	}

	cfgPath := filepath.Join(site.RootPath, "wp-config.php")
	if _, err := os.Stat(cfgPath); err == nil && site.DbName != "" {
		logger.Info("wp-config.php 已存在，跳过数据库配置")
		return &databaseCredentials{
			Name: site.DbName, User: site.DbUser, Host: site.DbHost, Port: site.DbPort,
		}, nil
	}

	mode := strings.TrimSpace(strings.ToLower(opts.Mode))
	if mode == "" {
		mode = "auto"
	}

	switch mode {
	case "existing":
		return s.useExistingDatabase(opts, logger)
	case "custom":
		return s.useCustomDatabase(site, opts, logger)
	default:
		return s.autoProvisionDatabase(site, logger)
	}
}

func (s *Service) autoProvisionDatabase(site *models.WordPressSite, logger *DeployLogger) (*databaseCredentials, error) {
	if s.database == nil {
		return nil, fmt.Errorf("数据库服务未就绪")
	}
	logger.Info("正在检查 MySQL…")
	if err := s.database.EnsureMySQLInstalled(func(msg string) { logger.Info(msg) }); err != nil {
		return nil, err
	}

	base := sanitizeDbName(site.Domain)
	dbName := base + "_wp"
	dbUser := base + "_u"
	dbPass := randomDbPassword(16)

	logger.Info(fmt.Sprintf("正在创建数据库 %s …", dbName))
	if err := s.database.ProvisionMySQL(dbName, dbUser, dbPass); err != nil {
		return nil, err
	}
	logger.Info("✓ MySQL 数据库与用户已创建")

	inst := &models.DatabaseInstance{
		Name: dbName, Type: "mysql", Host: "127.0.0.1", Port: 3306,
		Username: dbUser, Password: dbPass, Status: "running",
		Remark:   "WordPress: " + site.Domain,
	}
	if err := s.database.Create(inst); err != nil {
		logger.Warn("面板数据库记录写入失败: " + err.Error())
	}

	return &databaseCredentials{
		InstanceID: inst.ID,
		Name:       dbName,
		User:       dbUser,
		Password:   dbPass,
		Host:       "127.0.0.1",
		Port:       3306,
	}, nil
}

func (s *Service) useExistingDatabase(opts DatabaseOptions, logger *DeployLogger) (*databaseCredentials, error) {
	if s.database == nil {
		return nil, fmt.Errorf("数据库服务未就绪")
	}
	if opts.ID == 0 {
		return nil, fmt.Errorf("请选择已有数据库")
	}
	user, pass, host, port, err := s.database.GetCredentials(opts.ID)
	if err != nil {
		return nil, err
	}
	inst, err := s.database.Get(opts.ID)
	if err != nil {
		return nil, err
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = 3306
	}
	logger.Info(fmt.Sprintf("使用已有数据库: %s @ %s:%d", inst.Name, host, port))
	return &databaseCredentials{
		InstanceID: inst.ID,
		Name:       inst.Name,
		User:       user,
		Password:   pass,
		Host:       host,
		Port:       port,
	}, nil
}

func (s *Service) useCustomDatabase(site *models.WordPressSite, opts DatabaseOptions, logger *DeployLogger) (*databaseCredentials, error) {
	if s.database == nil {
		return nil, fmt.Errorf("数据库服务未就绪")
	}
	name := strings.TrimSpace(opts.Name)
	user := strings.TrimSpace(opts.User)
	pass := opts.Password
	host := strings.TrimSpace(opts.Host)
	port := opts.Port
	if name == "" || user == "" {
		return nil, fmt.Errorf("请填写数据库名和用户名")
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = 3306
	}
	if pass == "" {
		return nil, fmt.Errorf("请填写数据库密码")
	}

	isLocal := host == "127.0.0.1" || host == "localhost" || host == "::1"
	if isLocal {
		if err := s.database.EnsureMySQLInstalled(func(msg string) { logger.Info(msg) }); err != nil {
			return nil, err
		}
		logger.Info(fmt.Sprintf("正在创建数据库 %s …", name))
		if err := s.database.ProvisionMySQL(name, user, pass); err != nil {
			logger.Warn("自动创建失败，将仅写入 wp-config: " + err.Error())
		} else {
			logger.Info("✓ 数据库与用户已创建")
		}
	} else {
		logger.Info(fmt.Sprintf("使用外部数据库 %s @ %s:%d", name, host, port))
	}

	var instID uint
	inst := &models.DatabaseInstance{
		Name: name, Type: "mysql", Host: host, Port: port,
		Username: user, Password: pass, Status: "running",
	}
	if err := s.database.Create(inst); err == nil {
		instID = inst.ID
	}

	return &databaseCredentials{
		InstanceID: instID,
		Name:       name,
		User:       user,
		Password:   pass,
		Host:       host,
		Port:       port,
	}, nil
}

func (s *Service) writeWPConfig(root, domain string, cred *databaseCredentials, logger *DeployLogger) error {
	if cred == nil {
		return nil
	}
	target := filepath.Join(root, "wp-config.php")
	if _, err := os.Stat(target); err == nil {
		logger.Info("wp-config.php 已存在，跳过写入")
		return nil
	}
	sample := filepath.Join(root, "wp-config-sample.php")
	b, err := os.ReadFile(sample)
	if err != nil {
		return fmt.Errorf("读取 wp-config-sample.php 失败: %w", err)
	}
	content := string(b)
	content = strings.ReplaceAll(content, "database_name_here", cred.Name)
	content = strings.ReplaceAll(content, "username_here", cred.User)
	content = strings.ReplaceAll(content, "password_here", cred.Password)
	dbHost := cred.Host
	if cred.Port > 0 && cred.Port != 3306 {
		dbHost = fmt.Sprintf("%s:%d", cred.Host, cred.Port)
	}
	content = strings.ReplaceAll(content, "localhost", dbHost)

	content = stripDefaultWPSalts(content)
	salts := generateWPSalts()
	extra := fmt.Sprintf(
		"%sdefine('WP_HOME', 'http://%s');\ndefine('WP_SITEURL', 'http://%s');\n%s\n",
		buildWPProxyHTTPSBlock(), domain, domain, salts,
	)
	content = strings.Replace(content, "/* That's all, stop editing!", extra+"/* That's all, stop editing!", 1)

	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		return err
	}
	logger.Info("✓ 已生成 wp-config.php（数据库已配置）")
	return nil
}

func generateWPSalts() string {
	keys := []string{
		"AUTH_KEY", "SECURE_AUTH_KEY", "LOGGED_IN_KEY", "NONCE_KEY",
		"AUTH_SALT", "SECURE_AUTH_SALT", "LOGGED_IN_SALT", "NONCE_SALT",
	}
	var lines []string
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("define('%s', '%s');", k, wpRandomString(64)))
	}
	return strings.Join(lines, "\n")
}

func wpRandomString(n int) string {
	b := make([]byte, (n+1)/2)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

func sanitizeDbName(domain string) string {
	s := strings.NewReplacer(".", "_", "-", "_", ":", "_", "*", "w").Replace(domain)
	if len(s) > 40 {
		s = s[:40]
	}
	if s == "" {
		s = "wp"
	}
	return s
}

func randomDbPassword(n int) string {
	return wpRandomString(n)
}

func databaseOptionsFromRequest(req *CreateRequest) DatabaseOptions {
	mode := strings.TrimSpace(strings.ToLower(req.DatabaseMode))
	if mode == "" {
		mode = "auto"
	}
	return DatabaseOptions{
		Mode:     mode,
		ID:       req.DatabaseID,
		Name:     req.DbName,
		User:     req.DbUser,
		Password: req.DbPassword,
		Host:     req.DbHost,
		Port:     req.DbPort,
	}
}

func defaultDatabaseOptions(site *models.WordPressSite) DatabaseOptions {
	if site.DbName != "" {
		return DatabaseOptions{Mode: "skip"}
	}
	return DatabaseOptions{Mode: "auto"}
}
