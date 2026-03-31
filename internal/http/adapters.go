package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/bricef/taskflow/internal/model"
)

// resolveAtMe replaces "@me" with the authenticated actor's name.
func resolveAtMe(ctx context.Context, s *string) {
	if s != nil && *s == "@me" {
		name := ActorFrom(ctx).Name
		*s = name
	}
}

// handler is the inner function signature that all adapters produce.
// It takes the request context and the HTTP request, and returns a result to serialize.
type handler func(ctx context.Context, r *http.Request) (any, error)

// jsonBody reads a JSON request body into P and calls fn.
func jsonBody[P any, R any](fn func(context.Context, P) (R, error)) handler {
	return func(ctx context.Context, r *http.Request) (any, error) {
		var p P
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			return nil, &model.ValidationError{Field: "body", Message: "invalid JSON: " + err.Error()}
		}
		return fn(ctx, p)
	}
}

// noInput calls fn with no request parameters.
func noInput[R any](fn func(context.Context) (R, error)) handler {
	return func(ctx context.Context, _ *http.Request) (any, error) {
		return fn(ctx)
	}
}

// pathStr extracts a single string path parameter and calls fn.
func pathStr[R any](param string, fn func(context.Context, string) (R, error)) handler {
	return func(ctx context.Context, r *http.Request) (any, error) {
		return fn(ctx, chi.URLParam(r, param))
	}
}

// pathInt extracts a single integer path parameter and calls fn.
func pathInt[R any](param string, fn func(context.Context, int) (R, error)) handler {
	return func(ctx context.Context, r *http.Request) (any, error) {
		id, err := strconv.Atoi(chi.URLParam(r, param))
		if err != nil {
			return nil, &model.ValidationError{Field: param, Message: "must be an integer"}
		}
		return fn(ctx, id)
	}
}

// pathStrInt extracts a string and an integer path parameter and calls fn.
func pathStrInt[R any](p1, p2 string, fn func(context.Context, string, int) (R, error)) handler {
	return func(ctx context.Context, r *http.Request) (any, error) {
		s := chi.URLParam(r, p1)
		n, err := strconv.Atoi(chi.URLParam(r, p2))
		if err != nil {
			return nil, &model.ValidationError{Field: p2, Message: "must be an integer"}
		}
		return fn(ctx, s, n)
	}
}

// urlParamStr extracts a string path parameter from the request.
func urlParamStr(r *http.Request, param string) string {
	return chi.URLParam(r, param)
}

// urlParamInt extracts an integer path parameter from the request.
func urlParamInt(r *http.Request, param string) (int, error) {
	n, err := strconv.Atoi(chi.URLParam(r, param))
	if err != nil {
		return 0, &model.ValidationError{Field: param, Message: "must be an integer"}
	}
	return n, nil
}

// decodeBody reads the JSON request body into the given pointer.
func decodeBody(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return &model.ValidationError{Field: "body", Message: "invalid JSON: " + err.Error()}
	}
	return nil
}

// queryStr returns a query parameter as a *string (nil if absent).
func queryStr(r *http.Request, key string) *string {
	v := r.URL.Query().Get(key)
	if v == "" {
		return nil
	}
	return &v
}

// queryBool returns a query parameter as a bool (false if absent).
func queryBool(r *http.Request, key string) bool {
	v := r.URL.Query().Get(key)
	return v == "true" || v == "1"
}

// Ensure fmt is used.
var _ = fmt.Sprintf
