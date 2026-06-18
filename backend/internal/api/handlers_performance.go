package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
)

func (s *Server) handleDashboardPerformanceGet(c *gin.Context) {
	response.OK(c, s.performance.GetProfile())
}

func (s *Server) handleDashboardPerformancePut(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.performance.SetEnabled(req.Enabled); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.enterprise.Recorder().FromGin(c, "config", "performance_mode", "dashboard_performance",
		"enabled="+strconv.FormatBool(req.Enabled), "info", true)
	response.OK(c, s.performance.GetProfile())
}
