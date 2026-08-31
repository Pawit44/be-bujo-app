package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CSRFProtect requires a custom header on state-changing requests. A
// cross-site form post or <img>/<script> tag cannot set custom headers, so
// this blocks the classic CSRF vectors even though the session cookie rides
// along automatically; SameSite=Lax on the cookie covers the rest.
func CSRFProtect() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
			if c.GetHeader("X-Requested-With") == "" {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "missing required request header"})
				return
			}
		}
		c.Next()
	}
}
