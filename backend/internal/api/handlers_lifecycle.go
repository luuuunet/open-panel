package api

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/services/lifecycle"
)

func (s *Server) registerLifecycleRoutes(authorized *gin.RouterGroup) {
	g := authorized.Group("/lifecycle")
	g.GET("/local-rules", s.handleListLocalCleanupRules)
	g.GET("/local-rules/presets", s.handleLocalCleanupPresets)
	g.POST("/local-rules", s.handleCreateLocalCleanupRule)
	g.PUT("/local-rules/:id", s.handleUpdateLocalCleanupRule)
	g.DELETE("/local-rules/:id", s.handleDeleteLocalCleanupRule)
	g.POST("/local-rules/:id/run", s.handleRunLocalCleanupRule)
}

func (s *Server) handleListLocalCleanupRules(c *gin.Context) {
	list, err := s.lifecycle.ListRules()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleLocalCleanupPresets(c *gin.Context) {
	response.OK(c, s.lifecycle.ListPresets())
}

func (s *Server) handleCreateLocalCleanupRule(c *gin.Context) {
	var req lifecycle.RuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	rule, err := s.lifecycle.CreateRule(&req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, rule)
}

func (s *Server) handleUpdateLocalCleanupRule(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req lifecycle.RuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	rule, err := s.lifecycle.UpdateRule(uint(id), &req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, rule)
}

func (s *Server) handleDeleteLocalCleanupRule(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := s.lifecycle.DeleteRule(uint(id)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleRunLocalCleanupRule(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	res, err := s.lifecycle.RunRuleByID(uint(id))
	if err != nil {
		if errors.Is(err, lifecycle.ErrRuleNotFound) {
			response.Error(c, 404, err.Error())
			return
		}
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, res)
}
