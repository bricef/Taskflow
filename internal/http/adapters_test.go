package http

import (
	"testing"

	"github.com/bricef/taskflow/internal/model"
)

func TestMethodForAction(t *testing.T) {
	tests := []struct {
		action model.Action
		want   string
	}{
		{model.ActionCreate, "POST"},
		{model.ActionList, "GET"},
		{model.ActionGet, "GET"},
		{model.ActionUpdate, "PATCH"},
		{model.ActionDelete, "DELETE"},
		{model.ActionSet, "PUT"},
		{model.ActionTransition, "POST"},
		{model.ActionReassign, "POST"},
		{model.ActionHealth, "POST"},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			if got := MethodForAction(tt.action); got != tt.want {
				t.Errorf("MethodForAction(%q) = %q, want %q", tt.action, got, tt.want)
			}
		})
	}
}

func TestStatusForAction(t *testing.T) {
	tests := []struct {
		action model.Action
		want   int
	}{
		{model.ActionCreate, 201},
		{model.ActionList, 200},
		{model.ActionGet, 200},
		{model.ActionUpdate, 200},
		{model.ActionDelete, 204},
		{model.ActionSet, 204},
		{model.ActionTransition, 200},
		{model.ActionReassign, 200},
		{model.ActionHealth, 200},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			if got := statusForAction(tt.action); got != tt.want {
				t.Errorf("statusForAction(%q) = %d, want %d", tt.action, got, tt.want)
			}
		})
	}
}
