package api

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/aichat"
	"github.com/luuuunet/owpanel/internal/services/appstore"
	"github.com/luuuunet/owpanel/internal/services/webserver"
)

func (s *Server) registerModuleRoutes(authorized *gin.RouterGroup) {
	s.registerSoftwareRoutes(authorized)
	s.registerFTPRoutes(authorized)
	s.registerBackupRoutes(authorized)
	s.registerComposeRoutes(authorized)
	s.registerLogRoutes(authorized)
	s.registerWebserverRoutes(authorized)
}

func (s *Server) registerSoftwareRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/apps", s.handleListApps)
	authorized.GET("/software/store", s.handleListApps)
	authorized.POST("/software/store/sync", s.handleSyncStoreCatalog)
	authorized.POST("/software/store/refresh-versions", s.handleRefreshStoreVersions)
	authorized.GET("/software/installed", s.handleListInstalledSoftware)
	authorized.GET("/software/:key/install/logs", s.handleGetSoftwareInstallLogs)
	authorized.GET("/software/:key", s.handleGetSoftware)
	authorized.POST("/apps/:key/install", s.handleInstallApp)
	authorized.POST("/software/:key/install", s.handleInstallApp)
	authorized.POST("/apps/:key/uninstall", s.handleUninstallApp)
	authorized.POST("/software/:key/uninstall", s.handleUninstallApp)
	authorized.POST("/software/:key/upgrade", s.handleUpgradeApp)
	authorized.POST("/software/:key/:action", s.handleSoftwareAction)
	authorized.GET("/software/:key/config", s.handleGetSoftwareConfig)
	authorized.PUT("/software/:key/config", s.handleUpdateSoftwareConfig)
	authorized.GET("/software/:key/config/raw", s.handleGetSoftwareConfigRaw)
	authorized.PUT("/software/:key/config/raw", s.handleUpdateSoftwareConfigRaw)
	authorized.GET("/software/:key/php/detail", s.handleGetPHPDetail)
	authorized.PUT("/software/:key/php/disable-functions", s.handleSetPHPDisableFunctions)
	authorized.PUT("/software/:key/php/extensions/:name", s.handleSetPHPExtension)
	authorized.POST("/software/:key/php/extensions/install", s.handleInstallPHPExtension)
	authorized.GET("/software/:key/pgsql/detail", s.handleGetPgSQLDetail)
	authorized.POST("/software/:key/pgsql/extensions/install", s.handleInstallPgSQLExtensionPackage)
	authorized.POST("/software/:key/config/ai/chat", s.handleSoftwareConfigAIChat)
	authorized.PATCH("/software/:key/settings", s.handlePatchSoftwareSettings)
}

func (s *Server) registerFTPRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/ftp", s.handleListFTP)
	authorized.POST("/ftp", s.handleCreateFTP)
	authorized.POST("/ftp/sync", s.handleSyncFTP)
	authorized.DELETE("/ftp/:id", s.handleDeleteFTP)
}

func (s *Server) registerBackupRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/backup", s.handleListBackup)
	authorized.POST("/backup", s.handleCreateBackup)
	authorized.DELETE("/backup/:id", s.handleDeleteBackup)
	authorized.PATCH("/backup/:id/toggle", s.handleToggleBackup)
	authorized.POST("/backup/:id/run", s.handleRunBackupTask)
	authorized.POST("/backup/presets", s.handleBackupPresets)

	authorized.GET("/backup/remotes", s.handleListBackupRemotes)
	authorized.POST("/backup/remotes", s.handleCreateBackupRemote)
	authorized.GET("/backup/remotes/:id", s.handleGetBackupRemote)
	authorized.PUT("/backup/remotes/:id", s.handleUpdateBackupRemote)
	authorized.DELETE("/backup/remotes/:id", s.handleDeleteBackupRemote)
	authorized.POST("/backup/remotes/:id/test", s.handleTestBackupRemote)

	authorized.GET("/backup/panel/history", s.handlePanelBackupHistory)
	authorized.GET("/backup/panel/config", s.handlePanelBackupConfigGet)
	authorized.PUT("/backup/panel/config", s.handlePanelBackupConfigPut)
	authorized.POST("/backup/panel/run", s.handlePanelBackupRun)
	authorized.POST("/backup/panel/restore", s.handlePanelBackupRestore)
}

func (s *Server) registerComposeRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/compose/templates", s.handleComposeTemplates)
	authorized.GET("/compose", s.handleListCompose)
	authorized.POST("/compose", s.handleCreateCompose)
	authorized.DELETE("/compose/:id", s.handleDeleteCompose)
	authorized.PATCH("/compose/:id/toggle", s.handleToggleCompose)
	authorized.GET("/compose/:id/logs", s.handleComposeLogs)
	authorized.POST("/compose/:id/pull", s.handleComposePull)
	authorized.POST("/compose/:id/sync", s.handleComposeSync)
	authorized.POST("/compose/:id/restart", s.handleComposeRestart)
	authorized.GET("/compose/:id/compose-file", s.handleComposeFileGet)
	authorized.PUT("/compose/:id/compose-file", s.handleComposeFilePut)
	authorized.POST("/compose/:id/rolling", s.handleComposeRolling)
	authorized.POST("/compose/:id/blue-green", s.handleComposeBlueGreen)
}

func (s *Server) registerWebserverRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/php/versions", s.handlePHPVersions)
	authorized.POST("/php/:key/:action", s.handlePHPAction)
	authorized.GET("/nginx/status", s.handleNginxStatus)
	authorized.POST("/nginx/:key/install", s.handleNginxOneClickInstall)
	authorized.POST("/nginx/:key/setup", s.handleNginxSetup)
	authorized.POST("/nginx/stack/lnmp", s.handleLNMPStack)
	authorized.POST("/nginx/stack/lamp", s.handleLAMPStack)
	authorized.POST("/nginx/stack/:key", s.handleNginxStackByKey)
	authorized.POST("/nginx/:key/start", s.handleNginxStart)
	authorized.POST("/nginx/:key/stop", s.handleNginxStop)
	authorized.POST("/nginx/:key/reload", s.handleNginxReload)
	authorized.POST("/nginx/:key/test", s.handleNginxTest)
	authorized.GET("/nginx/:key/config", s.handleNginxGetConfig)
	authorized.PUT("/nginx/:key/config", s.handleNginxPutConfig)
}

func (s *Server) handleListApps(c *gin.Context) {
	list, err := s.appstore.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, s.enrichSoftwareApps(list))
}

func (s *Server) handleSyncStoreCatalog(c *gin.Context) {
	n := s.appstore.SyncCatalog()
	list, err := s.appstore.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{
		"count":   n,
		"listed":  len(list),
		"message": fmt.Sprintf("软件商店已同步 %d 个软件", len(list)),
	})
}

func (s *Server) handleRefreshStoreVersions(c *gin.Context) {
	result, err := s.appstore.RefreshStoreVersions()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleGetSoftwareInstallLogs(c *gin.Context) {
	key := c.Param("key")
	response.OK(c, s.appstore.GetInstallLogs(key))
}

func (s *Server) handleInstallApp(c *gin.Context) {
	var req struct {
		Version string `json:"version"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := s.appstore.Install(c.Param("key"), req.Version); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "installation started")
}

func (s *Server) handleUpgradeApp(c *gin.Context) {
	var req struct {
		Version string `json:"version"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := s.appstore.Upgrade(c.Param("key"), req.Version); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "upgrade started")
}

func (s *Server) handleListInstalledSoftware(c *gin.Context) {
	list, err := s.appstore.ListInstalled()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, s.enrichSoftwareApps(list))
}

type softwareAppResponse struct {
	models.App
	DockerApp      bool                  `json:"docker_app"`
	AccessURL      string                `json:"access_url,omitempty"`
	Grouped        bool                  `json:"grouped,omitempty"`
	FamilyKey      string                `json:"family_key,omitempty"`
	DescriptionEN  string                `json:"description_en,omitempty"`
	VersionEntries []appstore.StoreVersionEntry `json:"version_entries,omitempty"`
}

func (s *Server) enrichSoftwareApps(apps []models.App) []softwareAppResponse {
	enrich := func(app models.App) (bool, string) {
		return appstore.IsDockerStoreApp(app.Key), s.appstore.AccessURL(app.Key, app.BindDomain, app.Port)
	}
	grouped := appstore.GroupStoreListing(apps, enrich)
	out := make([]softwareAppResponse, len(grouped))
	for i, item := range grouped {
		out[i] = softwareAppResponse{
			App:            item.App,
			DockerApp:      item.DockerApp,
			AccessURL:      item.AccessURL,
			Grouped:        item.Grouped,
			FamilyKey:      item.FamilyKey,
			DescriptionEN:  item.DescriptionEN,
			VersionEntries: item.VersionEntries,
		}
	}
	return out
}

func (s *Server) handleGetSoftware(c *gin.Context) {
	app, err := s.appstore.Get(c.Param("key"))
	if err != nil {
		response.Error(c, 404, "software not found")
		return
	}
	response.OK(c, app)
}

func (s *Server) handleSoftwareAction(c *gin.Context) {
	key := c.Param("key")
	action := c.Param("action")
	switch action {
	case "start":
		if err := s.appstore.ServiceAction(key, "start"); err != nil {
			response.Error(c, 500, err.Error())
			return
		}
	case "stop":
		if err := s.appstore.ServiceAction(key, "stop"); err != nil {
			response.Error(c, 500, err.Error())
			return
		}
	case "restart", "reload":
		if err := s.appstore.ServiceAction(key, action); err != nil {
			response.Error(c, 500, err.Error())
			return
		}
	case "install", "uninstall":
		response.Error(c, 400, "use dedicated install/uninstall endpoints")
		return
	default:
		response.Error(c, 400, "unknown action")
		return
	}
	response.Message(c, action+" success")
}

func (s *Server) handleGetSoftwareConfig(c *gin.Context) {
	key := c.Param("key")
	cfg, err := s.appstore.GetConfig(key)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	meta, _ := s.appstore.ConfigMeta(key)
	caps, _ := s.appstore.ConfigCapabilities(key)
	app, _ := s.appstore.Get(key)
	response.OK(c, gin.H{
		"config":               cfg,
		"config_path":          app.ConfigPath,
		"resolved_config_path": meta.ResolvedConfigPath,
		"has_config_file":      meta.HasConfigFile,
		"is_php":               meta.IsPHP,
		"install_path":         app.InstallPath,
		"capabilities":         caps,
	})
}

func (s *Server) handleUpdateSoftwareConfig(c *gin.Context) {
	var req struct {
		Config map[string]interface{} `json:"config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.appstore.UpdateConfig(c.Param("key"), req.Config); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "config saved")
}

func (s *Server) handleGetSoftwareConfigRaw(c *gin.Context) {
	content, err := s.appstore.ReadConfigRaw(c.Param("key"))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	meta, _ := s.appstore.ConfigMeta(c.Param("key"))
	response.OK(c, gin.H{"content": content, "path": meta.ResolvedConfigPath})
}

func (s *Server) handleUpdateSoftwareConfigRaw(c *gin.Context) {
	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.appstore.WriteConfigRaw(c.Param("key"), req.Content); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "config saved")
}

func (s *Server) handleGetPHPDetail(c *gin.Context) {
	detail, err := s.appstore.PHPDetail(c.Param("key"))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, detail)
}

func (s *Server) handleSetPHPDisableFunctions(c *gin.Context) {
	var req struct {
		Functions string `json:"functions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.appstore.SetPHPDisableFunctions(c.Param("key"), req.Functions); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "disable_functions updated")
}

func (s *Server) handleSetPHPExtension(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.appstore.SetPHPExtension(c.Param("key"), c.Param("name"), req.Enabled); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "extension updated")
}

func (s *Server) handleInstallPHPExtension(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.appstore.InstallPHPExtension(c.Param("key"), req.Name); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "extension installed")
}

func (s *Server) handleGetPgSQLDetail(c *gin.Context) {
	if c.Param("key") != "postgresql" {
		response.Error(c, 400, "not PostgreSQL software")
		return
	}
	detail, err := s.database.ListPgExtensionCatalog(c.Query("database"))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, detail)
}

func (s *Server) handleInstallPgSQLExtensionPackage(c *gin.Context) {
	if c.Param("key") != "postgresql" {
		response.Error(c, 400, "not PostgreSQL software")
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.database.InstallPgExtensionPackage(req.Name); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	detail, err := s.database.ListPgExtensionCatalog(c.Query("database"))
	if err != nil {
		response.Message(c, "extension package installed")
		return
	}
	response.OK(c, detail)
}

func (s *Server) handlePatchSoftwareSettings(c *gin.Context) {
	var patch appstore.SettingsPatch
	if err := c.ShouldBindJSON(&patch); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.appstore.UpdateSettings(c.Param("key"), patch); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "settings updated")
}

func (s *Server) handleSoftwareConfigAIChat(c *gin.Context) {
	var req aichat.SoftwareConfigChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	req.AppKey = c.Param("key")
	if app, err := s.appstore.Get(req.AppKey); err == nil {
		if req.AppName == "" {
			req.AppName = app.Name
		}
		if req.Category == "" {
			req.Category = app.Category
		}
	}
	if caps, err := s.appstore.ConfigCapabilities(req.AppKey); err == nil && req.ConfigKind == "" {
		req.ConfigKind = caps.ConfigKind
	}
	result, err := s.aichat.SoftwareConfigChat(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleUninstallApp(c *gin.Context) {
	if err := s.appstore.Uninstall(c.Param("key")); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.emitExtension("app.uninstalled", map[string]interface{}{"key": c.Param("key")})
	response.Message(c, "uninstalled")
}

func (s *Server) handleListFTP(c *gin.Context) {
	list, err := s.ftp.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateFTP(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		Path     string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	acc := &models.FTPAccount{Username: req.Username, Path: req.Path}
	if err := s.ftp.Create(acc, req.Password); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, acc)
}

func (s *Server) handleDeleteFTP(c *gin.Context) {
	if err := s.ftp.Delete(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleListBackup(c *gin.Context) {
	list, err := s.backup.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateBackup(c *gin.Context) {
	var task models.BackupTask
	if err := c.ShouldBindJSON(&task); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.backup.Create(&task); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, task)
}

func (s *Server) handleDeleteBackup(c *gin.Context) {
	if err := s.backup.Delete(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleToggleBackup(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.backup.Toggle(parseID(c), req.Enabled); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "updated")
}

func (s *Server) handleRunBackupTask(c *gin.Context) {
	if err := s.backup.RunTask(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "started")
}

func (s *Server) handleBackupPresets(c *gin.Context) {
	var req struct {
		Preset       string `json:"preset"`
		Schedule     string `json:"schedule"`
		WebsiteIDs   []uint `json:"website_ids"`
		OSSStorageID *uint  `json:"oss_storage_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if req.Preset == "" {
		response.Error(c, 400, "preset required")
		return
	}
	res, err := s.backup.ApplyPreset(req.Preset, req.Schedule, req.WebsiteIDs, req.OSSStorageID)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, res)
}

func (s *Server) handleSyncFTP(c *gin.Context) {
	if err := s.ftp.SyncAll(); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "synced")
}

func (s *Server) handleListCompose(c *gin.Context) {
	list, err := s.compose.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleComposeTemplates(c *gin.Context) {
	response.OK(c, s.compose.Templates())
}

func (s *Server) handleCreateCompose(c *gin.Context) {
	var req struct {
		models.ComposeApp
		Scaffold   bool `json:"scaffold"`
		Template   string `json:"template"`
		AutoStart  *bool  `json:"auto_start"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	autoStart := true
	if req.AutoStart != nil {
		autoStart = *req.AutoStart
	} else if !req.Scaffold {
		autoStart = false
	}
	if err := s.compose.Create(&req.ComposeApp, req.Scaffold, req.Template, autoStart); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	d, _ := s.compose.Get(req.ComposeApp.ID)
	if d != nil {
		response.OK(c, d)
		return
	}
	response.OK(c, req.ComposeApp)
}

func (s *Server) handleComposeLogs(c *gin.Context) {
	logs, err := s.compose.Logs(parseID(c), 150)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"log": logs})
}

func (s *Server) handleComposePull(c *gin.Context) {
	if err := s.compose.Pull(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "pulled")
}

func (s *Server) handleComposeSync(c *gin.Context) {
	d, err := s.compose.SyncStatus(parseID(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, d)
}

func (s *Server) handleComposeRestart(c *gin.Context) {
	if err := s.compose.Restart(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	d, err := s.compose.Get(parseID(c))
	if err != nil {
		response.Message(c, "restarted")
		return
	}
	response.OK(c, d)
}

func (s *Server) handleComposeFileGet(c *gin.Context) {
	content, err := s.compose.ReadComposeFile(parseID(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"content": content})
}

func (s *Server) handleComposeFilePut(c *gin.Context) {
	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.compose.WriteComposeFile(parseID(c), req.Content); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "saved")
}

func (s *Server) handleDeleteCompose(c *gin.Context) {
	if err := s.compose.Delete(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleToggleCompose(c *gin.Context) {
	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.compose.Toggle(parseID(c), req.Status); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "updated")
}

func (s *Server) handleToolboxPing(c *gin.Context) {
	var req struct {
		Host string `json:"host" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.toolbox.Ping(req.Host)
	if err != nil {
		response.OK(c, result)
		return
	}
	response.OK(c, result)
}

func (s *Server) handleToolboxTraceroute(c *gin.Context) {
	var req struct {
		Host string `json:"host" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.toolbox.Traceroute(req.Host)
	if err != nil {
		response.OK(c, result)
		return
	}
	response.OK(c, result)
}

func (s *Server) handleToolboxDNS(c *gin.Context) {
	var req struct {
		Domain string `json:"domain" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.toolbox.DNSLookup(req.Domain)
	if err != nil {
		response.OK(c, result)
		return
	}
	response.OK(c, result)
}

func (s *Server) handleGetSettings(c *gin.Context) {
	s.settings.EnsureKeys("ai_enabled", "ai_provider", "ai_api_key", "ai_base_url", "ai_model")
	data, err := s.settings.GetAll()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	if data["ai_api_key"] != "" {
		data["ai_api_key_set"] = "true"
		data["ai_api_key"] = ""
	} else {
		data["ai_api_key_set"] = "false"
	}
	if data["hf_token"] != "" {
		data["hf_token_set"] = "true"
		data["hf_token"] = ""
	} else {
		data["hf_token_set"] = "false"
	}
	response.OK(c, data)
}

func (s *Server) handleUpdateSettings(c *gin.Context) {
	var data map[string]string
	if err := c.ShouldBindJSON(&data); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.settings.Update(data); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.enterprise.Recorder().FromGin(c, "config", "settings_update", "panel_settings", "keys updated", "info", true)
	response.Message(c, "saved")
}

func (s *Server) handleSyncAIModels(c *gin.Context) {
	var req struct {
		Provider string `json:"ai_provider"`
		APIKey   string `json:"ai_api_key"`
		BaseURL  string `json:"ai_base_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	models, err := s.aichat.ListModels(req.Provider, req.APIKey, req.BaseURL)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"models": models})
}

func (s *Server) handleSyncCursorModels(c *gin.Context) {
	var req struct {
		APIKey  string `json:"ai_api_key"`
		BaseURL string `json:"ai_base_url"`
	}
	_ = c.ShouldBindJSON(&req)
	models, err := s.aichat.ListModels("cursor", req.APIKey, req.BaseURL)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"models": models})
}

func (s *Server) handlePHPVersions(c *gin.Context) {
	response.OK(c, s.appstore.ListPHPVersions())
}

func (s *Server) handlePHPAction(c *gin.Context) {
	key := c.Param("key")
	action := c.Param("action")
	if err := s.appstore.ServiceAction(key, action); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	st := s.appstore.PHPRuntimeStatus(key)
	response.OK(c, gin.H{
		"key": key, "status": map[bool]string{true: "running", false: "stopped"}[st.Running],
		"port": st.Port, "pid": st.PID, "message": st.Message,
	})
}

func (s *Server) handleNginxStatus(c *gin.Context) {
	overview, err := s.webserver.Overview()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, overview)
}

func (s *Server) handleNginxOneClickInstall(c *gin.Context) {
	key := c.Param("key")
	if !webserver.IsWebServerKey(key) {
		response.Error(c, 400, "unsupported web server")
		return
	}
	app, err := s.appstore.Get(key)
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	if app.Installed && !appstore.IsSimulatedInstall(key, s.cfg.DataDir) {
		if err := s.webserver.Setup(key, true); err != nil {
			response.Error(c, 500, err.Error())
			return
		}
		overview, _ := s.webserver.Overview()
		response.OK(c, gin.H{"message": "configured and started", "overview": overview})
		return
	}
	if app.Status == "installing" {
		response.Error(c, 409, "installation already in progress")
		return
	}
	if err := s.appstore.Install(key, ""); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "installation started"})
}

func (s *Server) handleNginxSetup(c *gin.Context) {
	key := c.Param("key")
	if !webserver.IsWebServerKey(key) {
		response.Error(c, 400, "unsupported web server")
		return
	}
	var req struct {
		Start bool `json:"start"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := s.webserver.Setup(key, req.Start); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	overview, _ := s.webserver.Overview()
	response.OK(c, overview)
}

func (s *Server) handleLNMPStack(c *gin.Context) {
	_ = s.appstore.InstallStackNamed("lnmp")
	response.OK(c, gin.H{"message": "LNMP stack installation started"})
}

func (s *Server) handleLAMPStack(c *gin.Context) {
	_ = s.appstore.InstallStackNamed("lamp")
	response.OK(c, gin.H{"message": "LAMP stack installation started"})
}

func (s *Server) handleNginxStackByKey(c *gin.Context) {
	key := c.Param("key")
	if err := s.appstore.InstallStackNamed(key); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "stack installation started", "stack": key})
}

func (s *Server) handleNginxStart(c *gin.Context) {
	key := c.Param("key")
	if !webserver.IsWebServerKey(key) {
		response.Error(c, 400, "unsupported web server")
		return
	}
	if err := s.webserver.SwitchExclusive(key); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	overview, _ := s.webserver.Overview()
	response.OK(c, overview)
}

func (s *Server) handleNginxStop(c *gin.Context) {
	key := c.Param("key")
	if !webserver.IsWebServerKey(key) {
		response.Error(c, 400, "unsupported web server")
		return
	}
	if err := s.webserver.Stop(key); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	overview, _ := s.webserver.Overview()
	response.OK(c, overview)
}

func (s *Server) handleNginxReload(c *gin.Context) {
	key := c.Param("key")
	if !webserver.IsWebServerKey(key) {
		response.Error(c, 400, "unsupported web server")
		return
	}
	if err := s.webserver.Reload(key); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "reloaded"})
}

func (s *Server) handleNginxTest(c *gin.Context) {
	key := c.Param("key")
	if !webserver.IsWebServerKey(key) {
		response.Error(c, 400, "unsupported web server")
		return
	}
	output, err := s.webserver.TestConfig(key)
	if err != nil {
		msg := strings.TrimSpace(output)
		if msg == "" {
			msg = err.Error()
		}
		response.Error(c, 400, msg)
		return
	}
	response.OK(c, gin.H{"output": output})
}

func (s *Server) handleNginxGetConfig(c *gin.Context) {
	key := c.Param("key")
	if !webserver.IsWebServerKey(key) {
		response.Error(c, 400, "unsupported web server")
		return
	}
	content, err := s.webserver.ReadMainConfig(key)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"content": content})
}

func (s *Server) handleNginxPutConfig(c *gin.Context) {
	key := c.Param("key")
	if !webserver.IsWebServerKey(key) {
		response.Error(c, 400, "unsupported web server")
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.webserver.WriteMainConfig(key, req.Content); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "saved"})
}
