package api

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/models"
)

func (s *Server) registerRuntimeRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/runtimes", s.handleListRuntimes)
	authorized.GET("/runtimes/versions", s.handleRuntimeVersions)
	authorized.POST("/runtimes", s.handleCreateRuntime)
	authorized.PUT("/runtimes/:id", s.handleUpdateRuntime)
	authorized.DELETE("/runtimes/:id", s.handleDeleteRuntime)
	authorized.PATCH("/runtimes/:id/toggle", s.handleToggleRuntime)
}

func (s *Server) handleListRuntimes(c *gin.Context) {
	kind := strings.TrimSpace(c.Query("kind"))
	list, err := s.runtime.List(kind)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleRuntimeVersions(c *gin.Context) {
	kind := strings.TrimSpace(c.Query("kind"))
	response.OK(c, s.runtime.Versions(kind))
}

func (s *Server) handleCreateRuntime(c *gin.Context) {
	var p models.RuntimeProject
	if err := c.ShouldBindJSON(&p); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.runtime.Create(&p); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, p)
}

func (s *Server) handleUpdateRuntime(c *gin.Context) {
	var p models.RuntimeProject
	if err := c.ShouldBindJSON(&p); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.runtime.Update(parseID(c), &p); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "updated")
}

func (s *Server) handleDeleteRuntime(c *gin.Context) {
	legacySource := c.Query("legacy_source")
	var legacyID uint
	if v := c.Query("legacy_id"); v != "" {
		legacyID = parseIDParam(v)
	}
	if err := s.runtime.Delete(parseID(c), legacySource, legacyID); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleToggleRuntime(c *gin.Context) {
	var req struct {
		Status       string `json:"status"`
		LegacySource string `json:"legacy_source"`
		LegacyID     uint   `json:"legacy_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.runtime.Toggle(parseID(c), req.Status, req.LegacySource, req.LegacyID); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "updated")
}

func parseIDParam(v string) uint {
	var id uint
	for _, ch := range v {
		if ch >= '0' && ch <= '9' {
			id = id*10 + uint(ch-'0')
		}
	}
	return id
}
