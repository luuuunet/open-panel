package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type PanelIPAccessConfig struct {
	WhitelistEnabled bool
	Whitelist        string
	Blacklist        string
}

// PanelIPAccess blocks panel access by IP blacklist or optional whitelist mode.
func PanelIPAccess(load func() PanelIPAccessConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if load == nil {
			c.Next()
			return
		}
		cfg := load()
		ip := c.ClientIP()
		if ip == "" {
			c.Next()
			return
		}
		if matchIPList(ip, cfg.Blacklist) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "access denied from this IP"})
			return
		}
		if cfg.WhitelistEnabled {
			list := strings.TrimSpace(cfg.Whitelist)
			if list == "" {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "whitelist enabled but empty - configure IPs"})
				return
			}
			if !matchIPList(ip, list) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "IP not in panel access whitelist"})
				return
			}
		}
		c.Next()
	}
}

func matchIPList(ip, list string) bool {
	ip = strings.TrimSpace(ip)
	if ip == "" || strings.TrimSpace(list) == "" {
		return false
	}
	clientIP := net.ParseIP(ip)
	if clientIP == nil {
		return false
	}
	for _, part := range strings.FieldsFunc(list, func(r rune) bool {
		return r == '\n' || r == ',' || r == ';' || r == ' '
	}) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "/") {
			_, network, err := net.ParseCIDR(part)
			if err == nil && network.Contains(clientIP) {
				return true
			}
			continue
		}
		if host := net.ParseIP(part); host != nil && host.Equal(clientIP) {
			return true
		}
	}
	return false
}
