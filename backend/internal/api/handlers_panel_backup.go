package api

import (
	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/services/backup"
)

func (s *Server) handlePanelBackupHistory(c *gin.Context) {
	list, err := s.backup.ListPanelBackupHistory(50)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handlePanelBackupConfigGet(c *gin.Context) {
	cfg, err := s.backup.GetPanelBackupConfig()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, cfg)
}

func (s *Server) handlePanelBackupConfigPut(c *gin.Context) {
	var req backup.PanelBackupConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	cfg, err := s.backup.UpdatePanelBackupConfig(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, cfg)
}

func (s *Server) handlePanelBackupRun(c *gin.Context) {
	var req backup.PanelBackupOptions
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if req.KeepCount <= 0 {
		if cfg, err := s.backup.GetPanelBackupConfig(); err == nil && cfg.KeepCount > 0 {
			req.KeepCount = cfg.KeepCount
		} else {
			req.KeepCount = 5
		}
	}
	if req.OSSStorageID == nil {
		if cfg, err := s.backup.GetPanelBackupConfig(); err == nil {
			req.OSSStorageID = cfg.OSSStorageID
			if req.RemoteID == nil {
				req.RemoteID = cfg.RemoteID
			}
		}
	}
	rec, err := s.backup.RunPanelBackup(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, rec)
}

func (s *Server) handlePanelBackupRestore(c *gin.Context) {
	var req struct {
		RecordID uint   `json:"record_id"`
		Mode     string `json:"mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if req.RecordID == 0 {
		response.Error(c, 400, "record_id required")
		return
	}
	res, err := s.backup.RestorePanelFromRecord(req.RecordID, req.Mode)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, res)
}
