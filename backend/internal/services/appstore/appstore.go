package appstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/luuuunet/owpanel/internal/dbmigrate"
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/domaincheck"
	"github.com/luuuunet/owpanel/internal/services/php"
	"github.com/luuuunet/owpanel/internal/services/stack"
	"gorm.io/gorm"
)

type catalogItem struct {
	models.App
	defaultConfig map[string]interface{}
}

// Built-in software store catalog. Apps are defined in Go source (catalog + catalogExtraApps);
// the panel does not fetch app lists from external stores at runtime.
var catalog = []catalogItem{
	{
		App: models.App{Key: "nginx", Name: "Nginx", Category: "Web服务器", Versions: "1.26,1.25,1.24", Version: "1.26", Description: "高性能 Web 服务器", Port: 80, InstallPath: "server/nginx", ConfigPath: "/etc/nginx/nginx.conf", Icon: "SetUp"},
		defaultConfig: map[string]interface{}{"worker_processes": "auto", "client_max_body_size": "50m", "gzip": "on"},
	},
	{
		App: models.App{Key: "openresty", Name: "OpenResty", Category: "Web服务器", Versions: "1.25,1.21", Version: "1.25", Description: "Nginx + LuaJIT 增强版（1Panel 默认推荐，与 Nginx 二选一）", Port: 80, InstallPath: "/usr/local/openresty", ConfigPath: "/usr/local/openresty/nginx/conf/nginx.conf", Icon: "SetUp"},
		defaultConfig: map[string]interface{}{"worker_processes": "auto", "client_max_body_size": "50m", "lua_package_path": "/usr/local/openresty/lualib/?.lua;;"},
	},
	{
		App: models.App{Key: "apache", Name: "Apache", Category: "Web服务器", Versions: "2.4", Version: "2.4", Description: "Apache HTTP Server", Port: 80, InstallPath: "server/apache", ConfigPath: "server/apache/conf/httpd.conf", Icon: "SetUp"},
		defaultConfig: map[string]interface{}{"ServerRoot": "server/apache", "MaxRequestWorkers": "150"},
	},
	{
		App: models.App{Key: "openlitespeed", Name: "OpenLiteSpeed", Category: "Web服务器", Versions: "1.7", Version: "1.7", Description: "高性能开源 Web 服务器", Port: 8088, InstallPath: "/usr/local/lsws", ConfigPath: "/usr/local/lsws/conf/httpd_config.conf", Icon: "SetUp"},
		defaultConfig: map[string]interface{}{"enable_gzip": true, "max_connections": 10000},
	},
	{
		App: models.App{Key: "mysql", Name: "MySQL", Category: "数据库", Versions: "8.4,8.0,5.7,5.6,5.5", Version: "8.0", Description: "MySQL 关系型数据库", Port: 3306, InstallPath: "server/mysql", ConfigPath: "/etc/my.cnf", Icon: "Coin"},
		defaultConfig: map[string]interface{}{"max_connections": 500, "innodb_buffer_pool_size": "256M", "bind_address": "127.0.0.1"},
	},
	{
		App: models.App{Key: "mariadb", Name: "MariaDB", Category: "数据库", Versions: "10.11,10.6", Version: "10.11", Description: "MariaDB 数据库", Port: 3306, InstallPath: "server/mariadb", ConfigPath: "/etc/my.cnf", Icon: "Coin"},
		defaultConfig: map[string]interface{}{"max_connections": 500, "innodb_buffer_pool_size": "256M"},
	},
	{
		App: models.App{Key: "postgresql", Name: "PostgreSQL", Category: "数据库", Versions: "16,15,14", Version: "16", Description: "PostgreSQL 数据库", Port: 5432, InstallPath: "server/pgsql", ConfigPath: "server/pgsql/data/postgresql.conf", Icon: "Coin"},
		defaultConfig: map[string]interface{}{"max_connections": 200, "shared_buffers": "128MB"},
	},
	{
		App: models.App{Key: "redis", Name: "Redis", Category: "数据库", Versions: "7.2,6.2", Version: "7.2", Description: "Redis 缓存数据库", Port: 6379, InstallPath: "server/redis", ConfigPath: "server/redis/redis.conf", Icon: "Coin"},
		defaultConfig: map[string]interface{}{"maxmemory": "256mb", "bind": "127.0.0.1", "requirepass": ""},
	},
	{
		App: models.App{Key: "mongodb", Name: "MongoDB", Category: "数据库", Versions: "7.0,6.0", Version: "7.0", Description: "MongoDB 文档数据库", Port: 27017, InstallPath: "server/mongodb", ConfigPath: "/etc/mongod.conf", Icon: "Coin"},
		defaultConfig: map[string]interface{}{"storage.dbPath": "server/mongodb/data", "net.port": 27017},
	},
	{
		App: models.App{Key: "php83", Name: "PHP-8.3", Category: "运行环境", Versions: "8.3", Version: "8.3", Description: "PHP 8.3 运行环境", Port: 9000, InstallPath: "server/php/83", ConfigPath: "server/php/83/etc/php.ini", Icon: "Coffee"},
		defaultConfig: map[string]interface{}{"memory_limit": "128M", "upload_max_filesize": "50M", "post_max_size": "50M", "max_execution_time": "300", "date.timezone": "Asia/Shanghai", "open_basedir": "", "disable_functions": "exec,passthru,shell_exec,system,proc_open,popen"},
	},
	{
		App: models.App{Key: "php82", Name: "PHP-8.2", Category: "运行环境", Versions: "8.2", Version: "8.2", Description: "PHP 8.2 运行环境", Port: 9001, InstallPath: "server/php/82", ConfigPath: "server/php/82/etc/php.ini", Icon: "Coffee"},
		defaultConfig: map[string]interface{}{"memory_limit": "128M", "upload_max_filesize": "50M", "post_max_size": "50M", "max_execution_time": "300", "date.timezone": "Asia/Shanghai", "open_basedir": "", "disable_functions": "exec,passthru,shell_exec,system,proc_open,popen"},
	},
	{
		App: models.App{Key: "php81", Name: "PHP-8.1", Category: "运行环境", Versions: "8.1", Version: "8.1", Description: "PHP 8.1 运行环境", Port: 9002, InstallPath: "server/php/81", ConfigPath: "server/php/81/etc/php.ini", Icon: "Coffee"},
		defaultConfig: map[string]interface{}{"memory_limit": "128M", "upload_max_filesize": "50M", "post_max_size": "50M", "max_execution_time": "300", "date.timezone": "Asia/Shanghai", "open_basedir": "", "disable_functions": "exec,passthru,shell_exec,system,proc_open,popen"},
	},
	{
		App: models.App{Key: "php74", Name: "PHP-7.4", Category: "运行环境", Versions: "7.4", Version: "7.4", Description: "PHP 7.4 运行环境", Port: 9003, InstallPath: "server/php/74", ConfigPath: "server/php/74/etc/php.ini", Icon: "Coffee"},
		defaultConfig: map[string]interface{}{"memory_limit": "128M", "upload_max_filesize": "50M", "post_max_size": "50M", "max_execution_time": "300", "date.timezone": "Asia/Shanghai", "open_basedir": "", "disable_functions": "exec,passthru,shell_exec,system,proc_open,popen"},
	},
	{
		App: models.App{Key: "nodejs20", Name: "Node.js 20", Category: "运行环境", Versions: "20", Version: "20", Description: "Node.js 20 LTS 运行时（npm/nvm）", Port: 0, InstallPath: "server/nodejs/20", ConfigPath: "", Icon: "Platform"},
		defaultConfig: map[string]interface{}{"version": "20"},
	},
	{
		App: models.App{Key: "nodejs18", Name: "Node.js 18", Category: "运行环境", Versions: "18", Version: "18", Description: "Node.js 18 LTS 运行时（npm/nvm）", Port: 0, InstallPath: "server/nodejs/18", ConfigPath: "", Icon: "Platform"},
		defaultConfig: map[string]interface{}{"version": "18"},
	},
	{
		App: models.App{Key: "python", Name: "Python", Category: "运行环境", Versions: "3.11,3.10", Version: "3.11", Description: "Python 运行环境", Port: 0, InstallPath: "server/python", ConfigPath: "", Icon: "Platform"},
		defaultConfig: map[string]interface{}{"version": "3.11"},
	},
	{
		App: models.App{Key: "dotnet10", Name: ".NET 10", Category: "运行环境", Versions: "10.0", Version: "10.0", Description: ".NET 10 运行时（ASP.NET Core）", Port: 0, InstallPath: "server/dotnet/10", ConfigPath: "", Icon: "Platform"},
		defaultConfig: map[string]interface{}{"version": "10.0"},
	},
	{
		App: models.App{Key: "dotnet8", Name: ".NET 8", Category: "运行环境", Versions: "8.0", Version: "8.0", Description: ".NET 8 LTS 运行时（ASP.NET Core）", Port: 0, InstallPath: "server/dotnet/8", ConfigPath: "", Icon: "Platform"},
		defaultConfig: map[string]interface{}{"version": "8.0"},
	},
	{
		App: models.App{Key: "java21", Name: "JDK-21", Category: "运行环境", Versions: "21", Version: "21", Description: "OpenJDK 21 LTS，适用于 Spring Boot / Tomcat 等 Java 项目", Port: 0, InstallPath: "server/java/21", ConfigPath: "", Icon: "CoffeeCup"},
		defaultConfig: map[string]interface{}{"version": "21", "JAVA_HOME": "/usr/lib/jvm/java-21-openjdk"},
	},
	{
		App: models.App{Key: "java17", Name: "JDK-17", Category: "运行环境", Versions: "17", Version: "17", Description: "OpenJDK 17 LTS，企业 Java 项目推荐版本", Port: 0, InstallPath: "server/java/17", ConfigPath: "", Icon: "CoffeeCup"},
		defaultConfig: map[string]interface{}{"version": "17", "JAVA_HOME": "/usr/lib/jvm/java-17-openjdk"},
	},
	{
		App: models.App{Key: "java11", Name: "JDK-11", Category: "运行环境", Versions: "11", Version: "11", Description: "OpenJDK 11 LTS", Port: 0, InstallPath: "server/java/11", ConfigPath: "", Icon: "CoffeeCup"},
		defaultConfig: map[string]interface{}{"version": "11", "JAVA_HOME": "/usr/lib/jvm/java-11-openjdk"},
	},
	{
		App: models.App{Key: "java8", Name: "JDK-1.8", Category: "运行环境", Versions: "1.8", Version: "1.8", Description: "OpenJDK 8，兼容旧版 Java 项目", Port: 0, InstallPath: "server/java/8", ConfigPath: "", Icon: "CoffeeCup"},
		defaultConfig: map[string]interface{}{"version": "1.8", "JAVA_HOME": "/usr/lib/jvm/java-8-openjdk"},
	},
	{
		App: models.App{Key: "pureftpd", Name: "Pure-Ftpd", Category: "FTP", Versions: "1.0.49", Version: "1.0.49", Description: "FTP 服务", Port: 21, InstallPath: "server/pure-ftpd", ConfigPath: "server/pure-ftpd/etc/pure-ftpd.conf", Icon: "Upload"},
		defaultConfig: map[string]interface{}{"MaxClientsNumber": 50, "VerboseLog": "yes"},
	},
	{
		App: models.App{Key: "mail-server", Name: "邮件服务器", Category: "邮件", Versions: "latest", Version: "latest", Description: "Postfix + Dovecot 一键套件（SMTP 发信 + IMAP/POP3 收信），安装后请前往「邮件」管理域名与邮箱", Port: 25, InstallPath: "/etc/postfix", ConfigPath: "/etc/postfix/main.cf", Icon: "Message"},
		defaultConfig: map[string]interface{}{"inet_interfaces": "all", "virtual_mailbox_maps": "hash:/etc/postfix/owpanel-virtual"},
	},
	{
		App: models.App{Key: "phpmyadmin", Name: "phpMyAdmin", Category: "工具", Versions: "5.2", Version: "5.2", Description: "MySQL 在线管理", Port: 888, InstallPath: "server/phpmyadmin", ConfigPath: "server/phpmyadmin/config.inc.php", Icon: "Tools"},
		defaultConfig: map[string]interface{}{"auth_type": "cookie", "host": "127.0.0.1"},
	},
	{
		App: models.App{Key: "memcached", Name: "Memcached", Category: "工具", Versions: "1.6", Version: "1.6", Description: "内存缓存服务", Port: 11211, InstallPath: "server/memcached", ConfigPath: "/etc/memcached.conf", Icon: "Tools"},
		defaultConfig: map[string]interface{}{"memory": 64, "maxconn": 1024},
	},
	{
		App: models.App{Key: "docker", Name: "Docker", Category: "容器", Versions: "24.0", Version: "24.0", Description: "Docker 容器引擎", Port: 0, InstallPath: "/usr/bin/docker", ConfigPath: "/etc/docker/daemon.json", Icon: "Box"},
		defaultConfig: map[string]interface{}{"log-driver": "json-file", "log-opts": map[string]string{"max-size": "10m"}},
	},
	{
		App: models.App{Key: "fail2ban", Name: "Fail2ban", Category: "安全", Versions: "1.0", Version: "1.0", Description: "防暴力破解", Port: 0, InstallPath: "/etc/fail2ban", ConfigPath: "/etc/fail2ban/jail.local", Icon: "Warning"},
		defaultConfig: map[string]interface{}{"bantime": "3600", "findtime": "600", "maxretry": 5},
	},
	{
		App: models.App{Key: "supervisor", Name: "Supervisor", Category: "工具", Versions: "4.2", Version: "4.2", Description: "进程守护管理", Port: 0, InstallPath: "/usr/bin/supervisorctl", ConfigPath: "/etc/supervisor/supervisord.conf", Icon: "Tools"},
		defaultConfig: map[string]interface{}{"nodaemon": false},
	},
	{
		App: models.App{Key: "pm2", Name: "PM2", Category: "运行环境", Versions: "latest", Version: "latest", Description: "Node.js 进程守护，支持开机自启、日志、集群", Port: 0, InstallPath: "server/pm2", ConfigPath: "", Icon: "Platform"},
		defaultConfig: map[string]interface{}{"instances": 1, "max_memory_restart": "512M"},
	},
	{
		App: models.App{Key: "composer", Name: "Composer", Category: "运行环境", Versions: "2", Version: "2", Description: "PHP 依赖管理工具", Port: 0, InstallPath: "server/composer", ConfigPath: "", Icon: "Coffee"},
		defaultConfig: map[string]interface{}{"version": "2"},
	},
	{
		App: models.App{Key: "certbot", Name: "Certbot", Category: "工具", Versions: "latest", Version: "latest", Description: "Let's Encrypt 免费 SSL 证书申请与续期", Port: 0, InstallPath: "/usr/bin/certbot", ConfigPath: "/etc/letsencrypt", Icon: "Lock"},
		defaultConfig: map[string]interface{}{"email": "", "webroot": true},
	},
	{
		App: models.App{Key: "tomcat9", Name: "Tomcat 9", Category: "运行环境", Versions: "9.0", Version: "9.0", Description: "Apache Tomcat 9 Java Web 容器", Port: 8080, InstallPath: "/usr/share/tomcat9", ConfigPath: "/etc/tomcat9/server.xml", Icon: "CoffeeCup"},
		defaultConfig: map[string]interface{}{"port": 8080, "java_version": "17"},
	},
	{
		App: models.App{Key: "tomcat10", Name: "Tomcat 10", Category: "运行环境", Versions: "10.1", Version: "10.1", Description: "Apache Tomcat 10 Java Web 容器（Jakarta EE）", Port: 8080, InstallPath: "/usr/share/tomcat10", ConfigPath: "/etc/tomcat10/server.xml", Icon: "CoffeeCup"},
		defaultConfig: map[string]interface{}{"port": 8080, "java_version": "21"},
	},
	// ── 人工智能 ──
	{
		App: models.App{Key: "ollama", Name: "Ollama", Category: "人工智能", Versions: "latest", Version: "latest", Description: "本地大语言模型运行框架，支持 Llama/Qwen 等", Port: 11434, InstallPath: "/usr/local/bin/ollama", ConfigPath: "/etc/systemd/system/ollama.service", Icon: "Cpu"},
		defaultConfig: map[string]interface{}{"OLLAMA_HOST": "0.0.0.0:11434", "OLLAMA_MODELS": "ai/ollama/models"},
	},
	{
		App: models.App{Key: "open-webui", Name: "Open WebUI", Category: "人工智能", Versions: "latest", Version: "latest", Description: "Ollama 的 Web 对话界面，类似 ChatGPT", Port: 8080, InstallPath: "ai/open-webui", ConfigPath: "", Icon: "ChatDotRound"},
		defaultConfig: map[string]interface{}{"OLLAMA_BASE_URL": "http://127.0.0.1:11434"},
	},
	{
		App: models.App{Key: "localai", Name: "LocalAI", Category: "人工智能", Versions: "latest", Version: "latest", Description: "OpenAI 兼容 API，本地运行多种开源模型", Port: 8090, InstallPath: "ai/localai", ConfigPath: "ai/localai/config.yaml", Icon: "Connection"},
		defaultConfig: map[string]interface{}{"models_path": "ai/localai/models", "threads": 4},
	},
	{
		App: models.App{Key: "dify", Name: "Dify", Category: "人工智能", Versions: "latest", Version: "latest", Description: "LLM 应用开发平台，支持 RAG、Agent 工作流", Port: 8091, InstallPath: "ai/dify", ConfigPath: "", Icon: "Share"},
		defaultConfig: map[string]interface{}{"mode": "self-hosted"},
	},
	{
		App: models.App{Key: "jupyter", Name: "Jupyter Lab", Category: "人工智能", Versions: "4", Version: "4", Description: "交互式 AI/ML 开发 Notebook 环境", Port: 8889, InstallPath: "ai/jupyter", ConfigPath: "ai/jupyter/jupyter_lab_config.py", Icon: "Notebook"},
		defaultConfig: map[string]interface{}{"port": 8889, "ip": "0.0.0.0", "allow_root": false},
	},
	{
		App: models.App{Key: "vllm", Name: "vLLM", Category: "人工智能", Versions: "0.6", Version: "0.6", Description: "高性能 LLM 推理引擎，支持 GPU 加速", Port: 8000, InstallPath: "ai/vllm", ConfigPath: "", Icon: "Odometer"},
		defaultConfig: map[string]interface{}{"tensor_parallel_size": 1, "gpu_memory_utilization": 0.9},
	},
	{
		App: models.App{Key: "comfyui", Name: "ComfyUI", Category: "人工智能", Versions: "latest", Version: "latest", Description: "Stable Diffusion 节点式 AI 绘图工作流", Port: 8188, InstallPath: "ai/comfyui", ConfigPath: "", Icon: "Picture"},
		defaultConfig: map[string]interface{}{"listen": "0.0.0.0", "port": 8188},
	},
	{
		App: models.App{Key: "sd-webui", Name: "SD WebUI", Category: "人工智能", Versions: "latest", Version: "latest", Description: "Stable Diffusion WebUI（A1111）AI 文生图", Port: 7860, InstallPath: "ai/sd-webui", ConfigPath: "", Icon: "PictureFilled"},
		defaultConfig: map[string]interface{}{"xformers": true, "api": true},
	},
	{
		App: models.App{Key: "anythingllm", Name: "AnythingLLM", Category: "人工智能", Versions: "latest", Version: "latest", Description: "私有化 AI 文档问答与知识库助手", Port: 3001, InstallPath: "ai/anythingllm", ConfigPath: "", Icon: "Document"},
		defaultConfig: map[string]interface{}{"LLM_PROVIDER": "ollama", "EMBEDDING_ENGINE": "native"},
	},
	{
		App: models.App{Key: "fastgpt", Name: "FastGPT", Category: "人工智能", Versions: "latest", Version: "latest", Description: "国产开源 AI 知识库问答与应用编排平台", Port: 3002, InstallPath: "ai/fastgpt", ConfigPath: "", Icon: "Promotion"},
		defaultConfig: map[string]interface{}{"mongo_url": "mongodb://127.0.0.1:27017/fastgpt"},
	},
	{
		App: models.App{Key: "whisper", Name: "Whisper", Category: "人工智能", Versions: "2024", Version: "2024", Description: "OpenAI 开源语音识别（ASR）模型", Port: 0, InstallPath: "ai/whisper", ConfigPath: "", Icon: "Microphone"},
		defaultConfig: map[string]interface{}{"model": "medium", "language": "zh"},
	},
	{
		App: models.App{Key: "huggingface-ai", Name: "Hugging Face: AI", Category: "人工智能", Versions: "latest", Version: "latest", Description: "一键部署 Hugging Face TGI 推理 + Web 对话界面，自动接入面板 AI", Port: 8095, InstallPath: "ai/huggingface", ConfigPath: "", Icon: "Cpu"},
		defaultConfig: map[string]interface{}{"model_id": "Qwen/Qwen2.5-0.5B-Instruct", "tgi_port": 8095, "webui_port": 8097, "enable_chat_ui": true, "auto_configure_panel": true},
	},
	{
		App: models.App{Key: "chatchat", Name: "Langchain-Chatchat", Category: "人工智能", Versions: "0.3", Version: "0.3", Description: "基于 Langchain 的本地知识库问答（毕昇）", Port: 7861, InstallPath: "ai/chatchat", ConfigPath: "ai/chatchat/configs", Icon: "ChatLineRound"},
		defaultConfig: map[string]interface{}{"llm_model": "qwen2", "embedding_model": "bge-large-zh"},
	},
}

type Service struct {
	db      *gorm.DB
	dataDir string
	pma     PhpMyAdminActions

	catalogMu       sync.RWMutex
	catalogSyncedAt time.Time

	statusMu    sync.RWMutex
	statusCache map[string]statusEntry

	reconcileMu     sync.Mutex
	lastReconcileAt time.Time

	postInstallHook func(key string) error
	ws              WebServerHooks
	mailStack       MailStackActions
}

// PhpMyAdminActions is wired from the API layer to avoid import cycles.
type PhpMyAdminActions interface {
	Start(installPath string, port int) error
	Stop() error
	Status(port int) string
}

var installService *Service

func NewService(db *gorm.DB, dataDir string) *Service {
	initInstallLogs(dataDir)
	s := &Service{db: db, dataDir: dataDir}
	loadDynamicCatalog(dataDir)
	ensureDynamicPHPCatalog(dataDir)
	installService = s
	s.ensureCatalog()
	return s
}

func (s *Service) SetPhpMyAdminActions(a PhpMyAdminActions) {
	s.pma = a
}

func (s *Service) SetPostInstallHook(h func(key string) error) {
	s.postInstallHook = h
}

func (s *Service) AppendInstallLog(key, line string) {
	if globalInstallLogs != nil {
		globalInstallLogs.AppendLine(key, line)
	}
}

func (s *Service) WaitInstall(key string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		app, err := s.Get(key)
		if err != nil {
			return err
		}
		if app.Status != "installing" {
			if app.Status == "failed" {
				if app.InstallError != "" {
					return errors.New(app.InstallError)
				}
				return errors.New("installation failed")
			}
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("installation timeout for %s", key)
}

func (s *Service) InstallStack(keys []string) {
	s.installStackKeys(keys, "")
}

func (s *Service) InstallStackNamed(stackKey string) error {
	def, ok := stack.Get(stackKey)
	if !ok {
		return fmt.Errorf("unknown stack: %s", stackKey)
	}
	s.installStackKeys(def.Components, stackKey)
	return nil
}

func (s *Service) installStackKeys(keys []string, stackName string) {
	go func() {
		label := stackName
		if label == "" {
			label = "custom"
		}
		log.Printf("[appstore] stack %s started: %v", label, keys)
		for _, key := range keys {
			if stackName != "" {
				s.AppendInstallLog(key, fmt.Sprintf("[套件 %s] 正在安装组件 %s …", stackName, key))
			}
			app, err := s.Get(key)
			if err != nil {
				continue
			}
			if app.Installed && !IsSimulatedInstall(key, s.dataDir) && app.Status != "failed" {
				continue
			}
			if app.Status == "installing" {
				_ = s.WaitInstall(key, 20*time.Minute)
				continue
			}
			if err := s.Install(key, ""); err != nil {
				log.Printf("[appstore] stack install %s: %v", key, err)
				continue
			}
			if err := s.WaitInstall(key, 20*time.Minute); err != nil {
				log.Printf("[appstore] stack wait %s: %v", key, err)
			}
		}
	}()
}

func (s *Service) List() ([]models.App, error) {
	s.ensureCatalog()
	var apps []models.App
	if err := s.db.Find(&apps).Error; err != nil {
		return apps, err
	}
	apps = filterCatalogApps(apps)
	if len(apps) == 0 && len(mergedCatalog()) > 0 {
		var total int64
		_ = s.db.Model(&models.App{}).Count(&total)
		if total > 0 || dbmigrate.AppsHasLegacyKeyColumn(s.db) {
			dbmigrate.MigrateAppsKeyColumn(s.db)
			s.forceResyncCatalog()
			apps = nil
			if err := s.db.Find(&apps).Error; err != nil {
				return apps, err
			}
			apps = filterCatalogApps(apps)
		}
	}
	SortAppsByCategory(apps)
	return apps, nil
}

func (s *Service) ListInstalledNoSync() ([]models.App, error) {
	s.reconcileIfDue()
	var apps []models.App
	return apps, s.db.Where("installed = ?", true).Order("category, name").Find(&apps).Error
}

func (s *Service) ListInstalled() ([]models.App, error) {
	s.ensureCatalog()
	s.reconcileIfDue()
	var apps []models.App
	return apps, s.db.Where("installed = ?", true).Order("category, name").Find(&apps).Error
}

func (s *Service) SyncInstalledStatuses() {
	s.reconcileIfDue()
	var apps []models.App
	if err := s.db.Where("installed = ?", true).Order("category, name").Find(&apps).Error; err != nil {
		return
	}
	for _, app := range apps {
		if app.Status == "simulated" || IsSimulatedInstall(app.Key, s.dataDir) {
			continue
		}
		live := s.LiveStatus(app.Key)
		if live != app.Status {
			_ = s.db.Model(&app).Update("status", live).Error
		}
	}
}

func (s *Service) SyncCatalog() int {
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	s.syncCatalogLocked()
	s.catalogSyncedAt = time.Now()
	var n int64
	s.db.Model(&models.App{}).Count(&n)
	return int(n)
}

type PHPVersionInfo struct {
	Key         string `json:"key"`
	Version     string `json:"version"`
	Status      string `json:"status"`
	Default     bool   `json:"default"`
	Port        int    `json:"port"`
	PID         int    `json:"pid"`
	Mode        string `json:"mode"`
	Binary      string `json:"binary"`
	InstallPath string `json:"install_path,omitempty"`
	Message     string `json:"message,omitempty"`
	Installed   bool   `json:"installed"`
}

func (s *Service) ListPHPVersions() []PHPVersionInfo {
	s.ensureCatalog()
	mgr := php.NewManager(s.dataDir)
	var out []PHPVersionInfo
	for _, item := range mergedCatalog() {
		if !strings.HasPrefix(item.Key, "php") || item.Key == "phpmyadmin" {
			continue
		}
		if !s.phpVersionInstalledForListing(item.Key) {
			continue
		}
		st := mgr.Status(item.Key)
		status := "stopped"
		if st.Running {
			status = "running"
		}
		binary := st.Binary
		if binary == "" {
			binary = phpBinaryForKey(item.Key, s.dataDir)
		}
		out = append(out, PHPVersionInfo{
			Key: item.Key, Version: item.Version, Status: status,
			Default: item.Key == "php83", Port: st.Port, PID: st.PID,
			Mode: st.Mode, Binary: binary, InstallPath: binary,
			Message: st.Message, Installed: true,
		})
	}
	return out
}

func (s *Service) Get(key string) (*models.App, error) {
	var app models.App
	if err := s.db.Where("app_key = ?", key).First(&app).Error; err != nil {
		return nil, err
	}
	return &app, nil
}

func (s *Service) MarkInstalled(key, version string) error {
	app, err := s.Get(key)
	if err != nil {
		return err
	}
	if version == "" {
		version = app.Version
	}
	if version == "" {
		version = "latest"
	}
	status := s.detectAppStatus(key)
	return s.db.Model(app).Updates(map[string]interface{}{
		"installed":     true,
		"status":        status,
		"version":       version,
		"install_error": "",
	}).Error
}

func (s *Service) Install(key, version string) error {
	app, err := s.Get(key)
	if err != nil {
		return err
	}
	if app.Installed {
		if key == "certbot" && !CertbotInstalled(s.dataDir) {
			_ = s.db.Model(app).Updates(map[string]interface{}{
				"installed": false, "status": "stopped", "install_error": "",
			}).Error
			app.Installed = false
		} else {
			return errors.New("software already installed")
		}
	}
	if app.Status == "installing" {
		return errors.New("installation already in progress")
	}
	if version != "" {
		if !versionAllowed(app.Versions, version) {
			return fmt.Errorf("unsupported version: %s", version)
		}
		app.Version = version
	} else if app.Version == "" {
		app.Version = firstVersion(app.Versions)
	}

	installVersion := app.Version
	s.db.Model(app).Updates(map[string]interface{}{
		"status":        "installing",
		"version":       installVersion,
		"install_error": "",
	})

	go s.runInstallTask(key, installVersion)
	return nil
}

func (s *Service) Upgrade(key, version string) error {
	app, err := s.Get(key)
	if err != nil {
		return err
	}
	if !app.Installed {
		return s.Install(key, version)
	}
	if app.Status == "installing" {
		return errors.New("installation already in progress")
	}
	target := version
	if target == "" {
		target = firstVersion(app.Versions)
	}
	if target == "" {
		return errors.New("no version available")
	}
	if !versionAllowed(app.Versions, target) {
		return fmt.Errorf("unsupported version: %s", target)
	}
	if target == app.Version {
		return errors.New("already on this version")
	}
	_ = s.ServiceAction(key, "stop")
	s.db.Model(app).Updates(map[string]interface{}{
		"status":        "installing",
		"version":       target,
		"install_error": "",
	})
	go s.runUpgradeTask(key, target)
	return nil
}

func (s *Service) GetInstallLogs(key string) InstallLogSnapshot {
	if globalInstallLogs == nil {
		return InstallLogSnapshot{Key: key, Status: "idle", Lines: []string{}}
	}
	snap := globalInstallLogs.Snapshot(key)
	if snap.Status == "idle" && len(snap.Lines) > 0 {
		last := snap.Lines[len(snap.Lines)-1]
		if strings.Contains(last, "安装成功") {
			snap.Status = "success"
		} else if strings.Contains(last, "安装失败") {
			snap.Status = "failed"
		}
	}
	app, err := s.Get(key)
	if err != nil {
		return snap
	}
	if snap.Status == "installing" && app.Status != "installing" {
		globalInstallLogs.ClearSession(key)
		snap = globalInstallLogs.Snapshot(key)
	}
	if snap.Status == "idle" {
		switch app.Status {
		case "installing":
			snap.Status = "installing"
		case "failed":
			snap.Status = "failed"
			snap.InstallError = app.InstallError
		default:
			if app.Installed {
				snap.Status = "success"
			}
		}
	}
	return snap
}

func (s *Service) runInstallTask(key, version string) {
	s.doInstallTask(key, version, false)
}

func (s *Service) runUpgradeTask(key, version string) {
	s.doInstallTask(key, version, true)
}

func (s *Service) doInstallTask(key, version string, isUpgrade bool) {
	app, err := s.Get(key)
	if err != nil {
		return
	}

	if globalInstallLogs != nil {
		globalInstallLogs.Begin(key, version, app.Name)
	}
	done := installLogScope(key)
	defer done()

	installErr := runSystemInstall(key, version, app.InstallPath, s.dataDir)
	if globalInstallLogs != nil {
		globalInstallLogs.Finish(key, installErr)
	}
	if installErr != nil {
		action := "install"
		if isUpgrade {
			action = "upgrade"
		}
		log.Printf("[appstore] %s %s failed: %v", action, key, installErr)
		updates := map[string]interface{}{
			"status":        "failed",
			"install_error": installErr.Error(),
		}
		if !isUpgrade {
			updates["installed"] = false
		}
		s.db.Model(&models.App{}).Where("app_key = ?", key).Updates(updates)
		s.InvalidateLiveStatus(key)
		return
	}

	cfg := defaultConfigFor(key)
	cfgJSON := ""
	if cfg != nil {
		b, _ := json.Marshal(cfg)
		cfgJSON = string(b)
	}
	status := s.detectAppStatus(key)
	if IsSimulatedInstall(key, s.dataDir) {
		status = "simulated"
	}

	updates := map[string]interface{}{
		"installed":     true,
		"status":        status,
		"version":       version,
		"install_error": "",
	}
	if cfgJSON != "" {
		updates["config"] = cfgJSON
	}
	s.db.Model(&models.App{}).Where("app_key = ?", key).Updates(updates)
	s.InvalidateLiveStatus(key)

	if _, ok := dockerSpec(key); ok && key != "docker" {
		s.syncDockerAppRecordIfEngineReady()
	}

	if s.postInstallHook != nil {
		if err := s.postInstallHook(key); err != nil {
			log.Printf("[appstore] post-install %s: %v", key, err)
			s.AppendInstallLog(key, "安装后配置: "+err.Error())
		}
	}
}

func (s *Service) syncDockerAppRecordIfEngineReady() {
	if !dockerEngineReady() {
		return
	}
	app, err := s.Get("docker")
	if err != nil || app.Installed {
		return
	}
	status := detectServiceStatus("docker")
	if status == "stopped" && dockerEngineReady() {
		status = "running"
	}
	_ = s.db.Model(&models.App{}).Where("app_key = ?", "docker").Updates(map[string]interface{}{
		"installed":     true,
		"status":        status,
		"install_error": "",
	}).Error
	s.InvalidateLiveStatus("docker")
}

func (s *Service) Uninstall(key string) error {
	app, err := s.Get(key)
	if err != nil {
		return err
	}
	if !app.Installed {
		return errors.New("software not installed")
	}
	if err := runSystemUninstall(key, s.dataDir); err != nil {
		return err
	}
	_ = s.RemoveProxyVhost(key)
	err = s.db.Model(&models.App{}).Where("app_key = ?", key).Updates(map[string]interface{}{
		"installed":     false,
		"status":        "stopped",
		"config":        "",
		"install_error": "",
		"bind_domain":   "",
	}).Error
	if err == nil {
		s.InvalidateLiveStatus(key)
	}
	return err
}

func (s *Service) SetStatus(key, status string) error {
	switch status {
	case "running":
		return s.ServiceAction(key, "start")
	case "stopped":
		return s.ServiceAction(key, "stop")
	default:
		return fmt.Errorf("unknown status: %s", status)
	}
}

func (s *Service) detectAppStatus(key string) string {
	s.ClearSimulatedIfRealPresent(key)
	if IsSimulatedInstall(key, s.dataDir) {
		return "simulated"
	}
	if ok, status := tryDockerStatus(key); ok {
		return status
	}
	if strings.HasPrefix(key, "php") && key != "phpmyadmin" {
		st := php.NewManager(s.dataDir).Status(key)
		if st.Running {
			return "running"
		}
		return "stopped"
	}
	if strings.HasPrefix(key, "java") {
		return detectJavaStatusForInstall(key, s.dataDir)
	}
	if strings.HasPrefix(key, "nodejs") {
		return detectNodeStatusForInstall(key, s.dataDir)
	}
	if key == "pm2" {
		if fileExists(filepath.Join(s.dataDir, "server", "pm2", ".owpanel-installed")) {
			return "running"
		}
		return detectPM2()
	}
	if key == "composer" {
		return detectComposer(s.dataDir)
	}
	if key == "certbot" {
		if fileExists(filepath.Join(s.dataDir, "server", "certbot", ".owpanel-installed")) {
			return "running"
		}
		return detectCertbot()
	}
	if key == "phpmyadmin" {
		app, err := s.Get(key)
		port := 888
		if err == nil && app.Port > 0 {
			port = app.Port
		}
		return detectPhpMyAdminStatus(s.dataDir, port)
	}
	return detectServiceStatus(key)
}

func (s *Service) PHPRuntimeStatus(key string) php.Status {
	return php.NewManager(s.dataDir).Status(key)
}

func (s *Service) ensurePHPReady(key string) error {
	app, err := s.Get(key)
	if err != nil {
		return err
	}
	if app.Installed {
		return nil
	}
	if !phpPanelInstalled(key, s.dataDir) {
		return errors.New("software not installed")
	}
	st := php.NewManager(s.dataDir).Status(key)
	if st.Binary == "" {
		return errors.New("software not installed")
	}
	status := "stopped"
	if st.Running {
		status = "running"
	}
	return s.db.Model(app).Updates(map[string]interface{}{
		"installed":     true,
		"status":        status,
		"version":       php.VersionFromKey(key),
		"install_error": "",
	}).Error
}

func (s *Service) ServiceAction(key, action string) error {
	defer s.InvalidateLiveStatus(key)
	app, err := s.Get(key)
	if err != nil {
		return err
	}
	if strings.HasPrefix(key, "php") && key != "phpmyadmin" {
		if err := s.ensurePHPReady(key); err != nil {
			return err
		}
		app, _ = s.Get(key)
	} else if !app.Installed {
		return errors.New("software not installed")
	}
	if app.Status == "simulated" || IsSimulatedInstall(key, s.dataDir) {
		return errors.New("模拟安装，无法执行服务操作")
	}

	if key == "phpmyadmin" && s.pma != nil {
		port := app.Port
		if port <= 0 {
			port = 888
		}
		var actErr error
		switch action {
		case "start", "restart", "reload":
			actErr = s.pma.Start(app.InstallPath, port)
		case "stop":
			actErr = s.pma.Stop()
		default:
			return fmt.Errorf("unknown action: %s", action)
		}
		if actErr != nil {
			return actErr
		}
		return s.db.Model(app).Update("status", s.pma.Status(port)).Error
	}

	if strings.HasPrefix(key, "php") && key != "phpmyadmin" {
		mgr := php.NewManager(s.dataDir)
		var actErr error
		switch action {
		case "start":
			actErr = mgr.Start(key)
		case "stop":
			actErr = mgr.Stop(key)
		case "restart", "reload":
			actErr = mgr.Restart(key)
		default:
			return fmt.Errorf("unknown action: %s", action)
		}
		if actErr != nil {
			return actErr
		}
		status := "stopped"
		if mgr.Status(key).Running {
			status = "running"
		}
		return s.db.Model(app).Update("status", status).Error
	}

	if strings.HasPrefix(key, "java") {
		status := detectJavaStatusForInstall(key, s.dataDir)
		if action == "stop" {
			status = "stopped"
		}
		return s.db.Model(app).Update("status", status).Error
	}

	if key == "pm2" || key == "composer" || key == "certbot" {
		status := s.detectAppStatus(key)
		if action == "stop" {
			status = "stopped"
		}
		return s.db.Model(app).Update("status", status).Error
	}

	if err := runServiceAction(key, action, s.dataDir); err != nil {
		return err
	}
	status := detectServiceStatus(key)

	if (key == "nginx" || key == "openresty" || key == "apache") && (action == "start" || action == "restart") {
		s.stopOtherWebServer(key)
		s.setActiveWebServer(key)
	}

	return s.db.Model(app).Update("status", status).Error
}

func (s *Service) stopOtherWebServer(current string) {
	for _, key := range []string{"nginx", "openresty", "apache"} {
		if key == current {
			continue
		}
		otherApp, err := s.Get(key)
		if err != nil || !otherApp.Installed {
			continue
		}
		if s.detectAppStatus(key) == "running" {
			_ = runServiceAction(key, "stop", s.dataDir)
			_ = s.db.Model(otherApp).Update("status", "stopped")
		}
	}
}

func (s *Service) setActiveWebServer(key string) {
	s.db.Where("key = ?", "active_web_server").Assign(models.PanelSetting{Value: key}).FirstOrCreate(&models.PanelSetting{Key: "active_web_server"})
}

func (s *Service) GetActiveWebServer() string {
	var row models.PanelSetting
	if s.db.Where("key = ?", "active_web_server").First(&row).Error != nil || row.Value == "" {
		return "nginx"
	}
	return row.Value
}

func (s *Service) GetConfig(key string) (map[string]interface{}, error) {
	app, err := s.Get(key)
	if err != nil {
		return nil, err
	}
	cfg := map[string]interface{}{}
	if app.Config != "" {
		if err := json.Unmarshal([]byte(app.Config), &cfg); err != nil {
			return nil, err
		}
	}
	def := defaultConfigFor(key)
	for k, v := range def {
		if _, ok := cfg[k]; !ok {
			cfg[k] = v
		}
	}
	if IsPHPKey(key) {
		mgr := php.NewManager(s.dataDir)
		for _, k := range []string{"memory_limit", "upload_max_filesize", "post_max_size", "max_execution_time", "date.timezone", "open_basedir"} {
			if val := mgr.GetDirective(key, k); val != "" {
				cfg[k] = val
			}
		}
		if val := mgr.GetDisableFunctions(key); val != "" {
			cfg["disable_functions"] = val
		}
	}
	meta, _ := s.ConfigMeta(key)
	if meta.ResolvedConfigPath != "" && detectConfigKind(meta.ResolvedConfigPath, key) == "env" {
		if envCfg := loadEnvConfig(meta.ResolvedConfigPath); envCfg != nil {
			for k, v := range envCfg {
				if _, ok := cfg[k]; !ok {
					cfg[k] = v
				}
			}
		}
	}
	return cfg, nil
}

func (s *Service) UpdateConfig(key string, cfg map[string]interface{}) error {
	app, err := s.Get(key)
	if err != nil {
		return err
	}
	if !app.Installed {
		return errors.New("software not installed")
	}
	apply := map[string]interface{}{}
	skipKeys := map[string]bool{"disable_functions": true}
	for k, v := range cfg {
		if skipKeys[k] {
			continue
		}
		apply[k] = v
	}
	if IsPHPKey(key) {
		if err := php.NewManager(s.dataDir).ApplyDirectives(key, apply); err != nil {
			return fmt.Errorf("apply php.ini: %w", err)
		}
		if df, ok := cfg["disable_functions"]; ok {
			if err := php.NewManager(s.dataDir).SetDisableFunctions(key, fmt.Sprint(df)); err != nil {
				return fmt.Errorf("apply disable_functions: %w", err)
			}
		}
		_ = s.ServiceAction(key, "restart")
	} else if err := s.applyCommonConfigToFile(key, apply); err != nil {
		return fmt.Errorf("apply config file: %w", err)
	} else if len(apply) > 0 {
		action := "reload"
		if _, ok := dockerSpec(key); ok {
			action = "restart"
		}
		_ = s.ServiceAction(key, action)
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return s.db.Model(app).Update("config", string(b)).Error
}

type SettingsPatch struct {
	Port       *int   `json:"port"`
	AutoStart  *bool  `json:"auto_start"`
	Version    string `json:"version"`
	BindDomain *string `json:"bind_domain"`
}

func (s *Service) UpdateSettings(key string, patch SettingsPatch) error {
	app, err := s.Get(key)
	if err != nil {
		return err
	}
	updates := map[string]interface{}{}
	domainChanged := false
	portChanged := false
	if patch.Port != nil {
		updates["port"] = *patch.Port
		portChanged = *patch.Port != app.Port
	}
	if patch.AutoStart != nil {
		updates["auto_start"] = *patch.AutoStart
	}
	if patch.Version != "" {
		if !versionAllowed(app.Versions, patch.Version) {
			return fmt.Errorf("unsupported version: %s", patch.Version)
		}
		updates["version"] = patch.Version
	}
	if patch.BindDomain != nil {
		domain := domaincheck.HostOnly(*patch.BindDomain)
		if domain != "" {
			if err := domaincheck.AssertAvailable(s.db, []string{domain}, domaincheck.Scope{IgnoreAppKey: key}); err != nil {
				return err
			}
		}
		updates["bind_domain"] = domain
		domainChanged = domain != app.BindDomain
	}
	if len(updates) == 0 {
		return nil
	}
	if err := s.db.Model(app).Updates(updates).Error; err != nil {
		return err
	}
	if domainChanged || (portChanged && app.BindDomain != "") {
		app, _ = s.Get(key)
		if app.BindDomain != "" {
			return s.ApplyProxyVhost(key)
		}
		return s.RemoveProxyVhost(key)
	}
	return nil
}

func versionAllowed(versions, v string) bool {
	for _, item := range strings.Split(versions, ",") {
		if strings.TrimSpace(item) == v {
			return true
		}
	}
	return false
}

func firstVersion(versions string) string {
	parts := strings.Split(versions, ",")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func LatestVersion(versions string) string {
	return firstVersion(versions)
}

func defaultConfigFor(key string) map[string]interface{} {
	for _, item := range mergedCatalog() {
		if item.Key == key {
			return item.defaultConfig
		}
	}
	return nil
}

func (s *Service) ensureCatalog() {
	s.catalogMu.RLock()
	if !s.catalogSyncedAt.IsZero() && time.Since(s.catalogSyncedAt) < 5*time.Minute {
		s.catalogMu.RUnlock()
		return
	}
	s.catalogMu.RUnlock()

	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	if !s.catalogSyncedAt.IsZero() && time.Since(s.catalogSyncedAt) < 5*time.Minute {
		return
	}
	if dbmigrate.AppsHasLegacyKeyColumn(s.db) {
		dbmigrate.MigrateAppsKeyColumn(s.db)
	}
	s.syncCatalogLocked()
	var count int64
	_ = s.db.Model(&models.App{}).Count(&count)
	if count == 0 {
		s.syncCatalogLocked()
	}
	s.catalogSyncedAt = time.Now()
}

func (s *Service) forceResyncCatalog() {
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	s.catalogSyncedAt = time.Time{}
	s.syncCatalogLocked()
	s.catalogSyncedAt = time.Now()
}

func (s *Service) findCatalogApp(key string) (models.App, error) {
	var existing models.App
	err := s.db.Unscoped().Where("app_key = ?", key).First(&existing).Error
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return existing, err
	}
	if dbmigrate.AppsHasLegacyKeyColumn(s.db) {
		err = s.db.Unscoped().Where(`"key" = ?`, key).First(&existing).Error
		if err == nil {
			return existing, nil
		}
	}
	return existing, err
}

func (s *Service) syncCatalogLocked() {
	for _, item := range mergedCatalog() {
		existing, err := s.findCatalogApp(item.Key)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			a := item.App
			a.Category = NormalizeCategory(a.Category)
			if err := s.db.Create(&a).Error; err != nil {
				log.Printf("[appstore] seed %s: %v", item.Key, err)
			}
			continue
		}
		if err != nil {
			log.Printf("[appstore] lookup %s: %v", item.Key, err)
			continue
		}
		s.db.Unscoped().Model(&existing).Updates(map[string]interface{}{
			"app_key":      item.Key,
			"name":         item.Name,
			"category":     NormalizeCategory(item.Category),
			"versions":     item.Versions,
			"description":  item.Description,
			"port":         item.Port,
			"install_path": item.InstallPath,
			"config_path":  item.ConfigPath,
			"icon":         item.Icon,
			"deleted_at":   nil,
		})
	}
	// Remove merged/deprecated catalog entries from store listing.
	for _, key := range []string{"nodejs"} {
		s.db.Where("app_key = ? AND installed = ?", key, false).Delete(&models.App{})
	}
	s.purgeRemovedStoreApps()
	s.normalizeStoredCategories()
}

func (s *Service) purgeRemovedStoreApps() {
	allowed := make(map[string]struct{})
	for _, item := range mergedCatalog() {
		allowed[item.Key] = struct{}{}
	}
	var apps []models.App
	if err := s.db.Find(&apps).Error; err != nil {
		return
	}
	for _, app := range apps {
		if app.Installed {
			continue
		}
		if _, ok := allowed[app.Key]; ok {
			continue
		}
		s.db.Delete(&app)
	}
	// Remove stale cached external store metadata if present.
	_ = os.RemoveAll(filepath.Join(s.dataDir, "server", "apps", ".meta"))
	_ = os.RemoveAll(filepath.Join(s.dataDir, "appstore"))
}
