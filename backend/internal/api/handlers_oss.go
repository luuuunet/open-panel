package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
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
