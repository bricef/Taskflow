package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/bricef/taskflow/internal/taskflow"
)

// Server is the HTTP API server for TaskFlow.
type Server struct {
	svc         taskflow.TaskFlow
	router      *chi.Mux
	openAPISpec []byte // cached, generated once at startup
}

// NewServer creates a new HTTP server backed by the given TaskFlow service.
func NewServer(svc taskflow.TaskFlow) *Server {
	s := &Server{
		svc:    svc,
		router: chi.NewRouter(),
	}
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
