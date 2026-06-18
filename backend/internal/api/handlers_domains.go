package api

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/services/domaincheck"
)

func (s *Server) handleCheckDomains(c *gin.Context) {
	var req struct {
		Domains          []string `json:"domains"`
		DomainsText      string   `json:"domains_text"`
		ExcludeWebsiteID uint     `json:"exclude_website_id"`
		ExcludeWPSiteID  uint     `json:"exclude_wp_site_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}

	domains := append([]string{}, req.Domains...)
	if req.DomainsText != "" {
		for _, part := range strings.FieldsFunc(req.DomainsText, func(r rune) bool {
			return r == '\n' || r == ',' || r == ';' || r == ' '
		}) {
			if part = strings.TrimSpace(part); part != "" {
				domains = append(domains, part)
			}
		}
	}

	conflicts := domaincheck.CheckList(s.db, domains, domaincheck.Scope{
		IgnoreWebsiteID: req.ExcludeWebsiteID,
		IgnoreWPSiteID:  req.ExcludeWPSiteID,
	})
	response.OK(c, gin.H{
		"available": len(conflicts) == 0,
		"conflicts": conflicts,
	})
}
