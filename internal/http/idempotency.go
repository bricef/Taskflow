package http

import (
	"bytes"
	"net/http"
	"sync"
)

// idempotencyCache stores responses keyed by Idempotency-Key header.
// It is bounded by total byte size of cached entries, not by count.
// When the budget is exceeded, the oldest entries are evicted.
type idempotencyCache struct {
	mu        sync.Mutex
	maxBytes  int
	usedBytes int
	entries   map[string]*cacheEntry
	order     []string // oldest first for eviction
}

type cacheEntry struct {
	status int
	header http.Header
	body   []byte
	size   int // estimated memory footprint in bytes
}

func entrySize(header http.Header, body []byte) int {
	size := len(body)
	for k, vs := range header {
		size += len(k)
		for _, v := range vs {
			size += len(v)
		}
	}
	return size
}

// NewIdempotencyCache creates a cache bounded by maxBytes of total cached data.
// A reasonable default is 1–10 MB.
func newIdempotencyCache(maxBytes int) *idempotencyCache {
	return &idempotencyCache{
		maxBytes: maxBytes,
		entries:  make(map[string]*cacheEntry),
	}
}

func (c *idempotencyCache) get(key string) (*cacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	return e, ok
}

func (c *idempotencyCache) put(key string, entry *cacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[key]; exists {
		return
	}

	// Evict oldest entries until there's room.
	for c.usedBytes+entry.size > c.maxBytes && len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		if old, ok := c.entries[oldest]; ok {
			c.usedBytes -= old.size
			delete(c.entries, oldest)
		}
	}

	// If a single entry exceeds the budget, don't cache it.
	if entry.size > c.maxBytes {
		return
	}

	c.entries[key] = entry
	c.order = append(c.order, key)
	c.usedBytes += entry.size
}

// idempotencyMiddleware caches responses for mutating requests that include
// an Idempotency-Key header. Repeated requests with the same key return
// the cached response without re-executing the handler.
func idempotencyMiddleware(cache *idempotencyCache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawKey := r.Header.Get("Idempotency-Key")
			if rawKey == "" || r.Method == "GET" {
				next.ServeHTTP(w, r)
				return
			}
			// Scope the idempotency key per caller to prevent cross-actor collisions.
			key := r.Header.Get("Authorization") + ":" + rawKey

			if entry, ok := cache.get(key); ok {
				for k, vs := range entry.header {
					for _, v := range vs {
						w.Header().Add(k, v)
					}
				}
				w.WriteHeader(entry.status)
				w.Write(entry.body)
				return
			}

			rec := &responseRecorder{
				ResponseWriter: w,
				body:           &bytes.Buffer{},
				status:         200,
			}
			next.ServeHTTP(rec, r)

			header := w.Header().Clone()
			body := rec.body.Bytes()
			cache.put(key, &cacheEntry{
				status: rec.status,
				header: header,
				body:   body,
				size:   entrySize(header, body),
			})
		})
	}
}

type responseRecorder struct {
	http.ResponseWriter
	body   *bytes.Buffer
	status int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}
