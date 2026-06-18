package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/auth"
)

func Auth(authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := ""
		header := c.GetHeader("Authorization")
		if header != "" {
			parts := strings.SplitN(header, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				token = parts[1]
			}
		}
		if token == "" {
			token = c.Query("token")
		}
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization"})
			return
		}

		claims, err := authSvc.ParseToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Set("permissions", claims.Permissions)
		c.Set("disk_quota_mb", claims.DiskQuotaMB)
		c.Next()
	}
}

func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		if role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin required"})
			return
		}
		c.Next()
	}
}

func RequirePermission(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		perms, _ := c.Get("permissions")
		roleStr, _ := role.(string)
		permStr, _ := perms.(string)
		if auth.CanAccess(roleStr, permStr, perm) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "permission denied"})
	}
}

func RequireShellAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		perms, _ := c.Get("permissions")
		roleStr, _ := role.(string)
		permStr, _ := perms.(string)
		switch roleStr {
		case "admin", "user":
			c.Next()
			return
		case "subuser":
			if auth.CanAccess(roleStr, permStr, "bastion") {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "shell access denied"})
	}
}
