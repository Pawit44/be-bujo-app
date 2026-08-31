package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS allows exactly one origin, with credentials. A wildcard origin
// cannot be combined with credentialed (cookie-based) requests — browsers
// reject that combination, and so do we: an explicit origin is required
// once sessions exist.
func CORS(origin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Vary", "Origin")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, X-Requested-With")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
