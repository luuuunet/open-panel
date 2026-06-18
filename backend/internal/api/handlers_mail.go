package api

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/services/mail"
)

func (s *Server) registerMailRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/mail/status", s.handleMailStatus)
	authorized.POST("/mail/install", s.handleMailInstall)
	authorized.POST("/mail/uninstall", s.handleMailUninstall)
	authorized.POST("/mail/restart", s.handleMailRestart)
	authorized.POST("/mail/test", s.handleMailTest)
	authorized.POST("/mail/ssl", s.handleMailApplySSL)
	authorized.GET("/mail/dns/:domain", s.handleMailDNSHints)
	authorized.GET("/mail/domains", s.handleListMailDomains)
	authorized.POST("/mail/domains", s.handleCreateMailDomain)
	authorized.DELETE("/mail/domains/:id", s.handleDeleteMailDomain)
	authorized.GET("/mail/mailboxes", s.handleListMailboxes)
	authorized.POST("/mail/mailboxes", s.handleCreateMailbox)
	authorized.PATCH("/mail/mailboxes/:id", s.handleUpdateMailbox)
	authorized.DELETE("/mail/mailboxes/:id", s.handleDeleteMailbox)
	authorized.POST("/mail/mailboxes/batch", s.handleBatchCreateMailboxes)
	authorized.GET("/mail/mailboxes/export", s.handleExportMailboxes)
	authorized.POST("/mail/mailboxes/import", s.handleImportMailboxes)
	authorized.POST("/mail/mailboxes/import-file", s.handleImportMailboxesFile)
	authorized.GET("/mail/backups", s.handleListMailBackups)
	authorized.POST("/mail/backups", s.handleCreateMailBackup)
	authorized.GET("/mail/backups/:id/download", s.handleDownloadMailBackup)
	authorized.POST("/mail/backups/:id/restore", s.handleRestoreMailBackup)
	authorized.DELETE("/mail/backups/:id", s.handleDeleteMailBackup)
	authorized.POST("/mail/backups/import", s.handleImportMailBackupFile)
	authorized.POST("/mail/sync", s.handleSyncMail)
	authorized.GET("/mail/webmail", s.handleMailWebmailStatus)
	authorized.POST("/mail/webmail/install", s.handleMailWebmailInstall)
	authorized.GET("/mail/webmail/install/logs", s.handleMailWebmailInstallLogs)
	authorized.POST("/mail/webmail/uninstall", s.handleMailWebmailUninstall)
	authorized.POST("/mail/webmail/repair", s.handleMailWebmailRepair)

	authorized.GET("/mail/bulk/providers/catalog", s.handleMailBulkProviderCatalog)
	authorized.GET("/mail/bulk/providers", s.handleMailBulkProviders)
	authorized.POST("/mail/bulk/providers", s.handleMailBulkProviderCreate)
	authorized.PUT("/mail/bulk/providers/:id", s.handleMailBulkProviderUpdate)
	authorized.DELETE("/mail/bulk/providers/:id", s.handleMailBulkProviderDelete)
	authorized.POST("/mail/bulk/providers/:id/test", s.handleMailBulkProviderTest)
	authorized.GET("/mail/bulk/campaigns", s.handleMailBulkCampaigns)
	authorized.POST("/mail/bulk/campaigns", s.handleMailBulkCampaignCreate)
	authorized.GET("/mail/bulk/campaigns/:id", s.handleMailBulkCampaignGet)
	authorized.POST("/mail/bulk/campaigns/:id/start", s.handleMailBulkCampaignStart)
	authorized.POST("/mail/bulk/campaigns/:id/cancel", s.handleMailBulkCampaignCancel)
	authorized.DELETE("/mail/bulk/campaigns/:id", s.handleMailBulkCampaignDelete)
}

func (s *Server) handleMailStatus(c *gin.Context) {
	st, err := s.mail.Status()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, st)
}

func (s *Server) handleMailInstall(c *gin.Context) {
	if err := s.mail.InstallStack(); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.appstore.SyncMailStackRecords(true)
	response.Message(c, "installed")
}

func (s *Server) handleMailUninstall(c *gin.Context) {
	if err := s.mail.UninstallStack(); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.appstore.SyncMailStackRecords(false)
	response.Message(c, "uninstalled")
}

func (s *Server) handleMailRestart(c *gin.Context) {
	if err := s.mail.RestartServices(); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "restarted")
}

func (s *Server) handleMailTest(c *gin.Context) {
	var req struct {
		From    string `json:"from" binding:"required"`
		To      string `json:"to" binding:"required"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.mail.SendTestMail(req.From, req.To, req.Subject, req.Body); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "sent")
}

func (s *Server) handleMailApplySSL(c *gin.Context) {
	var req struct {
		Domain string `json:"domain" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.mail.ApplySSL(req.Domain); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "applied")
}

func (s *Server) handleMailDNSHints(c *gin.Context) {
	domain := c.Param("domain")
	response.OK(c, s.mail.DNSHints(domain))
}

func (s *Server) handleListMailDomains(c *gin.Context) {
	list, err := s.mail.ListDomains()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateMailDomain(c *gin.Context) {
	var d models.MailDomain
	if err := c.ShouldBindJSON(&d); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.mail.CreateDomain(&d); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, d)
}

func (s *Server) handleDeleteMailDomain(c *gin.Context) {
	if err := s.mail.DeleteDomain(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleListMailboxes(c *gin.Context) {
	list, err := s.mail.ListMailboxes(c.Query("domain"))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateMailbox(c *gin.Context) {
	var req struct {
		Domain   string `json:"domain" binding:"required"`
		Address  string `json:"address" binding:"required"`
		Password string `json:"password" binding:"required"`
		Quota    int    `json:"quota"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	m := &models.MailBox{Domain: req.Domain, Address: req.Address, Quota: req.Quota}
	if err := s.mail.CreateMailbox(m, req.Password); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, m)
}

func (s *Server) handleUpdateMailbox(c *gin.Context) {
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.mail.UpdateMailboxPassword(parseID(c), req.Password); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "updated")
}

func (s *Server) handleSyncMail(c *gin.Context) {
	if err := s.mail.SyncAll(); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "synced")
}

func (s *Server) handleDeleteMailbox(c *gin.Context) {
	if err := s.mail.DeleteMailbox(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleMailWebmailStatus(c *gin.Context) {
	st, err := s.mail.WebmailStatus()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, st)
}

func (s *Server) handleMailWebmailInstall(c *gin.Context) {
	var req mail.WebmailInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.mail.StartInstallWebmail(&req); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"started": true, "key": "snappymail"})
}

func (s *Server) handleMailWebmailInstallLogs(c *gin.Context) {
	response.OK(c, s.mail.GetWebmailInstallLogs())
}

func (s *Server) handleMailWebmailUninstall(c *gin.Context) {
	if err := s.mail.UninstallWebmail(); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "uninstalled")
}

func (s *Server) handleMailWebmailRepair(c *gin.Context) {
	if err := s.mail.RepairWebmail(); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	st, _ := s.mail.WebmailStatus()
	response.OK(c, st)
}

func (s *Server) handleBatchCreateMailboxes(c *gin.Context) {
	var req mail.BatchMailboxRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	res, err := s.mail.BatchCreateMailboxes(&req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, res)
}

func (s *Server) handleExportMailboxes(c *gin.Context) {
	data, filename, err := s.mail.ExportMailboxes(c.Query("domain"), c.DefaultQuery("format", "csv"))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	ctype := "text/csv; charset=utf-8"
	if strings.HasSuffix(filename, ".json") {
		ctype = "application/json; charset=utf-8"
	}
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(200, ctype, data)
}

func (s *Server) handleImportMailboxes(c *gin.Context) {
	var req mail.ImportMailboxRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	res, err := s.mail.ImportMailboxes(&req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, res)
}

func (s *Server) handleImportMailboxesFile(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.Error(c, 400, "请上传文件")
		return
	}
	f, err := file.Open()
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	defer f.Close()
	format := c.DefaultPostForm("format", "csv")
	rows, err := s.mail.ParseImportData(format, f)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	res, err := s.mail.ImportMailboxes(&mail.ImportMailboxRequest{
		Accounts:       rows,
		SkipExisting:   c.DefaultPostForm("skip_existing", "false") == "true",
		UpdatePassword: c.DefaultPostForm("update_password", "true") == "true",
	})
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, res)
}

func (s *Server) handleListMailBackups(c *gin.Context) {
	list, err := s.mail.ListMailBackups()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateMailBackup(c *gin.Context) {
	var req mail.BackupRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		response.Error(c, 400, err.Error())
		return
	}
	rec, err := s.mail.RunMailBackup(&req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, rec)
}

func (s *Server) handleDownloadMailBackup(c *gin.Context) {
	path, err := s.mail.GetMailBackupFile(parseID(c))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	c.FileAttachment(path, filepath.Base(path))
}

func (s *Server) handleRestoreMailBackup(c *gin.Context) {
	if err := s.mail.RestoreMailBackup(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "restored")
}

func (s *Server) handleDeleteMailBackup(c *gin.Context) {
	if err := s.mail.DeleteMailBackup(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleImportMailBackupFile(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.Error(c, 400, "请上传备份文件")
		return
	}
	tmp := filepath.Join(os.TempDir(), file.Filename)
	if err := c.SaveUploadedFile(file, tmp); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	defer os.Remove(tmp)
	restoreMaildir := c.DefaultPostForm("restore_maildir", "true") == "true"
	if err := s.mail.ImportMailBackupFile(tmp, restoreMaildir); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "imported")
}

func (s *Server) handleMailBulkProviderCatalog(c *gin.Context) {
	response.OK(c, s.mail.BulkProviderCatalog())
}

func (s *Server) handleMailBulkProviders(c *gin.Context) {
	list, err := s.mail.ListBulkProviders()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleMailBulkProviderCreate(c *gin.Context) {
	var req mail.BulkProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	p, err := s.mail.CreateBulkProvider(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, p)
}

func (s *Server) handleMailBulkProviderUpdate(c *gin.Context) {
	var req mail.BulkProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	p, err := s.mail.UpdateBulkProvider(parseID(c), req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, p)
}

func (s *Server) handleMailBulkProviderDelete(c *gin.Context) {
	if err := s.mail.DeleteBulkProvider(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleMailBulkProviderTest(c *gin.Context) {
	var req struct {
		To string `json:"to" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.mail.TestBulkProvider(parseID(c), req.To); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "sent")
}

func (s *Server) handleMailBulkCampaigns(c *gin.Context) {
	list, err := s.mail.ListBulkCampaigns()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleMailBulkCampaignCreate(c *gin.Context) {
	var req mail.BulkCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	camp, err := s.mail.CreateBulkCampaign(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, camp)
}

func (s *Server) handleMailBulkCampaignGet(c *gin.Context) {
	camp, rec, err := s.mail.GetBulkCampaign(parseID(c))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, gin.H{"campaign": camp, "recipients": rec})
}

func (s *Server) handleMailBulkCampaignStart(c *gin.Context) {
	if err := s.mail.StartBulkCampaign(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "started")
}

func (s *Server) handleMailBulkCampaignCancel(c *gin.Context) {
	if err := s.mail.CancelBulkCampaign(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "cancelled")
}

func (s *Server) handleMailBulkCampaignDelete(c *gin.Context) {
	if err := s.mail.DeleteBulkCampaign(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}
