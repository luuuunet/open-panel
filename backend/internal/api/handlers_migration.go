package api

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/services/migration"
)

func (s *Server) handleMigrationPreview(c *gin.Context) {
	preview, err := s.migration.Preview()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, preview)
}

func (s *Server) handleMigrationExport(c *gin.Context) {
	var req struct {
		IncludeLogs    bool  `json:"include_logs"`
		IncludeSecrets *bool `json:"include_secrets"`
	}
	_ = c.ShouldBindJSON(&req)
	opts := migration.ExportOptions{
		IncludeLogs:    req.IncludeLogs,
		IncludeSecrets: true,
	}
	if req.IncludeSecrets != nil {
		opts.IncludeSecrets = *req.IncludeSecrets
	}
	result, err := s.migration.Export(opts)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.enterprise.Recorder().FromGin(c, "migration", "export", "panel_migration", result.Filename, "warn", true)
	response.OK(c, result)
}

func (s *Server) handleMigrationDownload(c *gin.Context) {
	name := c.Query("file")
	if name == "" {
		name = c.Param("file")
	}
	path, err := s.migration.ResolveExportPath(name)
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	c.FileAttachment(path, filepath.Base(path))
}

func (s *Server) handleMigrationImport(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.Error(c, 400, "请上传迁移包 (.tar.gz)")
		return
	}
	defer file.Close()

	tmpDir := filepath.Join(os.TempDir(), "owpanel-migration-upload")
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

	mode := c.PostForm("mode")
	if mode == "" {
		mode = "replace"
	}
	result, err := s.migration.ImportBundle(tmpPath, migration.ImportOptions{Mode: mode})
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	s.enterprise.Recorder().FromGin(c, "migration", "import", "panel_migration", mode, "critical", true)
	response.OK(c, result)
}
