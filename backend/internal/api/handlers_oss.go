package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/ossstorage"
)

func (s *Server) registerOSSRoutes(authorized *gin.RouterGroup) {
	g := authorized.Group("/oss")
	g.GET("/providers", s.handleOSSProviders)
	g.GET("/storages", s.handleListOSSStorages)
	g.POST("/storages", s.handleCreateOSSStorage)
	g.GET("/storages/:id", s.handleGetOSSStorage)
	g.PUT("/storages/:id", s.handleUpdateOSSStorage)
	g.DELETE("/storages/:id", s.handleDeleteOSSStorage)
	g.POST("/storages/:id/test", s.handleTestOSSStorage)
	g.GET("/storages/:id/browse", s.handleBrowseOSSStorage)

	g.GET("/sync-tasks", s.handleListOSSSyncTasks)
	g.POST("/sync-tasks", s.handleCreateOSSSyncTask)
	g.PUT("/sync-tasks/:id", s.handleUpdateOSSSyncTask)
	g.DELETE("/sync-tasks/:id", s.handleDeleteOSSSyncTask)
	g.POST("/sync-tasks/:id/run", s.handleRunOSSSyncTask)
	g.GET("/sync-tasks/:id/logs", s.handleGetOSSSyncTaskLogs)
	g.GET("/export", s.handleExportOSSConfig)
	g.POST("/import", s.handleImportOSSConfig)

	g.GET("/lifecycle-rules", s.handleListOSSLifecycleRules)
	g.POST("/lifecycle-rules", s.handleCreateOSSLifecycleRule)
	g.PUT("/lifecycle-rules/:id", s.handleUpdateOSSLifecycleRule)
	g.DELETE("/lifecycle-rules/:id", s.handleDeleteOSSLifecycleRule)
	g.POST("/lifecycle-rules/:id/run", s.handleRunOSSLifecycleRule)
	g.POST("/lifecycle-rules/:id/dry-run", s.handleDryRunOSSLifecycleRule)

	g.GET("/archive-rules", s.handleListOSSArchiveRules)
	g.POST("/archive-rules", s.handleCreateOSSArchiveRule)
	g.PUT("/archive-rules/:id", s.handleUpdateOSSArchiveRule)
	g.DELETE("/archive-rules/:id", s.handleDeleteOSSArchiveRule)
	g.POST("/archive-rules/:id/run", s.handleRunOSSArchiveRule)
}

func (s *Server) handleOSSProviders(c *gin.Context) {
	response.OK(c, s.ossstorage.ListProviders())
}

func (s *Server) handleListOSSStorages(c *gin.Context) {
	list, err := s.ossstorage.ListStorages()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleGetOSSStorage(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	st, err := s.ossstorage.GetStorage(uint(id))
	if err != nil {
		response.Error(c, 404, "not found")
		return
	}
	response.OK(c, st)
}

func (s *Server) handleCreateOSSStorage(c *gin.Context) {
	var req ossstorage.StorageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	st, err := s.ossstorage.CreateStorage(&req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, st)
}

func (s *Server) handleUpdateOSSStorage(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req ossstorage.StorageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	st, err := s.ossstorage.UpdateStorage(uint(id), &req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, st)
}

func (s *Server) handleDeleteOSSStorage(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := s.ossstorage.DeleteStorage(uint(id)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleTestOSSStorage(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := s.ossstorage.TestStorage(uint(id)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "ok")
}

func (s *Server) handleBrowseOSSStorage(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	prefix := c.Query("prefix")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	items, err := s.ossstorage.BrowseStorage(uint(id), prefix, limit)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, items)
}

func (s *Server) handleListOSSSyncTasks(c *gin.Context) {
	list, err := s.ossstorage.ListSyncTasks()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateOSSSyncTask(c *gin.Context) {
	var req ossstorage.SyncTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	task, err := s.ossstorage.CreateSyncTask(&req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, task)
}

func (s *Server) handleUpdateOSSSyncTask(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req ossstorage.SyncTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	task, err := s.ossstorage.UpdateSyncTask(uint(id), &req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, task)
}

func (s *Server) handleDeleteOSSSyncTask(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := s.ossstorage.DeleteSyncTask(uint(id)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleRunOSSSyncTask(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := s.ossstorage.RunSyncTask(uint(id)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "started")
}

func (s *Server) handleGetOSSSyncTaskLogs(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	task, err := s.ossstorage.GetSyncTaskLogs(uint(id))
	if err != nil {
		response.Error(c, 404, "not found")
		return
	}
	response.OK(c, task)
}

func (s *Server) handleExportOSSConfig(c *gin.Context) {
	includeSecrets := c.Query("include_secrets") == "true" || c.Query("include_secrets") == "1"
	cfg, err := s.ossstorage.ExportConfig(includeSecrets)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, cfg)
}

func (s *Server) handleImportOSSConfig(c *gin.Context) {
	var req ossstorage.ImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.ossstorage.ImportConfig(&req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleListOSSLifecycleRules(c *gin.Context) {
	list, err := s.ossstorage.ListLifecycleRules()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateOSSLifecycleRule(c *gin.Context) {
	var req ossstorage.LifecycleRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	rule, err := s.ossstorage.CreateLifecycleRule(&req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, rule)
}

func (s *Server) handleUpdateOSSLifecycleRule(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req ossstorage.LifecycleRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	rule, err := s.ossstorage.UpdateLifecycleRule(uint(id), &req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, rule)
}

func (s *Server) handleDeleteOSSLifecycleRule(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := s.ossstorage.DeleteLifecycleRule(uint(id)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleRunOSSLifecycleRule(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	list, err := s.ossstorage.ListLifecycleRules()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	var rule *models.OSSLifecycleRule
	for i := range list {
		if list[i].ID == uint(id) {
			rule = &list[i]
			break
		}
	}
	if rule == nil {
		response.Error(c, 404, "not found")
		return
	}
	res, err := s.ossstorage.RunLifecycleRule(rule, false)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, res)
}

func (s *Server) handleDryRunOSSLifecycleRule(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	list, err := s.ossstorage.ListLifecycleRules()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	for _, r := range list {
		if r.ID == uint(id) {
			res, err := s.ossstorage.RunLifecycleRule(&r, true)
			if err != nil {
				response.Error(c, 500, err.Error())
				return
			}
			response.OK(c, res)
			return
		}
	}
	response.Error(c, 404, "not found")
}

func (s *Server) handleListOSSArchiveRules(c *gin.Context) {
	list, err := s.ossstorage.ListArchiveRules()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateOSSArchiveRule(c *gin.Context) {
	var req ossstorage.ArchiveRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	rule, err := s.ossstorage.CreateArchiveRule(&req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, rule)
}

func (s *Server) handleUpdateOSSArchiveRule(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req ossstorage.ArchiveRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	rule, err := s.ossstorage.UpdateArchiveRule(uint(id), &req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, rule)
}

func (s *Server) handleDeleteOSSArchiveRule(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := s.ossstorage.DeleteArchiveRule(uint(id)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleRunOSSArchiveRule(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	list, err := s.ossstorage.ListArchiveRules()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	for _, r := range list {
		if r.ID == uint(id) {
			res, err := s.ossstorage.RunArchiveRule(&r)
			if err != nil {
				response.Error(c, 500, err.Error())
				return
			}
			response.OK(c, res)
			return
		}
	}
	response.Error(c, 404, "not found")
}
