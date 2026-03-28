package http

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/bricef/taskflow/internal/model"
	"github.com/bricef/taskflow/internal/taskflow"
)

// authMiddleware resolves the API key from the Authorization header
// and injects the actor into the request context.
func authMiddleware(svc taskflow.TaskFlow) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := extractBearerToken(r)
			if key == "" {
				writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid Authorization header", nil)
				return
			}

			hash := hashAPIKey(key)
			actor, err := svc.GetActorByAPIKeyHash(r.Context(), hash)
			if err != nil {
				var nfe *model.NotFoundError
				if errors.As(err, &nfe) {
					writeError(w, http.StatusUnauthorized, "unauthorized", "invalid API key", nil)
					return
				}
				writeError(w, http.StatusInternalServerError, "internal", "authentication error", nil)
				return
			}

			if !actor.Active {
				writeError(w, http.StatusForbidden, "forbidden", "actor is deactivated", nil)
				return
			}

			ctx := withActor(r.Context(), actor)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// roleAtLeast returns true if the actor's role meets or exceeds the required role.
func roleAtLeast(actorRole, required model.Role) bool {
	hierarchy := map[model.Role]int{
		model.RoleAdmin:    3,
		model.RoleMember:   2,
		model.RoleReadOnly: 1,
	}
	return hierarchy[actorRole] >= hierarchy[required]
}

// handle wraps a handler with RBAC checking, error mapping, and JSON response formatting.
func (s *Server) handle(minRole model.Role, successStatus int, h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor := ActorFrom(r.Context())
		if !roleAtLeast(actor.Role, minRole) {
			writeError(w, http.StatusForbidden, "forbidden", "insufficient permissions", map[string]any{
				"required_role": minRole,
				"actor_role":    actor.Role,
			})
			return
		}

		result, err := h(r.Context(), r)
		if err != nil {
			writeServiceError(w, err)
			return
		}

		if result == nil || successStatus == http.StatusNoContent {
			w.WriteHeader(successStatus)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(successStatus)
		if err := json.NewEncoder(w).Encode(result); err != nil {
			log.Printf("error encoding response: %v", err)
		}
	}
}

// writeServiceError maps service errors to HTTP status codes.
func writeServiceError(w http.ResponseWriter, err error) {
	var ve *model.ValidationError
	var nfe *model.NotFoundError
	var ce *model.ConflictError

	switch {
	case errors.As(err, &ve):
		detail := map[string]any{"field": ve.Field}
		if ve.Detail != nil {
			detail["context"] = ve.Detail
		}
		writeError(w, http.StatusBadRequest, "validation_error", ve.Error(), detail)
	case errors.As(err, &nfe):
		writeError(w, http.StatusNotFound, "not_found", nfe.Error(), nil)
	case errors.As(err, &ce):
		writeError(w, http.StatusConflict, "conflict", ce.Error(), nil)
	default:
		log.Printf("internal error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal", "an internal error occurred", nil)
	}
}

// writeError writes a consistent JSON error response.
func writeError(w http.ResponseWriter, status int, code, message string, detail any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]any{
		"error":   code,
		"message": message,
	}
	if detail != nil {
		resp["detail"] = detail
	}
	json.NewEncoder(w).Encode(resp)
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}

// hashAPIKey produces a SHA-256 hex digest of the API key.
func hashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// HashAPIKey is the exported version for use by bootstrap/tests.
func HashAPIKey(key string) string {
	return hashAPIKey(key)
}
