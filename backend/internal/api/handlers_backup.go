package api

import (
	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/extension"
	"github.com/luuuunet/owpanel/internal/services/backup"
)

func (s *Server) handleListBackupRemotes(c *gin.Context) {
	list, err := s.backup.ListRemotes()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleGetBackupRemote(c *gin.Context) {
	r, err := s.backup.GetRemote(parseID(c))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, r)
}

func (s *Server) handleCreateBackupRemote(c *gin.Context) {
	var req backup.RemoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	r, err := s.backup.CreateRemote(&req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, r)
}

func (s *Server) handleUpdateBackupRemote(c *gin.Context) {
	var req backup.RemoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	r, err := s.backup.UpdateRemote(parseID(c), &req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, r)
}

func (s *Server) handleDeleteBackupRemote(c *gin.Context) {
	if err := s.backup.DeleteRemote(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleTestBackupRemote(c *gin.Context) {
	if err := s.backup.TestRemote(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "ok")
}

func (s *Server) handleListWebsiteBackups(c *gin.Context) {
	list, err := s.backup.ListWebsiteBackups(parseID(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleRunWebsiteBackup(c *gin.Context) {
	var req struct {
		RemoteID *uint `json:"remote_id"`
	}
	_ = c.ShouldBindJSON(&req)
	siteID := parseID(c)
	rec, err := s.backup.RunWebsiteBackup(siteID, req.RemoteID)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.emitExtension(extension.EventBackupCompleted, map[string]interface{}{
		"website_id": siteID,
		"backup_id":  rec.ID,
		"domain":     rec.Domain,
		"file_path":  rec.FilePath,
	})
	response.OK(c, rec)
}

func (s *Server) handleDeleteWebsiteBackup(c *gin.Context) {
	siteID := parseID(c)
	backupID := parseParamID(c, "backupId")
	if err := s.backup.DeleteWebsiteBackup(siteID, backupID); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleGetWebsiteBackupConfig(c *gin.Context) {
	cfg, err := s.backup.GetWebsiteBackupConfig(parseID(c))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	summary := s.backup.WebsiteBackupSummary(parseID(c))
	response.OK(c, gin.H{"config": cfg, "summary": summary})
}

func (s *Server) handleUpdateWebsiteBackupConfig(c *gin.Context) {
	var req backup.WebsiteBackupConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.backup.UpdateWebsiteBackupConfig(parseID(c), req); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "saved")
}
