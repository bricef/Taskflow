// Package transport maps domain concepts to transport-layer semantics.
// Both the HTTP server and HTTP client import from here.
package transport

import "github.com/bricef/taskflow/internal/model"

// MethodForAction maps a domain action to an HTTP method.
func MethodForAction(action model.Action) string {
	switch action {
	case model.ActionCreate:
		return "POST"
	case model.ActionList, model.ActionGet:
		return "GET"
	case model.ActionUpdate:
		return "PATCH"
	case model.ActionDelete:
		return "DELETE"
	case model.ActionSet:
		return "PUT"
	default:
		return "POST"
	}
}

// StatusForAction maps a domain action to a default HTTP success status code.
func StatusForAction(action model.Action) int {
	switch action {
	case model.ActionCreate:
		return 201
	case model.ActionDelete, model.ActionSet:
		return 204
	default:
		return 200
	}
}
