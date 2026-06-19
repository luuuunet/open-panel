package models

import (
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const MinPasswordLength = 8

var (
	ErrPasswordTooShort  = errors.New("password must be at least 8 characters")
	ErrPasswordTooWeak   = errors.New("password must include upper and lower case letters and a digit")
	ErrPasswordTooCommon = errors.New("password is too common or predictable")
)

type User struct {
	ID                 uint           `gorm:"primaryKey" json:"id"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`
	Username           string         `gorm:"uniqueIndex;size:64" json:"username"`
	Password           string         `json:"-"`
	Role               string         `gorm:"size:32;default:user" json:"role"`
	MustChangePassword bool           `gorm:"default:false" json:"must_change_password"`
	ParentID           *uint          `json:"parent_id"`
	Permissions        string         `gorm:"size:1024" json:"permissions"`
	DiskQuotaMB        int64          `gorm:"default:0" json:"disk_quota_mb"`
	DiskUsedMB         int64          `gorm:"default:0" json:"disk_used_mb"`
	Remark             string         `gorm:"size:255" json:"remark"`
	TotpSecret         string         `gorm:"size:1024" json:"-"`
	TotpEnabled        bool           `gorm:"default:false" json:"totp_enabled"`
}

func (u *User) SetPassword(plain string) error {
	if len(plain) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Password = string(hash)
	return nil
}

func (u *User) CheckPassword(plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(plain)) == nil
}

type Website struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
	Domain     string         `gorm:"size:255" json:"domain"`
	RootPath   string         `gorm:"size:512" json:"root_path"`
	ProjectType string        `gorm:"size:16;default:php" json:"project_type"`
	WebServer  string         `gorm:"size:16;default:nginx" json:"web_server"`
	PHP        bool           `json:"php"`
	PhpVersion string         `gorm:"size:16" json:"php_version"`
	SSL        bool           `json:"ssl"`
	ForceHTTPS bool           `gorm:"default:false" json:"force_https"`
	Port       int            `gorm:"default:80" json:"port"`
	Status     string         `gorm:"size:32;default:stopped" json:"status"`
	Remark     string         `gorm:"size:255" json:"remark"`
	Category   string         `gorm:"size:64;default:default" json:"category"`
	NginxConf  string         `gorm:"size:512" json:"nginx_conf"`
	FtpUser    string         `gorm:"size:64" json:"ftp_user,omitempty"`
	DbName     string         `gorm:"size:128" json:"db_name,omitempty"`
	DnsMode        string         `gorm:"size:16;default:manual" json:"dns_mode"`
	IndexFiles     string         `gorm:"size:255" json:"index_files"`
	RewriteRules   string         `gorm:"type:text" json:"rewrite_rules"`
	ExtraNginxConf string         `gorm:"type:text" json:"extra_nginx_conf"`
	RedirectURL    string         `gorm:"size:512" json:"redirect_url"`
	ProxyPass      string         `gorm:"size:512" json:"proxy_pass"`
	CacheEnabled   bool           `gorm:"default:false" json:"cache_enabled"`
	CacheDevMode   bool           `gorm:"default:false" json:"cache_dev_mode"`
	CacheHtmlTTL   int            `gorm:"default:0" json:"cache_html_ttl"`   // minutes, 0 = use global
	CacheStaticTTL int            `gorm:"default:0" json:"cache_static_ttl"` // hours, 0 = use global
	AccessAuthEnabled bool        `gorm:"default:false" json:"access_auth_enabled"`
	AccessAuthUser    string      `gorm:"size:64" json:"access_auth_user"`
	AccessAuthPass    string      `gorm:"size:128" json:"-"`
	AccessAllowIPs    string      `gorm:"type:text" json:"access_allow_ips"`
	AccessDenyIPs     string      `gorm:"type:text" json:"access_deny_ips"`
	TrafficLimitEnabled bool      `gorm:"default:false" json:"traffic_limit_enabled"`
	TrafficRate         string    `gorm:"size:32;default:10r/s" json:"traffic_rate"`
	TrafficBurst        int       `gorm:"default:20" json:"traffic_burst"`
	HotlinkEnabled      bool      `gorm:"default:false" json:"hotlink_enabled"`
	HotlinkAllowEmpty   bool      `gorm:"default:true" json:"hotlink_allow_empty"`
	HotlinkAllowDomains string    `gorm:"size:512" json:"hotlink_allow_domains"`
	CrossSiteProtectEnabled bool  `gorm:"default:false" json:"cross_site_protect_enabled"`
	PhpAccelEnabled         bool  `gorm:"default:false" json:"php_accel_enabled"`
	BackupStatus   string         `gorm:"size:32;default:none" json:"backup_status"`
	BackupAutoEnabled bool        `gorm:"default:false" json:"backup_auto_enabled"`
	BackupSchedule    string      `gorm:"size:64;default:0 3 * * *" json:"backup_schedule"`
	BackupKeepCount   int         `gorm:"default:5" json:"backup_keep_count"`
	BackupRemoteID    *uint       `json:"backup_remote_id"`
	ExpiresAt      *time.Time     `json:"expires_at"`
	Aliases        []WebsiteAlias `gorm:"foreignKey:WebsiteID" json:"aliases,omitempty"`
	Subdirs        []WebsiteSubdir `gorm:"foreignKey:WebsiteID" json:"subdirs,omitempty"`
	ProductAnalyticsEnabled  bool   `gorm:"default:false" json:"product_analytics_enabled"`
	ProductAnalyticsClientID string `gorm:"size:128" json:"product_analytics_client_id"`
	ProductAnalyticsAPIURL   string `gorm:"size:512" json:"product_analytics_api_url"`
}

// WebsiteSubdir 子目录绑定
type WebsiteSubdir struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	WebsiteID uint           `gorm:"index" json:"website_id"`
	Prefix    string         `gorm:"size:255" json:"prefix"`
	RootPath  string         `gorm:"size:512" json:"root_path"`
	Remark    string         `gorm:"size:255" json:"remark"`
}

type WebsiteAlias struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	WebsiteID uint           `gorm:"index" json:"website_id"`
	Domain    string         `gorm:"size:255" json:"domain"`
	Port      int            `gorm:"default:80" json:"port"`
	Type      string         `gorm:"size:16;default:alias" json:"type"`
}

type SiteCategory struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Name      string         `gorm:"uniqueIndex;size:64" json:"name"`
	Sort      int            `gorm:"default:0" json:"sort"`
}

type DatabaseInstance struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Name      string         `gorm:"size:128" json:"name"`
	Type      string         `gorm:"size:32" json:"type"`
	Host      string         `gorm:"size:128" json:"host"`
	Port      int            `json:"port"`
	Username  string         `gorm:"size:128" json:"username"`
	Password  string         `gorm:"size:256" json:"-"`
	Status    string         `gorm:"size:32;default:running" json:"status"`
	BackupStatus string      `gorm:"size:32;default:none" json:"backup_status"`
	Remark    string         `gorm:"size:255" json:"remark"`
	AllowRemote bool         `gorm:"default:false" json:"allow_remote"`
	AccessMode  string       `gorm:"size:16;default:local" json:"access_mode"`
	Charset   string         `gorm:"size:32;default:utf8mb4" json:"charset"`
	ForceSSL  bool           `gorm:"default:false" json:"force_ssl"`
	BackupAutoEnabled bool   `gorm:"default:false" json:"backup_auto_enabled"`
	BackupSchedule    string `gorm:"size:64;default:0 3 * * *" json:"backup_schedule"`
	BackupKeepCount   int    `gorm:"default:5" json:"backup_keep_count"`
	BackupRemoteID    *uint  `json:"backup_remote_id"`
	BackupOSSStorageID *uint `json:"backup_oss_storage_id"`
}

type DatabaseBackup struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	DatabaseID   uint           `gorm:"index" json:"database_id"`
	DbName       string         `gorm:"size:128" json:"db_name"`
	DbType       string         `gorm:"size:32" json:"db_type"`
	FilePath     string         `gorm:"size:1024" json:"file_path"`
	Size         int64          `json:"size"`
	Status       string         `gorm:"size:32;default:done" json:"status"`
	ErrorMsg     string         `gorm:"size:512" json:"error_msg,omitempty"`
	RemoteStatus string         `gorm:"size:32;default:none" json:"remote_status"`
	RemoteError  string         `gorm:"size:512" json:"remote_error,omitempty"`
	OSSStorageID *uint          `json:"oss_storage_id"`
	RemoteID     *uint          `json:"remote_id"`
}

// CacheSnapshot CDN 缓存存储快照（用于命中率趋势）
type CacheSnapshot struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	Domain       string    `gorm:"size:255;index" json:"domain"`
	Requests     int64     `json:"requests"`
	CachedReqs   int64     `json:"cached_requests"`
	HitRate      float64   `json:"hit_rate"`
	StorageBytes int64     `json:"storage_bytes"`
}

type CronJob struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	Name        string         `gorm:"size:128" json:"name"`
	Schedule    string         `gorm:"size:64" json:"schedule"`
	Command     string         `gorm:"size:1024" json:"command"`
	Enabled     bool           `gorm:"default:true" json:"enabled"`
	LastRunAt   *time.Time     `json:"last_run_at"`
	LastStatus  string         `gorm:"size:32" json:"last_status"`
	LastOutput  string         `gorm:"size:1024" json:"last_output,omitempty"`
	SyncStatus  string         `gorm:"size:32;default:pending" json:"sync_status"`
	SyncMessage string         `gorm:"size:512" json:"sync_message,omitempty"`
}

type FirewallRule struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
	Port       int            `json:"port"`
	Protocol   string         `gorm:"size:16;default:tcp" json:"protocol"`
	Action     string         `gorm:"size:16;default:allow" json:"action"`
	SourceIP   string         `gorm:"size:64" json:"source_ip"`
	Remark     string         `gorm:"size:255" json:"remark"`
	Applied    bool           `gorm:"default:false" json:"applied"`
	ApplyError string         `gorm:"size:512" json:"apply_error,omitempty"`
}

// UptimeMonitor HTTP 可用性监控
type UptimeMonitor struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
	Name            string         `gorm:"size:128" json:"name"`
	URL             string         `gorm:"size:512" json:"url"`
	Method          string         `gorm:"size:16;default:GET" json:"method"`
	IntervalSec     int            `gorm:"default:60" json:"interval_sec"`
	TimeoutSec      int            `gorm:"default:10" json:"timeout_sec"`
	ExpectedStatus  int            `gorm:"default:200" json:"expected_status"`
	Keyword         string         `gorm:"size:255" json:"keyword"`
	Enabled         bool           `gorm:"default:true" json:"enabled"`
	LastStatus      string         `gorm:"size:16;default:unknown" json:"last_status"`
	LastLatencyMs   int            `json:"last_latency_ms"`
	LastCheckAt     *time.Time     `json:"last_check_at"`
	LastError       string         `gorm:"size:512" json:"last_error,omitempty"`
	NotifyWebhook   string         `gorm:"size:512" json:"notify_webhook"`
	FailCount       int            `json:"fail_count"`
	UptimePercent   float64        `json:"uptime_percent"`
}

type SSLCertificate struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
	Domain     string         `gorm:"size:255;index" json:"domain"`
	SanDomains string         `gorm:"size:1024" json:"san_domains"`
	Email      string         `gorm:"size:128" json:"email"`
	Webroot    string         `gorm:"size:512" json:"webroot"`
	Provider   string         `gorm:"size:64;default:letsencrypt" json:"provider"`
	AutoRenew  bool           `gorm:"default:true" json:"auto_renew"`
	Issuer     string         `gorm:"size:255" json:"issuer"`
	ExpiresAt  *time.Time     `json:"expires_at"`
	Status     string         `gorm:"size:32;default:pending" json:"status"`
	ErrorMsg   string         `gorm:"size:512" json:"error_msg,omitempty"`
}

type App struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	Key          string         `gorm:"column:app_key;uniqueIndex;size:64" json:"key"`
	Name         string         `gorm:"size:128" json:"name"`
	Category     string         `gorm:"size:64" json:"category"`
	Version      string         `gorm:"size:32" json:"version"`
	Versions     string         `gorm:"size:512" json:"versions"`
	Description  string         `gorm:"size:512" json:"description"`
	Installed    bool           `gorm:"default:false" json:"installed"`
	Status       string         `gorm:"size:32;default:stopped" json:"status"`
	InstallError string         `gorm:"size:1024" json:"install_error,omitempty"`
	Port        int            `json:"port"`
	InstallPath string         `gorm:"size:512" json:"install_path"`
	ConfigPath  string         `gorm:"size:512" json:"config_path"`
	Config      string         `gorm:"type:text" json:"config"`
	AutoStart    bool           `gorm:"default:true" json:"auto_start"`
	WatchEnabled bool           `gorm:"default:false" json:"watch_enabled"`
	AutoRestart  bool           `gorm:"default:false" json:"auto_restart"`
	Icon         string         `gorm:"size:32" json:"icon"`
	IconURL     string         `gorm:"size:512" json:"icon_url,omitempty"`
	BindDomain  string         `gorm:"size:255" json:"bind_domain,omitempty"`
	Meta        string         `gorm:"type:text" json:"meta,omitempty"`
}

type FTPAccount struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Username  string         `gorm:"uniqueIndex;size:64" json:"username"`
	Password  string         `json:"-"`
	Path      string         `gorm:"size:512" json:"path"`
	Status    string         `gorm:"size:32;default:enabled" json:"status"`
	Synced    bool           `gorm:"default:false" json:"synced"`
	SyncError string         `gorm:"size:512" json:"sync_error,omitempty"`
}

type BackupTask struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	Name         string         `gorm:"size:128" json:"name"`
	Type         string         `gorm:"size:32" json:"type"`
	Target       string         `gorm:"size:512" json:"target"`
	Schedule     string         `gorm:"size:64" json:"schedule"`
	Enabled      bool           `gorm:"default:true" json:"enabled"`
	LastRun      *time.Time     `json:"last_run"`
	LastStatus   string         `gorm:"size:32" json:"last_status"`
	LastError    string         `gorm:"size:512" json:"last_error,omitempty"`
	WebsiteID    *uint          `json:"website_id"`
	DatabaseID   *uint          `json:"database_id"`
	RemoteID     *uint          `json:"remote_id"`
	OSSStorageID *uint          `json:"oss_storage_id"`
}

// WebsiteBackup 站点备份记录
type WebsiteBackup struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	WebsiteID    uint           `gorm:"index" json:"website_id"`
	Domain       string         `gorm:"size:255" json:"domain"`
	FilePath     string         `gorm:"size:1024" json:"file_path"`
	Size         int64          `json:"size"`
	Status       string         `gorm:"size:32;default:done" json:"status"`
	RemoteStatus string         `gorm:"size:32;default:none" json:"remote_status"`
	RemoteError  string         `gorm:"size:512" json:"remote_error,omitempty"`
	RemoteID     *uint          `json:"remote_id"`
}

// BackupRemote 远程备份目标（FTP / SFTP / WebDAV / Google Drive / OneDrive 等）
type BackupRemote struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
	Name       string         `gorm:"size:128" json:"name"`
	Provider   string         `gorm:"size:32" json:"provider"`
	Host       string         `gorm:"size:255" json:"host"`
	Port       int            `gorm:"default:21" json:"port"`
	Username   string         `gorm:"size:128" json:"username"`
	Password   string         `gorm:"size:512" json:"-"`
	RemotePath string         `gorm:"size:512" json:"remote_path"`
	AccessToken string        `gorm:"size:2048" json:"-"`
	ExtraConfig string         `gorm:"type:text" json:"extra_config"`
	Enabled    bool           `gorm:"default:true" json:"enabled"`
	OSSStorageID *uint        `json:"oss_storage_id"`
}

type SSHKey struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
	Name       string         `gorm:"size:128" json:"name"`
	PublicKey  string         `gorm:"size:2048" json:"public_key,omitempty"`
	PrivateKey string         `gorm:"type:text" json:"-"`
	Remark     string         `gorm:"size:255" json:"remark"`
	HasPrivate bool           `gorm:"-" json:"has_private"`
}

type PanelSetting struct {
	ID    uint   `gorm:"primaryKey" json:"id"`
	Key   string `gorm:"uniqueIndex;size:64" json:"key"`
	Value string `gorm:"size:1024" json:"value"`
}

type ComposeApp struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Name      string         `gorm:"size:128" json:"name"`
	Path      string         `gorm:"size:512" json:"path"`
	Status    string         `gorm:"size:32;default:stopped" json:"status"`
}

// DockerContainerBinding maps a container to a domain via Nginx reverse proxy.
type DockerContainerBinding struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
	ContainerID   string         `gorm:"uniqueIndex;size:128" json:"container_id"`
	ContainerName string         `gorm:"size:255" json:"container_name"`
	Domain        string         `gorm:"uniqueIndex;size:255" json:"domain"`
	HostPort      int            `json:"host_port"`
	SSL           bool           `gorm:"default:false" json:"ssl"`
}

type WAFRule struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Name      string         `gorm:"size:128" json:"name"`
	Type      string         `gorm:"size:32" json:"type"` // uri, header, ua, sql, xss, path, custom
	Pattern   string         `gorm:"size:512" json:"pattern"`
	Action    string         `gorm:"size:16;default:block" json:"action"`
	Enabled   bool           `gorm:"default:true" json:"enabled"`
}

// SecurityConfig Nginx 安全策略（六大模块）
type SecurityConfig struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Scope     string         `gorm:"uniqueIndex;size:32;default:global" json:"scope"`

	RateLimitEnabled bool   `gorm:"default:true" json:"rate_limit_enabled"`
	RateLimitRate    string `gorm:"size:32;default:10r/s" json:"rate_limit_rate"`
	RateLimitBurst   int    `gorm:"default:20" json:"rate_limit_burst"`
	RateLimitNodelay bool   `gorm:"default:true" json:"rate_limit_nodelay"`

	ConnLimitEnabled bool `gorm:"default:true" json:"conn_limit_enabled"`
	ConnLimitPerIP   int  `gorm:"default:50" json:"conn_limit_per_ip"`

	GeoBlockEnabled  bool   `gorm:"default:false" json:"geo_block_enabled"`
	GeoMode          string `gorm:"size:16;default:block" json:"geo_mode"` // block=拒绝列表, allow=仅允许列表
	BlockedCountries string `gorm:"size:512" json:"blocked_countries"`     // ISO 3166-1 alpha-2, comma separated
	GeoDbPath        string `gorm:"size:512" json:"geo_db_path"`           // optional GeoLite2-Country.mmdb path

	BlacklistEnabled bool `gorm:"default:true" json:"blacklist_enabled"`
	WhitelistEnabled bool `gorm:"default:false" json:"whitelist_enabled"`

	AllowSearchBots  bool `gorm:"default:true" json:"allow_search_bots"`
	BlockHeadlessBots bool `gorm:"default:true" json:"block_headless_bots"`
	BlockHttpMethods string `gorm:"size:128;default:TRACE,TRACK,DEBUG,CONNECT" json:"block_http_methods"`

	SlowAttackEnabled      bool `gorm:"default:true" json:"slow_attack_enabled"`
	ClientBodyTimeoutSec   int  `gorm:"default:12" json:"client_body_timeout_sec"`
	ClientHeaderTimeoutSec int  `gorm:"default:12" json:"client_header_timeout_sec"`

	ApiRateLimitEnabled bool   `gorm:"default:false" json:"api_rate_limit_enabled"`
	ApiRateLimitRate    string `gorm:"size:32;default:30r/s" json:"api_rate_limit_rate"`
	ApiRateLimitBurst   int    `gorm:"default:60" json:"api_rate_limit_burst"`

	HotlinkEnabled      bool   `gorm:"default:false" json:"hotlink_enabled"`
	HotlinkAllowEmpty   bool   `gorm:"default:true" json:"hotlink_allow_empty"`
	HotlinkAllowDomains string `gorm:"size:512" json:"hotlink_allow_domains"`

	HeaderPreset string `gorm:"size:16;default:custom" json:"header_preset"` // custom|strict|balanced|none

	FilterEnabled       bool `gorm:"default:true" json:"filter_enabled"`
	BlockBadUserAgent   bool `gorm:"default:true" json:"block_bad_user_agent"`
	BlockScannerUA      bool `gorm:"default:true" json:"block_scanner_ua"`

	HeadersEnabled   bool   `gorm:"default:true" json:"headers_enabled"`
	CSP              string `gorm:"type:text" json:"csp"`
	XFrameOptions    string `gorm:"size:64;default:SAMEORIGIN" json:"x_frame_options"`
	HSTSEnabled      bool   `gorm:"default:true" json:"hsts_enabled"`
	HSTSMaxAge       int    `gorm:"default:31536000" json:"hsts_max_age"`
	XContentTypeOpts bool   `gorm:"default:true" json:"x_content_type_options"`
	ReferrerPolicy   string `gorm:"size:64;default:strict-origin-when-cross-origin" json:"referrer_policy"`

	LogFormatEnabled bool   `gorm:"default:true" json:"log_format_enabled"`
	SecurityLogPath  string `gorm:"size:512" json:"security_log_path"`
}

// CacheConfig 全局 CDN 缓存（Nginx proxy_cache / fastcgi_cache）
type CacheConfig struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Scope     string    `gorm:"uniqueIndex;size:32;default:global" json:"scope"`

	Enabled        bool   `gorm:"default:false" json:"enabled"`
	DevMode        bool   `gorm:"default:false" json:"dev_mode"`
	AutoSiteEnable bool   `gorm:"default:true" json:"auto_site_enable"`
	ProxyMaxSize   string `gorm:"size:16;default:5g" json:"proxy_max_size"`
	ProxyInactive  string `gorm:"size:16;default:60m" json:"proxy_inactive"`
	FastcgiMaxSize string `gorm:"size:16;default:2g" json:"fastcgi_max_size"`
	FastcgiInactive string `gorm:"size:16;default:30m" json:"fastcgi_inactive"`
	ZoneMemory     string `gorm:"size:16;default:100m" json:"zone_memory"`
	HtmlTTLMinutes int    `gorm:"default:5" json:"html_ttl_minutes"`
	StaticTTLHours int    `gorm:"default:168" json:"static_ttl_hours"`
	BypassCookies  string `gorm:"size:512;default:PHPSESSID|wordpress_logged_in|session" json:"bypass_cookies"`
	BypassPaths    string `gorm:"size:512;default:/admin|/wp-admin|/api/" json:"bypass_paths"`
	StaleEnabled   bool   `gorm:"default:true" json:"stale_enabled"`
	HonorOrigin    bool   `gorm:"default:false" json:"honor_origin"`
	CacheQueryString bool `gorm:"default:true" json:"cache_query_string"`
}

// CacheRule CDN 缓存规则（类似 Cloudflare Page Rules 简化版）
type CacheRule struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
	Name       string         `gorm:"size:128" json:"name"`
	Pattern    string         `gorm:"size:512" json:"pattern"` // URI 正则或前缀
	Action     string         `gorm:"size:16;default:bypass" json:"action"` // bypass | cache
	TTLMinutes int            `gorm:"default:0" json:"ttl_minutes"`
	WebsiteID  uint           `gorm:"index;default:0" json:"website_id"` // 0 = 全局
	Priority   int            `gorm:"default:100" json:"priority"`
	Enabled    bool           `gorm:"default:true" json:"enabled"`
}

type IPBlacklist struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	IP        string         `gorm:"uniqueIndex;size:64" json:"ip"`
	Reason    string         `gorm:"size:255" json:"reason"`
	Source    string         `gorm:"size:32;default:manual" json:"source"`
	Enabled   bool           `gorm:"default:true" json:"enabled"`
}

type IPWhitelist struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	IP        string         `gorm:"uniqueIndex;size:64" json:"ip"`
	Reason    string         `gorm:"size:255" json:"reason"`
	Source    string         `gorm:"size:32;default:manual" json:"source"`
	Enabled   bool           `gorm:"default:true" json:"enabled"`
}

type MailDomain struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Domain    string         `gorm:"uniqueIndex;size:255" json:"domain"`
	Mailboxes int            `gorm:"default:0" json:"mailboxes"`
	Status    string         `gorm:"size:32;default:active" json:"status"`
}

type MailBox struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Domain    string         `gorm:"size:255" json:"domain"`
	Address   string         `gorm:"uniqueIndex;size:255" json:"address"`
	Password  string         `gorm:"size:512" json:"-"`
	Maildir   string         `gorm:"size:512" json:"maildir"`
	Quota     int            `gorm:"default:1024" json:"quota"`
	Status    string         `gorm:"size:32;default:active" json:"status"`
	Synced    bool           `gorm:"default:false" json:"synced"`
	SyncError string         `gorm:"size:512" json:"sync_error,omitempty"`
}

type MailBackup struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
	Domain         string         `gorm:"size:255" json:"domain"`
	FilePath       string         `gorm:"size:512" json:"file_path"`
	Size           int64          `json:"size"`
	MailboxCount   int            `json:"mailbox_count"`
	IncludeMaildir bool           `json:"include_maildir"`
	Status         string         `gorm:"size:32;default:done" json:"status"`
	ErrorMsg       string         `gorm:"size:512" json:"error_msg,omitempty"`
}

// MailSendProvider stores outbound / bulk email provider (local Postfix or third-party API).
type MailSendProvider struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
	Name            string         `gorm:"size:128" json:"name"`
	ProviderType    string         `gorm:"size:32;index" json:"provider_type"`
	Enabled         bool           `gorm:"default:true" json:"enabled"`
	IsDefault       bool           `gorm:"default:false" json:"is_default"`
	DefaultFrom     string         `gorm:"size:255" json:"default_from"`
	DefaultFromName string         `gorm:"size:128" json:"default_from_name"`
	ConfigJSON      string         `gorm:"type:text" json:"-"`
}

// MailBulkCampaign is a batch email send job.
type MailBulkCampaign struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
	Name            string         `gorm:"size:128" json:"name"`
	ProviderID      uint           `gorm:"index" json:"provider_id"`
	FromEmail       string         `gorm:"size:255" json:"from_email"`
	FromName        string         `gorm:"size:128" json:"from_name"`
	ReplyTo         string         `gorm:"size:255" json:"reply_to"`
	Subject         string         `gorm:"size:512" json:"subject"`
	BodyHTML        string         `gorm:"type:text" json:"body_html"`
	BodyText        string         `gorm:"type:text" json:"body_text"`
	Status          string         `gorm:"size:32;default:draft;index" json:"status"`
	TotalRecipients int            `gorm:"default:0" json:"total_recipients"`
	SentCount       int            `gorm:"default:0" json:"sent_count"`
	FailedCount     int            `gorm:"default:0" json:"failed_count"`
	RatePerMinute   int            `gorm:"default:60" json:"rate_per_minute"`
	LastError       string         `gorm:"size:512" json:"last_error,omitempty"`
	StartedAt       *time.Time     `json:"started_at,omitempty"`
	FinishedAt      *time.Time     `json:"finished_at,omitempty"`
}

// MailBulkRecipient is one recipient row for a campaign.
type MailBulkRecipient struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	CampaignID uint       `gorm:"index" json:"campaign_id"`
	Email      string     `gorm:"size:255;index" json:"email"`
	Name       string     `gorm:"size:128" json:"name"`
	Status     string     `gorm:"size:32;default:pending;index" json:"status"`
	Error      string     `gorm:"size:512" json:"error,omitempty"`
	SentAt     *time.Time `json:"sent_at,omitempty"`
}

type DNSRecord struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
	Domain     string         `gorm:"size:255;index" json:"domain"`
	Type       string         `gorm:"size:16" json:"type"`
	Name       string         `gorm:"size:255" json:"name"`
	Value      string         `gorm:"size:512" json:"value"`
	TTL        int            `gorm:"default:600" json:"ttl"`
	ProviderID uint           `gorm:"index" json:"provider_id"`
	ZoneID     string         `gorm:"size:64" json:"zone_id"`
	ExternalID string         `gorm:"size:64" json:"external_id"`
	Proxied    bool           `gorm:"default:false" json:"proxied"`
	SyncStatus string         `gorm:"size:32;default:local" json:"sync_status"`
	SyncError  string         `gorm:"size:512" json:"sync_error,omitempty"`
	WebsiteID  uint           `gorm:"index" json:"website_id,omitempty"`
	Comment    string         `gorm:"size:255" json:"comment,omitempty"`
}

type DNSProviderAccount struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Name      string         `gorm:"size:128" json:"name"`
	Provider  string         `gorm:"size:32;index" json:"provider"`
	APIToken  string         `gorm:"size:512" json:"-"`
	AccessKey string         `gorm:"size:128" json:"access_key,omitempty"`
	SecretKey string         `gorm:"size:512" json:"-"`
	Extra     string         `gorm:"size:512" json:"extra,omitempty"`
	Enabled   bool           `gorm:"default:true" json:"enabled"`
	IsDefault bool           `gorm:"default:false" json:"is_default"`
}

type DNSZone struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	ProviderID  uint           `gorm:"index" json:"provider_id"`
	ZoneID      string         `gorm:"size:64;index" json:"zone_id"`
	Name        string         `gorm:"size:255;index" json:"name"`
	Status      string         `gorm:"size:32" json:"status"`
	NameServers string         `gorm:"size:1024" json:"name_servers"`
}

type WordPressSite struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	Domain       string         `gorm:"size:255" json:"domain"` // primary domain
	Path         string         `gorm:"size:512" json:"path"`
	RootPath     string         `gorm:"size:512" json:"root_path"`
	Version      string         `gorm:"size:32" json:"version"`
	PhpVersion   string         `gorm:"size:16" json:"php_version"`
	NginxVersion string         `gorm:"size:32" json:"nginx_version"`
	NginxConf    string         `gorm:"size:512" json:"nginx_conf"`
	WebsiteID    uint           `json:"website_id"`
	DatabaseID   uint           `json:"database_id"`
	DbName       string         `gorm:"size:128" json:"db_name"`
	DbUser       string         `gorm:"size:128" json:"db_user"`
	DbHost       string         `gorm:"size:128" json:"db_host"`
	DbPort       int            `json:"db_port"`
	Status       string         `gorm:"size:32;default:pending" json:"status"`
	BackupStatus string         `gorm:"size:32;default:none" json:"backup_status"`
	SSL          bool           `gorm:"default:false" json:"ssl"`
	AutoSSL      bool           `gorm:"default:false" json:"auto_ssl"`
	ForceHTTPS   bool           `gorm:"default:true" json:"force_https"`
	CloudflareCDN bool          `gorm:"default:false" json:"cloudflare_cdn"`
	SSLEmail     string         `gorm:"size:255" json:"ssl_email"`
	SSLStatus    string         `gorm:"size:32;default:none" json:"ssl_status"` // none | active | failed | skipped
	Remark       string         `gorm:"size:255" json:"remark"`
	Domains      []WordPressDomain `gorm:"foreignKey:SiteID" json:"domains,omitempty"`
	// SEO push to search engines (sitemap ping / IndexNow)
	SEOPushEnabled    bool       `gorm:"default:true" json:"seo_push_enabled"`
	SEOPushOnDeploy   bool       `gorm:"default:true" json:"seo_push_on_deploy"`
	SEOPushGoogle     bool       `gorm:"default:true" json:"seo_push_google"`
	SEOPushBing       bool       `gorm:"default:true" json:"seo_push_bing"`
	SEOPushIndexNow   bool       `gorm:"default:true" json:"seo_push_indexnow"`
	SEOPushBaidu      bool       `gorm:"default:false" json:"seo_push_baidu"`
	SEOPushYandex     bool       `gorm:"default:false" json:"seo_push_yandex"`
	IndexNowKey       string     `gorm:"size:64" json:"indexnow_key"`
	SitemapURL        string     `gorm:"size:512" json:"sitemap_url"`
	BaiduPushToken    string     `gorm:"size:128" json:"baidu_push_token"`
	LastSEOPushAt     *time.Time `json:"last_seo_push_at"`
	LastSEOPushStatus string     `gorm:"size:32" json:"last_seo_push_status"`
	LastSEOPushLog    string     `gorm:"type:text" json:"last_seo_push_log"`
}

type WordPressBackup struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	SiteID      uint           `gorm:"index" json:"site_id"`
	Domain      string         `gorm:"size:255" json:"domain"`
	FilePath    string         `gorm:"size:1024" json:"file_path"`
	Size        int64          `json:"size"`
	HasDatabase bool           `gorm:"default:false" json:"has_database"`
	DbName      string         `gorm:"size:128" json:"db_name"`
	Status      string         `gorm:"size:32;default:done" json:"status"`
	ErrorMsg    string         `gorm:"size:512" json:"error_msg,omitempty"`
}

// WordPressDomain bound domain (primary or alias) for a WP site.
type WordPressDomain struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	SiteID    uint           `gorm:"index" json:"site_id"`
	Domain    string         `gorm:"uniqueIndex;size:255" json:"domain"`
	Type      string         `gorm:"size:16;default:alias" json:"type"` // primary | alias
	SSL       bool           `gorm:"default:false" json:"ssl"`
	Enabled   bool           `gorm:"default:true" json:"enabled"`
}

type JavaProject struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Name      string         `gorm:"size:128" json:"name"`
	Domain    string         `gorm:"size:255" json:"domain"`
	Path      string         `gorm:"size:512" json:"path"`
	Port      int            `gorm:"default:8080" json:"port"`
	JavaVer   string         `gorm:"size:16" json:"java_ver"`
	TomcatKey string         `gorm:"size:32;default:tomcat9" json:"tomcat_key"`
	ContextPath string       `gorm:"size:128;default:/" json:"context_path"`
	Status    string         `gorm:"size:32;default:stopped" json:"status"`
	Remark    string         `gorm:"size:255" json:"remark"`
}

type NodeProject struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Name      string         `gorm:"size:128" json:"name"`
	Domain    string         `gorm:"size:255" json:"domain"`
	Path      string         `gorm:"size:512" json:"path"`
	Port      int            `json:"port"`
	NodeVer   string         `gorm:"size:16" json:"node_ver"`
	Status    string         `gorm:"size:32;default:stopped" json:"status"`
	Remark    string         `gorm:"size:255" json:"remark"`
}

// RuntimeProject 统一运行环境（Node / Java / Go / Python / .NET 等）
type RuntimeProject struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
	Kind          string         `gorm:"size:16;index" json:"kind"` // php|java|nodejs|go|python|dotnet
	Name          string         `gorm:"size:128" json:"name"`
	Path          string         `gorm:"size:512" json:"path"`
	Version       string         `gorm:"size:16" json:"version"`
	RunScript     string         `gorm:"size:512" json:"run_script"`
	ContainerName string         `gorm:"size:128" json:"container_name"`
	Remark        string         `gorm:"size:255" json:"remark"`
	Ports         string         `gorm:"type:text" json:"ports"`          // JSON: [{host_port,container_port,protocol}]
	EnvVars       string         `gorm:"type:text" json:"env_vars"`       // JSON: [{key,value}]
	Mounts        string         `gorm:"type:text" json:"mounts"`         // JSON: [{host,container,read_only}]
	HostMappings  string         `gorm:"type:text" json:"host_mappings"`  // JSON: [{host,ip}]
	ExternalPort  int            `json:"external_port"`
	Status        string         `gorm:"size:32;default:stopped" json:"status"`
	ContainerID   string         `gorm:"size:64" json:"container_id"`
	LegacySource  string         `gorm:"size:16" json:"legacy_source,omitempty"` // node|java when migrated from old tables
	LegacyID      uint           `json:"legacy_id,omitempty"`
}

type RuntimePort struct {
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
}

type RuntimeEnvVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type RuntimeMount struct {
	Host      string `json:"host"`
	Container string `json:"container"`
	ReadOnly  bool   `json:"read_only"`
}

type RuntimeHostMapping struct {
	Host string `json:"host"`
	IP   string `json:"ip"`
}

type AutoOpsEvent struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	AppKey    string         `gorm:"size:64;index" json:"app_key"`
	AppName   string         `gorm:"size:128" json:"app_name"`
	EventType string         `gorm:"size:32;index" json:"event_type"`
	Message   string         `gorm:"size:512" json:"message"`
	Status    string         `gorm:"size:32" json:"status"`
}

// ClusterNode 集群节点（含本机 master）
type ClusterNode struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	Name        string         `gorm:"size:128" json:"name"`
	Host        string         `gorm:"size:255" json:"host"`
	Port        int            `gorm:"default:8888" json:"port"`
	SafePath    string         `gorm:"size:64" json:"safe_path"`
	AgentToken  string         `gorm:"size:256" json:"-"`
	Role        string         `gorm:"size:32;default:worker" json:"role"` // master | worker
	IsLocal     bool           `gorm:"default:false" json:"is_local"`
	Status      string         `gorm:"size:32;default:unknown" json:"status"` // online | offline | unknown
	Tags        string         `gorm:"size:255" json:"tags"`
	Remark      string         `gorm:"size:255" json:"remark"`
	LastSeenAt  *time.Time     `json:"last_seen_at"`
	LastError   string         `gorm:"size:512" json:"last_error,omitempty"`
	Hostname    string         `gorm:"size:128" json:"hostname,omitempty"`
	CPUPercent  float64        `json:"cpu_percent"`
	MemPercent  float64        `json:"mem_percent"`
	WebsiteHost string         `gorm:"size:255" json:"website_host,omitempty"` // optional app backend host override
	WebsitePort int            `gorm:"default:80" json:"website_port"`
	// SSH 远程管理与自动搭建
	SSHHost         string  `gorm:"size:255" json:"ssh_host,omitempty"`
	SSHPort         int     `gorm:"default:22" json:"ssh_port"`
	SSHUser         string  `gorm:"size:64;default:root" json:"ssh_user"`
	SSHPassword     string  `gorm:"size:512" json:"-"`
	ProvisionRole   string  `gorm:"size:32;default:lb_backend" json:"provision_role"` // lb_backend | worker | db_slave | db_master
	ProvisionStatus string  `gorm:"size:32;default:none" json:"provision_status"`     // none | provisioning | ready | failed
	ProvisionLog    string  `gorm:"type:text" json:"provision_log,omitempty"`
	DiskPercent     float64 `json:"disk_percent"`
	Load1           float64 `json:"load1"`
	HasSSHPassword  bool    `gorm:"-" json:"has_ssh_password"`
}

// LoadBalancer 七层负载均衡
type LoadBalancer struct {
	ID               uint           `gorm:"primaryKey" json:"id"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
	Name             string         `gorm:"size:128" json:"name"`
	Domain           string         `gorm:"size:255;index" json:"domain"`
	ListenPort       int            `gorm:"default:80" json:"listen_port"`
	SSL              bool           `gorm:"default:false" json:"ssl"`
	Algorithm        string         `gorm:"size:32;default:round_robin" json:"algorithm"`
	HealthCheck      bool           `gorm:"default:true" json:"health_check"`
	HealthPath       string         `gorm:"size:255;default:/" json:"health_path"`
	HealthInterval   int            `gorm:"default:10" json:"health_interval"`
	StickySession    bool           `gorm:"default:false" json:"sticky_session"`
	WebSocket        bool           `gorm:"default:true" json:"websocket"`
	Enabled          bool           `gorm:"default:true" json:"enabled"`
	Status           string         `gorm:"size:32;default:pending" json:"status"`
	NginxConf        string         `gorm:"size:512" json:"nginx_conf,omitempty"`
	Remark           string         `gorm:"size:255" json:"remark"`
	Backends         []LoadBalancerBackend `gorm:"foreignKey:LoadBalancerID" json:"backends,omitempty"`
}

// LoadBalancerBackend 负载均衡后端
type LoadBalancerBackend struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
	LoadBalancerID uint           `gorm:"index" json:"load_balancer_id"`
	NodeID         uint           `gorm:"index;default:0" json:"node_id"`
	Host           string         `gorm:"size:255" json:"host"`
	Port           int            `gorm:"default:80" json:"port"`
	Weight         int            `gorm:"default:1" json:"weight"`
	Enabled        bool           `gorm:"default:true" json:"enabled"`
	Status         string         `gorm:"size:32;default:unknown" json:"status"`
	LastCheckAt    *time.Time     `json:"last_check_at"`
}

// ClusterWorkflow 可视化集群编排（流程图）
type ClusterWorkflow struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Name       string         `gorm:"size:128;default:default" json:"name"`
	GraphJSON  string         `gorm:"type:text" json:"graph_json"`
	Status     string         `gorm:"size:32;default:draft" json:"status"`
	LastRunAt  *time.Time     `json:"last_run_at"`
	LastRunLog string         `gorm:"type:text" json:"last_run_log"`
}

// OSSStorage 对象存储端点（本机目录 / MinIO / 阿里云 OSS / 腾讯云 COS / AWS S3 / GCS / IBM COS 等）
type OSSStorage struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	Name         string         `gorm:"size:128" json:"name"`
	Provider     string         `gorm:"size:32" json:"provider"`
	Endpoint     string         `gorm:"size:512" json:"endpoint"`
	Region       string         `gorm:"size:64" json:"region"`
	Bucket       string         `gorm:"size:128" json:"bucket"`
	AccessKey    string         `gorm:"size:256" json:"-"`
	SecretKey    string         `gorm:"size:512" json:"-"`
	LocalPath    string         `gorm:"size:512" json:"local_path"`
	PathPrefix   string         `gorm:"size:512" json:"path_prefix"`
	UsePathStyle bool           `gorm:"default:false" json:"use_path_style"`
	Enabled      bool           `gorm:"default:true" json:"enabled"`
	Remark       string         `gorm:"size:512" json:"remark"`
}

// OSSSyncTask 同步 / 迁移 / 备份任务
type OSSSyncTask struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
	Name            string         `gorm:"size:128" json:"name"`
	Mode            string         `gorm:"size:32" json:"mode"`
	SourceStorageID *uint          `json:"source_storage_id"`
	TargetStorageID *uint          `json:"target_storage_id"`
	ExtraTargetIDs  string         `gorm:"size:256" json:"extra_target_ids"`
	SourcePath      string         `gorm:"size:512" json:"source_path"`
	TargetPath      string         `gorm:"size:512" json:"target_path"`
	LocalPath       string         `gorm:"size:512" json:"local_path"`
	DeleteExtra     bool           `gorm:"default:false" json:"delete_extra"`
	Schedule        string         `gorm:"size:64" json:"schedule"`
	Enabled         bool           `gorm:"default:true" json:"enabled"`
	Running         bool           `gorm:"default:false" json:"running"`
	LastStatus      string         `gorm:"size:32;default:idle" json:"last_status"`
	LastError       string         `gorm:"size:1024" json:"last_error,omitempty"`
	LastLog           string         `gorm:"type:text" json:"last_log,omitempty"`
	LastRunAt         *time.Time     `json:"last_run_at"`
}

// SiteDeployConfig Git / CI 自动部署配置
type SiteDeployConfig struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
	WebsiteID      uint           `gorm:"uniqueIndex" json:"website_id"`
	Enabled        bool           `gorm:"default:false" json:"enabled"`
	RepoURL        string         `gorm:"size:512" json:"repo_url"`
	Branch         string         `gorm:"size:64;default:main" json:"branch"`
	DeployScript   string         `gorm:"type:text" json:"deploy_script"`
	WebhookToken   string         `gorm:"uniqueIndex;size:64" json:"webhook_token"`
	CIProvider     string         `gorm:"size:32;default:manual" json:"ci_provider"`
	WebhookSecret  string         `gorm:"size:256" json:"-"`
	ComposeAppID   *uint          `json:"compose_app_id"`
	AutoRestart    bool           `gorm:"default:true" json:"auto_restart"`
}

// SiteDeployJob 部署任务记录
type SiteDeployJob struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	WebsiteID uint           `gorm:"index" json:"website_id"`
	Trigger   string         `gorm:"size:32" json:"trigger"`
	Status    string         `gorm:"size:32" json:"status"`
	Log       string         `gorm:"type:text" json:"log"`
	Error     string         `gorm:"size:1024" json:"error,omitempty"`
	StartedAt time.Time      `json:"started_at"`
	EndedAt   *time.Time     `json:"ended_at,omitempty"`
}

// AISiteBootstrapJob AI 从 GitHub 自动建站任务
type AISiteBootstrapJob struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	RepoURL     string         `gorm:"size:512" json:"repo_url"`
	Branch      string         `gorm:"size:64" json:"branch"`
	Domain      string         `gorm:"size:256" json:"domain"`
	Status       string         `gorm:"size:32" json:"status"`
	CurrentPhase string         `gorm:"size:32" json:"current_phase"`
	Log          string         `gorm:"type:text" json:"log"`
	StepsJSON    string         `gorm:"type:text" json:"steps_json"`
	PlanJSON     string         `gorm:"type:text" json:"plan_json"`
	WebsiteID   uint           `json:"website_id"`
	DeployJobID uint           `json:"deploy_job_id"`
	Error       string         `gorm:"size:1024" json:"error,omitempty"`
	StartedAt   time.Time      `json:"started_at"`
	EndedAt     *time.Time     `json:"ended_at,omitempty"`
}

// EdgeWorker Cloudflare Workers-style edge script (OpenResty Lua / Nginx njs).
type EdgeWorker struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	Name         string         `gorm:"size:128" json:"name"`
	Description  string         `gorm:"size:512" json:"description"`
	RoutePattern string         `gorm:"size:512" json:"route_pattern"`
	ScriptType   string         `gorm:"size:16;default:lua" json:"script_type"` // lua, njs, template
	Script       string         `gorm:"type:text" json:"script"`
	WebsiteID    uint           `gorm:"default:0;index" json:"website_id"` // 0 = all sites
	Domains      string         `gorm:"size:1024" json:"domains"`          // comma-separated hostnames, e.g. ku.lulunet.cc,www.example.com; * = all
	Enabled      bool           `gorm:"default:true" json:"enabled"`
	Priority     int            `gorm:"default:100" json:"priority"`
	Triggers     string         `gorm:"size:64;default:request" json:"triggers"` // request, response (comma-separated)
	Bindings     []EdgeWorkerBinding `gorm:"foreignKey:WorkerID" json:"bindings,omitempty"`
}

// EdgeKVNamespace Workers KV-style key-value namespace.
type EdgeKVNamespace struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	Name        string         `gorm:"uniqueIndex;size:128" json:"name"`
	Description string         `gorm:"size:512" json:"description"`
}

// EdgeKVEntry key-value pair within a namespace.
type EdgeKVEntry struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	NamespaceID uint           `gorm:"index:idx_kv_ns_key,unique" json:"namespace_id"`
	Key         string         `gorm:"size:512;index:idx_kv_ns_key,unique" json:"key"`
	Value       string         `gorm:"type:text" json:"value"`
	ExpiresAt   *time.Time     `json:"expires_at,omitempty"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// EdgeD1Database edge SQLite database (Cloudflare D1-style).
type EdgeD1Database struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	Name        string         `gorm:"uniqueIndex;size:128" json:"name"`
	Description string         `gorm:"size:512" json:"description"`
	FilePath    string         `gorm:"size:512" json:"file_path"`
}

// EdgeWorkerBinding binds KV/D1/Redis/OSS resources to a worker script variable.
type EdgeWorkerBinding struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	WorkerID     uint           `gorm:"index" json:"worker_id"`
	BindingType  string         `gorm:"size:16" json:"binding_type"` // kv, d1, redis, oss
	BindingName  string         `gorm:"size:64" json:"binding_name"`
	ResourceID   uint           `json:"resource_id"`
	ResourceKey  string         `gorm:"size:512" json:"resource_key,omitempty"`
}

// PanelAuditEvent unified panel audit log for enterprise compliance.
type PanelAuditEvent struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Username  string    `gorm:"size:64;index" json:"username"`
	IP        string    `gorm:"size:64;index" json:"ip"`
	UserAgent string    `gorm:"size:512" json:"user_agent"`
	Category  string    `gorm:"size:32;index" json:"category"`
	Action    string    `gorm:"size:64;index" json:"action"`
	Resource  string    `gorm:"size:255" json:"resource"`
	Detail    string    `gorm:"type:text" json:"detail"`
	Level     string    `gorm:"size:16;index" json:"level"`
	Success   bool      `gorm:"index" json:"success"`
}

// LoginEvent records panel login attempts for audit.
type LoginEvent struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Username  string    `gorm:"size:64;index" json:"username"`
	IP        string    `gorm:"size:64;index" json:"ip"`
	UserAgent string    `gorm:"size:512" json:"user_agent"`
	Success   bool      `gorm:"index" json:"success"`
	Reason    string    `gorm:"size:64" json:"reason"`
}

// CommandSnippet 可复用的 Shell 命令片段（工具箱）
type CommandSnippet struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Name      string         `gorm:"size:128;not null" json:"name"`
	Command   string         `gorm:"size:4096;not null" json:"command"`
	Category  string         `gorm:"size:64" json:"category"`
	Remark    string         `gorm:"size:512" json:"remark"`
}

// BastionAssetGroup 堡垒机资产分组
type BastionAssetGroup struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Name      string         `gorm:"size:128;not null" json:"name"`
	ParentID  *uint          `gorm:"index" json:"parent_id,omitempty"`
	Sort      int            `gorm:"default:0" json:"sort"`
	Remark    string         `gorm:"size:255" json:"remark"`
}

// BastionAsset 堡垒机托管资产
type BastionAsset struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	Name        string         `gorm:"size:128;not null" json:"name"`
	Host        string         `gorm:"size:255;not null" json:"host"`
	Port        int            `gorm:"default:22" json:"port"`
	Protocol    string         `gorm:"size:16;default:ssh" json:"protocol"` // ssh | mysql | pgsql | redis
	Username    string         `gorm:"size:128" json:"username"`
	AuthMethod  string         `gorm:"size:16;default:password" json:"auth_method"` // password | key
	PasswordEnc string         `gorm:"size:1024" json:"-"`
	KeyID       *uint          `json:"key_id,omitempty"`
	GroupID     *uint          `gorm:"index" json:"group_id,omitempty"`
	Tags        string         `gorm:"size:255" json:"tags"`
	Remark      string         `gorm:"size:512" json:"remark"`
	NodeID      *uint          `gorm:"index" json:"node_id,omitempty"`
	HasPassword bool           `gorm:"-" json:"has_password"`
	GroupName   string         `gorm:"-" json:"group_name,omitempty"`
}

// BastionAccount 堡垒机托管账号（凭据治理）
type BastionAccount struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
	AssetID        uint           `gorm:"index;not null" json:"asset_id"`
	Username       string         `gorm:"size:128;not null" json:"username"`
	AuthMethod     string         `gorm:"size:16;default:password" json:"auth_method"` // password | key
	PasswordEnc    string         `gorm:"size:1024" json:"-"`
	KeyID          *uint          `json:"key_id,omitempty"`
	IsPrivileged   bool           `gorm:"default:false" json:"is_privileged"`
	Source         string         `gorm:"size:16;default:manual" json:"source"` // manual | discovered | pushed
	Status         string         `gorm:"size:16;default:active" json:"status"`   // active | disabled | locked
	LastLoginAt    *time.Time     `json:"last_login_at,omitempty"`
	LastRotatedAt  *time.Time     `json:"last_rotated_at,omitempty"`
	ExpiresAt      *time.Time     `json:"expires_at,omitempty"`
	AutoRotate          bool           `gorm:"default:false" json:"auto_rotate"`
	RotateAfterSession  bool           `gorm:"default:false" json:"rotate_after_session"`
	RotateDays          int            `gorm:"default:90" json:"rotate_days"`
	Remark              string         `gorm:"size:512" json:"remark"`
	HasPassword    bool           `gorm:"-" json:"has_password"`
	AssetName      string         `gorm:"-" json:"asset_name,omitempty"`
	AssetHost      string         `gorm:"-" json:"asset_host,omitempty"`
}

// BastionAccountRotationLog 账号改密记录
type BastionAccountRotationLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	AccountID uint      `gorm:"index" json:"account_id"`
	AssetID   uint      `gorm:"index" json:"asset_id"`
	Username  string    `gorm:"size:128" json:"username"`
	Status    string    `gorm:"size:16" json:"status"` // success | failed
	Message   string    `gorm:"size:1024" json:"message"`
	RotatedAt time.Time `json:"rotated_at"`
}

// BastionAccessRequest JIT 临时访问申请
type BastionAccessRequest struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	UserID        uint       `gorm:"index;not null" json:"user_id"`
	AssetID       uint       `gorm:"index;not null" json:"asset_id"`
	AccountID     *uint      `gorm:"index" json:"account_id,omitempty"`
	Reason        string     `gorm:"size:512" json:"reason"`
	DurationHours int        `gorm:"default:4" json:"duration_hours"`
	Status        string     `gorm:"size:16;default:pending" json:"status"` // pending | approved | rejected | expired
	ApprovedBy    *uint      `json:"approved_by,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	Username      string     `gorm:"-" json:"username,omitempty"`
	AssetName     string     `gorm:"-" json:"asset_name,omitempty"`
	ApproverName  string     `gorm:"-" json:"approver_name,omitempty"`
}

// BastionKnownHost SSH 已知主机密钥
type BastionKnownHost struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	AssetID     uint      `gorm:"uniqueIndex;not null" json:"asset_id"`
	Host        string    `gorm:"size:255" json:"host"`
	Port        int       `gorm:"default:22" json:"port"`
	KeyType     string    `gorm:"size:32" json:"key_type"` // ecdsa-sha2-nistp256, ssh-rsa, etc.
	PublicKey   string    `gorm:"type:text" json:"public_key"`
	Fingerprint string    `gorm:"size:128" json:"fingerprint"`
	Status      string    `gorm:"size:16;default:pending" json:"status"` // pending | accepted | rejected
	AssetName   string    `gorm:"-" json:"asset_name,omitempty"`
}

// BastionPermission 用户-资产授权
type BastionPermission struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
	UserID     uint           `gorm:"index;not null" json:"user_id"`
	AssetID    uint           `gorm:"index;not null" json:"asset_id"`
	Permission string         `gorm:"size:32;default:connect" json:"permission"` // connect | sftp | readonly
	ExpiresAt  *time.Time     `json:"expires_at,omitempty"`
	CreatedBy  uint           `json:"created_by,omitempty"`
	Username   string         `gorm:"-" json:"username,omitempty"`
	AssetName  string         `gorm:"-" json:"asset_name,omitempty"`
}

// BastionSession 堡垒机会话审计记录
type BastionSession struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
	SessionKey string         `gorm:"size:64;uniqueIndex" json:"session_key"`
	UserID     uint           `gorm:"index" json:"user_id"`
	Username   string         `gorm:"size:64" json:"username"`
	AssetID    *uint          `gorm:"index" json:"asset_id,omitempty"`
	AccountID  *uint          `gorm:"index" json:"account_id,omitempty"`
	AssetName  string         `gorm:"size:128" json:"asset_name"`
	Host       string         `gorm:"size:255" json:"host"`
	Port       int            `json:"port"`
	StartTime  time.Time      `json:"start_time"`
	EndTime    *time.Time     `json:"end_time,omitempty"`
	LogPath    string         `gorm:"size:512" json:"log_path"`
	LogSize    int64          `json:"log_size"`
	Status     string         `gorm:"size:32;default:active" json:"status"` // active | closed | killed
	Commands   string         `gorm:"type:text" json:"commands,omitempty"`  // JSON array
}

// BastionCommandAudit 危险命令拦截审计
type BastionCommandAudit struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	SessionID uint      `gorm:"index" json:"session_id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Username  string    `gorm:"size:64" json:"username"`
	Command   string    `gorm:"size:2048" json:"command"`
	Action    string    `gorm:"size:16" json:"action"` // blocked | warned
}

// OpsTemplate 自动化运维命令/Playbook 模板
type OpsTemplate struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Name      string         `gorm:"size:128;not null" json:"name"`
	Type      string         `gorm:"size:16;default:command" json:"type"`       // command | playbook
	Language  string         `gorm:"size:16;default:shell" json:"language"`     // shell | python | mysql | pgsql
	Content   string         `gorm:"type:text" json:"content"`
	Remark    string         `gorm:"size:512" json:"remark"`
	Builtin   bool           `gorm:"default:false" json:"builtin"`
}

// OpsJob 作业定义（基于模板 + 资产 + 可选定时）
type OpsJob struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
	Name       string         `gorm:"size:128;not null" json:"name"`
	TemplateID uint           `gorm:"index" json:"template_id"`
	AssetIDs   string         `gorm:"type:text" json:"asset_ids"` // JSON array of uint
	Schedule   string         `gorm:"size:64" json:"schedule"`
	TimeoutSec int            `gorm:"default:30" json:"timeout_sec"`
	Cwd        string         `gorm:"size:512" json:"cwd"`
	Enabled    bool           `gorm:"default:true" json:"enabled"`
	LastRunAt  *time.Time     `json:"last_run_at,omitempty"`
	LastStatus string         `gorm:"size:32" json:"last_status"`
	TemplateName string       `gorm:"-" json:"template_name,omitempty"`
}

// OpsJobRun 作业执行记录
type OpsJobRun struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	JobID       *uint          `gorm:"index" json:"job_id,omitempty"`
	Status      string         `gorm:"size:32;default:running" json:"status"` // running | success | partial | failed
	StartedAt   time.Time      `json:"started_at"`
	FinishedAt  *time.Time     `json:"finished_at,omitempty"`
	TriggeredBy string         `gorm:"size:64" json:"triggered_by"` // manual | cron | adhoc
	UserID      uint           `gorm:"index" json:"user_id,omitempty"`
	Username    string         `gorm:"size:64" json:"username,omitempty"`
	Command     string         `gorm:"type:text" json:"command,omitempty"`
	Language    string         `gorm:"size:16" json:"language,omitempty"`
	TimeoutSec  int            `json:"timeout_sec,omitempty"`
	AssetIDs    string         `gorm:"type:text" json:"asset_ids,omitempty"`
	JobName     string         `gorm:"-" json:"job_name,omitempty"`
	Results     []OpsJobResult `gorm:"-" json:"results,omitempty"`
}

// OpsJobResult 单资产执行结果
type OpsJobResult struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	RunID      uint      `gorm:"index;not null" json:"run_id"`
	AssetID    uint      `gorm:"index" json:"asset_id"`
	AssetName  string    `gorm:"size:128" json:"asset_name"`
	Status     string    `gorm:"size:32" json:"status"` // success | failed | timeout | blocked
	Output     string    `gorm:"type:text" json:"output"`
	ExitCode   int       `json:"exit_code"`
	DurationMs int64     `json:"duration_ms"`
}
