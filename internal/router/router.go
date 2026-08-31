// Package router wires HTTP routes to controllers and middleware. It has
// no business logic of its own — every route just names which controller
// method handles it and which middleware guards it.
package router

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"bujo/internal/controller"
	"bujo/internal/middleware"
)

// Controllers groups every controller the router needs, so New has one
// parameter to update instead of growing a long argument list.
type Controllers struct {
	Auth       *controller.AuthController
	Entry      *controller.EntryController
	Collection *controller.CollectionController
	Admin      *controller.AdminController
	Overview   *controller.OverviewController
}

// Deps are the cross-cutting pieces routes are wrapped with.
type Deps struct {
	CORSOrigin       string
	SessionValidator middleware.SessionValidator
}

func New(ctrls Controllers, deps Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), middleware.CORS(deps.CORSOrigin), middleware.SecurityHeaders())

	requireAuth := middleware.RequireAuth(deps.SessionValidator)
	requireAdmin := middleware.RequireAdmin()
	csrf := middleware.CSRFProtect()

	// Tight limit on register/login — this is the endpoint a credential-
	// stuffing or signup-spam bot would hammer. ~10/min with a small burst
	// is generous for a real person mistyping a password a few times.
	authLimiter := middleware.NewRateLimiter(10.0/60.0, 6)
	// Looser general limit so normal page loads (several GETs at once)
	// never trip it, but a runaway script or scraper does.
	apiLimiter := middleware.NewRateLimiter(4, 60)

	api := r.Group("/api", apiLimiter.Middleware())
	{
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now()})
		})

		// Public: no account exists yet at this point.
		public := api.Group("/auth", csrf, authLimiter.Middleware())
		public.POST("/register", ctrls.Auth.Register)
		public.POST("/login", ctrls.Auth.Login)

		// Everything below requires a valid session cookie.
		authed := api.Group("", requireAuth)

		authed.GET("/auth/me", ctrls.Auth.Me)
		authed.POST("/auth/logout", csrf, ctrls.Auth.Logout)
		authed.DELETE("/auth/me", csrf, authLimiter.Middleware(), ctrls.Auth.DeleteMe)

		authed.GET("/index", ctrls.Overview.Index)
		authed.GET("/stats", ctrls.Overview.Stats)

		authed.GET("/entries", ctrls.Entry.List)
		authed.GET("/entries/:id", ctrls.Entry.Get)
		mutating := authed.Group("", csrf)
		mutating.POST("/entries", ctrls.Entry.Create)
		mutating.POST("/entries/reorder", ctrls.Entry.Reorder)
		mutating.PATCH("/entries/:id", ctrls.Entry.Update)
		mutating.DELETE("/entries/:id", ctrls.Entry.Delete)
		mutating.POST("/entries/:id/toggle", ctrls.Entry.Toggle)
		mutating.POST("/entries/:id/migrate", ctrls.Entry.Migrate)

		authed.GET("/collections", ctrls.Collection.List)
		authed.GET("/collections/:id", ctrls.Collection.Get)
		mutating.POST("/collections", ctrls.Collection.Create)
		mutating.PATCH("/collections/:id", ctrls.Collection.Update)
		mutating.DELETE("/collections/:id", ctrls.Collection.Delete)

		// Admin-only: account management, never journal content.
		admin := authed.Group("/admin", requireAdmin)
		admin.GET("/users", ctrls.Admin.ListUsers)
		adminMutating := admin.Group("", csrf)
		adminMutating.PATCH("/users/:id", ctrls.Admin.UpdateRole)
		adminMutating.DELETE("/users/:id", ctrls.Admin.DeleteUser)
	}

	return r
}
