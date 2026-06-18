package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/auth"
)

func (s *Server) registerSecurityExtraRoutes(admin *gin.RouterGroup) {
	admin.GET("/security/score", s.handleSecurityScore)
	admin.GET("/security/login-logs", s.handleSecurityLoginLogs)
	admin.DELETE("/security/login-logs", s.handleSecurityCleanupLoginLogs)
	admin.GET("/security/panel-access", s.handleSecurityPanelAccessGet)
	admin.PUT("/security/panel-access", s.handleSecurityPanelAccessPut)
}

func (s *Server) handleSecurityScore(c *gin.Context) {
	response.OK(c, s.security.ComputeScore())
}

func (s *Server) handleSecurityLoginLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	rows, total, err := auth.ListLoginEvents(s.db, limit, offset)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"items": rows, "total": total})
}

func (s *Server) handleSecurityCleanupLoginLogs(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "90"))
	n, err := auth.CleanupLoginEvents(s.db, days)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": n})
}

func (s *Server) handleSecurityPanelAccessGet(c *gin.Context) {
	all, err := s.settings.GetAll()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{
		"panel_ip_whitelist_enabled": all["panel_ip_whitelist_enabled"],
		"panel_ip_whitelist":         all["panel_ip_whitelist"],
		"panel_ip_blacklist":         all["panel_ip_blacklist"],
		"password_require_strong":    all["password_require_strong"],
		"panel_security_headers":     all["panel_security_headers"],
	})
}

func (s *Server) handleSecurityPanelAccessPut(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	allowed := map[string]bool{
		"panel_ip_whitelist_enabled": true,
		"panel_ip_whitelist":         true,
		"panel_ip_blacklist":         true,
		"password_require_strong":    true,
		"panel_security_headers":     true,
	}
	data := map[string]string{}
	for k, v := range req {
		if allowed[k] {
			data[k] = v
		}
	}
	if len(data) == 0 {
		response.Error(c, 400, "no valid fields")
		return
	}
	if err := s.settings.Update(data); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.enterprise.Recorder().FromGin(c, "security", "panel_access_update", "panel_access", "security settings saved", "info", true)
	response.Message(c, "saved")
}

func (s *Server) passwordRequireStrong() bool {
	all, err := s.settings.GetAll()
	if err != nil {
		return true
	}
	return all["password_require_strong"] != "false"
}
