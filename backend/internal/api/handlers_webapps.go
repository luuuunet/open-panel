package api

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/settings"
	"github.com/luuuunet/owpanel/internal/services/waf"
	"github.com/luuuunet/owpanel/internal/services/wordpress"
)

func (s *Server) registerWAFRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/waf", s.handleListWAF)
	authorized.POST("/waf", s.handleCreateWAF)
	authorized.DELETE("/waf/:id", s.handleDeleteWAF)
	authorized.PATCH("/waf/:id/toggle", s.handleToggleWAF)

	authorized.GET("/waf/config", s.handleGetWAFConfig)
	authorized.PUT("/waf/config", s.handleUpdateWAFConfig)
	authorized.GET("/waf/blacklist", s.handleListIPBlacklist)
	authorized.POST("/waf/blacklist", s.handleAddIPBlacklist)
	authorized.POST("/waf/blacklist/import", s.handleImportIPBlacklist)
	authorized.DELETE("/waf/blacklist/:id", s.handleDeleteIPBlacklist)
	authorized.GET("/waf/whitelist", s.handleListIPWhitelist)
	authorized.POST("/waf/whitelist", s.handleAddIPWhitelist)
	authorized.POST("/waf/whitelist/import", s.handleImportIPWhitelist)
	authorized.DELETE("/waf/whitelist/:id", s.handleDeleteIPWhitelist)
	authorized.GET("/waf/preview", s.handlePreviewWAF)
	authorized.POST("/waf/apply", s.handleApplyWAF)
	authorized.GET("/waf/status", s.handleWAFStatus)
	authorized.GET("/waf/logs/tail", s.handleWAFSecurityLogTail)
	authorized.GET("/waf/geoip/countries", s.handleListGeoCountries)
	authorized.GET("/waf/geoip/status", s.handleGeoIPStatus)

	authorized.GET("/waf/crawlers", s.handleListCrawlerPresets)
	authorized.GET("/waf/crawlers/rules", s.handleGetCrawlerRules)
	authorized.PUT("/waf/crawlers/rules", s.handleSaveCrawlerRules)
	authorized.POST("/waf/crawlers/apply", s.handleApplyCrawlerRules)
}

func (s *Server) registerWordPressRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/wordpress", s.handleListWordPress)
	authorized.GET("/wordpress/:id", s.handleGetWordPress)
	authorized.GET("/wordpress/:id/credentials", s.handleGetWordPressCredentials)
	authorized.PATCH("/wordpress/:id", s.handleUpdateWordPress)
	authorized.POST("/wordpress", s.handleCreateWordPress)
	authorized.GET("/wordpress/deploy/:jobId", s.handleWordPressDeployStatus)
	authorized.POST("/wordpress/:id/repair", s.handleRepairWordPress)
	authorized.POST("/wordpress/:id/redeploy", s.handleRedeployWordPress)
	authorized.POST("/wordpress/:id/ssl", s.handleIssueWordPressSSL)
	authorized.GET("/wordpress/:id/domains", s.handleListWordPressDomains)
	authorized.POST("/wordpress/:id/domains", s.handleAddWordPressDomain)
	authorized.POST("/wordpress/:id/domains/apply", s.handleApplyWordPressDomains)
	authorized.DELETE("/wordpress/:id/domains/:domainId", s.handleRemoveWordPressDomain)
	authorized.DELETE("/wordpress/:id", s.handleDeleteWordPress)
	authorized.POST("/wordpress/:id/backup", s.handleBackupWordPress)
	authorized.GET("/wordpress/:id/backups", s.handleListWordPressBackups)
	authorized.DELETE("/wordpress/:id/backups/:backupId", s.handleDeleteWordPressBackup)
	authorized.GET("/wordpress/:id/backups/:backupId/download", s.handleDownloadWordPressBackup)
	authorized.GET("/wordpress/:id/backup/config", s.handleWordPressBackupConfig)
}

func (s *Server) registerNodeJSRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/nodejs", s.handleListNodeJS)
	authorized.POST("/nodejs", s.handleCreateNodeJS)
	authorized.DELETE("/nodejs/:id", s.handleDeleteNodeJS)
	authorized.PATCH("/nodejs/:id/toggle", s.handleToggleNodeJS)
	authorized.PATCH("/nodejs/:id", s.handlePatchNodeJS)
}

func (s *Server) registerJavaRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/java", s.handleListJava)
	authorized.POST("/java", s.handleCreateJava)
	authorized.DELETE("/java/:id", s.handleDeleteJava)
	authorized.PATCH("/java/:id/toggle", s.handleToggleJava)
	authorized.PATCH("/java/:id", s.handlePatchJava)
}

func (s *Server) registerSecurityRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/security/scan", s.handleSecurityScan)
	authorized.POST("/security/check/:key/fix", s.handleSecurityFix)
	authorized.POST("/security/check/fix-all", s.handleSecurityFixAll)
	s.registerSecurityExtraRoutes(authorized)
}

func (s *Server) handleListWAF(c *gin.Context) {
	s.waf.SeedDefaults()
	list, err := s.waf.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateWAF(c *gin.Context) {
	var rule models.WAFRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.waf.Create(&rule); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, rule)
}

func (s *Server) handleDeleteWAF(c *gin.Context) {
	if err := s.waf.Delete(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleToggleWAF(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.waf.Toggle(parseID(c), req.Enabled); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "updated")
}

func (s *Server) handleGetWAFConfig(c *gin.Context) {
	cfg, err := s.waf.GetConfig()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, cfg)
}

func (s *Server) handleUpdateWAFConfig(c *gin.Context) {
	var patch models.SecurityConfig
	if err := c.ShouldBindJSON(&patch); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	cfg, err := s.waf.UpdateConfig(&patch)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, cfg)
}

func (s *Server) handleListIPBlacklist(c *gin.Context) {
	list, err := s.waf.ListBlacklist()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleAddIPBlacklist(c *gin.Context) {
	var req struct {
		IP     string `json:"ip" binding:"required"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	entry, err := s.waf.AddBlacklist(req.IP, req.Reason, "manual")
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, entry)
}

func (s *Server) handleImportIPBlacklist(c *gin.Context) {
	var req struct {
		IPs    []string `json:"ips"`
		Text   string   `json:"text"`
		Reason string   `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	ips := req.IPs
	if req.Text != "" {
		for _, line := range strings.Split(req.Text, "\n") {
			ips = append(ips, strings.TrimSpace(line))
		}
	}
	n, err := s.waf.ImportBlacklist(ips, req.Reason)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"imported": n})
}

func (s *Server) handleDeleteIPBlacklist(c *gin.Context) {
	if err := s.waf.RemoveBlacklist(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleListIPWhitelist(c *gin.Context) {
	list, err := s.waf.ListWhitelist()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleAddIPWhitelist(c *gin.Context) {
	var req struct {
		IP     string `json:"ip" binding:"required"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	entry, err := s.waf.AddWhitelist(req.IP, req.Reason, "manual")
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, entry)
}

func (s *Server) handleImportIPWhitelist(c *gin.Context) {
	var req struct {
		IPs    []string `json:"ips"`
		Text   string   `json:"text"`
		Reason string   `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	ips := req.IPs
	if req.Text != "" {
		for _, line := range strings.Split(req.Text, "\n") {
			ips = append(ips, strings.TrimSpace(line))
		}
	}
	count, err := s.waf.ImportWhitelist(ips, req.Reason)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"imported": count})
}

func (s *Server) handleDeleteIPWhitelist(c *gin.Context) {
	if err := s.waf.RemoveWhitelist(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handlePreviewWAF(c *gin.Context) {
	preview, err := s.waf.Preview()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"preview": preview})
}

func (s *Server) handleApplyWAF(c *gin.Context) {
	result, err := s.waf.Apply()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleWAFStatus(c *gin.Context) {
	response.OK(c, s.waf.StatusSummary())
}

func (s *Server) handleWAFSecurityLogTail(c *gin.Context) {
	cfg, err := s.waf.GetConfig()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	path := strings.TrimSpace(cfg.SecurityLogPath)
	if path == "" {
		path = settings.DefaultSecurityLogPath(s.cfg.DataDir)
	}
	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "300"))
	result, err := s.logs.TailPath(path, lines)
	if err != nil {
		response.Error(c, 404, "log source not found")
		return
	}
	response.OK(c, result)
}

func (s *Server) handleListGeoCountries(c *gin.Context) {
	response.OK(c, waf.ListCountries())
}

func (s *Server) handleGeoIPStatus(c *gin.Context) {
	st, err := s.waf.GeoIPStatus()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, st)
}

func (s *Server) handleListCrawlerPresets(c *gin.Context) {
	response.OK(c, waf.ListCrawlerPresets())
}

func (s *Server) handleGetCrawlerRules(c *gin.Context) {
	websiteID, _ := strconv.ParseUint(c.DefaultQuery("website_id", "0"), 10, 64)
	response.OK(c, s.waf.GetCrawlerRules(uint(websiteID)))
}

func (s *Server) handleSaveCrawlerRules(c *gin.Context) {
	var req waf.SaveCrawlerRulesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.waf.SaveCrawlerRules(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, s.waf.GetCrawlerRules(req.WebsiteID))
}

func (s *Server) handleApplyCrawlerRules(c *gin.Context) {
	var body struct {
		WebsiteID uint `json:"website_id"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.WebsiteID > 0 {
		if err := s.website.ApplyVhost(body.WebsiteID); err != nil {
			response.Error(c, 500, err.Error())
			return
		}
		response.OK(c, gin.H{"message": "站点 Nginx 配置已更新", "website_id": body.WebsiteID})
		return
	}
	if err := s.website.RegenerateAll(); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "全部站点 Nginx 配置已更新"})
}

func (s *Server) handleListWordPress(c *gin.Context) {
	list, err := s.wordpress.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleGetWordPress(c *gin.Context) {
	site, err := s.wordpress.Get(parseID(c))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, site)
}

func (s *Server) handleGetWordPressCredentials(c *gin.Context) {
	creds, err := s.wordpress.GetSiteCredentials(parseID(c))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, creds)
}

func (s *Server) handleUpdateWordPress(c *gin.Context) {
	var req wordpress.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	site, err := s.wordpress.Update(parseID(c), &req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, site)
}

func (s *Server) handleCreateWordPress(c *gin.Context) {
	var req wordpress.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	job, err := s.wordpress.StartDeploy(&req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{
		"job_id":  job.ID,
		"site_id": job.SiteID,
		"domain":  job.Domain,
		"status":  job.Status,
		"logs":    job.Logs,
	})
}

func (s *Server) handleWordPressDeployStatus(c *gin.Context) {
	job, ok := wordpress.GetDeployJob(c.Param("jobId"))
	if !ok {
		response.Error(c, 404, "deploy job not found")
		return
	}
	response.OK(c, job)
}

func (s *Server) handleListWordPressDomains(c *gin.Context) {
	list, err := s.wordpress.ListDomains(parseID(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleAddWordPressDomain(c *gin.Context) {
	var req struct {
		Domain string `json:"domain" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	entry, err := s.wordpress.AddDomain(parseID(c), req.Domain)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, entry)
}

func (s *Server) handleRemoveWordPressDomain(c *gin.Context) {
	siteID := parseID(c)
	domainID := parseParamID(c, "domainId")
	if err := s.wordpress.RemoveDomain(siteID, domainID); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleApplyWordPressDomains(c *gin.Context) {
	if err := s.wordpress.ApplyDomains(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	site, _ := s.wordpress.Get(parseID(c))
	response.OK(c, site)
}

func (s *Server) handleRepairWordPress(c *gin.Context) {
	id := parseID(c)
	if err := s.wordpress.Repair(id); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	site, _ := s.wordpress.Get(id)
	creds, _ := s.wordpress.EnsureFTPForSite(id)
	out := gin.H{"site": site}
	if creds != nil && (creds.FtpUser != "" || creds.FtpPassword != "") {
		out["ftp_user"] = creds.FtpUser
		out["ftp_password"] = creds.FtpPassword
	}
	response.OK(c, out)
}

func (s *Server) handleIssueWordPressSSL(c *gin.Context) {
	var req struct {
		Email string `json:"email"`
	}
	_ = c.ShouldBindJSON(&req)
	id := parseID(c)
	if err := s.wordpress.IssueSSLForSite(id, req.Email); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	site, _ := s.wordpress.Get(id)
	response.OK(c, site)
}

func (s *Server) handleRedeployWordPress(c *gin.Context) {
	job, err := s.wordpress.Redeploy(parseID(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{
		"job_id":  job.ID,
		"site_id": job.SiteID,
		"domain":  job.Domain,
		"status":  job.Status,
		"logs":    job.Logs,
	})
}

func (s *Server) handleDeleteWordPress(c *gin.Context) {
	if err := s.wordpress.Delete(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleBackupWordPress(c *gin.Context) {
	rec, err := s.wordpress.RunBackup(parseID(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, rec)
}

func (s *Server) handleListWordPressBackups(c *gin.Context) {
	list, err := s.wordpress.ListBackups(parseID(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleDeleteWordPressBackup(c *gin.Context) {
	if err := s.wordpress.DeleteBackup(parseID(c), parseParamID(c, "backupId")); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleDownloadWordPressBackup(c *gin.Context) {
	path, err := s.wordpress.GetBackupFile(parseID(c), parseParamID(c, "backupId"))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	c.FileAttachment(path, filepath.Base(path))
}

func (s *Server) handleWordPressBackupConfig(c *gin.Context) {
	response.OK(c, s.wordpress.BackupConfig())
}

func (s *Server) handleListNodeJS(c *gin.Context) {
	list, err := s.nodejs.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateNodeJS(c *gin.Context) {
	var p models.NodeProject
	if err := c.ShouldBindJSON(&p); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.nodejs.Create(&p); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, p)
}

func (s *Server) handleDeleteNodeJS(c *gin.Context) {
	if err := s.nodejs.Delete(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleToggleNodeJS(c *gin.Context) {
	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.nodejs.Toggle(parseID(c), req.Status); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "updated")
}

func (s *Server) handleListJava(c *gin.Context) {
	list, err := s.java.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateJava(c *gin.Context) {
	var p models.JavaProject
	if err := c.ShouldBindJSON(&p); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.java.Create(&p); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, p)
}

func (s *Server) handleDeleteJava(c *gin.Context) {
	if err := s.java.Delete(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleToggleJava(c *gin.Context) {
	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.java.Toggle(parseID(c), req.Status); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "updated")
}

func (s *Server) handlePatchNodeJS(c *gin.Context) {
	var req struct {
		Remark string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	p, err := s.nodejs.UpdateRemark(parseID(c), req.Remark)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, p)
}

func (s *Server) handlePatchJava(c *gin.Context) {
	var req struct {
		Remark string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	p, err := s.java.UpdateRemark(parseID(c), req.Remark)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, p)
}

func (s *Server) handleSecurityScan(c *gin.Context) {
	response.OK(c, s.security.Scan())
}

func (s *Server) handleSecurityFix(c *gin.Context) {
	result, err := s.security.Fix(c.Param("key"))
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleSecurityFixAll(c *gin.Context) {
	result, err := s.security.FixAll()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}
