package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/bricef/taskflow/internal/eventbus"
	"github.com/bricef/taskflow/internal/taskflow"
)

// ServerConfig holds configuration for the HTTP server.
type ServerConfig struct {
	// IdempotencyCacheBytes is the maximum memory budget for the idempotency
	// key cache in bytes. Defaults to 50 MB if zero.
	IdempotencyCacheBytes int

	// EventBus enables SSE event streaming. If nil, the SSE endpoint returns 503.
	EventBus *eventbus.EventBus

	// MaxRequestBodyBytes limits the size of request bodies.
	// Defaults to 1 MB if zero.
	MaxRequestBodyBytes int64

	// ReadTimeout is the maximum duration for reading the entire request.
	// Defaults to 30 seconds if zero.
	ReadTimeout time.Duration

	// WriteTimeout is the maximum duration for writing the response.
	// Defaults to 60 seconds if zero (longer to support SSE).
	WriteTimeout time.Duration

	// IdleTimeout is the maximum duration an idle keep-alive connection
	// remains open. Defaults to 120 seconds if zero.
	IdleTimeout time.Duration

	// RateLimitPerSecond is the maximum requests per second per API key.
	// Defaults to 50 if zero. Set to -1 to disable.
	RateLimitPerSecond int

	// DevMode disables all rate limiting (per-key and global IP-based).
	// Opt-in only via TASKFLOW_DEV_MODE=true.
	DevMode bool
}

func (c *ServerConfig) applyDefaults() {
	if c.IdempotencyCacheBytes == 0 {
		c.IdempotencyCacheBytes = 50 << 20 // 50 MB
	}
	if c.MaxRequestBodyBytes == 0 {
		c.MaxRequestBodyBytes = 1 << 20 // 1 MB
	}
	if c.ReadTimeout == 0 {
		c.ReadTimeout = 30 * time.Second
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = 60 * time.Second
	}
	if c.IdleTimeout == 0 {
		c.IdleTimeout = 120 * time.Second
	}
	if c.RateLimitPerSecond == 0 {
		c.RateLimitPerSecond = 50
	}
}

// Server is the HTTP API server for TaskFlow.
type Server struct {
	svc    taskflow.TaskFlow
	bus    *eventbus.EventBus
	cfg    ServerConfig
	router *chi.Mux
	openAPISpec []byte
}

// NewServer creates a new HTTP server backed by the given TaskFlow service.
func NewServer(svc taskflow.TaskFlow, cfg ...ServerConfig) *Server {
	var c ServerConfig
	if len(cfg) > 0 {
		c = cfg[0]
	}
	c.applyDefaults()

	s := &Server{
		svc:    svc,
		bus:    c.EventBus,
		cfg:    c,
		router: chi.NewRouter(),
	}

	// Global middleware.
	s.router.Use(securityHeadersMiddleware)
	s.router.Use(bodyLimitMiddleware(c.MaxRequestBodyBytes))
	s.router.Use(idempotencyMiddleware(newIdempotencyCache(c.IdempotencyCacheBytes)))

	routes := s.allRoutes()
	s.openAPISpec = generateOpenAPISpec(routes)
	s.registerRoutes()
	return s
}

// Handler returns the http.Handler for use with httptest or a real server.
func (s *Server) Handler() http.Handler {
	return s.router
}

// ListenAndServe starts the server on the given address with configured timeouts.
func (s *Server) ListenAndServe(addr string) error {
	srv := &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
		IdleTimeout:  s.cfg.IdleTimeout,
	}
	return srv.ListenAndServe()
}
