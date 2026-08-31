package controller

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// internalError responds to an unexpected failure (a database hiccup, a bug —
// never something the caller did wrong) with a message safe to show anyone,
// while logging the real error server-side so it's still debuggable.
//
// A 500 is by definition the "this should never happen" path, so the
// underlying Go error can be anything — including a raw driver error like
// `duplicate key value violates unique constraint "idx_users_email"`, which
// is exactly what reached a user before the register race was caught further
// up the stack (see repository.ErrEmailTaken). That was one instance of a
// general shape: every `err.Error()` placed directly into a 500 response has
// the same failure mode waiting in it, so this is the one place all of them
// route through instead of each controller repeating the mistake.
func internalError(c *gin.Context, context string, err error) {
	log.Printf("%s: %v", context, err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong, please try again"})
}
