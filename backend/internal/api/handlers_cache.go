package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/aichat"
	"github.com/luuuunet/owpanel/internal/services/cache"
)

func (s *Server) registerCacheRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/cache/config", s.handleGetCacheConfig)
	authorized.PUT("/cache/config", s.handleUpdateCacheConfig)
	authorized.GET("/cache/status", s.handleCacheStatus)
	authorized.GET("/cache/analytics", s.handleCacheAnalytics)
	authorized.GET("/cache/preview", s.handlePreviewCache)
	authorized.POST("/cache/apply", s.handleApplyCache)
	authorized.POST("/cache/purge", s.handlePurgeCacheAll)
	authorized.POST("/cache/purge/:domain", s.handlePurgeCacheSite)
	authorized.POST("/cache/purge/:domain/paths", s.handlePurgeCachePaths)
	authorized.POST("/cache/presets/:name", s.handleApplyCachePreset)
	authorized.GET("/cache/sites", s.handleListCacheSites)
	authorized.POST("/cache/sites/enable-all", s.handleEnableCacheAllSites)
	authorized.PATCH("/cache/sites/:id", s.handleToggleCacheSite)
	authorized.POST("/cache/sites/:id/auto-enable", s.handleAutoEnableSiteCache)

	authorized.GET("/cache/rules", s.handleListCacheRules)
	authorized.POST("/cache/rules", s.handleCreateCacheRule)
	authorized.POST("/cache/rules/ai/suggest", s.handleCacheRuleAISuggest)
	authorized.PUT("/cache/rules/:id", s.handleUpdateCacheRule)
	authorized.DELETE("/cache/rules/:id", s.handleDeleteCacheRule)
}

func (s *Server) handleGetCacheConfig(c *gin.Context) {
	cfg, err := s.cache.GetConfig()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, cfg)
}

func (s *Server) handleUpdateCacheConfig(c *gin.Context) {
	var patch models.CacheConfig
	if err := c.ShouldBindJSON(&patch); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	cfg, err := s.cache.UpdateConfig(&patch)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, cfg)
}

func (s *Server) handleCacheStatus(c *gin.Context) {
	st, err := s.cache.StatusSummary()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, st)
}

func (s *Server) handleCacheAnalytics(c *gin.Context) {
	hours := 24
	if h := c.Query("hours"); h != "" {
		if n, err := strconv.Atoi(h); err == nil && n > 0 {
			hours = n
		}
	}
	report, err := s.cache.GetAnalytics(hours, c.Query("domain"))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	if report.Summary.TotalRequests > 0 {
		_ = s.cache.RecordSnapshot(c.Query("domain"), report)
	}
	response.OK(c, report)
}

func (s *Server) handlePreviewCache(c *gin.Context) {
	preview, err := s.cache.Preview()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"preview": preview})
}

func (s *Server) handleApplyCache(c *gin.Context) {
	result, err := s.cache.Apply()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handlePurgeCacheAll(c *gin.Context) {
	result, err := s.cache.PurgeAll()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handlePurgeCacheSite(c *gin.Context) {
	result, err := s.cache.PurgeSite(c.Param("domain"))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handlePurgeCachePaths(c *gin.Context) {
	var req cache.PurgePathRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.cache.PurgePaths(c.Param("domain"), req.Paths)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleApplyCachePreset(c *gin.Context) {
	result, err := s.cache.ApplyPreset(c.Param("name"))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleListCacheSites(c *gin.Context) {
	list, err := s.cache.ListSites()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleEnableCacheAllSites(c *gin.Context) {
	n, err := s.cache.EnableAllSites()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	if _, err := s.cache.Apply(); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"updated": n})
}

func (s *Server) handleToggleCacheSite(c *gin.Context) {
	var req struct {
		Enabled        *bool `json:"enabled"`
		DevMode        *bool `json:"cache_dev_mode"`
		CacheHtmlTTL   *int  `json:"cache_html_ttl"`
		CacheStaticTTL *int  `json:"cache_static_ttl"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	id := parseID(c)
	if err := s.cache.UpdateSiteCache(id, req.Enabled, req.DevMode, req.CacheHtmlTTL, req.CacheStaticTTL); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	site, err := s.website.Get(id)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	if err := s.website.ApplyVhost(id); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, site)
}

func (s *Server) handleAutoEnableSiteCache(c *gin.Context) {
	result, err := s.cache.AutoEnableSite(parseID(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleListCacheRules(c *gin.Context) {
	rules, err := s.cache.ListAllRules()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, rules)
}

func (s *Server) handleCreateCacheRule(c *gin.Context) {
	var rule models.CacheRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	created, err := s.cache.CreateRule(&rule)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	if _, err := s.cache.Apply(); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, created)
}

func (s *Server) handleUpdateCacheRule(c *gin.Context) {
	var patch models.CacheRule
	if err := c.ShouldBindJSON(&patch); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	rule, err := s.cache.UpdateRule(parseID(c), &patch)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	if _, err := s.cache.Apply(); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, rule)
}

func (s *Server) handleDeleteCacheRule(c *gin.Context) {
	if err := s.cache.DeleteRule(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	if _, err := s.cache.Apply(); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

func (s *Server) handleCacheRuleAISuggest(c *gin.Context) {
	var req aichat.CacheRuleChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if req.Message == "" {
		response.Error(c, 400, "message is required")
		return
	}
	result, err := s.aichat.CacheRuleChat(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}
