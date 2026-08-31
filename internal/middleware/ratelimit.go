package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter is a small in-memory per-IP token bucket.
//
// This is intentionally simple (no external dependency, no shared store):
// Bujo runs as a single instance, so an in-process map is sufficient and
// costs nothing to operate. If this is ever scaled to multiple instances,
// replace it with a shared store (Redis, or the platform's own edge rate
// limiting) — a per-instance bucket would let a client get N requests per
// instance instead of N total.
type RateLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*bucket
	perSecond float64
	burst     float64
}

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

// NewRateLimiter allows `burst` requests immediately, then refills at
// `perSecond` tokens per second, per client key (IP address). It starts a
// background goroutine that evicts idle buckets.
func NewRateLimiter(perSecond float64, burst int) *RateLimiter {
	l := &RateLimiter{
		buckets:   make(map[string]*bucket),
		perSecond: perSecond,
		burst:     float64(burst),
	}
	go l.cleanupLoop()
	return l
}

func (l *RateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &bucket{tokens: l.burst - 1, lastSeen: now}
		return true
	}

	elapsed := now.Sub(b.lastSeen).Seconds()
	b.lastSeen = now
	b.tokens += elapsed * l.perSecond
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (l *RateLimiter) cleanupLoop() {
	for range time.Tick(5 * time.Minute) {
		l.mu.Lock()
		cutoff := time.Now().Add(-10 * time.Minute)
		for k, b := range l.buckets {
			if b.lastSeen.Before(cutoff) {
				delete(l.buckets, k)
			}
		}
		l.mu.Unlock()
	}
}

// Middleware rejects requests over the limit with 429. Behind a reverse
// proxy, Gin's ClientIP() honors X-Forwarded-For only when trusted proxies
// are configured — fine for a standard single-origin deployment.
func (l *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.allow(c.ClientIP()) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests — slow down and try again"})
			return
		}
		c.Next()
	}
}
