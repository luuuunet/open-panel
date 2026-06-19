package middleware

import (
	"github.com/gin-gonic/gin"
)

// SecurityHeaders adds baseline HTTP security headers for the panel UI and API.
func SecurityHeaders(enabled func() bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if enabled == nil || enabled() {
			h := c.Writer.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "SAMEORIGIN")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			h.Set("X-XSS-Protection", "1; mode=block")
			h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; font-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'self'; base-uri 'self'; form-action 'self'")
			if c.Request.TLS != nil {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
		}
		c.Next()
	}
}
