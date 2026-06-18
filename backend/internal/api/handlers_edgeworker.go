package api

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/edgeworker"
)

func (s *Server) registerEdgeWorkerRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/edge-workers", s.handleListEdgeWorkers)
	authorized.GET("/edge-workers/available-domains", s.handleListEdgeWorkerAvailableDomains)
	authorized.POST("/edge-workers", s.handleCreateEdgeWorker)
	authorized.PATCH("/edge-workers/:id", s.handleUpdateEdgeWorker)
	authorized.DELETE("/edge-workers/:id", s.handleDeleteEdgeWorker)
	authorized.PATCH("/edge-workers/:id/toggle", s.handleToggleEdgeWorker)
	authorized.GET("/edge-workers/preview", s.handlePreviewEdgeWorkers)
	authorized.POST("/edge-workers/apply", s.handleApplyEdgeWorkers)
	authorized.GET("/edge-workers/templates", s.handleEdgeWorkerTemplates)
	authorized.GET("/edge-workers/runtime", s.handleEdgeWorkerRuntime)

	authorized.GET("/edge-workers/kv/namespaces", s.handleListEdgeKVNamespaces)
	authorized.POST("/edge-workers/kv/namespaces", s.handleCreateEdgeKVNamespace)
	authorized.PATCH("/edge-workers/kv/namespaces/:id", s.handleUpdateEdgeKVNamespace)
	authorized.DELETE("/edge-workers/kv/namespaces/:id", s.handleDeleteEdgeKVNamespace)
	authorized.GET("/edge-workers/kv/namespaces/:id/keys", s.handleListEdgeKVKeys)
	authorized.GET("/edge-workers/kv/namespaces/:id/keys/*key", s.handleGetEdgeKVKey)
	authorized.PUT("/edge-workers/kv/namespaces/:id/keys/*key", s.handlePutEdgeKVKey)
	authorized.DELETE("/edge-workers/kv/namespaces/:id/keys/*key", s.handleDeleteEdgeKVKey)
	authorized.GET("/edge-workers/kv/namespaces/:id/export", s.handleExportEdgeKVNamespace)

	authorized.GET("/edge-workers/d1/databases", s.handleListEdgeD1Databases)
	authorized.POST("/edge-workers/d1/databases", s.handleCreateEdgeD1Database)
	authorized.PATCH("/edge-workers/d1/databases/:id", s.handleUpdateEdgeD1Database)
	authorized.DELETE("/edge-workers/d1/databases/:id", s.handleDeleteEdgeD1Database)
	authorized.POST("/edge-workers/d1/databases/:id/query", s.handleEdgeD1AdminQuery)
}

func (s *Server) registerEdgeInternalRoutes(r gin.IRouter) {
	internal := r.Group("/edge-internal")
	internal.Use(s.edgeInternalAuth())
	{
		internal.GET("/kv/:nsID/*key", s.handleInternalEdgeKVGet)
		internal.PUT("/kv/:nsID/*key", s.handleInternalEdgeKVPut)
		internal.DELETE("/kv/:nsID/*key", s.handleInternalEdgeKVDelete)
		internal.POST("/d1/:id/query", s.handleInternalEdgeD1Query)
	}
}

func (s *Server) edgeInternalAuth() gin.HandlerFunc {
	secret := s.edgeworker.InternalSecret()
	return func(c *gin.Context) {
		if c.GetHeader("X-Edge-Worker-Secret") == secret {
			c.Next()
			return
		}
		ip := c.ClientIP()
		if ip == "127.0.0.1" || ip == "::1" {
			c.Next()
			return
		}
		if host, _, err := net.SplitHostPort(strings.TrimSpace(c.Request.RemoteAddr)); err == nil {
			if host == "127.0.0.1" || host == "::1" {
				c.Next()
				return
			}
		}
		response.Error(c, 403, "edge internal API forbidden")
		c.Abort()
	}
}

func (s *Server) handleListEdgeWorkers(c *gin.Context) {
	list, err := s.edgeworker.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, s.edgeworker.EnrichWorkers(list))
}

func (s *Server) handleListEdgeWorkerAvailableDomains(c *gin.Context) {
	list, err := s.edgeworker.ListAvailableDomains()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateEdgeWorker(c *gin.Context) {
	var req struct {
		models.EdgeWorker
		Bindings []edgeworker.BindingInput `json:"bindings"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.edgeworker.Create(&req.EdgeWorker); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if len(req.Bindings) > 0 {
		if err := s.edgeworker.SetBindings(req.EdgeWorker.ID, req.Bindings); err != nil {
			response.Error(c, 400, err.Error())
			return
		}
	}
	w, _ := s.edgeworker.Get(req.EdgeWorker.ID)
	response.OK(c, s.edgeworker.EnrichWorker(w))
}

func (s *Server) handleUpdateEdgeWorker(c *gin.Context) {
	var req struct {
		models.EdgeWorker
		Bindings []edgeworker.BindingInput `json:"bindings"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	id := parseID(c)
	if err := s.edgeworker.Update(id, &req.EdgeWorker); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if req.Bindings != nil {
		if err := s.edgeworker.SetBindings(id, req.Bindings); err != nil {
			response.Error(c, 400, err.Error())
			return
		}
	}
	w, _ := s.edgeworker.Get(id)
	response.OK(c, s.edgeworker.EnrichWorker(w))
}

func (s *Server) handleDeleteEdgeWorker(c *gin.Context) {
	if err := s.edgeworker.Delete(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

func (s *Server) handleToggleEdgeWorker(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.edgeworker.Toggle(parseID(c), req.Enabled); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"enabled": req.Enabled})
}

func (s *Server) handlePreviewEdgeWorkers(c *gin.Context) {
	preview, err := s.edgeworker.Preview()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"preview": preview})
}

func (s *Server) handleApplyEdgeWorkers(c *gin.Context) {
	result, err := s.edgeworker.Apply()
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, result)
}

func (s *Server) handleEdgeWorkerTemplates(c *gin.Context) {
	response.OK(c, s.edgeworker.Templates())
}

func (s *Server) handleEdgeWorkerRuntime(c *gin.Context) {
	response.OK(c, s.edgeworker.DetectRuntime())
}

// --- KV admin ---

func (s *Server) handleListEdgeKVNamespaces(c *gin.Context) {
	list, err := s.edgekv.ListNamespaces()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateEdgeKVNamespace(c *gin.Context) {
	var ns models.EdgeKVNamespace
	if err := c.ShouldBindJSON(&ns); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.edgekv.CreateNamespace(&ns); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, ns)
}

func (s *Server) handleUpdateEdgeKVNamespace(c *gin.Context) {
	var patch models.EdgeKVNamespace
	if err := c.ShouldBindJSON(&patch); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.edgekv.UpdateNamespace(parseID(c), &patch); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	ns, _ := s.edgekv.GetNamespace(parseID(c))
	response.OK(c, ns)
}

func (s *Server) handleDeleteEdgeKVNamespace(c *gin.Context) {
	if err := s.edgekv.DeleteNamespace(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

func (s *Server) handleListEdgeKVKeys(c *gin.Context) {
	prefix := c.Query("prefix")
	list, err := s.edgekv.ListKeys(parseID(c), prefix)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func kvKeyParam(c *gin.Context) string {
	return strings.TrimPrefix(c.Param("key"), "/")
}

func paramUint(c *gin.Context, name string) uint {
	v, _ := strconv.ParseUint(c.Param(name), 10, 64)
	return uint(v)
}

func (s *Server) handleGetEdgeKVKey(c *gin.Context) {
	item, err := s.edgekv.GetKey(parseID(c), kvKeyParam(c))
	if err != nil {
		response.Error(c, 404, err.Error())
		return
	}
	response.OK(c, item)
}

func (s *Server) handlePutEdgeKVKey(c *gin.Context) {
	var req struct {
		Value string `json:"value"`
		TTL   int    `json:"ttl"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	var expiresAt *time.Time
	if req.TTL > 0 {
		t := time.Now().Add(time.Duration(req.TTL) * time.Second)
		expiresAt = &t
	}
	if err := s.edgekv.PutKey(parseID(c), kvKeyParam(c), req.Value, expiresAt); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"ok": true})
}

func (s *Server) handleDeleteEdgeKVKey(c *gin.Context) {
	if err := s.edgekv.DeleteKey(parseID(c), kvKeyParam(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

func (s *Server) handleExportEdgeKVNamespace(c *gin.Context) {
	list, err := s.edgekv.ExportNamespace(parseID(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

// --- D1 admin ---

func (s *Server) handleListEdgeD1Databases(c *gin.Context) {
	list, err := s.edged1.List()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateEdgeD1Database(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	db, err := s.edged1.Create(req.Name, req.Description)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, db)
}

func (s *Server) handleUpdateEdgeD1Database(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.edged1.Update(parseID(c), req.Name, req.Description); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	db, _ := s.edged1.Get(parseID(c))
	response.OK(c, db)
}

func (s *Server) handleDeleteEdgeD1Database(c *gin.Context) {
	if err := s.edged1.Delete(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

func (s *Server) handleEdgeD1AdminQuery(c *gin.Context) {
	var req struct {
		SQL string `json:"sql"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.edged1.ExecAdmin(parseID(c), req.SQL)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, result)
}

// --- Internal API (localhost / worker secret) ---

func (s *Server) handleInternalEdgeKVGet(c *gin.Context) {
	nsID := paramUint(c, "nsID")
	key := kvKeyParam(c)
	item, err := s.edgekv.GetKey(nsID, key)
	if err != nil {
		response.Error(c, 404, "not found")
		return
	}
	response.OK(c, item)
}

func (s *Server) handleInternalEdgeKVPut(c *gin.Context) {
	var req struct {
		Value string `json:"value"`
		TTL   int    `json:"ttl"`
	}
	_ = c.ShouldBindJSON(&req)
	var expiresAt *time.Time
	if req.TTL > 0 {
		t := time.Now().Add(time.Duration(req.TTL) * time.Second)
		expiresAt = &t
	}
	if err := s.edgekv.PutKey(paramUint(c, "nsID"), kvKeyParam(c), req.Value, expiresAt); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, gin.H{"ok": true})
}

func (s *Server) handleInternalEdgeKVDelete(c *gin.Context) {
	if err := s.edgekv.DeleteKey(paramUint(c, "nsID"), kvKeyParam(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

func (s *Server) handleInternalEdgeD1Query(c *gin.Context) {
	var req struct {
		SQL string `json:"sql"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	result, err := s.edged1.Query(paramUint(c, "id"), req.SQL, true)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, result)
}
