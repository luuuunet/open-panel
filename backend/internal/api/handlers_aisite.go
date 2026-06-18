package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/services/aisite"
)

func (s *Server) registerAISiteRoutes(g *gin.RouterGroup) {
	g.GET("/ai/assistant/status", s.handleAIAssistantStatus)
	g.POST("/websites/ai/analyze", s.handleAISiteAnalyze)
	g.POST("/websites/ai/auto", s.handleAISiteAutoDeploy)
	g.POST("/websites/ai/deploy", s.handleAISiteDeploy)
	g.GET("/websites/ai/jobs", s.handleListAISiteJobs)
	g.GET("/websites/ai/jobs/:id", s.handleGetAISiteJob)
	g.POST("/websites/:id/ai/diagnose-repair", s.handleWebsiteAIDiagnoseRepair)
	g.POST("/websites/:id/logs/ai/chat", s.handleWebsiteLogAIChat)
	g.POST("/websites/:id/logs/ai/chat/stream", s.handleWebsiteLogAIChatStream)
	g.POST("/websites/:id/logs/ai/repair", s.handleWebsiteLogAIRepair)
	g.POST("/websites/:id/project/ai/chat", s.handleWebsiteProjectAIChat)
	g.POST("/websites/:id/project/ai/apply", s.handleWebsiteProjectAIApply)
}

func (s *Server) handleAIAssistantStatus(c *gin.Context) {
	response.OK(c, s.aisite.AIAssistantStatus())
}

func (s *Server) handleWebsiteAIDiagnoseRepair(c *gin.Context) {
	id := parseID(c)
	result, err := s.aisite.DiagnoseRepair(id)
	if err != nil {
		if result != nil {
			response.OK(c, result)
			return
		}
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleWebsiteLogAIChat(c *gin.Context) {
	id := parseID(c)
	var req aisite.SiteLogChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.aisite.SiteLogChat(id, req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleWebsiteLogAIChatStream(c *gin.Context) {
	id := parseID(c)
	var req aisite.SiteLogChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	s.aisite.SiteLogChatStream(id, req, c)
}

func (s *Server) handleWebsiteLogAIRepair(c *gin.Context) {
	id := parseID(c)
	var req aisite.SiteLogRepairRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.aisite.SiteLogRepair(id, req)
	if err != nil {
		if result != nil {
			response.OK(c, result)
			return
		}
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleAISiteAnalyze(c *gin.Context) {
	var req aisite.AnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.aisite.Analyze(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	if result.Repo != nil {
		result.Repo.ClonePath = ""
	}
	response.OK(c, result)
}

func (s *Server) handleAISiteDeploy(c *gin.Context) {
	var req aisite.DeployRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	job, err := s.aisite.Deploy(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, job)
}

func (s *Server) handleAISiteAutoDeploy(c *gin.Context) {
	var req aisite.AutoDeployRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	job, err := s.aisite.AutoDeploy(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, job)
}

func (s *Server) handleListAISiteJobs(c *gin.Context) {
	list, err := s.aisite.ListJobs(30)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleGetAISiteJob(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	job, err := s.aisite.GetJob(uint(id))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, job)
}

func (s *Server) handleWebsiteProjectAIChat(c *gin.Context) {
	id := parseID(c)
	var req aisite.SiteProjectChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.aisite.SiteProjectChat(id, req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleWebsiteProjectAIApply(c *gin.Context) {
	id := parseID(c)
	var req aisite.SiteProjectApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.aisite.SiteProjectApply(id, req)
	if err != nil {
		if result != nil {
			response.OK(c, result)
			return
		}
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, result)
}
