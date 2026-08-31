package controller

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"bujo/internal/config"
	"bujo/internal/middleware"
	"bujo/internal/service"
)

type AuthController struct {
	auth *service.AuthService
	cfg  config.Config
}

func NewAuthController(auth *service.AuthService, cfg config.Config) *AuthController {
	return &AuthController{auth: auth, cfg: cfg}
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// Register POST /api/auth/register
func (ctrl *AuthController) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	user, err := ctrl.auth.Register(req.Email, req.Password, req.Name)
	if err != nil {
		ctrl.respondAuthError(c, err)
		return
	}
	// No session cookie is set here on purpose — the account exists, but the
	// client still has to sign in through /auth/login to get one.
	c.JSON(http.StatusCreated, user)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login POST /api/auth/login
func (ctrl *AuthController) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	user, token, expiresAt, err := ctrl.auth.Login(req.Email, req.Password)
	if err != nil {
		ctrl.respondAuthError(c, err)
		return
	}
	ctrl.setSession(c, token, expiresAt)
	c.JSON(http.StatusOK, user)
}

// Logout POST /api/auth/logout — invalidates the session server-side, not
// just the cookie, so a copied cookie stops working immediately too.
func (ctrl *AuthController) Logout(c *gin.Context) {
	if token, err := c.Cookie(middleware.CookieName); err == nil {
		ctrl.auth.Logout(token)
	}
	middleware.ClearSessionCookie(c, ctrl.cfg.CookieSecure, ctrl.cfg.CookieDomain)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Me GET /api/auth/me — must run behind RequireAuth.
func (ctrl *AuthController) Me(c *gin.Context) {
	c.JSON(http.StatusOK, middleware.CurrentUser(c))
}

type deleteMeRequest struct {
	Password string `json:"password"`
}

// DeleteMe DELETE /api/auth/me — self-service account deletion (the
// right-to-erasure path): a user erasing their own account and everything
// they wrote, with no admin involved.
func (ctrl *AuthController) DeleteMe(c *gin.Context) {
	user := middleware.CurrentUser(c)
	var req deleteMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if err := ctrl.auth.DeleteAccount(user, req.Password); err != nil {
		switch {
		case errors.Is(err, service.ErrWrongPassword):
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrLastAdminAccount):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete account"})
		}
		return
	}
	middleware.ClearSessionCookie(c, ctrl.cfg.CookieSecure, ctrl.cfg.CookieDomain)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (ctrl *AuthController) setSession(c *gin.Context, token string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	middleware.SetSessionCookie(c, ctrl.cfg.CookieSecure, ctrl.cfg.CookieDomain, token, maxAge)
}

func (ctrl *AuthController) respondAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrEmailTaken):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidLogin):
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrAccountLocked):
		c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
	default:
		// Validation errors (bad email format, weak password, etc.) are
		// plain messages meant to be shown to the user as-is.
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
}
