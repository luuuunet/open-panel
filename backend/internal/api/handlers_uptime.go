package api

import (
	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/models"
)

func (s *Server) registerUptimeRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/uptime", s.handleListUptime)
	authorized.POST("/uptime", s.handleCreateUptime)
	authorized.PATCH("/uptime/:id", s.handleUpdateUptime)
	authorized.DELETE("/uptime/:id", s.handleDeleteUptime)
	authorized.POST("/uptime/:id/check", s.handleCheckUptime)
}

func (s *Server) handleListUptime(c *gin.Context) {
	list, err := s.uptime.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateUptime(c *gin.Context) {
	var m models.UptimeMonitor
	if err := c.ShouldBindJSON(&m); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.uptime.Create(&m); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, m)
}

func (s *Server) handleUpdateUptime(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.uptime.Update(parseID(c), req); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	var m models.UptimeMonitor
	if err := s.db.First(&m, parseID(c)).Error; err != nil {
		response.Message(c, "updated")
		return
	}
	response.OK(c, m)
}

func (s *Server) handleDeleteUptime(c *gin.Context) {
	if err := s.uptime.Delete(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleCheckUptime(c *gin.Context) {
	m, err := s.uptime.CheckNow(parseID(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, m)
}
