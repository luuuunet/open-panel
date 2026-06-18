package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/backup"
	"github.com/luuuunet/owpanel/internal/services/database"
)

func (s *Server) registerDatabaseRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/databases", s.handleListDatabases)
	authorized.POST("/databases/sync", s.handleSyncDatabases)
	authorized.POST("/databases", s.handleCreateDatabase)
	authorized.POST("/databases/provision", s.handleProvisionDatabase)
	authorized.GET("/databases/mysql/status", s.handleMySQLStatus)
	authorized.GET("/databases/mongodb/status", s.handleMongoDBStatus)
	authorized.GET("/databases/pgsql/status", s.handlePostgreSQLStatus)
	authorized.GET("/databases/engines/status", s.handleDatabaseEnginesStatus)
	authorized.GET("/databases/pgsql/extensions", s.handleListPgExtensions)
	authorized.POST("/databases/pgsql/extensions/:name/install", s.handleInstallPgExtensionPackage)
	authorized.PUT("/databases/:id/pgsql/extensions/:name", s.handleSetPgDatabaseExtension)
	authorized.POST("/databases/mysql/root-password", s.handleChangeMySQLRootPassword)
	authorized.GET("/databases/:id", s.handleGetDatabase)
	authorized.PATCH("/databases/:id", s.handleUpdateDatabase)
	authorized.DELETE("/databases/:id", s.handleDeleteDatabase)

	authorized.GET("/databases/:id/backups", s.handleListDatabaseBackups)
	authorized.POST("/databases/:id/backups", s.handleRunDatabaseBackup)
	authorized.DELETE("/databases/:id/backups/:backupId", s.handleDeleteDatabaseBackup)
	authorized.GET("/databases/:id/backups/:backupId/download", s.handleDownloadDatabaseBackup)
	authorized.POST("/databases/:id/import", s.handleImportDatabase)
	authorized.GET("/databases/:id/backup/config", s.handleDatabaseBackupConfig)
	authorized.PATCH("/databases/:id/backup/config", s.handleUpdateDatabaseBackupConfig)
	authorized.GET("/databases/:id/credentials", s.handleGetDatabaseCredentials)
}

func (s *Server) handleListDatabases(c *gin.Context) {
	list, err := s.database.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleSyncDatabases(c *gin.Context) {
	result := s.database.SyncFromServer()
	list, err := s.database.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	msg := fmt.Sprintf("已同步 %d 个数据库", result.Added)
	if result.Added == 0 {
		msg = "数据库列表已是最新"
	}
	response.OK(c, gin.H{
		"added":     result.Added,
		"databases": list,
		"message":   msg,
	})
}

func (s *Server) handleGetDatabase(c *gin.Context) {
	inst, err := s.database.Get(parseID(c))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, gin.H{
		"id":            inst.ID,
		"name":          inst.Name,
		"type":          inst.Type,
		"host":          inst.Host,
		"port":          inst.Port,
		"username":      inst.Username,
		"status":        inst.Status,
		"backup_status": inst.BackupStatus,
		"has_password":  inst.Password != "",
		"remark":        inst.Remark,
		"allow_remote":  inst.AllowRemote,
		"access_mode":   database.AccessModeFromInstance(inst.AccessMode, inst.AllowRemote),
	})
}

func (s *Server) handleCreateDatabase(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Type        string `json:"type"`
		Host        string `json:"host"`
		Port        int    `json:"port"`
		Username    string `json:"username"`
		Password    string `json:"password"`
		Remark      string `json:"remark"`
		Charset     string `json:"charset"`
		AllowRemote bool   `json:"allow_remote"`
		AccessMode  string `json:"access_mode"`
		ForceSSL    bool   `json:"force_ssl"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if req.Type == "" {
		req.Type = "mysql"
	}
	if s.requiresLocalDatabaseEngine(req.Type, req.Host) && !s.appstore.DatabaseEngineInstalled(engineTypeForDB(req.Type)) {
		response.Error(c, 409, fmt.Sprintf("请先安装 %s", s.appstore.DatabaseEngineStatus(engineTypeForDB(req.Type)).Name))
		return
	}
	mode := database.NormalizeAccessMode(req.AccessMode)
	if req.AccessMode == "" && req.AllowRemote {
		mode = database.AccessModeBoth
	}
	inst := &models.DatabaseInstance{
		Name:     req.Name,
		Type:     req.Type,
		Host:     req.Host,
		Port:     req.Port,
		Username: req.Username,
		Password: req.Password,
		Remark:   req.Remark,
		Charset:  database.CharsetFromInput(req.Charset),
		AccessMode:  mode,
		AllowRemote: database.AllowRemoteFromAccessMode(mode),
		ForceSSL: req.ForceSSL,
	}
	if err := s.database.Create(inst); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{
		"id": inst.ID, "name": inst.Name, "type": inst.Type,
		"host": inst.Host, "port": inst.Port, "username": inst.Username,
		"status": inst.Status, "has_password": req.Password != "", "remark": inst.Remark,
	})
}

func (s *Server) handleProvisionDatabase(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Username    string `json:"username" binding:"required"`
		Password    string `json:"password" binding:"required"`
		Host        string `json:"host"`
		Port        int    `json:"port"`
		Remark      string `json:"remark"`
		Charset     string `json:"charset"`
		AllowRemote bool   `json:"allow_remote"`
		AccessMode  string `json:"access_mode"`
		ForceSSL    bool   `json:"force_ssl"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if !s.appstore.DatabaseEngineInstalled("mysql") {
		response.Error(c, 409, "请先安装 MySQL 或 MariaDB")
		return
	}
	cs := database.CharsetFromInput(req.Charset)
	mode := database.NormalizeAccessMode(req.AccessMode)
	if req.AccessMode == "" && req.AllowRemote {
		mode = database.AccessModeBoth
	}
	if err := s.database.ProvisionMySQLWith(database.ProvisionMySQLOptions{
		Name: req.Name, Username: req.Username, Password: req.Password,
		Charset: cs, AccessMode: mode, AllowRemote: database.AllowRemoteFromAccessMode(mode), ForceSSL: req.ForceSSL,
	}); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	inst := &models.DatabaseInstance{
		Name: req.Name, Type: "mysql", Host: req.Host, Port: req.Port,
		Username: req.Username, Password: req.Password, Status: "running",
		Remark: req.Remark, AccessMode: mode, AllowRemote: database.AllowRemoteFromAccessMode(mode), ForceSSL: req.ForceSSL, Charset: cs,
	}
	if inst.Host == "" {
		inst.Host = "127.0.0.1"
	}
	if inst.Port == 0 {
		inst.Port = 3306
	}
	if err := s.database.Create(inst); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{
		"id": inst.ID, "name": inst.Name, "type": inst.Type,
		"host": inst.Host, "port": inst.Port, "username": inst.Username,
		"status": inst.Status, "has_password": true, "provisioned": true,
	})
}

func (s *Server) handleUpdateDatabase(c *gin.Context) {
	var req struct {
		Host        string  `json:"host"`
		Port        int     `json:"port"`
		Username    string  `json:"username"`
		Password    string  `json:"password"`
		Remark      *string `json:"remark"`
		AllowRemote *bool   `json:"allow_remote"`
		AccessMode  *string `json:"access_mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.database.UpdateInstance(parseID(c), database.UpdateRequest{
		Host: req.Host, Port: req.Port, Username: req.Username, Password: req.Password, Remark: req.Remark, AllowRemote: req.AllowRemote, AccessMode: req.AccessMode,
	}); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	inst, _ := s.database.Get(parseID(c))
	if inst == nil {
		response.Message(c, "updated")
		return
	}
	response.OK(c, gin.H{
		"id": inst.ID, "name": inst.Name, "type": inst.Type,
		"host": inst.Host, "port": inst.Port, "username": inst.Username,
		"has_password": inst.Password != "", "remark": inst.Remark, "allow_remote": inst.AllowRemote,
		"access_mode": database.AccessModeFromInstance(inst.AccessMode, inst.AllowRemote),
	})
}

func (s *Server) handleDeleteDatabase(c *gin.Context) {
	if err := s.database.Delete(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleListDatabaseBackups(c *gin.Context) {
	list, err := s.database.ListBackups(parseID(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleRunDatabaseBackup(c *gin.Context) {
	var req struct {
		OSSStorageID *uint `json:"oss_storage_id"`
		RemoteID     *uint `json:"remote_id"`
	}
	_ = c.ShouldBindJSON(&req)
	rec, err := s.database.RunBackup(parseID(c), database.BackupOptions{
		OSSStorageID: req.OSSStorageID,
		RemoteID:     req.RemoteID,
	})
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, rec)
}

func (s *Server) handleDeleteDatabaseBackup(c *gin.Context) {
	if err := s.database.DeleteBackup(parseID(c), parseParamID(c, "backupId")); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleDownloadDatabaseBackup(c *gin.Context) {
	path, err := s.database.GetBackupFile(parseID(c), parseParamID(c, "backupId"))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	c.FileAttachment(path, filepath.Base(path))
}

func (s *Server) handleImportDatabase(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.Error(c, 400, "请上传 SQL 文件")
		return
	}
	defer file.Close()

	tmpDir := filepath.Join(os.TempDir(), "owpanel-db-import")
	_ = os.MkdirAll(tmpDir, 0755)
	tmpPath := filepath.Join(tmpDir, fmt.Sprintf("%d-%s", time.Now().UnixNano(), header.Filename))
	out, err := os.Create(tmpPath)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	if _, err := out.ReadFrom(file); err != nil {
		out.Close()
		_ = os.Remove(tmpPath)
		response.Error(c, 500, err.Error())
		return
	}
	out.Close()
	defer os.Remove(tmpPath)

	if err := s.database.ImportSQL(parseID(c), tmpPath); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "imported")
}

func (s *Server) handleDatabaseBackupConfig(c *gin.Context) {
	cfg, summary, err := s.backup.GetDatabaseBackupConfig(parseID(c))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, gin.H{"config": cfg, "summary": summary})
}

func (s *Server) handleUpdateDatabaseBackupConfig(c *gin.Context) {
	var req backup.DatabaseBackupConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.backup.UpdateDatabaseBackupConfig(parseID(c), req); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "updated")
}

func (s *Server) handleGetDatabaseCredentials(c *gin.Context) {
	user, pass, host, port, err := s.database.GetCredentials(parseID(c))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, gin.H{
		"username": user,
		"password": pass,
		"host":     host,
		"port":     port,
	})
}

func (s *Server) handleMySQLStatus(c *gin.Context) {
	response.OK(c, s.database.MySQLStatus())
}

func (s *Server) handleMongoDBStatus(c *gin.Context) {
	response.OK(c, s.database.MongoDBStatus())
}

func (s *Server) handleChangeMySQLRootPassword(c *gin.Context) {
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.database.ChangeMySQLRootPassword(req.Password); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "updated")
}

func (s *Server) handlePostgreSQLStatus(c *gin.Context) {
	response.OK(c, s.database.PostgreSQLStatus())
}

func (s *Server) handleDatabaseEnginesStatus(c *gin.Context) {
	engines := s.appstore.AllDatabaseEngineStatuses()
	if st, ok := engines["mysql"]; ok {
		if live := s.database.MySQLStatus(); live.Installed {
			if live.ServerVersion != "" {
				st.Version = live.ServerVersion
			} else if live.Version != "" {
				st.Version = live.Version
			}
			st.Running = live.Running
		}
		engines["mysql"] = st
	}
	if st, ok := engines["postgresql"]; ok {
		if live := s.database.PostgreSQLStatus(); live.Installed {
			if live.Version != "" {
				st.Version = live.Version
			}
			st.Running = live.Running
		}
		engines["postgresql"] = st
	}
	if st, ok := engines["mongodb"]; ok {
		if live := s.database.MongoDBStatus(); live.Installed {
			if live.Version != "" {
				st.Version = live.Version
			}
			st.Running = live.Running
		}
		engines["mongodb"] = st
	}
	if st, ok := engines["redis"]; ok && st.Installed {
		st.Running = s.appstore.LiveStatus("redis") == "running"
		engines["redis"] = st
	}
	response.OK(c, engines)
}

func engineTypeForDB(dbType string) string {
	switch strings.ToLower(strings.TrimSpace(dbType)) {
	case "mariadb":
		return "mysql"
	case "postgres":
		return "postgresql"
	default:
		if dbType == "" {
			return "mysql"
		}
		return strings.ToLower(dbType)
	}
}

func (s *Server) requiresLocalDatabaseEngine(dbType, host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host != "" && host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return false
	}
	switch engineTypeForDB(dbType) {
	case "mysql", "postgresql", "mongodb", "redis":
		return true
	default:
		return false
	}
}

func (s *Server) handleListPgExtensions(c *gin.Context) {
	detail, err := s.database.ListPgExtensionCatalog(c.Query("database"))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, detail)
}

func (s *Server) handleInstallPgExtensionPackage(c *gin.Context) {
	if err := s.database.InstallPgExtensionPackage(c.Param("name")); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	detail, err := s.database.ListPgExtensionCatalog(c.Query("database"))
	if err != nil {
		response.Message(c, "extension package installed")
		return
	}
	response.OK(c, detail)
}

func (s *Server) handleSetPgDatabaseExtension(c *gin.Context) {
	inst, err := s.database.Get(parseID(c))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.database.SetPgDatabaseExtension(inst, c.Param("name"), req.Enabled); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	detail, err := s.database.ListPgExtensionCatalog(inst.Name)
	if err != nil {
		response.Message(c, "extension updated")
		return
	}
	response.OK(c, detail)
}
