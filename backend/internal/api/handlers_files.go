package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/auth"
	"github.com/luuuunet/owpanel/internal/services/aichat"
)

func (s *Server) registerFileRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/files/trash", s.handleListTrash)
	authorized.POST("/files/trash/empty", s.handleEmptyTrash)
	authorized.POST("/files/trash/:id/restore", s.handleRestoreTrash)
	authorized.DELETE("/files/trash/:id", s.handleDeleteTrashPermanent)
	authorized.GET("/files", s.handleListFiles)
	authorized.GET("/files/roots", s.handleFileRoots)
	authorized.GET("/files/info", s.handleFileInfo)
	authorized.GET("/files/content", s.handleReadFile)
	authorized.PUT("/files/content", s.handleWriteFile)
	authorized.DELETE("/files", s.handleDeleteFile)
	authorized.POST("/files/delete-batch", s.handleDeleteFilesBatch)
	authorized.POST("/files/mkdir", s.handleMkdir)
	authorized.POST("/files/create", s.handleCreateFile)
	authorized.POST("/files/rename", s.handleRenameFile)
	authorized.PATCH("/files/permissions", s.handleFilePermissions)
	authorized.POST("/files/upload", s.handleUploadFile)
	authorized.POST("/files/download-url", s.handleDownloadFromURL)
	authorized.POST("/files/compress", s.handleCompressFiles)
	authorized.POST("/files/extract", s.handleExtractFile)
	authorized.GET("/files/download", s.handleDownloadFile)
	authorized.POST("/files/download-batch", s.handleDownloadBatch)
	authorized.GET("/files/size", s.handleFileTreeSize)
	authorized.GET("/files/search", s.handleSearchFiles)
	authorized.POST("/files/copy", s.handleCopyFiles)
	authorized.POST("/files/move", s.handleMoveFiles)
	authorized.POST("/files/duplicate", s.handleDuplicateFile)
	authorized.POST("/files/ai/chat", s.handleFileAIChat)
}

func (s *Server) handleListFiles(c *gin.Context) {
	dir := c.Query("path")
	entries, err := s.filemgr.List(dir)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, entries)
}

func (s *Server) handleFileRoots(c *gin.Context) {
	response.OK(c, gin.H{
		"default_root": s.filemgr.DefaultRoot(),
		"roots":        s.filemgr.Roots(),
	})
}

func (s *Server) handleFileInfo(c *gin.Context) {
	path := c.Query("path")
	info, err := s.filemgr.Stat(path)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, info)
}

func (s *Server) handleReadFile(c *gin.Context) {
	path := c.Query("path")
	info, err := s.filemgr.Stat(path)
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	if info.IsDir {
		response.Error(c, 400, "path is a directory")
		return
	}
	content, err := s.filemgr.Read(path)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"path": path, "content": string(content)})
}

func (s *Server) handleWriteFile(c *gin.Context) {
	var req struct {
		Path    string `json:"path" binding:"required"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	content := []byte(req.Content)
	if err := s.authSvc.CheckDiskQuota(c.GetUint("user_id"), int64(len(content))); err != nil {
		response.Error(c, 403, err.Error())
		return
	}
	if err := s.filemgr.Write(req.Path, content); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	_ = s.authSvc.AddDiskUsage(c.GetUint("user_id"), auth.QuotaMBFromBytes(int64(len(content))))
	response.Message(c, "saved")
}

func (s *Server) handleDeleteFile(c *gin.Context) {
	path := c.Query("path")
	username, _ := c.Get("username")
	user, _ := username.(string)
	if _, err := s.filemgr.MoveToTrash(path, c.GetUint("user_id"), user); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "moved to trash")
}

func (s *Server) handleDeleteFilesBatch(c *gin.Context) {
	var req struct {
		Paths []string `json:"paths" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if len(req.Paths) == 0 {
		response.Error(c, 400, "no paths selected")
		return
	}
	username, _ := c.Get("username")
	user, _ := username.(string)
	result, err := s.filemgr.MoveManyToTrash(req.Paths, c.GetUint("user_id"), user)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	if result.Moved == 0 && result.Failed > 0 {
		response.Error(c, 500, strings.Join(result.Errors, "; "))
		return
	}
	response.OK(c, result)
}

func (s *Server) handleListTrash(c *gin.Context) {
	items, err := s.filemgr.ListTrash()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, items)
}

func (s *Server) handleRestoreTrash(c *gin.Context) {
	id := c.Param("id")
	path, err := s.filemgr.RestoreTrash(id)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"path": path})
}

func (s *Server) handleDeleteTrashPermanent(c *gin.Context) {
	id := c.Param("id")
	size, err := s.filemgr.DeleteTrashPermanent(id)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	_ = s.authSvc.SubDiskUsage(c.GetUint("user_id"), auth.QuotaMBFromBytes(size))
	response.Message(c, "permanently deleted")
}

func (s *Server) handleEmptyTrash(c *gin.Context) {
	size, err := s.filemgr.EmptyTrash()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	_ = s.authSvc.SubDiskUsage(c.GetUint("user_id"), auth.QuotaMBFromBytes(size))
	response.OK(c, gin.H{"freed_bytes": size})
}

func (s *Server) handleMkdir(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.filemgr.Mkdir(req.Path); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "created")
}

func (s *Server) handleCreateFile(c *gin.Context) {
	var req struct {
		Path    string `json:"path" binding:"required"`
		IsDir   bool   `json:"is_dir"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if !req.IsDir {
		if err := s.authSvc.CheckDiskQuota(c.GetUint("user_id"), int64(len(req.Content))); err != nil {
			response.Error(c, 403, err.Error())
			return
		}
	}
	if err := s.filemgr.CreateFile(req.Path, []byte(req.Content), req.IsDir); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	if !req.IsDir {
		_ = s.authSvc.AddDiskUsage(c.GetUint("user_id"), auth.QuotaMBFromBytes(int64(len(req.Content))))
	}
	response.Message(c, "created")
}

func (s *Server) handleRenameFile(c *gin.Context) {
	var req struct {
		Path    string `json:"path" binding:"required"`
		NewName string `json:"new_name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.filemgr.Rename(req.Path, req.NewName); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "renamed")
}

func (s *Server) handleFilePermissions(c *gin.Context) {
	var req struct {
		Path      string `json:"path" binding:"required"`
		Mode      string `json:"mode" binding:"required"`
		Recursive bool   `json:"recursive"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if req.Recursive {
		stats, err := s.filemgr.ChmodRecursive(req.Path, req.Mode)
		if err != nil {
			response.Error(c, 500, err.Error())
			return
		}
		response.OK(c, stats)
		return
	}
	if err := s.filemgr.Chmod(req.Path, req.Mode); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "updated")
}

func (s *Server) handleUploadFile(c *gin.Context) {
	dir := c.PostForm("path")
	if dir == "" {
		dir = c.Query("path")
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	defer file.Close()
	if err := s.authSvc.CheckDiskQuota(c.GetUint("user_id"), header.Size); err != nil {
		response.Error(c, 403, err.Error())
		return
	}
	target, err := s.filemgr.Upload(dir, file, header.Filename)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	_ = s.authSvc.AddDiskUsage(c.GetUint("user_id"), auth.QuotaMBFromBytes(header.Size))
	response.OK(c, gin.H{"path": target, "name": header.Filename})
}

func (s *Server) handleDownloadFromURL(c *gin.Context) {
	var req struct {
		URL      string `json:"url" binding:"required"`
		Path     string `json:"path" binding:"required"`
		Filename string `json:"filename"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.filemgr.DownloadFromURL(req.Path, req.URL, req.Filename)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	if err := s.authSvc.CheckDiskQuota(c.GetUint("user_id"), result.Size); err != nil {
		_ = os.Remove(result.Path)
		response.Error(c, 403, err.Error())
		return
	}
	_ = s.authSvc.AddDiskUsage(c.GetUint("user_id"), auth.QuotaMBFromBytes(result.Size))
	response.OK(c, result)
}

func (s *Server) handleCompressFiles(c *gin.Context) {
	var req struct {
		Paths  []string `json:"paths" binding:"required"`
		Format string   `json:"format"`
		Dest   string   `json:"dest" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.filemgr.Compress(req.Paths, req.Format, req.Dest)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleExtractFile(c *gin.Context) {
	var req struct {
		Path    string `json:"path" binding:"required"`
		DestDir string `json:"dest_dir" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.filemgr.Extract(req.Path, req.DestDir); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "extracted")
}

func (s *Server) handleDownloadFile(c *gin.Context) {
	path := c.Query("path")
	info, err := s.filemgr.Stat(path)
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	if info.IsDir {
		response.Error(c, 400, "cannot download directory")
		return
	}
	c.FileAttachment(info.Path, info.Name)
}

func (s *Server) handleDownloadBatch(c *gin.Context) {
	var req struct {
		Paths []string `json:"paths" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if len(req.Paths) == 0 {
		response.Error(c, 400, "no paths selected")
		return
	}
	dest := filepath.Join(os.TempDir(), fmt.Sprintf("owpanel-dl-%d.zip", time.Now().UnixNano()))
	result, err := s.filemgr.Compress(req.Paths, "zip", dest)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	defer os.Remove(result.Path)
	c.FileAttachment(result.Path, "download.zip")
}

func (s *Server) handleFileTreeSize(c *gin.Context) {
	path := c.Query("path")
	size, err := s.filemgr.TreeSize(path)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"path": path, "size": size})
}

func (s *Server) handleSearchFiles(c *gin.Context) {
	dir := c.Query("path")
	query := c.Query("q")
	entries, err := s.filemgr.SearchNames(dir, query, 200)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, entries)
}

func (s *Server) handleCopyFiles(c *gin.Context) {
	var req struct {
		Paths []string `json:"paths" binding:"required"`
		Dest  string   `json:"dest" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.filemgr.CopyItems(req.Paths, req.Dest); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "copied")
}

func (s *Server) handleMoveFiles(c *gin.Context) {
	var req struct {
		Paths []string `json:"paths" binding:"required"`
		Dest  string   `json:"dest" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.filemgr.MoveItems(req.Paths, req.Dest); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "moved")
}

func (s *Server) handleDuplicateFile(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	newPath, err := s.filemgr.Duplicate(req.Path)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"path": newPath})
}

func (s *Server) handleFileAIChat(c *gin.Context) {
	var req aichat.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if req.Message == "" {
		response.Error(c, 400, "message is required")
		return
	}
	result, err := s.aichat.Chat(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}
