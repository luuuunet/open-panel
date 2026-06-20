package api

import (
	"errors"
	"net/http"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/services/aichat"
	"github.com/luuuunet/owpanel/internal/services/logs"
)

func (s *Server) registerLogRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/logs", s.handleListLogs)
	authorized.GET("/logs/sources", s.handleLogSources)
	authorized.PUT("/logs/sources", s.handleUpdateLogSources)
	authorized.GET("/logs/tail/:id", s.handleLogTail)
	authorized.POST("/logs/ai/chat", s.handleLogAIChat)
	authorized.POST("/logs/ai/chat/stream", s.handleLogAIChatStream)
	authorized.POST("/logs/clear-all", s.handleLogClearAll)
	authorized.POST("/logs/cleanup", s.handleLogCleanup)
	authorized.GET("/logs/retention", s.handleLogRetentionGet)
	authorized.PUT("/logs/retention", s.handleLogRetentionPut)
}

func (s *Server) handleListLogs(c *gin.Context) {
	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "200"))
	list, err := s.logs.Combined(lines)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleLogSources(c *gin.Context) {
	sources, categories := s.logs.ListSources()
	board := s.buildInstalledAppBoard(sources)
	response.OK(c, gin.H{
		"sources":        sources,
		"categories":     categories,
		"installed_apps": board,
	})
}

func (s *Server) buildInstalledAppBoard(sources []logs.Source) []logs.InstalledAppBoard {
	apps, err := s.appstore.ListInstalled()
	if err != nil {
		return nil
	}
	logsByKey := map[string][]logs.Source{}
	for _, src := range sources {
		if src.AppKey == "" {
			continue
		}
		logsByKey[src.AppKey] = append(logsByKey[src.AppKey], src)
	}
	out := make([]logs.InstalledAppBoard, 0, len(apps))
	for _, app := range apps {
		live := s.appstore.LiveStatus(app.Key)
		status := app.Status
		if live != "" {
			status = live
		}
		out = append(out, logs.InstalledAppBoard{
			Key:        app.Key,
			Name:       app.Name,
			Icon:       app.Icon,
			IconURL:    app.IconURL,
			Status:     status,
			LiveStatus: live,
			Version:    app.Version,
			Port:       app.Port,
			Category:   app.Category,
			Logs:       logsByKey[app.Key],
		})
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := logsLiveRank(out[i].LiveStatus, out[i].Status), logsLiveRank(out[j].LiveStatus, out[j].Status)
		if ri != rj {
			return ri < rj
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func logsLiveRank(live, status string) int {
	if live != "" {
		return logs.LiveRank(live)
	}
	return logs.LiveRank(status)
}

func (s *Server) handleUpdateLogSources(c *gin.Context) {
	var req struct {
		Enabled map[string]bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if len(req.Enabled) == 0 {
		response.Error(c, 400, "enabled map required")
		return
	}
	if err := s.logs.SetEnabled(req.Enabled); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	sources, categories := s.logs.ListSources()
	board := s.buildInstalledAppBoard(sources)
	response.OK(c, gin.H{
		"sources":        sources,
		"categories":     categories,
		"installed_apps": board,
	})
}

func (s *Server) handleLogTail(c *gin.Context) {
	id := c.Param("id")
	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "300"))
	result, err := s.logs.Tail(id, lines)
	if err != nil {
		if errors.Is(err, logs.ErrLoggingDisabled) {
			response.Error(c, http.StatusForbidden, "logging is disabled")
			return
		}
		response.Error(c, http.StatusNotFound, "log source not found")
		return
	}
	response.OK(c, result)
}

func (s *Server) handleLogAIChat(c *gin.Context) {
	var req aichat.LogChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.aichat.LogChat(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleLogAIChatStream(c *gin.Context) {
	var req aichat.LogChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	s.aichat.StreamLogChat(req, c)
}

func (s *Server) handleLogClearAll(c *gin.Context) {
	result, err := s.logs.ClearAll()
	if err != nil {
		if errors.Is(err, logs.ErrLoggingDisabled) {
			response.Error(c, http.StatusForbidden, "logging is disabled")
			return
		}
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleLogCleanup(c *gin.Context) {
	var req struct {
		Days *int `json:"days"`
	}
	_ = c.ShouldBindJSON(&req)
	days := s.logs.GetRetentionSettings().RetentionDays
	if req.Days != nil {
		days = *req.Days
	}
	if days <= 0 {
		response.Error(c, 400, "days must be greater than 0")
		return
	}
	result, err := s.logs.CleanOlderThan(days)
	if err != nil {
		if errors.Is(err, logs.ErrLoggingDisabled) {
			response.Error(c, http.StatusForbidden, "logging is disabled")
			return
		}
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleLogRetentionGet(c *gin.Context) {
	response.OK(c, s.logs.GetRetentionSettings())
}

func (s *Server) handleLogRetentionPut(c *gin.Context) {
	var req struct {
		RetentionDays   int   `json:"retention_days"`
		AutoCleanup     bool  `json:"auto_cleanup"`
		LoggingEnabled  *bool `json:"logging_enabled"`
		MaxSizeMB       int   `json:"max_size_mb"`
		MaxRotatedFiles int   `json:"max_rotated_files"`
		CompressRotated *bool `json:"compress_rotated"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if req.RetentionDays < 0 {
		response.Error(c, 400, "retention_days must be >= 0")
		return
	}
	if err := s.logs.SetRetentionSettings(req.RetentionDays, req.AutoCleanup, req.LoggingEnabled, req.MaxSizeMB, req.MaxRotatedFiles, req.CompressRotated); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, s.logs.GetRetentionSettings())
}
