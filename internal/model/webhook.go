package model

import "time"

type Webhook struct {
	ID        int       `json:"id"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`     // Event types to subscribe to (e.g., "task.created", "task.transitioned").
	BoardSlug *string   `json:"board_slug"` // Nil means all boards.
	Secret    string    `json:"-"`          // Shared secret for HMAC-SHA256 webhook signatures.
	Active    bool      `json:"active"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateWebhookParams struct {
	URL       string
	Events    []string
	BoardSlug *string
	Secret    string
	CreatedBy string
}

func (p CreateWebhookParams) Validate() error {
	if p.URL == "" {
		return &ValidationError{Field: "url", Message: "must not be empty"}
	}
	if len(p.Events) == 0 {
		return &ValidationError{Field: "events", Message: "must not be empty"}
	}
	if p.Secret == "" {
		return &ValidationError{Field: "secret", Message: "must not be empty"}
	}
	return nil
}

type UpdateWebhookParams struct {
	ID     int
	URL    Optional[string]
	Events Optional[[]string]
	Active Optional[bool]
}
