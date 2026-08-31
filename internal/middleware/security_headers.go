package middleware

import "github.com/gin-gonic/gin"

// SecurityHeaders sets a few defensive response headers that cost nothing
// and close off common browser-side attack surface.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}
