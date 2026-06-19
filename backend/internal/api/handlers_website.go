package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/extension"
	"github.com/luuuunet/owpanel/internal/services/website"
)

func (s *Server) registerWebsiteRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/websites", s.handleListWebsites)
	authorized.GET("/websites/projects", s.handleListWebsiteProjects)
	authorized.GET("/websites/webserver", s.handleWebServerOverview)
	authorized.POST("/websites/webserver/:key/start", s.handleStartWebServer)
	authorized.GET("/websites/options", s.handleWebsiteOptions)
	authorized.POST("/websites/batch-delete", s.handleBatchDeleteWebsites)
	authorized.POST("/websites", s.handleCreateWebsite)
	authorized.POST("/websites/batch", s.handleBatchCreateWebsites)
	s.registerAISiteRoutes(authorized)
	authorized.POST("/domains/check", s.handleCheckDomains)
	authorized.GET("/websites/:id", s.handleGetWebsite)
	authorized.PATCH("/websites/:id", s.handleUpdateWebsite)
	authorized.POST("/websites/:id/toggle", s.handleToggleWebsite)
	authorized.POST("/websites/:id/cross-site-protect/toggle", s.handleToggleCrossSiteProtect)
	authorized.POST("/websites/:id/php-accel/toggle", s.handleTogglePHPAccel)
	authorized.DELETE("/websites/:id", s.handleDeleteWebsite)

	authorized.GET("/websites/:id/domains", s.handleListWebsiteDomains)
	authorized.POST("/websites/:id/domains", s.handleAddWebsiteDomains)
	authorized.DELETE("/websites/:id/domains/:aliasId", s.handleRemoveWebsiteDomain)
	authorized.POST("/websites/:id/domains/batch-delete", s.handleBatchRemoveWebsiteDomains)
	authorized.POST("/websites/:id/apply", s.handleApplyWebsiteVhost)
	authorized.GET("/websites/:id/nginx", s.handleGetWebsiteNginx)
	authorized.PUT("/websites/:id/nginx", s.handleSaveWebsiteNginx)
	authorized.GET("/websites/:id/logs", s.handleGetWebsiteLogs)
	authorized.POST("/websites/:id/composer", s.handleRunComposer)
	authorized.POST("/websites/:id/ssl/issue", s.handleIssueSiteSSL)

	authorized.GET("/websites/:id/backups", s.handleListWebsiteBackups)
	authorized.POST("/websites/:id/backups", s.handleRunWebsiteBackup)
	authorized.DELETE("/websites/:id/backups/:backupId", s.handleDeleteWebsiteBackup)
	authorized.GET("/websites/:id/backup/config", s.handleGetWebsiteBackupConfig)
	authorized.PATCH("/websites/:id/backup/config", s.handleUpdateWebsiteBackupConfig)

	authorized.GET("/websites/:id/subdirs", s.handleListWebsiteSubdirs)
	authorized.POST("/websites/:id/subdirs", s.handleAddWebsiteSubdir)
	authorized.PUT("/websites/:id/subdirs/:subId", s.handleUpdateWebsiteSubdir)
	authorized.DELETE("/websites/:id/subdirs/:subId", s.handleDeleteWebsiteSubdir)
}

func (s *Server) handleListWebsites(c *gin.Context) {
	sites, err := s.website.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, sites)
}

func (s *Server) handleWebsiteOptions(c *gin.Context) {
	categories, _ := s.website.ListCategories()
	phpVersions := s.appstore.ListPHPVersions()
	var phpOpts []gin.H
	phpOpts = append(phpOpts, gin.H{"value": "static", "label": "纯静态"})
	for _, p := range phpVersions {
		phpOpts = append(phpOpts, gin.H{
			"value": p.Version, "label": "PHP-" + p.Version, "key": p.Key,
			"installed": p.Installed, "status": p.Status,
		})
	}
	response.OK(c, gin.H{
		"default_root": s.website.DefaultRootBase(),
		"categories":   categories,
		"php_versions": phpOpts,
		"ftp_options": []gin.H{
			{"value": "none", "label": "不创建"},
			{"value": "create", "label": "创建 FTP"},
		},
		"database_options": []gin.H{
			{"value": "none", "label": "不创建"},
			{"value": "mysql", "label": "MySQL"},
		},
		"dns_modes": []gin.H{
			{"value": "manual", "label": "手动添加记录"},
			{"value": "auto", "label": "自动添加记录"},
		},
	})
}

func (s *Server) handleCreateWebsite(c *gin.Context) {
	var req website.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.website.Create(&req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.emitExtension(extension.EventWebsiteCreated, map[string]interface{}{
		"id": result.Site.ID, "domain": result.Site.Domain, "root_path": result.Site.RootPath,
	})
	response.OK(c, result)
}

func (s *Server) handleBatchCreateWebsites(c *gin.Context) {
	var req website.BatchCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.website.BatchCreate(&req)
	if err != nil && (result == nil || len(result.Created) == 0) {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleDeleteWebsite(c *gin.Context) {
	id := parseID(c)
	site, err := s.website.Get(id)
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	if err := s.website.Delete(id); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.emitExtension(extension.EventWebsiteDeleted, map[string]interface{}{
		"id": id, "domain": site.Domain,
	})
	response.Message(c, "deleted")
}

func (s *Server) handleGetWebsite(c *gin.Context) {
	site, err := s.website.Get(parseID(c))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, site)
}

func (s *Server) handleUpdateWebsite(c *gin.Context) {
	var req website.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	site, err := s.website.UpdateSite(parseID(c), &req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, site)
}

func (s *Server) handleListWebsiteDomains(c *gin.Context) {
	list, err := s.website.ListDomains(parseID(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleAddWebsiteDomains(c *gin.Context) {
	var req struct {
		DomainsText string `json:"domains_text"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	added, err := s.website.AddDomains(parseID(c), req.DomainsText)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, added)
}

func (s *Server) handleRemoveWebsiteDomain(c *gin.Context) {
	siteID := parseID(c)
	aliasID := parseParamID(c, "aliasId")
	if err := s.website.RemoveDomain(siteID, aliasID); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "removed")
}

func (s *Server) handleBatchRemoveWebsiteDomains(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.website.BatchRemoveDomains(parseID(c), req.IDs); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "removed")
}

func (s *Server) handleApplyWebsiteVhost(c *gin.Context) {
	if err := s.website.ApplyVhost(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "applied")
}

func (s *Server) handleGetWebsiteNginx(c *gin.Context) {
	content, err := s.website.ReadNginxConf(parseID(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"content": content})
}

func (s *Server) handleSaveWebsiteNginx(c *gin.Context) {
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.website.SaveNginxConf(parseID(c), req.Content); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "saved")
}

func (s *Server) handleGetWebsiteLogs(c *gin.Context) {
	lines := 100
	if v := c.Query("lines"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			lines = n
		}
	}
	data, err := s.website.SiteLogs(parseID(c), lines)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, data)
}

func (s *Server) handleListWebsiteProjects(c *gin.Context) {
	projectType := c.DefaultQuery("type", "php")
	search := c.Query("search")
	list, err := s.website.ListProjects(projectType, search)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleWebServerOverview(c *gin.Context) {
	overview, err := s.webserver.Overview()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, overview)
}

func (s *Server) handleStartWebServer(c *gin.Context) {
	key := c.Param("key")
	if key != "nginx" && key != "openresty" && key != "apache" {
		response.Error(c, 400, "仅支持 nginx、openresty 或 apache")
		return
	}
	if err := s.webserver.SwitchExclusive(key); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	overview, _ := s.webserver.Overview()
	response.OK(c, overview)
}

func (s *Server) handleToggleWebsite(c *gin.Context) {
	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	site, err := s.website.ToggleSite(parseID(c), req.Status)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, site)
}

func (s *Server) handleToggleCrossSiteProtect(c *gin.Context) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	_ = c.ShouldBindJSON(&req)
	site, err := s.website.ToggleCrossSiteProtect(parseID(c), req.Enabled)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{
		"cross_site_protect_enabled": site.CrossSiteProtectEnabled,
		"hotlink_enabled":            site.HotlinkEnabled,
	})
}

func (s *Server) handleTogglePHPAccel(c *gin.Context) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	_ = c.ShouldBindJSON(&req)
	restartPHP := func(key string) error {
		return s.appstore.ServiceAction(key, "restart")
	}
	result, err := s.website.TogglePHPAccel(parseID(c), req.Enabled, restartPHP)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleBatchDeleteWebsites(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.website.BatchDelete(req.IDs); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleRunComposer(c *gin.Context) {
	var req struct {
		Command string `json:"command"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.website.RunComposer(parseID(c), req.Command)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleIssueSiteSSL(c *gin.Context) {
	var req struct {
		Email      string `json:"email"`
		SanDomains string `json:"san_domains"`
		Deploy     bool   `json:"deploy"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := s.website.IssueSSL(parseID(c), req.Email, req.SanDomains, true); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "ssl issued")
}

func (s *Server) handleListWebsiteSubdirs(c *gin.Context) {
	list, err := s.website.ListSubdirs(parseID(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleAddWebsiteSubdir(c *gin.Context) {
	var req website.SubdirRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	sub, err := s.website.AddSubdir(parseID(c), &req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, sub)
}

func (s *Server) handleUpdateWebsiteSubdir(c *gin.Context) {
	var req website.SubdirRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	sub, err := s.website.UpdateSubdir(parseID(c), parseParamID(c, "subId"), &req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, sub)
}

func (s *Server) handleDeleteWebsiteSubdir(c *gin.Context) {
	if err := s.website.DeleteSubdir(parseID(c), parseParamID(c, "subId")); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}
