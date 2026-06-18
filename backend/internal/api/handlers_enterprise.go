package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/services/enterprise"
)

func (s *Server) registerEnterpriseRoutes(admin *gin.RouterGroup) {
	admin.GET("/enterprise/overview", s.handleEnterpriseOverview)
	admin.GET("/enterprise/ha", s.handleEnterpriseHA)
	admin.GET("/enterprise/monitoring", s.handleEnterpriseMonitoring)
	admin.GET("/enterprise/compliance", s.handleEnterpriseCompliance)
	admin.GET("/enterprise/audit-logs", s.handleEnterpriseAuditList)
	admin.GET("/enterprise/audit-logs/export", s.handleEnterpriseAuditExport)
	admin.DELETE("/enterprise/audit-logs", s.handleEnterpriseAuditCleanup)
	admin.GET("/enterprise/audit-settings", s.handleEnterpriseAuditSettingsGet)
	admin.PUT("/enterprise/audit-settings", s.handleEnterpriseAuditSettingsPut)
}

func (s *Server) handleEnterpriseOverview(c *gin.Context) {
	ov, err := s.enterprise.GetOverview()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, ov)
}

func (s *Server) handleEnterpriseHA(c *gin.Context) {
	ha, err := s.enterprise.GetHAStatus()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, ha)
}

func (s *Server) handleEnterpriseMonitoring(c *gin.Context) {
	m, err := s.enterprise.GetAdvancedMonitoring()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, m)
}

func (s *Server) handleEnterpriseCompliance(c *gin.Context) {
	response.OK(c, s.enterprise.RunComplianceChecks())
}

func (s *Server) auditFiltersFromQuery(c *gin.Context) enterprise.AuditFilters {
	f := enterprise.AuditFilters{
		Category: c.Query("category"),
		Action:   c.Query("action"),
		Level:    c.Query("level"),
		Username: c.Query("username"),
	}
	if v := c.Query("success"); v != "" {
		b := v == "true" || v == "1"
		f.Success = &b
	}
	return f
}

func (s *Server) handleEnterpriseAuditList(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	res, err := s.enterprise.List(s.auditFiltersFromQuery(c), limit, offset)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, res)
}

func (s *Server) handleEnterpriseAuditExport(c *gin.Context) {
	format := c.DefaultQuery("format", "json")
	data, name, err := s.enterprise.Export(s.auditFiltersFromQuery(c), format)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	ct := "application/json"
	if strings.ToLower(format) == "csv" {
		ct = "text/csv"
	}
	c.Header("Content-Disposition", "attachment; filename="+name)
	c.Data(http.StatusOK, ct, data)
}

func (s *Server) handleEnterpriseAuditCleanup(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "0"))
	n, err := s.enterprise.Cleanup(days)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.enterprise.Recorder().FromGin(c, "config", "audit_cleanup", "audit_logs",
		"deleted="+strconv.FormatInt(n, 10), "info", true)
	response.OK(c, gin.H{"deleted": n})
}

func (s *Server) handleEnterpriseAuditSettingsGet(c *gin.Context) {
	response.OK(c, s.enterprise.GetAuditSettings())
}

func (s *Server) handleEnterpriseAuditSettingsPut(c *gin.Context) {
	var req struct {
		RetentionDays int  `json:"retention_days"`
		SyslogForward bool `json:"syslog_forward"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if req.RetentionDays <= 0 {
		req.RetentionDays = 90
	}
	if req.RetentionDays > 3650 {
		req.RetentionDays = 3650
	}
	if err := s.enterprise.UpdateAuditSettings(req.RetentionDays, req.SyslogForward); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.enterprise.Recorder().FromGin(c, "config", "audit_settings", "audit_settings",
		"retention="+strconv.Itoa(req.RetentionDays), "info", true)
	response.OK(c, s.enterprise.GetAuditSettings())
}
