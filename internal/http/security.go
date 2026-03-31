package http

import (
	"net/http"
	"sync"
	"time"
)

// bodyLimitMiddleware limits request body size.
func bodyLimitMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// rateLimiter implements a simple per-key token bucket rate limiter.
type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     int // tokens per second
	disabled bool
}

type bucket struct {
	tokens   float64
	lastTime time.Time
}

func newRateLimiter(perSecond int) *rateLimiter {
	return &rateLimiter{
		buckets:  make(map[string]*bucket),
		rate:     perSecond,
		disabled: perSecond < 0,
	}
}

func (rl *rateLimiter) allow(key string) bool {
	if rl.disabled {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(rl.rate), lastTime: now}
		rl.buckets[key] = b
	}

	// Add tokens based on elapsed time.
	elapsed := now.Sub(b.lastTime).Seconds()
	b.tokens += elapsed * float64(rl.rate)
	if b.tokens > float64(rl.rate) {
		b.tokens = float64(rl.rate) // cap at max burst
	}
	b.lastTime = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// rateLimitMiddleware applies per-API-key rate limiting.
// Must be placed after auth middleware so the actor is available.
func rateLimitMiddleware(rl *rateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := ActorFrom(r.Context())
			if actor.Name != "" && !rl.allow(actor.Name) {
				writeError(w, http.StatusTooManyRequests, "rate_limited",
					"too many requests, please slow down", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
