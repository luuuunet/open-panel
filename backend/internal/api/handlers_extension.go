package api

import (
	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/middleware"
)

func (s *Server) registerExtensionRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/extensions/menu", s.handleExtensionMenu)

	admin := authorized.Group("/extensions")
	admin.Use(middleware.RequireAdmin())
	admin.GET("", s.handleListExtensions)
	admin.POST("/reload", s.handleReloadExtensions)
	admin.PATCH("/:id/enabled", s.handleSetExtensionEnabled)
	authorized.GET("/extensions/embed/:id", s.handleExtensionEmbed)
	authorized.GET("/extensions/detail/:id", s.handleExtensionDetail)
}

func (s *Server) handleListExtensions(c *gin.Context) {
	if s.extensions == nil {
		response.OK(c, gin.H{"items": []any{}, "dir": ""})
		return
	}
	response.OK(c, gin.H{
		"items": s.extensions.List(),
		"dir":   s.extensions.ExtensionsDir(),
	})
}

func (s *Server) handleExtensionMenu(c *gin.Context) {
	if s.extensions == nil {
		response.OK(c, []any{})
		return
	}
	response.OK(c, s.extensions.MenuItems())
}

func (s *Server) handleReloadExtensions(c *gin.Context) {
	if s.extensions == nil {
		response.Error(c, 500, "extension registry unavailable")
		return
	}
	n := s.extensions.Reload()
	if s.appstore != nil {
		s.appstore.SyncCatalog()
	}
	response.OK(c, gin.H{"count": n, "message": "extensions reloaded"})
}

func (s *Server) handleSetExtensionEnabled(c *gin.Context) {
	if s.extensions == nil {
		response.Error(c, 500, "extension registry unavailable")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.extensions.SetEnabled(c.Param("id"), req.Enabled); err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	if s.appstore != nil {
		s.appstore.SyncCatalog()
	}
	response.OK(c, gin.H{"enabled": req.Enabled})
}

func (s *Server) handleExtensionEmbed(c *gin.Context) {
	id := c.Param("id")
	if s.extensions == nil {
		response.Error(c, 404, "not found")
		return
	}
	embedURL, title := s.extensions.ResolveEmbedURL(id)
	if embedURL != "" {
		response.OK(c, gin.H{"embed_url": embedURL, "title": title})
		return
	}
	if info, ok := s.extensions.Get(id); ok {
		response.OK(c, gin.H{"title": title, "detail": info})
		return
	}
	response.Error(c, 404, "extension not found")
}

func (s *Server) handleExtensionDetail(c *gin.Context) {
	id := c.Param("id")
	if s.extensions == nil {
		response.Error(c, 404, "not found")
		return
	}
	if info, ok := s.extensions.Get(id); ok {
		response.OK(c, info)
		return
	}
	response.Error(c, 404, "extension not found")
}

func (s *Server) emitExtension(event string, payload map[string]interface{}) {
	if s.extensions == nil {
		return
	}
	s.extensions.Emit(event, payload)
}
