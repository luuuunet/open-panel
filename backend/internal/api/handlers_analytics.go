package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/services/website"
)

func (s *Server) registerAnalyticsRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/analytics/traffic-map", s.handleTrafficMap)
	authorized.GET("/analytics/traffic-map/countries/:code/domains", s.handleTrafficMapCountryDomains)
	authorized.GET("/analytics/traffic-map/countries/:code/domains/:host/details", s.handleTrafficMapDomainDetails)
	authorized.GET("/analytics/traffic-map/websites", s.handleTrafficMapWebsites)
	authorized.GET("/analytics/geo-policies", s.handleListGeoPolicies)
	authorized.POST("/analytics/geo-policies", s.handleCreateGeoPolicy)
	authorized.PUT("/analytics/geo-policies/:id", s.handleUpdateGeoPolicy)
	authorized.DELETE("/analytics/geo-policies/:id", s.handleDeleteGeoPolicy)
	authorized.POST("/analytics/geo-policies/apply/:websiteId", s.handleApplyGeoPolicies)
	authorized.POST("/analytics/geoip/install", s.handleInstallGeoIP)
}

func parseAnalyticsHours(c *gin.Context) int {
	hours := 24
	if q := c.Query("hours"); q != "" {
		if n, err := strconv.Atoi(q); err == nil {
			hours = n
		}
	}
	return hours
}

func (s *Server) handleTrafficMap(c *gin.Context) {
	if c.Query("live") == "1" {
		s.performance.TouchDashboardLive()
	}
	data, err := s.analytics.GetTrafficMap(parseAnalyticsHours(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, data)
}

func (s *Server) handleTrafficMapCountryDomains(c *gin.Context) {
	code := c.Param("code")
	data, err := s.analytics.GetCountryDomains(code, parseAnalyticsHours(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, data)
}

func (s *Server) handleTrafficMapDomainDetails(c *gin.Context) {
	code := c.Param("code")
	host := c.Param("host")
	data, err := s.analytics.GetCountryDomainDetails(code, host, parseAnalyticsHours(c))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, data)
}

func (s *Server) handleTrafficMapWebsites(c *gin.Context) {
	data, err := s.analytics.ListTrafficWebsites()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, data)
}

func (s *Server) handleListGeoPolicies(c *gin.Context) {
	var websiteID uint
	if q := c.Query("website_id"); q != "" {
		if n, err := strconv.ParseUint(q, 10, 64); err == nil {
			websiteID = uint(n)
		}
	}
	list, err := s.website.ListGeoPolicies(websiteID)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, list)
}

func (s *Server) handleCreateGeoPolicy(c *gin.Context) {
	var req website.GeoPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	p, err := s.website.CreateGeoPolicy(&req)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, p)
}

func (s *Server) handleUpdateGeoPolicy(c *gin.Context) {
	var req website.GeoPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	p, err := s.website.UpdateGeoPolicy(parseID(c), &req)
	if err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	response.OK(c, p)
}

func (s *Server) handleDeleteGeoPolicy(c *gin.Context) {
	if err := s.website.DeleteGeoPolicy(parseID(c)); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "deleted")
}

func (s *Server) handleApplyGeoPolicies(c *gin.Context) {
	id := parseIDParam(c.Param("websiteId"))
	if err := s.website.ApplyGeoPolicies(id); err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.Message(c, "applied")
}

func (s *Server) handleInstallGeoIP(c *gin.Context) {
	result, err := s.analytics.InstallGeoIP()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, result)
}
