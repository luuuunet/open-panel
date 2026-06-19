package api

import (
	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/services/cloudhub"
)

func (s *Server) registerCloudRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/cloud/hub", s.handleCloudHub)
	authorized.POST("/cloud/presets/:vendor", s.handleCloudPreset)
}

func (s *Server) handleCloudHub(c *gin.Context) {
	hub, err := s.cloudhub.GetHub()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, hub)
}

func (s *Server) handleCloudPreset(c *gin.Context) {
	var req cloudhub.CloudPresetRequest
	_ = c.ShouldBindJSON(&req)
	res, err := s.cloudhub.ApplyCloudPreset(c.Param("vendor"), &req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, res)
}
