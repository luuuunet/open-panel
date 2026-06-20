package api

import (
	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/services/panelupdate"
)

func (s *Server) handleUpdateStatus(c *gin.Context) {
	st, err := s.panelupdate.Status(c.Request.Context())
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, st)
}

func (s *Server) handleUpdateCheck(c *gin.Context) {
	check, err := s.panelupdate.Check(c.Request.Context())
	if err != nil {
		response.Error(c, 502, err.Error())
		return
	}
	response.OK(c, check)
}

func (s *Server) handleUpdateConfig(c *gin.Context) {
	var req panelupdate.Config
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.panelupdate.SaveConfig(req); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.enterprise.Recorder().FromGin(c, "settings", "update_config", "panel_update", "", "warn", true)
	response.OK(c, gin.H{"message": "update settings saved"})
}

func (s *Server) handleUpdateApply(c *gin.Context) {
	var req struct {
		Version string `json:"version"`
	}
	_ = c.ShouldBindJSON(&req)
	record, err := s.panelupdate.Apply(c.Request.Context(), req.Version, "manual")
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	s.enterprise.Recorder().FromGin(c, "settings", "apply_update", "panel_update", record.ToVersion, "critical", true)
	response.OK(c, gin.H{
		"message": "update scheduled; panel will restart shortly",
		"record":  record,
	})
}
