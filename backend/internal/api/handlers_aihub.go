package api

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/services/appstore"
)

func (s *Server) registerAIHubRoutes(g *gin.RouterGroup) {
	g.GET("/ai/hub/status", s.handleAIHubStatus)
	g.GET("/ai/gpu", s.handleAIGPUInfo)
	g.GET("/ai/agents", s.handleAIAgents)
	g.GET("/ai/huggingface/status", s.handleHuggingFaceStatus)
	g.GET("/ai/huggingface/catalog", s.handleHuggingFaceCatalog)
	g.GET("/ai/huggingface/tasks", s.handleHuggingFaceTasks)
	g.GET("/ai/huggingface/models", s.handleHuggingFaceModels)
	g.GET("/ai/huggingface/search", s.handleHuggingFaceSearch)
	g.GET("/ai/huggingface/model", s.handleHuggingFaceModelDetail)
	g.GET("/ai/huggingface/token", s.handleHFTokenStatus)
	g.PUT("/ai/huggingface/token", s.handleSaveHFToken)
	g.POST("/ai/huggingface/token/test", s.handleTestHFToken)
	g.GET("/ai/huggingface/install/logs", s.handleHuggingFaceInstallLogs)
	g.POST("/ai/huggingface/setup", s.handleHuggingFaceSetup)
	g.POST("/ai/huggingface/uninstall", s.handleHuggingFaceUninstall)
}

func (s *Server) handleAIHubStatus(c *gin.Context) {
	response.OK(c, gin.H{
		"huggingface": s.aihub.HuggingFaceStatus(),
		"gpu":         s.aihub.GPUInfo(),
	})
}

func (s *Server) handleAIGPUInfo(c *gin.Context) {
	response.OK(c, s.aihub.GPUInfo())
}

func (s *Server) handleAIAgents(c *gin.Context) {
	list, err := s.aihub.ListAIAgents()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleHuggingFaceStatus(c *gin.Context) {
	response.OK(c, s.aihub.HuggingFaceStatus())
}

func (s *Server) handleHuggingFaceCatalog(c *gin.Context) {
	modality := strings.TrimSpace(c.Query("modality"))
	if modality != "" && modality != "all" {
		response.OK(c, s.aihub.CatalogByModality(modality))
		return
	}
	response.OK(c, s.aihub.ModelCatalog())
}

func (s *Server) handleHuggingFaceTasks(c *gin.Context) {
	response.OK(c, s.aihub.HubTasks())
}

func (s *Server) handleHuggingFaceModels(c *gin.Context) {
	response.OK(c, s.aihub.DefaultModels())
}

func (s *Server) handleHuggingFaceSearch(c *gin.Context) {
	q := c.Query("q")
	task := c.DefaultQuery("task", "text-generation")
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	token := c.Query("hf_token")
	list, err := s.aihub.SearchHubModels(q, task, limit, token)
	if err != nil {
		response.Error(c, 502, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleHuggingFaceModelDetail(c *gin.Context) {
	id := strings.TrimSpace(c.Query("id"))
	if id == "" {
		response.Error(c, 400, "id required")
		return
	}
	token := c.Query("hf_token")
	model, err := s.aihub.GetHubModel(id, token)
	if err != nil {
		response.Error(c, 502, err.Error())
		return
	}
	response.OK(c, model)
}

func (s *Server) handleHFTokenStatus(c *gin.Context) {
	response.OK(c, gin.H{"configured": s.aihub.HFTokenConfigured()})
}

func (s *Server) handleSaveHFToken(c *gin.Context) {
	var req struct {
		HFToken string `json:"hf_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.aihub.SaveHFToken(req.HFToken); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Message(c, "saved")
}

func (s *Server) handleTestHFToken(c *gin.Context) {
	var req struct {
		HFToken string `json:"hf_token"`
	}
	_ = c.ShouldBindJSON(&req)
	info, err := s.aihub.TestHFToken(req.HFToken)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, info)
}

func (s *Server) handleHuggingFaceInstallLogs(c *gin.Context) {
	response.OK(c, s.aihub.GetInstallLogs())
}

func (s *Server) handleHuggingFaceSetup(c *gin.Context) {
	var opts appstore.HuggingFaceOptions
	if err := c.ShouldBindJSON(&opts); err != nil {
		opts = appstore.HuggingFaceOptions{}
	}
	if err := s.aihub.SetupHuggingFace(opts); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "Hugging Face AI deployment started")
}

func (s *Server) handleHuggingFaceUninstall(c *gin.Context) {
	if err := s.aihub.UninstallHuggingFace(); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "uninstalled")
}
