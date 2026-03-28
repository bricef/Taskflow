package model

import "time"

type ActorType string

const (
	ActorTypeHuman   ActorType = "human"
	ActorTypeAIAgent ActorType = "ai_agent"
)

func ValidateActorType(t ActorType) error {
	switch t {
	case ActorTypeHuman, ActorTypeAIAgent:
		return nil
	default:
		return &ValidationError{Field: "type", Message: "must be 'human' or 'ai_agent'"}
	}
}

type Role string

const (
	RoleAdmin    Role = "admin"
	RoleMember   Role = "member"
	RoleReadOnly Role = "read_only"
)

func ValidateRole(r Role) error {
	switch r {
	case RoleAdmin, RoleMember, RoleReadOnly:
		return nil
	default:
		return &ValidationError{Field: "role", Message: "must be 'admin', 'member', or 'read_only'"}
	}
}

type Actor struct {
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Type        ActorType `json:"type"`
	Role        Role      `json:"role"`
	APIKeyHash  string    `json:"-"`
	CreatedAt   time.Time `json:"created_at"`
	Active      bool      `json:"active"`
}

type CreateActorParams struct {
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name,omitempty"`
	Type        ActorType `json:"type"`
	Role        Role      `json:"role"`
	APIKeyHash  string    `json:"-"`
}

func (p CreateActorParams) Validate() error {
	if p.Name == "" {
		return &ValidationError{Field: "name", Message: "must not be empty"}
	}
	if err := ValidateActorType(p.Type); err != nil {
		return err
	}
	if err := ValidateRole(p.Role); err != nil {
		return err
	}
	return nil
}

type UpdateActorParams struct {
	Name        string           `json:"-"`
	DisplayName Optional[string] `json:"display_name"`
	Role        Optional[Role]   `json:"role"`
	Active      Optional[bool]   `json:"active"`
}
