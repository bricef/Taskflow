package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
)

const maxBatchSize = 50

type batchRequest struct {
	Operations []batchOp `json:"operations"`
}

type batchOp struct {
	Method         string `json:"method"`
	Path           string `json:"path"`
	Body           any    `json:"body,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type batchResponse struct {
	Results []batchResult `json:"results"`
}

type batchResult struct {
	Status int `json:"status"`
	Body   any `json:"body,omitempty"`
}

func (s *Server) batchHandler(w http.ResponseWriter, r *http.Request) {
	var req batchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "invalid JSON: "+err.Error(), nil)
		return
	}

	if len(req.Operations) == 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "operations array must not be empty", nil)
		return
	}
	if len(req.Operations) > maxBatchSize {
		writeError(w, http.StatusBadRequest, "validation_error",
			fmt.Sprintf("too many operations (max %d)", maxBatchSize), nil)
		return
	}

	// Inherit auth from the batch request.
	authHeader := r.Header.Get("Authorization")

	results := make([]batchResult, len(req.Operations))
	for i, op := range req.Operations {
		results[i] = s.executeBatchOp(op, authHeader)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(batchResponse{Results: results}); err != nil {
		log.Printf("error encoding batch response: %v", err)
	}
}

func (s *Server) executeBatchOp(op batchOp, authHeader string) batchResult {
	// Validate path to prevent traversal and SSRF.
	if op.Path == "" || op.Path[0] != '/' || strings.Contains(op.Path, "..") {
		return batchResult{Status: 400, Body: map[string]string{"error": "invalid path"}}
	}

	var bodyReader io.Reader
	if op.Body != nil {
		b, err := json.Marshal(op.Body)
		if err != nil {
			return batchResult{Status: 400, Body: map[string]string{"error": "invalid body"}}
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(op.Method, op.Path, bodyReader)
	if err != nil {
		return batchResult{Status: 400, Body: map[string]string{"error": "invalid request"}}
	}
	if op.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if op.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", op.IdempotencyKey)
	}

	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)

	var body any
	if rec.Body.Len() > 0 {
		json.Unmarshal(rec.Body.Bytes(), &body)
	}

	return batchResult{Status: rec.Code, Body: body}
}
