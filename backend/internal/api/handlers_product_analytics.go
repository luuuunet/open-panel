package api

import (
	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/services/productanalytics"
)

func (s *Server) registerProductAnalyticsRoutes(authorized *gin.RouterGroup) {
	authorized.GET("/product-analytics/status", s.handleProductAnalyticsStatus)
	authorized.GET("/product-analytics/tracking-snippet", s.handleProductAnalyticsTrackingSnippet)
	authorized.PUT("/websites/:id/product-analytics", s.handleUpdateWebsiteProductAnalytics)
}

func (s *Server) handleProductAnalyticsStatus(c *gin.Context) {
	response.OK(c, s.productAnalytics.Status())
}

func (s *Server) handleProductAnalyticsTrackingSnippet(c *gin.Context) {
	clientID := c.Query("client_id")
	apiURL := c.Query("api_url")
	response.OK(c, s.productAnalytics.TrackingSnippet(clientID, apiURL))
}

func (s *Server) handleUpdateWebsiteProductAnalytics(c *gin.Context) {
	var req productanalytics.WebsiteConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	site, err := s.productAnalytics.UpdateWebsiteConfig(parseID(c), req)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, site)
}
