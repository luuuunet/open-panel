package api

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/auth"
	"github.com/luuuunet/owpanel/internal/bootstrap"
	"github.com/luuuunet/owpanel/internal/config"
	"github.com/luuuunet/owpanel/internal/middleware"
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/aichat"
	"github.com/luuuunet/owpanel/internal/services/aisite"
	"github.com/luuuunet/owpanel/internal/services/aihub"
	"github.com/luuuunet/owpanel/internal/services/analytics"
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"github.com/luuuunet/owpanel/internal/services/audit"
	"github.com/luuuunet/owpanel/internal/services/autops"
	"github.com/luuuunet/owpanel/internal/services/backup"
	"github.com/luuuunet/owpanel/internal/services/bastion"
	"github.com/luuuunet/owpanel/internal/services/cache"
	"github.com/luuuunet/owpanel/internal/services/cilium"
	"github.com/luuuunet/owpanel/internal/services/cloudhub"
	"github.com/luuuunet/owpanel/internal/services/cluster"
	"github.com/luuuunet/owpanel/internal/services/compose"
	"github.com/luuuunet/owpanel/internal/services/cron"
	"github.com/luuuunet/owpanel/internal/services/dashboard"
	dbsvc "github.com/luuuunet/owpanel/internal/services/database"
	"github.com/luuuunet/owpanel/internal/services/devops"
	"github.com/luuuunet/owpanel/internal/services/dns"
	"github.com/luuuunet/owpanel/internal/services/docker"
	"github.com/luuuunet/owpanel/internal/extension"
	"github.com/luuuunet/owpanel/internal/services/edged1"
	"github.com/luuuunet/owpanel/internal/services/edgeworker"
	"github.com/luuuunet/owpanel/internal/services/edgekv"
	"github.com/luuuunet/owpanel/internal/services/enterprise"
	"github.com/luuuunet/owpanel/internal/services/filemgr"
	"github.com/luuuunet/owpanel/internal/services/firewall"
	"github.com/luuuunet/owpanel/internal/services/ftp"
	"github.com/luuuunet/owpanel/internal/services/java"
	"github.com/luuuunet/owpanel/internal/services/k8s"
	"github.com/luuuunet/owpanel/internal/services/kafkaaccel"
	"github.com/luuuunet/owpanel/internal/services/logs"
	"github.com/luuuunet/owpanel/internal/services/mail"
	"github.com/luuuunet/owpanel/internal/services/migration"
	"github.com/luuuunet/owpanel/internal/services/nodejs"
	"github.com/luuuunet/owpanel/internal/services/runtime"
	"github.com/luuuunet/owpanel/internal/services/ossstorage"
	"github.com/luuuunet/owpanel/internal/services/phpmyadmin"
	"github.com/luuuunet/owpanel/internal/services/performance"
	"github.com/luuuunet/owpanel/internal/services/process"
	"github.com/luuuunet/owpanel/internal/services/productanalytics"
	"github.com/luuuunet/owpanel/internal/services/security"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"github.com/luuuunet/owpanel/internal/services/ssl"
	"github.com/luuuunet/owpanel/internal/services/sshmgr"
	"github.com/luuuunet/owpanel/internal/services/toolbox"
	"github.com/luuuunet/owpanel/internal/services/uptime"
	"github.com/luuuunet/owpanel/internal/services/waf"
	"github.com/luuuunet/owpanel/internal/services/website"
	"github.com/luuuunet/owpanel/internal/services/webserver"
	"github.com/luuuunet/owpanel/internal/services/wordpress"
	"gorm.io/gorm"
)

type Server struct {
	cfg         *config.Config
	db          *gorm.DB
	authSvc     *auth.Service
	dashboard   *dashboard.Service
	website     *website.Service
	database    *dbsvc.Service
	filemgr     *filemgr.Service
	docker      *docker.Service
	ssl         *ssl.Service
	firewall    *firewall.Service
	cron        *cron.Service
	appstore    *appstore.Service
	ftp         *ftp.Service
	backup      *backup.Service
	sshmgr      *sshmgr.Service
	compose     *compose.Service
	process     *process.Service
	logs        *logs.Service
	toolbox     *toolbox.Service
	settings    *settings.Service
	performance *performance.Service
	aichat      *aichat.Service
	waf         *waf.Service
	edgeworker  *edgeworker.Service
	edgekv      *edgekv.Service
	edged1      *edged1.Service
	cache       *cache.Service
	analytics   *analytics.Service
	productAnalytics *productanalytics.Service
	mail        *mail.Service
	dns         *dns.Service
	wordpress   *wordpress.Service
	nodejs      *nodejs.Service
	java        *java.Service
	runtime     *runtime.Service
	security    *security.Service
	kafkaaccel  *kafkaaccel.Service
	cilium      *cilium.Service
	k8s         *k8s.Service
	webserver   *webserver.Manager
	phpmyadmin  *phpmyadmin.Service
	autops      *autops.Service
	cloudhub    *cloudhub.Service
	cluster     *cluster.Service
	ossstorage  *ossstorage.Service
	uptime      *uptime.Service
	devops      *devops.Service
	aihub       *aihub.Service
	aisite      *aisite.Service
	extensions  *extension.Registry
	bastion     *bastion.Service
	syslog      *audit.Syslog
	migration   *migration.Service
	enterprise  *enterprise.Service

	monitorExtrasMu sync.Mutex
	monitorExtrasAt time.Time
	monitorExtras   dashboard.MonitorExtras
}

func NewServer(cfg *config.Config, db *gorm.DB) *Server {
	wafSvc := waf.NewService(db, cfg.DataDir)
	edgeWorkerSvc := edgeworker.NewService(db, cfg.DataDir)
	edgeKVSvc := edgekv.NewService(db)
	edgeD1Svc := edged1.NewService(db, cfg.DataDir)
	cacheSvc := cache.NewService(db, cfg.DataDir)
	appSvc := appstore.NewService(db, cfg.DataDir)
	go appSvc.ReconcileInstalledFromSystem()
	wsMgr := webserver.NewManager(db, cfg.DataDir, appSvc)
	pmaSvc := phpmyadmin.New(cfg.DataDir, db)
	pmaSvc.SetWebServerHooks(phpmyadmin.WebServerHooks{
		GetActive: wsMgr.GetActive,
		Reload:    wsMgr.Reload,
		EnsureInc: wsMgr.EnsureVhostInclude,
		IsRunning: func(key string) bool {
			app, err := appSvc.Get(key)
			if err != nil || !app.Installed {
				return false
			}
			return appSvc.LiveStatus(key) == "running"
		},
	})
	appSvc.SetPhpMyAdminActions(pmaSvc)
	appSvc.SetWebServerHooks(appstore.WebServerHooks{
		GetActive: wsMgr.GetActive,
		Reload:    wsMgr.Reload,
		EnsureInc: wsMgr.EnsureVhostInclude,
		IsRunning: func(key string) bool {
			app, err := appSvc.Get(key)
			if err != nil || !app.Installed {
				return false
			}
			return appSvc.LiveStatus(key) == "running"
		},
	})
	webPostInstall := func(key string) error {
		if strings.HasPrefix(key, "php") && key != "phpmyadmin" {
			_ = appSvc.ServiceAction(key, "start")
		}
		if !webserver.IsWebServerKey(key) {
			return nil
		}
		return wsMgr.Setup(key, true)
	}
	appSvc.SetPostInstallHook(webPostInstall)
	ftpSvc := ftp.NewService(db, cfg.DataDir)
	settingsSvc := settings.NewServiceWithDataDir(db, cfg.DataDir)
	edgeWorkerSvc.SetPanelInfo(cfg.Port, func() string {
		all, _ := settingsSvc.GetAll()
		sp := strings.Trim(strings.TrimSpace(all["panel_safe_path"]), "/")
		if sp == "" {
			return ""
		}
		return "/" + sp
	})
	dbSvc := dbsvc.NewService(db, cfg.DataDir, settingsSvc)
	mailSvc := mail.NewService(db, cfg.DataDir)
	mailSvc.SetSettings(settingsSvc)
	mailSvc.SetWebServerHooks(mail.WebServerHooks{
		GetActive: wsMgr.GetActive,
		Reload:    wsMgr.Reload,
		EnsureInc: wsMgr.EnsureVhostInclude,
		IsRunning: func(key string) bool {
			app, err := appSvc.Get(key)
			if err != nil || !app.Installed {
				return false
			}
			return appSvc.LiveStatus(key) == "running"
		},
	})
	appSvc.SetMailStackActions(mailSvc)
	basePostInstall := func(key string) error {
		if err := webPostInstall(key); err != nil {
			return err
		}
		switch key {
		case "mysql", "mariadb", "phpmyadmin":
			return dbSvc.EnsureMySQLRootPasswordAuth()
		case "postfix", "dovecot":
			return mailSvc.EnsureConfigured()
		case "mail-server":
			appSvc.SyncMailStackRecords(true)
			return mailSvc.EnsureConfigured()
		}
		return nil
	}
	appSvc.SetPostInstallHook(basePostInstall)
	dnsSvc := dns.NewService(db, settingsSvc)
	websiteSvc := website.NewService(db, cfg.DataDir, ftpSvc, dbSvc, dnsSvc, wsMgr, cacheSvc)
	websiteSvc.SetEdgeWorker(edgeWorkerSvc)
	wpSvc := wordpress.NewService(db, cfg.DataDir, appSvc, dbSvc, ftpSvc)
	sslSvc := ssl.NewService(db, cfg.DataDir)
	sslSvc.SetDeployHook(func(domain string) error {
		if err := wpSvc.DeploySSLForDomain(domain); err == nil {
			return nil
		}
		return websiteSvc.DeploySSLForDomain(domain)
	})
	cacheSvc.SetHooks(
		func() error { return websiteSvc.RegenerateAll() },
		func() error { return wsMgr.Reload(wsMgr.GetActive()) },
	)
	cacheSvc.SetNginxConfResolver(func() string {
		active := wsMgr.GetActive()
		app, err := appSvc.Get(active)
		if err != nil {
			return ""
		}
		return wsMgr.ResolveConfigPath(active, app.ConfigPath)
	})
	cacheSvc.SetAutoEnableHooks(appSvc, wsMgr, websiteSvc.ApplyVhost)
	edgeWorkerSvc.SetHooks(
		func() error { return websiteSvc.RegenerateAll() },
		func() error { return wsMgr.Reload(wsMgr.GetActive()) },
		func() string {
			active := wsMgr.GetActive()
			app, err := appSvc.Get(active)
			if err != nil {
				return ""
			}
			return wsMgr.ResolveConfigPath(active, app.ConfigPath)
		},
		func() string { return wsMgr.GetActive() },
	)
	perfSvc := performance.NewService(settingsSvc)
	dashSvc := dashboard.NewService(db, perfSvc)
	autoOpsSvc := autops.NewService(db, settingsSvc, appSvc, dashSvc, websiteSvc, cfg.DataDir)
	autoOpsSvc.Start()
	clusterSvc := cluster.NewService(db, cfg.DataDir, settingsSvc, dashSvc, wsMgr, perfSvc)
	clusterSvc.StartWatcher()
	ossSvc := ossstorage.NewService(db, cfg.DataDir)
	backupSvc := backup.NewService(db, cfg.DataDir, settingsSvc)
	backupSvc.SetDeps(dbSvc, ossSvc)
	dbSvc.SetOSS(ossSvc)
	dbSvc.SetRemoteUploader(backupSvc)
	cronSvc := cron.NewService(db, cfg.DataDir)
	cronSvc.SetFailureHook(func(job models.CronJob, msg string) {
		autoOpsSvc.LogCronFailure(job.Name, msg, job.ID)
	})
	cronSvc.Start()
	uptimeSvc := uptime.NewService(db, perfSvc)
	uptimeSvc.Start()
	cloudhubSvc := cloudhub.NewService(db, cfg.DataDir, autoOpsSvc, backupSvc, uptimeSvc, ossSvc)
	composeSvc := compose.NewService(db)
	devopsSvc := devops.NewService(db, cfg.DataDir, composeSvc, wsMgr, appSvc, settingsSvc)
	aihubSvc := aihub.NewService(cfg.DataDir, appSvc, settingsSvc)
	aichatSvc := aichat.NewService(settingsSvc)
	aisiteSvc := aisite.NewService(db, cfg.DataDir, websiteSvc, devopsSvc, aichatSvc, appSvc, settingsSvc, nodejs.NewService(db, cfg.DataDir), cronSvc)
	extReg := extension.NewRegistry(cfg.DataDir)
	appstore.SetExtensionCatalogLoader(func() []models.App { return extReg.CatalogApps() })
	sshSvc := sshmgr.NewService(db)
	bastionSvc := bastion.NewService(db, cfg.DataDir, cfg.JWTSecret, sshSvc)
	syslogSvc := audit.NewSyslog(settingsSvc)
	securitySvc := security.NewService(wafSvc, appSvc, settingsSvc, db)
	enterpriseSvc := enterprise.NewService(db, settingsSvc, clusterSvc, dashSvc, securitySvc, uptimeSvc, syslogSvc)
	enterpriseSvc.StartRetentionJob()
	bastionSvc.SetSyslogEmitter(func(eventType, message string) {
		syslogSvc.Emit(eventType, message)
	})
	dockerSvc := docker.NewService(db, cfg.DataDir)
	dockerSvc.SetWebServerHooks(docker.WebServerHooks{
		GetActive: wsMgr.GetActive,
		Reload:    wsMgr.Reload,
		EnsureInc: wsMgr.EnsureVhostInclude,
		EnsureReady: func() error {
			active := wsMgr.GetActive()
			if active == "" {
				active = "nginx"
			}
			app, err := appSvc.Get(active)
			if err != nil || !app.Installed {
				return fmt.Errorf("请先安装并启动 Nginx 或 OpenResty，才能绑定域名")
			}
			return nil
		},
	})
	go func() { _ = dockerSvc.ReconcileBindings() }()
	authSvc := auth.NewService(db, cfg.JWTSecret)
	authSvc.SetTokenTTL(func() time.Duration {
		all, err := settingsSvc.GetAll()
		if err != nil {
			return 24 * time.Hour
		}
		v := strings.TrimSpace(all["session_timeout"])
		if v == "" {
			return 24 * time.Hour
		}
		secs, err := strconv.Atoi(v)
		if err != nil || secs <= 0 {
			return 24 * time.Hour
		}
		return time.Duration(secs) * time.Second
	})
	srv := &Server{
		cfg:       cfg,
		db:        db,
		authSvc:   authSvc,
		dashboard: dashSvc,
		website:   websiteSvc,
		database:  dbSvc,
		filemgr:   filemgr.NewService(cfg.DataDir),
		docker:    dockerSvc,
		ssl:       sslSvc,
		firewall:  firewall.NewService(db),
		cron:      cronSvc,
		appstore:  appSvc,
		ftp:       ftpSvc,
		backup:    backupSvc,
		sshmgr:    sshSvc,
		compose:   composeSvc,
		process:   process.NewService(),
		logs:      logs.NewService(cfg.DataDir, db),
		toolbox:   toolbox.NewService(db),
		settings:  settingsSvc,
		performance: perfSvc,
		aichat:    aichatSvc,
		waf:        wafSvc,
		edgeworker: edgeWorkerSvc,
		edgekv:     edgeKVSvc,
		edged1:     edgeD1Svc,
		cache:      cacheSvc,
		analytics: analytics.NewService(db, cfg.DataDir, wafSvc, perfSvc),
		productAnalytics: productanalytics.NewService(db, cfg.DataDir),
		mail:      mailSvc,
		dns:       dnsSvc,
		wordpress: wpSvc,
		nodejs:    nodejs.NewService(db, cfg.DataDir),
		java:      java.NewService(db, cfg.DataDir),
		runtime:   runtime.NewService(db, cfg.DataDir),
		security:   securitySvc,
		kafkaaccel: kafkaaccel.NewService(db, appSvc),
		cilium:     cilium.NewService(db, appSvc, cfg.DataDir),
		k8s:        k8s.NewService(appSvc, settingsSvc, cfg.DataDir),
		webserver:  wsMgr,
		phpmyadmin: pmaSvc,
		autops:     autoOpsSvc,
		cloudhub:   cloudhubSvc,
		cluster:    clusterSvc,
		ossstorage: ossSvc,
		uptime:     uptimeSvc,
		devops:     devopsSvc,
		aihub:      aihubSvc,
		aisite:     aisiteSvc,
		extensions: extReg,
		bastion:    bastionSvc,
		syslog:     syslogSvc,
		migration:  migration.NewService(db, cfg.DataDir, settingsSvc),
		enterprise: enterpriseSvc,
	}
	appSvc.SetPostInstallHook(func(key string) error {
		if err := basePostInstall(key); err != nil {
			return err
		}
		srv.emitExtension(extension.EventAppInstalled, map[string]interface{}{"key": key})
		return nil
	})
	return srv
}

func (s *Server) Run() error {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.PanelAccessLog(s.cfg.DataDir, s.analytics.RecordHTTPAccess))
	r.Use(middleware.SecurityHeaders(func() bool {
		all, err := s.settings.GetAll()
		if err != nil {
			return true
		}
		return all["panel_security_headers"] != "false"
	}))
	r.Use(middleware.PanelIPAccess(func() middleware.PanelIPAccessConfig {
		all, err := s.settings.GetAll()
		if err != nil {
			return middleware.PanelIPAccessConfig{}
		}
		return middleware.PanelIPAccessConfig{
			WhitelistEnabled: all["panel_ip_whitelist_enabled"] == "true",
			Whitelist:        all["panel_ip_whitelist"],
			Blacklist:        all["panel_ip_blacklist"],
		}
	}))

	sp := s.safePathPrefix()
	router := gin.IRouter(r)
	if sp != "" {
		r.Use(middleware.BlockWithoutSafePath(sp))
		router = r.Group("/" + sp)
	}
	s.registerRoutes(router, r, sp)

	addr := fmt.Sprintf(":%d", s.cfg.Port)
	fmt.Printf("OWPanel listening on http://0.0.0.0%s\n", addr)
	if sp != "" {
		fmt.Printf("Security entrance: /%s/\n", sp)
	}
	go s.startAutoBackupLoop()
	go s.startTrashPurgeLoop()
	go s.startLogCleanupLoop()
	go s.startSiteExpiryLoop()
	go s.startAutomationScheduler()
	go s.runInitialSecurityBootstrap()
	go bootstrap.Host(s.appstore, s.webserver, s.settings, s.cfg.DataDir)
	go s.appstore.WarmStoreCatalog()
	s.emitExtension(extension.EventPanelStartup, map[string]interface{}{"version": "owpanel"})
	return r.Run(addr)
}

func (s *Server) startAutoBackupLoop() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.backup.RunDueAutoBackups()
		s.backup.RunDueDatabaseAutoBackups()
		s.backup.RunDueBackupTasks()
	}
}

func (s *Server) startTrashPurgeLoop() {
	s.purgeExpiredTrash()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		s.purgeExpiredTrash()
	}
}

func (s *Server) purgeExpiredTrash() {
	settings, err := s.settings.GetAll()
	if err != nil {
		return
	}
	days := filemgr.ParseTrashRetentionDays(settings)
	_, _, _ = s.filemgr.PurgeExpiredTrash(days)
}

func (s *Server) startLogCleanupLoop() {
	s.runLogAutoCleanup()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		s.runLogAutoCleanup()
	}
}

func (s *Server) runLogAutoCleanup() {
	settings := s.logs.GetRetentionSettings()
	if !settings.LoggingEnabled || !settings.AutoCleanup || settings.RetentionDays <= 0 {
		return
	}
	_, _ = s.logs.CleanOlderThan(settings.RetentionDays)
}

func (s *Server) startSiteExpiryLoop() {
	s.enforceExpiredSites()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		s.enforceExpiredSites()
	}
}

func (s *Server) enforceExpiredSites() {
	if n := s.website.EnforceExpiredSites(); n > 0 {
		fmt.Printf("[website] auto-stopped %d expired site(s)\n", n)
	}
}

func (s *Server) startAutomationScheduler() {
	run := func() {
		s.autops.ScanExpiryAlerts()
		s.autops.RunSSLAutoRenew(s.ssl.RenewAll)
		s.autops.ScanWebsiteAudits(false)
		s.runSecurityAutoFix(false)
	}
	run()
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		run()
	}
}

const securityBootstrapKey = "security_bootstrap_v1"

func (s *Server) runInitialSecurityBootstrap() {
	all, err := s.settings.GetAll()
	if err == nil && all[securityBootstrapKey] == "done" {
		return
	}
	s.runSecurityAutoFix(true)
	_ = s.settings.Update(map[string]string{securityBootstrapKey: "done"})
}

func (s *Server) runSecurityAutoFix(initial bool) {
	if s.security == nil {
		return
	}
	result, err := s.security.FixAll()
	if err != nil {
		fmt.Printf("[security] auto-fix: %v\n", err)
		return
	}
	if initial {
		fmt.Printf("[security] initial bootstrap auto-fix: %d item(s)\n", len(result.Results))
	}
}

func (s *Server) safePathPrefix() string {
	all, err := s.settings.GetAll()
	if err != nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(all["panel_safe_path"]), "/")
}

func (s *Server) registerRoutes(r gin.IRouter, engine *gin.Engine, safePath string) {
	r.GET("/health", func(c *gin.Context) {
		response.OK(c, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1")
	{
		api.GET("/auth/bootstrap", s.handleAuthBootstrap)
		api.POST("/auth/login", s.handleLogin)
		api.POST("/auth/totp/login", s.handleTotpLogin)
		api.GET("/cluster/agent/ping", s.handleClusterAgentPing)
		api.POST("/cluster/agent/register", s.handleClusterAgentRegister)
		api.GET("/cluster/agent/join.sh", s.handleClusterJoinScript)
		api.POST("/deploy/hook/:token", s.handleDeployWebhook)
		api.POST("/deploy/ci/:token", s.handleDeployCI)
		s.registerEdgeInternalRoutes(api)

		wsShell := api.Group("")
		wsShell.Use(middleware.AuthAllowQueryToken(s.authSvc))
		wsShell.Use(middleware.RequireShellAccess())
		wsShell.Use(middleware.RateLimitStrict("terminal"))
		wsShell.Use(enterprise.InfraAuditMiddleware(s.enterprise))
		wsShell.GET("/terminal/ws", s.handleTerminalWSAuth)

		authorized := api.Group("")
		authorized.Use(middleware.Auth(s.authSvc))
		{
			authorized.GET("/auth/me", s.handleMe)
			authorized.POST("/auth/change-password", s.handleChangePassword)
			s.registerTotpRoutes(authorized)
			s.registerExtensionRoutes(authorized)
			s.registerBastionRoutes(authorized)

			authorized.GET("/dashboard/stats", s.handleDashboardStats)
			authorized.GET("/dashboard/processes", s.handleDashboardProcesses)
			authorized.GET("/dashboard/history", s.handleDashboardHistory)
			authorized.GET("/dashboard/monitor", s.handleDashboardMonitor)
			authorized.GET("/dashboard/alerts", s.handleDashboardAlerts)
			authorized.GET("/dashboard/health", s.handleDashboardHealth)
			authorized.GET("/dashboard/performance", s.handleDashboardPerformanceGet)
			s.registerSystemRoutes(authorized)

			web := authorized.Group("")
			web.Use(middleware.RequirePermission("websites"))
			web.Use(middleware.RateLimitSensitive("websites"))
			web.Use(enterprise.InfraAuditMiddleware(s.enterprise))
			{
				s.registerWebsiteRoutes(web)
				s.registerSSLRoutes(web)
				s.registerCacheRoutes(web)
				s.registerWordPressRoutes(web)
				s.registerNodeJSRoutes(web)
				s.registerJavaRoutes(web)
				s.registerRuntimeRoutes(web)
				s.registerWebserverRoutes(web)
				s.registerAnalyticsRoutes(web)
				s.registerProductAnalyticsRoutes(web)
			}

			dbRoutes := authorized.Group("")
			dbRoutes.Use(middleware.RequirePermission("databases"))
			{
				s.registerDatabaseRoutes(dbRoutes)
				s.registerPhpMyAdminRoutes(dbRoutes)
			}

			files := authorized.Group("")
			files.Use(middleware.RequirePermission("files"))
			{
				s.registerFileRoutes(files)
				s.registerOSSRoutes(files)
			}

			docker := authorized.Group("")
			docker.Use(middleware.RequirePermission("docker"))
			docker.Use(middleware.RateLimitSensitive("docker"))
			docker.Use(enterprise.InfraAuditMiddleware(s.enterprise))
			{
				docker.GET("/docker/status", s.handleDockerStatus)
				docker.GET("/docker/containers", s.handleListContainers)
				docker.GET("/docker/images", s.handleListImages)
				docker.GET("/docker/volumes", s.handleListDockerVolumes)
				docker.GET("/docker/networks", s.handleListDockerNetworks)
				docker.POST("/docker/containers/:id/start", s.handleStartContainer)
				docker.POST("/docker/containers/:id/stop", s.handleStopContainer)
				docker.DELETE("/docker/containers/:id", s.handleRemoveContainer)
				s.registerDockerExtraRoutes(docker)
				s.registerComposeRoutes(docker)
			}

			ftp := authorized.Group("")
			ftp.Use(middleware.RequirePermission("ftp"))
			{
				s.registerFTPRoutes(ftp)
			}

			mail := authorized.Group("")
			mail.Use(middleware.RequirePermission("mail"))
			mail.Use(middleware.RateLimitSensitive("mail"))
			mail.Use(enterprise.InfraAuditMiddleware(s.enterprise))
			{
				s.registerMailRoutes(mail)
			}

			backup := authorized.Group("")
			backup.Use(middleware.RequirePermission("backup"))
			backup.Use(middleware.RateLimitSensitive("backup"))
			backup.Use(enterprise.InfraAuditMiddleware(s.enterprise))
			{
				backup.GET("/cron", s.handleListCron)
				backup.GET("/cron/templates", s.handleCronTemplates)
				backup.GET("/cron/status", s.handleCronStatus)
				backup.POST("/cron", s.handleCreateCron)
				backup.PUT("/cron/:id", s.handleUpdateCron)
				backup.DELETE("/cron/:id", s.handleDeleteCron)
				backup.PATCH("/cron/:id/toggle", s.handleToggleCron)
				backup.POST("/cron/:id/run", s.handleRunCron)
				backup.GET("/cron/:id/logs", s.handleCronLogs)
				backup.POST("/cron/reload", s.handleReloadCron)
				s.registerBackupRoutes(backup)
			}

			monitor := authorized.Group("")
			monitor.Use(middleware.RequirePermission("monitor"))
			{
				s.registerUptimeRoutes(monitor)
				s.registerAutoOpsRoutes(monitor)
				s.registerCloudRoutes(monitor)
				s.registerClusterRoutes(monitor)
			}

			admin := authorized.Group("")
			admin.Use(middleware.RequireAdmin())
			admin.Use(middleware.RateLimitSensitive("admin"))
			admin.Use(enterprise.AuditMiddleware(s.enterprise))
			{
				admin.POST("/dashboard/free-memory", s.handleFreeMemory)
				admin.POST("/dashboard/memory-tune", s.handleMemoryTune)
				admin.POST("/dashboard/optimize", s.handleDashboardOptimize)
				admin.GET("/firewall", s.handleListFirewall)
				admin.GET("/firewall/status", s.handleFirewallStatus)
				admin.POST("/firewall", s.handleCreateFirewall)
				admin.POST("/firewall/sync", s.handleSyncFirewall)
				admin.DELETE("/firewall/:id", s.handleDeleteFirewall)

				s.registerSoftwareRoutes(admin)
				s.registerWAFRoutes(admin)
				s.registerEdgeWorkerRoutes(admin)
				s.registerDNSRoutes(admin)
				s.registerLogRoutes(admin)
				s.registerSecurityRoutes(admin)
				s.registerKafkaAccelRoutes(admin)
				s.registerCiliumRoutes(admin)
				s.registerK8sRoutes(admin)
				s.registerDevOpsRoutes(admin)
				s.registerAIHubRoutes(admin)
				s.registerToolboxRoutes(admin)
				s.registerTerminalRoutes(admin)

				admin.GET("/users", s.handleListUsers)
				admin.POST("/users", s.handleCreateUser)
				admin.PATCH("/users/:id", s.handleUpdateUser)
				admin.DELETE("/users/:id", s.handleDeleteUser)

				admin.GET("/settings", s.handleGetSettings)
				admin.PUT("/settings", s.handleUpdateSettings)
				admin.PUT("/dashboard/performance", s.handleDashboardPerformancePut)
				admin.POST("/settings/ai-models/sync", s.handleSyncAIModels)
				admin.POST("/settings/cursor-models/sync", s.handleSyncCursorModels)
				admin.GET("/settings/migration/preview", s.handleMigrationPreview)
				admin.POST("/settings/migration/export", s.handleMigrationExport)
				admin.GET("/settings/migration/download", s.handleMigrationDownload)
				admin.POST("/settings/migration/import", s.handleMigrationImport)
				s.registerEnterpriseRoutes(admin)
				s.registerAdminSystemRoutes(admin)
			}
		}
	}

	r.Static("/assets", s.cfg.WebDir+"/assets")
	r.Static("/software-icons", s.cfg.WebDir+"/software")
	r.Static("/models", s.cfg.WebDir+"/models")
	r.Static("/geo", s.cfg.WebDir+"/geo")
	r.StaticFile("/favicon.svg", s.cfg.WebDir+"/favicon.svg")

	engine.NoRoute(func(c *gin.Context) {
		if safePath != "" {
			path := c.Request.URL.Path
			prefix := "/" + safePath
			if path != prefix && !strings.HasPrefix(path, prefix+"/") {
				c.String(http.StatusNotFound, "404 — use panel security entrance /%s", safePath)
				return
			}
		}
		path := c.Request.URL.Path
		if looksLikeStaticAsset(path) {
			c.Status(http.StatusNotFound)
			return
		}
		if c.Request.Method == http.MethodGet {
			s.serveIndexHTML(c)
			return
		}
		c.Status(http.StatusNotFound)
	})
}

func (s *Server) handleAuthBootstrap(c *gin.Context) {
	sp := s.safePathPrefix()
	all, _ := s.settings.GetAll()
	response.OK(c, gin.H{
		"panel_name":   all["panel_name"],
		"safe_path":    sp,
		"base_path":    s.safePathBase(),
		"website_path": all["website_path"],
	})
}

func (s *Server) handleLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}

	ip := c.ClientIP()
	ua := c.GetHeader("User-Agent")
	if err := auth.CheckLoginIPAllowed(ip); err != nil {
		response.Error(c, 429, err.Error())
		return
	}
	auth.RecordLoginIPAttempt(ip)
	if err := auth.CheckLoginAllowed(ip, req.Username); err != nil {
		auth.RecordLoginEvent(s.db, req.Username, ip, ua, false, "locked")
		s.enterprise.Recorder().Login(req.Username, ip, ua, false, "locked")
		if s.syslog != nil {
			s.syslog.LoginFailure(req.Username, ip, "locked")
		}
		response.Error(c, 429, err.Error())
		return
	}

	token, user, err := s.authSvc.Login(req.Username, req.Password)
	if err != nil {
		auth.RecordLoginFailure(ip, req.Username)
		auth.RecordLoginEvent(s.db, req.Username, ip, ua, false, "invalid_credentials")
		s.enterprise.Recorder().Login(req.Username, ip, ua, false, "invalid_credentials")
		if s.syslog != nil {
			s.syslog.LoginFailure(req.Username, ip, "invalid_credentials")
		}
		response.Error(c, 401, "invalid username or password")
		return
	}
	if user.TotpEnabled {
		tempToken, err := s.authSvc.IssueTotpPendingToken(user)
		if err != nil {
			response.Error(c, 500, err.Error())
			return
		}
		response.OK(c, gin.H{"require_totp": true, "temp_token": tempToken})
		return
	}
	auth.RecordLoginSuccess(ip, req.Username)
	auth.RecordLoginEvent(s.db, req.Username, ip, ua, true, "ok")
	s.enterprise.Recorder().Login(req.Username, ip, ua, true, "ok")
	if s.syslog != nil {
		s.syslog.LoginSuccess(req.Username, ip)
	}

	response.OK(c, gin.H{
		"token": token,
		"user": gin.H{
			"id":                   user.ID,
			"username":             user.Username,
			"role":                 user.Role,
			"must_change_password": user.MustChangePassword,
			"permissions":          user.Permissions,
			"disk_quota_mb":        user.DiskQuotaMB,
			"disk_used_mb":         user.DiskUsedMB,
			"totp_enabled":         user.TotpEnabled,
		},
	})
}

func (s *Server) handleMe(c *gin.Context) {
	var user models.User
	if err := s.db.First(&user, c.GetUint("user_id")).Error; err != nil {
		response.Error(c, 404, "user not found")
		return
	}
	response.OK(c, gin.H{
		"id":                   user.ID,
		"username":             user.Username,
		"role":                 user.Role,
		"must_change_password": user.MustChangePassword,
		"permissions":          user.Permissions,
		"disk_quota_mb":        user.DiskQuotaMB,
		"disk_used_mb":         user.DiskUsedMB,
		"totp_enabled":         user.TotpEnabled,
	})
}

func (s *Server) handleChangePassword(c *gin.Context) {
	var req struct {
		Password string `json:"password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := auth.ValidatePassword(req.Password, s.passwordRequireStrong()); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	uid := c.GetUint("user_id")
	ip := c.ClientIP()
	if err := auth.CheckPasswordChangeAllowed(uid, ip); err != nil {
		response.Error(c, 429, err.Error())
		return
	}
	if err := s.authSvc.ChangePassword(uid, req.Password); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	auth.RecordPasswordChange(uid, ip)
	response.Message(c, "password updated")
}

func (s *Server) handleDashboardStats(c *gin.Context) {
	stats, err := s.dashboard.GetStats()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, stats)
}

func (s *Server) handleDashboardProcesses(c *gin.Context) {
	sortBy := strings.ToLower(strings.TrimSpace(c.Query("sort")))
	if sortBy == "" {
		sortBy = "cpu"
	}
	if sortBy != "cpu" && sortBy != "memory" {
		response.Error(c, 400, "sort must be cpu or memory")
		return
	}
	limit := 15
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 50 {
		limit = 50
	}
	var (
		data []process.Info
		err  error
	)
	if sortBy == "memory" {
		data, err = s.process.TopByMemory(limit)
	} else {
		data, err = s.process.TopByCPU(limit)
	}
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, data)
}

func (s *Server) handleFreeMemory(c *gin.Context) {
	result, err := s.dashboard.FreeMemory()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleMemoryTune(c *gin.Context) {
	bootstrap.TuneMemoryForce(s.settings, true)
	result, err := s.dashboard.OneClickOptimize()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleDashboardOptimize(c *gin.Context) {
	result, err := s.dashboard.OneClickOptimize()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleDashboardAlerts(c *gin.Context) {
	all, _ := s.settings.GetAll()
	th := dashboard.DefaultAlertThresholds()
	if all != nil {
		th = dashboard.ParseAlertThresholds(all)
	}
	alerts := s.dashboard.ComputeResourceAlerts(th)
	response.OK(c, gin.H{
		"alerts":     alerts,
		"thresholds": th,
	})
}

func (s *Server) handleDashboardHistory(c *gin.Context) {
	hours := 1.0
	if v := c.Query("hours"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			hours = f
		}
	}
	response.OK(c, s.dashboard.GetHistory(hours))
}

const monitorExtrasTTL = 45 * time.Second

func (s *Server) getMonitorExtras() (dashboard.MonitorExtras, bool) {
	s.monitorExtrasMu.Lock()
	defer s.monitorExtrasMu.Unlock()
	if !s.monitorExtrasAt.IsZero() && time.Since(s.monitorExtrasAt) < monitorExtrasTTL {
		return s.monitorExtras, true
	}
	return dashboard.MonitorExtras{}, false
}

func (s *Server) setMonitorExtras(extras dashboard.MonitorExtras) {
	s.monitorExtrasMu.Lock()
	s.monitorExtras = extras
	s.monitorExtrasAt = time.Now()
	s.monitorExtrasMu.Unlock()
}

func (s *Server) buildMonitorExtras() dashboard.MonitorExtras {
	var extras dashboard.MonitorExtras
	var allProcs []dashboard.ProcessBrief
	if top, err := s.process.TopByCPU(32); err == nil {
		for _, p := range top {
			cmd := p.Command
			if len(cmd) > 200 {
				cmd = cmd[:200]
			}
			allProcs = append(allProcs, dashboard.ProcessBrief{
				PID: p.PID, Name: p.Name, CPU: p.CPU, Memory: p.Memory, Command: cmd,
			})
		}
		briefs := make([]dashboard.ProcessBrief, len(allProcs))
		copy(briefs, allProcs)
		sort.Slice(briefs, func(i, j int) bool { return briefs[i].CPU > briefs[j].CPU })
		if len(briefs) > 8 {
			briefs = briefs[:8]
		}
		extras.TopProcesses = briefs
	}

	if apps, err := s.appstore.ListInstalledNoSync(); err == nil && len(apps) > 0 {
		keys := make([]string, len(apps))
		for i, a := range apps {
			keys[i] = a.Key
		}
		statusMap := s.appstore.LiveStatusMap(keys)
		lookup := func(key string) string { return statusMap[key] }

		running := make([]dashboard.RunningAppInfo, 0)
		for _, a := range apps {
			st := lookup(a.Key)
			if st != "running" {
				continue
			}
			running = append(running, dashboard.RunningAppInfo{
				Key: a.Key, Name: a.Name, Category: a.Category,
				Status: st, Port: a.Port, Version: a.Version,
			})
		}
		extras.RunningApps = running
		extras.InstalledApps = dashboard.BuildInstalledAppMetrics(apps, lookup, allProcs)
	}
	return extras
}

func (s *Server) handleDashboardMonitor(c *gin.Context) {
	s.performance.TouchDashboardLive()
	hours := 1.0
	if v := c.Query("hours"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			hours = f
		}
	}
	lite := c.Query("lite") == "1"
	mon := s.dashboard.GetMonitor(hours)
	if lite {
		response.OK(c, mon)
		return
	}

	if extras, ok := s.getMonitorExtras(); ok {
		mon.TopProcesses = extras.TopProcesses
		mon.RunningApps = extras.RunningApps
		mon.InstalledApps = extras.InstalledApps
	} else {
		extras := s.buildMonitorExtras()
		s.setMonitorExtras(extras)
		mon.TopProcesses = extras.TopProcesses
		mon.RunningApps = extras.RunningApps
		mon.InstalledApps = extras.InstalledApps
	}

	mon.AIModels = dashboard.FetchOllamaModels()
	response.OK(c, mon)
}

func (s *Server) handleDockerStatus(c *gin.Context) {
	response.OK(c, s.docker.Status())
}

func (s *Server) handleListContainers(c *gin.Context) {
	list, err := s.docker.ListContainers()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleListImages(c *gin.Context) {
	list, err := s.docker.ListImages()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleStartContainer(c *gin.Context) {
	if err := s.docker.Start(c.Param("id")); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "started")
}

func (s *Server) handleStopContainer(c *gin.Context) {
	if err := s.docker.Stop(c.Param("id")); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "stopped")
}

func (s *Server) handleRemoveContainer(c *gin.Context) {
	id := c.Param("id")
	s.docker.RemoveBindingForContainer(id)
	if err := s.docker.RemoveContainer(id); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "removed")
}

func (s *Server) handleListDockerVolumes(c *gin.Context) {
	list, err := s.docker.ListVolumes()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleListDockerNetworks(c *gin.Context) {
	list, err := s.docker.ListNetworks()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleListFirewall(c *gin.Context) {
	list, err := s.firewall.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateFirewall(c *gin.Context) {
	var rule models.FirewallRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.firewall.Create(&rule); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, rule)
}

func (s *Server) handleDeleteFirewall(c *gin.Context) {
	id := parseID(c)
	if err := s.firewall.Delete(id); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleListCron(c *gin.Context) {
	list, err := s.cron.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCronTemplates(c *gin.Context) {
	lang := c.Query("lang")
	if lang == "" {
		lang = "zh"
	}
	response.OK(c, s.cron.Templates(lang))
}

func (s *Server) handleCronStatus(c *gin.Context) {
	response.OK(c, gin.H{
		"mode":     s.cron.SchedulerMode(),
		"data_dir": s.cron.DataDir(),
	})
}

func (s *Server) handleCreateCron(c *gin.Context) {
	var job models.CronJob
	if err := c.ShouldBindJSON(&job); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.cron.Create(&job); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, job)
}

func (s *Server) handleUpdateCron(c *gin.Context) {
	var req models.CronJob
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	job, err := s.cron.Update(parseID(c), req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, job)
}

func (s *Server) handleDeleteCron(c *gin.Context) {
	id := parseID(c)
	if err := s.cron.Delete(id); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleToggleCron(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.cron.Toggle(parseID(c), req.Enabled); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "updated")
}

func (s *Server) handleRunCron(c *gin.Context) {
	if err := s.cron.RunNow(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "started")
}

func (s *Server) handleCronLogs(c *gin.Context) {
	log, err := s.cron.GetLog(parseID(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"log": log})
}

func (s *Server) handleReloadCron(c *gin.Context) {
	s.cron.ReloadAll()
	response.Message(c, "reloaded")
}

func (s *Server) handleFirewallStatus(c *gin.Context) {
	response.OK(c, s.firewall.Status())
}

func (s *Server) handleSyncFirewall(c *gin.Context) {
	if err := s.firewall.SyncAll(); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "synced")
}

func (s *Server) handleListUsers(c *gin.Context) {
	users, err := s.authSvc.ListUsers()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, users)
}

func (s *Server) handleCreateUser(c *gin.Context) {
	var req struct {
		Username    string `json:"username" binding:"required"`
		Password    string `json:"password" binding:"required,min=8"`
		Role        string `json:"role"`
		Permissions string `json:"permissions"`
		DiskQuotaMB int64  `json:"disk_quota_mb"`
		Remark      string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if req.Role == "" {
		req.Role = "user"
	}
	if err := auth.ValidatePassword(req.Password, s.passwordRequireStrong()); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	user, err := s.authSvc.CreateUserExtended(auth.CreateUserRequest{
		Username: req.Username, Password: req.Password, Role: req.Role,
		Permissions: req.Permissions, DiskQuotaMB: req.DiskQuotaMB, Remark: req.Remark,
	})
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.enterprise.Recorder().FromGin(c, "user", "user_create", user.Username, user.Role, "info", true)
	response.OK(c, user)
}

func (s *Server) handleUpdateUser(c *gin.Context) {
	var req struct {
		Role        string `json:"role"`
		Permissions string `json:"permissions"`
		DiskQuotaMB *int64 `json:"disk_quota_mb"`
		Remark      string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	quota := int64(-1)
	if req.DiskQuotaMB != nil {
		quota = *req.DiskQuotaMB
	}
	if err := s.authSvc.UpdateUser(parseID(c), req.Role, req.Permissions, req.Remark, quota); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.enterprise.Recorder().FromGin(c, "user", "user_update", c.Param("id"), req.Role, "info", true)
	response.Message(c, "updated")
}

func (s *Server) handleDeleteUser(c *gin.Context) {
	if err := s.authSvc.DeleteUser(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.enterprise.Recorder().FromGin(c, "user", "user_delete", c.Param("id"), "", "warn", true)
	response.Message(c, "deleted")
}

func parseID(c *gin.Context) uint {
	var id uint
	fmt.Sscanf(c.Param("id"), "%d", &id)
	return id
}

func parseParamID(c *gin.Context, name string) uint {
	var id uint
	fmt.Sscanf(c.Param(name), "%d", &id)
	return id
}
