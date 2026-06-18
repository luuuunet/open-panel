package api

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/devops"
)

func (s *Server) registerDevOpsRoutes(g *gin.RouterGroup) {
	g.GET("/devops/deploy/configs", s.handleListDeployConfigs)
	g.GET("/devops/deploy/config/:websiteId", s.handleGetDeployConfig)
	g.PUT("/devops/deploy/config/:websiteId", s.handleSaveDeployConfig)
	g.POST("/devops/deploy/trigger/:websiteId", s.handleTriggerDeploy)
	g.GET("/devops/deploy/jobs", s.handleListDeployJobs)
	g.GET("/devops/deploy/dockerfile/:websiteId", s.handleExportDockerfile)
	g.POST("/devops/deploy/dockerfile/:websiteId/save", s.handleSaveDockerfile)

	g.GET("/devops/diagnostics/slow-logs", s.handleSlowLogs)
	g.GET("/devops/diagnostics/traffic-anomalies", s.handleTrafficAnomalies)

	g.GET("/devops/audit/config", s.handleConfigAudit)

	g.GET("/devops/security/cve", s.handleCVEScan)
}

func (s *Server) handleListDeployConfigs(c *gin.Context) {
	list, err := s.devops.ListDeployConfigs()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleGetDeployConfig(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("websiteId"), 10, 64)
	cfg, err := s.devops.GetDeployConfig(uint(id))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, cfg)
}

func (s *Server) handleSaveDeployConfig(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("websiteId"), 10, 64)
	var body models.SiteDeployConfig
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	body.WebsiteID = uint(id)
	cfg, err := s.devops.SaveDeployConfig(&body)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, cfg)
}

func (s *Server) handleTriggerDeploy(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("websiteId"), 10, 64)
	job, err := s.devops.TriggerDeploy(uint(id), "manual")
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, job)
}

func (s *Server) handleListDeployJobs(c *gin.Context) {
	wid, _ := strconv.ParseUint(c.Query("website_id"), 10, 64)
	jobs, err := s.devops.ListDeployJobs(uint(wid), 30)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, jobs)
}

func (s *Server) handleExportDockerfile(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("websiteId"), 10, 64)
	content, err := s.devops.ExportDockerfile(uint(id))
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"content": content})
}

func (s *Server) handleSaveDockerfile(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("websiteId"), 10, 64)
	var body struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	content := body.Content
	if content == "" {
		var err error
		content, err = s.devops.ExportDockerfile(uint(id))
		if err != nil {
			response.Error(c, 400, err.Error())
			return
		}
	}
	path, err := s.devops.SaveDockerfileExport(uint(id), content)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"path": path})
}

func (s *Server) handleSlowLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	sum, err := s.devops.SlowLogSummary(limit)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, sum)
}

func (s *Server) handleTrafficAnomalies(c *gin.Context) {
	list, err := s.devops.TrafficAnomalies()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleConfigAudit(c *gin.Context) {
	report, err := s.devops.ConfigAudit()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, report)
}

func (s *Server) handleCVEScan(c *gin.Context) {
	result, err := s.devops.ScanCVE()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleComposeRolling(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	res, err := s.compose.RollingUpdate(uint(id))
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, res)
}

func (s *Server) handleComposeBlueGreen(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	res, err := s.compose.BlueGreenUpdate(uint(id))
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, res)
}

func (s *Server) handleDeployWebhook(c *gin.Context) {
	token := c.Param("token")
	body, _ := io.ReadAll(c.Request.Body)
	sig := c.GetHeader("X-Hub-Signature-256")
	job, err := s.devops.HandleWebhook(token, "webhook", body, sig)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "job_id": job.ID})
}

func (s *Server) handleDeployCI(c *gin.Context) {
	token := c.Param("token")
	body, _ := io.ReadAll(c.Request.Body)
	req := devops.ParseCIBody(body)
	gitlabToken := c.GetHeader("X-Gitlab-Token")
	job, err := s.devops.HandleCIWebhook(token, req, body, gitlabToken)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "job_id": job.ID})
}
