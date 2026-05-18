package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter implements a simple sliding-window per-IP rate limiter.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string][]time.Time
	limit    int
	window   time.Duration
}

// NewRateLimiter creates a rate limiter. limit is the max requests per window.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-rl.window)
		for ip, times := range rl.visitors {
			idx := 0
			for _, t := range times {
				if t.After(cutoff) {
					break
				}
				idx++
			}
			if idx >= len(times) {
				delete(rl.visitors, ip)
			} else {
				rl.visitors[ip] = times[idx:]
			}
		}
		rl.mu.Unlock()
	}
}

// Middleware returns a Gin handler that enforces the rate limit.
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions || c.FullPath() == "/api/health" || c.FullPath() == "/assets/*filepath" {
			c.Next()
			return
		}

		ip := c.ClientIP()

		rl.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-rl.window)

		times := rl.visitors[ip]
		idx := 0
		for _, t := range times {
			if t.After(cutoff) {
				break
			}
			idx++
		}
		times = times[idx:]

		if len(times) >= rl.limit {
			rl.mu.Unlock()
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{"code": "RATE_LIMITED", "message": "too many requests, try again later"},
			})
			return
		}

		rl.visitors[ip] = append(times, now)
		rl.mu.Unlock()

		c.Next()
	}
}

// DefaultRateLimiter is a browser-friendly default for self-hosted reading.
// Mobile browsers may load static assets, API data, and sync channels in a
// short burst; keep this high enough to avoid false positives during startup.
func DefaultRateLimiter() gin.HandlerFunc {
	return NewRateLimiter(600, time.Minute).Middleware()
}
