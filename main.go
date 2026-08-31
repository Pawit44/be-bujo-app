package main

import (
	"log"

	"github.com/joho/godotenv"

	"bujo/internal/config"
	"bujo/internal/controller"
	"bujo/internal/database"
	"bujo/internal/repository"
	"bujo/internal/router"
	"bujo/internal/service"
)

func main() {
	// .env is optional — real environment variables (e.g. in production)
	// always take precedence, and a missing file is not an error.
	if err := godotenv.Load(); err != nil {
		log.Printf("no .env file found, using process environment")
	}

	cfg := config.Load()

	if !cfg.CookieSecure {
		log.Printf("warning: COOKIE_SECURE=false — session cookies will be sent over plain HTTP. Set COOKIE_SECURE=true in production (behind HTTPS).")
	}
	if len(cfg.AdminEmails) == 0 {
		log.Printf("warning: ADMIN_EMAILS is empty — no email will register as admin. Set it to a comma-separated allowlist, e.g. ADMIN_EMAILS=you@example.com")
	}

	db := database.Connect(cfg.DatabaseURL, cfg.AutoMigrate)

	// Repositories — the only layer that touches *gorm.DB.
	users := repository.NewUserRepository(db)
	sessions := repository.NewSessionRepository(db)
	entries := repository.NewEntryRepository(db)
	collections := repository.NewCollectionRepository(db)

	// Services — business logic, depending only on repository interfaces.
	authService := service.NewAuthService(users, sessions, cfg.AdminEmails)
	entryService := service.NewEntryService(entries, collections)
	collectionService := service.NewCollectionService(collections, entries)
	adminService := service.NewAdminService(users, sessions)
	overviewService := service.NewOverviewService(entries, collections)

	// Controllers — thin HTTP adapters over the services above.
	ctrls := router.Controllers{
		Auth:       controller.NewAuthController(authService, cfg),
		Entry:      controller.NewEntryController(entryService),
		Collection: controller.NewCollectionController(collectionService),
		Admin:      controller.NewAdminController(adminService),
		Overview:   controller.NewOverviewController(overviewService),
	}

	r := router.New(ctrls, router.Deps{
		CORSOrigin:       cfg.CORSOrigin,
		SessionValidator: authService,
	})

	log.Printf("BUJO API listening on http://localhost:%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
