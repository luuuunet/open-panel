package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/models"
	"github.com/luuuunet/owpanel/internal/services/dns"
)

func (s *Server) registerDNSRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/dns", s.handleListDNS)
	authorized.POST("/dns", s.handleCreateDNS)
	authorized.PUT("/dns/:id", s.handleUpdateDNS)
	authorized.DELETE("/dns/:id", s.handleDeleteDNS)

	authorized.GET("/dns/providers", s.handleListDNSProviders)
	authorized.GET("/dns/providers/supported", s.handleSupportedDNSProviders)
	authorized.POST("/dns/providers", s.handleCreateDNSProvider)
	authorized.PUT("/dns/providers/:id", s.handleUpdateDNSProvider)
	authorized.DELETE("/dns/providers/:id", s.handleDeleteDNSProvider)
	authorized.POST("/dns/providers/:id/test", s.handleTestDNSProvider)
	authorized.POST("/dns/providers/:id/sync-zones", s.handleSyncDNSZones)

	authorized.GET("/dns/zones", s.handleListDNSZones)
	authorized.POST("/dns/zones/pull", s.handlePullDNSZoneRecords)
	authorized.GET("/dns/detect", s.handleDetectDNS)
	authorized.POST("/dns/apply", s.handleApplyDNS)
	authorized.GET("/dns/server-ip", s.handleDNSServerIP)
}

func (s *Server) handleListDNS(c *gin.Context) {
	list, err := s.dns.List(c.Query("domain"))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateDNS(c *gin.Context) {
	var r models.DNSRecord
	if err := c.ShouldBindJSON(&r); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if r.TTL == 0 {
		r.TTL = 600
	}
	if err := s.dns.CreateAndSync(&r); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, r)
}

func (s *Server) handleUpdateDNS(c *gin.Context) {
	id := parseID(c)
	rec, err := s.dns.Get(id)
	if err != nil {
		response.Error(c, 404, "record not found")
		return
	}
	var patch struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Value   string `json:"value"`
		TTL     int    `json:"ttl"`
		Proxied *bool  `json:"proxied"`
	}
	if err := c.ShouldBindJSON(&patch); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if patch.Type != "" {
		rec.Type = patch.Type
	}
	if patch.Name != "" {
		rec.Name = patch.Name
	}
	if patch.Value != "" {
		rec.Value = patch.Value
	}
	if patch.TTL > 0 {
		rec.TTL = patch.TTL
	}
	if patch.Proxied != nil {
		rec.Proxied = *patch.Proxied
	}
	if err := s.dns.UpdateAndSync(rec); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, rec)
}

func (s *Server) handleDeleteDNS(c *gin.Context) {
	if err := s.dns.Delete(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleListDNSProviders(c *gin.Context) {
	list, err := s.dns.ListProviders()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleSupportedDNSProviders(c *gin.Context) {
	response.OK(c, dns.SupportedProviders)
}

func (s *Server) handleCreateDNSProvider(c *gin.Context) {
	var req dns.CreateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	acc, err := s.dns.CreateProvider(req)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, acc)
}

func (s *Server) handleUpdateDNSProvider(c *gin.Context) {
	var req dns.UpdateProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	if err := s.dns.UpdateProvider(parseID(c), req); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "updated")
}

func (s *Server) handleDeleteDNSProvider(c *gin.Context) {
	if err := s.dns.DeleteProvider(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleTestDNSProvider(c *gin.Context) {
	if err := s.dns.TestProvider(parseID(c)); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.Message(c, "connection ok")
}

func (s *Server) handleSyncDNSZones(c *gin.Context) {
	n, err := s.dns.SyncProviderZones(parseID(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"synced": n})
}

func (s *Server) handleListDNSZones(c *gin.Context) {
	pid, _ := strconv.ParseUint(c.Query("provider_id"), 10, 64)
	list, err := s.dns.ListZones(uint(pid))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handlePullDNSZoneRecords(c *gin.Context) {
	var req struct {
		ProviderID uint   `json:"provider_id"`
		Zone       string `json:"zone"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	n, err := s.dns.PullZoneRecords(req.ProviderID, req.Zone)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{"imported": n})
}

func (s *Server) handleDetectDNS(c *gin.Context) {
	list, err := s.dns.DetectDomains()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleApplyDNS(c *gin.Context) {
	var req dns.ApplyDNSRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	records, err := s.dns.ApplyRecords(req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, records)
}

func (s *Server) handleDNSServerIP(c *gin.Context) {
	response.OK(c, gin.H{"ip": s.dns.ServerIP()})
}
