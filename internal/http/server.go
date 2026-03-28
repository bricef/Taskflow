package http

import (
	"net/http"

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
}

// Server is the HTTP API server for TaskFlow.
type Server struct {
	svc         taskflow.TaskFlow
	bus         *eventbus.EventBus
	router      *chi.Mux
	openAPISpec []byte
}

// NewServer creates a new HTTP server backed by the given TaskFlow service.
func NewServer(svc taskflow.TaskFlow, cfg ...ServerConfig) *Server {
	var c ServerConfig
	if len(cfg) > 0 {
		c = cfg[0]
	}
	if c.IdempotencyCacheBytes == 0 {
		c.IdempotencyCacheBytes = 50 << 20 // 50 MB
	}

	s := &Server{
		svc:    svc,
		bus:    c.EventBus,
		router: chi.NewRouter(),
	}

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

// ListenAndServe starts the server on the given address.
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s.router)
}
