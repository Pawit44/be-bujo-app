// Package config loads every environment-driven setting into one struct,
// read once at startup — nothing else in the app calls os.Getenv directly.
package config

import (
	"os"
	"strings"
)

type Config struct {
	Port string

	// DatabaseURL is a standard Postgres connection string, e.g.
	// postgres://user:password@host:5432/bujo?sslmode=disable
	DatabaseURL string

	// CORSOrigin must be a concrete origin, never "*" — the API sets
	// session cookies, and browsers refuse a wildcard origin on
	// credentialed requests anyway.
	CORSOrigin string

	// CookieSecure should be true whenever the app is served over HTTPS
	// (always, in production) so the session cookie is never sent in the
	// clear. It's only false for plain-HTTP local development.
	CookieSecure bool
	CookieDomain string

	// AdminEmails is the allowlist of accounts that register as admin.
	// Registration order plays no part — an admin account only ever comes
	// from a matching, normalized email in this set.
	AdminEmails map[string]struct{}
}

func Load() Config {
	return Config{
		Port:         env("PORT", "8080"),
		DatabaseURL:  env("DATABASE_URL", "postgres://bujo:bujo@localhost:5432/bujo?sslmode=disable"),
		CORSOrigin:   env("CORS_ORIGIN", "http://localhost:3000"),
		CookieSecure: env("COOKIE_SECURE", "false") == "true",
		CookieDomain: env("COOKIE_DOMAIN", ""),
		AdminEmails:  parseAdminEmails(env("ADMIN_EMAILS", "")),
	}
}

// parseAdminEmails turns a comma-separated ADMIN_EMAILS value into a
// normalized lookup set.
func parseAdminEmails(raw string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, e := range strings.Split(raw, ",") {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" {
			set[e] = struct{}{}
		}
	}
	return set
}

func (c Config) IsAdminEmail(email string) bool {
	_, ok := c.AdminEmails[email]
	return ok
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
