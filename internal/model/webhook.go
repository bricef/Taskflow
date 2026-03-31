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
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	BoardSlug *string  `json:"board_slug"`
	Secret    string   `json:"secret"`
	CreatedBy string   `json:"-"`
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

// WebhookDelivery records a single delivery attempt.
type WebhookDelivery struct {
	ID          int        `json:"id"`
	WebhookID   int        `json:"webhook_id"`
	EventType   string     `json:"event_type"`
	EventID     string     `json:"event_id"`
	Attempt     int        `json:"attempt"`
	StatusCode  *int       `json:"status_code"`  // nil if request failed before response
	Error       *string    `json:"error"`         // nil on success
	RequestBody string     `json:"-"`             // not exposed in API responses
	DurationMs  *int       `json:"duration_ms"`
	CreatedAt   time.Time  `json:"created_at"`
}

type UpdateWebhookParams struct {
	ID     int                `json:"-"`
	URL    Optional[string]   `json:"url"`
	Events Optional[[]string] `json:"events"`
	Active Optional[bool]     `json:"active"`
}
