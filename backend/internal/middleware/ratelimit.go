package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/auth"
)

func RateLimitSensitive(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			c.Next()
			return
		}
		uid := c.GetUint("user_id")
		key := auth.SensitiveAPIKey(uid, c.ClientIP(), scope)
		if err := auth.CheckSensitiveAPI(key); err != nil {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
			return
		}
		auth.RecordSensitiveAPI(key)
		c.Next()
	}
}

func RateLimitStrict(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			c.Next()
			return
		}
		uid := c.GetUint("user_id")
		key := auth.SensitiveAPIKey(uid, c.ClientIP(), scope)
		if err := auth.CheckStrictAPI(key); err != nil {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
			return
		}
		auth.RecordStrictAPI(key)
		c.Next()
	}
}
