package api

import (
	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/platform"
	"github.com/luuuunet/owpanel/internal/services/stack"
	"github.com/luuuunet/owpanel/internal/services/system"
)

func (s *Server) registerSystemRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/system/platform", s.handleSystemPlatform)
	authorized.GET("/system/readiness", s.handleSystemReadiness)
	authorized.GET("/system/stacks", s.handleSystemStacks)
}

func (s *Server) registerAdminSystemRoutes(admin *gin.RouterGroup) {
	admin.POST("/system/stacks/:key/install", s.handleInstallStack)
}

func (s *Server) handleSystemPlatform(c *gin.Context) {
	response.OK(c, platform.Detect())
}

func (s *Server) handleSystemReadiness(c *gin.Context) {
	response.OK(c, system.BuildReadiness(s.appstore, s.cfg.DataDir))
}

func (s *Server) handleSystemStacks(c *gin.Context) {
	response.OK(c, stack.List())
}

func (s *Server) handleInstallStack(c *gin.Context) {
	key := c.Param("key")
	if err := s.appstore.InstallStackNamed(key); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"message": "stack installation started", "stack": key})
}
