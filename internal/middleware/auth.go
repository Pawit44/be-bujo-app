package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"bujo/internal/models"
)

const (
	userContextKey = "auth.user"
	CookieName     = "bujo_session"
)

// SessionValidator is the one thing this middleware needs from AuthService —
// named narrowly here (not imported as a concrete type) so the HTTP layer
// doesn't need to know about the service layer's other dependencies.
type SessionValidator interface {
	ValidateSession(token string) (*models.User, error)
}

// SetSessionCookie writes the session cookie with the hardening flags that
// matter most for a cookie holding an authentication token: HttpOnly (not
// readable by JavaScript, so XSS can't steal it) and SameSite=Lax (not sent
// on cross-site requests, which blocks most CSRF vectors).
func SetSessionCookie(c *gin.Context, secure bool, domain, token string, maxAgeSeconds int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(CookieName, token, maxAgeSeconds, "/", domain, secure, true)
}

func ClearSessionCookie(c *gin.Context, secure bool, domain string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(CookieName, "", -1, "/", domain, secure, true)
}

// RequireAuth loads the session cookie, validates it, and stores the user on
// the request context. Anything mounted behind it can assume an
// authenticated user is present.
func RequireAuth(validator SessionValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(CookieName)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		user, err := validator.ValidateSession(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		c.Set(userContextKey, user)
		c.Next()
	}
}

// RequireAdmin must run after RequireAuth. It never grants an admin access
// to another user's journal data — it only gates the /api/admin endpoints.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := CurrentUser(c)
		if user == nil || user.Role != models.RoleAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			return
		}
		c.Next()
	}
}

// CurrentUser returns the authenticated user set by RequireAuth, or nil.
func CurrentUser(c *gin.Context) *models.User {
	v, ok := c.Get(userContextKey)
	if !ok {
		return nil
	}
	user, ok := v.(*models.User)
	if !ok {
		return nil
	}
	return user
}
