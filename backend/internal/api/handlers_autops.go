package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/services/autops"
)

func (s *Server) registerAutoOpsRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/auto-ops/status", s.handleAutoOpsStatus)
	authorized.GET("/auto-ops/overview", s.handleAutoOpsOverview)
	authorized.PUT("/auto-ops/config", s.handleAutoOpsConfig)
	authorized.GET("/auto-ops/events", s.handleAutoOpsEvents)
	authorized.POST("/auto-ops/scan", s.handleAutoOpsScan)
	authorized.GET("/auto-ops/website-audits", s.handleAutoOpsWebsiteAudits)
	authorized.GET("/auto-ops/website-audits/:id", s.handleAutoOpsWebsiteAuditDetail)
	authorized.POST("/auto-ops/website-audits/:id/scan", s.handleAutoOpsWebsiteAuditScan)
	authorized.POST("/auto-ops/website-scan", s.handleAutoOpsWebsiteScanAll)
	authorized.PATCH("/auto-ops/watch/:key", s.handleAutoOpsWatch)
	authorized.POST("/auto-ops/watch/bulk", s.handleAutoOpsWatchBulk)
}

func (s *Server) handleAutoOpsOverview(c *gin.Context) {
	ov, err := s.autops.GetOverview()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, ov)
}

func (s *Server) handleAutoOpsStatus(c *gin.Context) {
	st, err := s.autops.GetStatus()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, st)
}

func (s *Server) handleAutoOpsConfig(c *gin.Context) {
	var cfg autops.Config
	if err := c.ShouldBindJSON(&cfg); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.autops.UpdateConfig(cfg); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "updated")
}

func (s *Server) handleAutoOpsEvents(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	list, err := s.autops.ListEventsFiltered(autops.EventFilter{
		AppKey:    c.Query("app_key"),
		EventType: c.Query("event_type"),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleAutoOpsScan(c *gin.Context) {
	if err := s.autops.ScanNow(); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	st, _ := s.autops.GetStatus()
	response.OK(c, st)
}

func (s *Server) handleAutoOpsWebsiteAudits(c *gin.Context) {
	response.OK(c, s.autops.ListWebsiteAudits())
}

func (s *Server) handleAutoOpsWebsiteAuditDetail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, 400, "invalid id")
		return
	}
	report, err := s.autops.GetWebsiteAudit(uint(id))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, report)
}

func (s *Server) handleAutoOpsWebsiteAuditScan(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Error(c, 400, "invalid id")
		return
	}
	report, err := s.autops.AuditWebsiteNow(uint(id))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, report)
}

func (s *Server) handleAutoOpsWebsiteScanAll(c *gin.Context) {
	s.autops.ScanWebsiteAudits(true)
	response.OK(c, s.autops.ListWebsiteAudits())
}

func (s *Server) handleAutoOpsWatch(c *gin.Context) {
	var body struct {
		WatchEnabled *bool `json:"watch_enabled"`
		AutoRestart  *bool `json:"auto_restart"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.autops.UpdateWatch(c.Param("key"), body.WatchEnabled, body.AutoRestart); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "updated")
}

func (s *Server) handleAutoOpsWatchBulk(c *gin.Context) {
	var body struct {
		Keys         []string `json:"keys"`
		WatchEnabled bool     `json:"watch_enabled"`
		AutoRestart  bool     `json:"auto_restart"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.autops.BulkUpdateWatch(body.Keys, body.WatchEnabled, body.AutoRestart); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "updated")
}
