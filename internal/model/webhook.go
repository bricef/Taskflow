package model

import (
	"net"
	"net/url"
	"time"
)

// AllowPrivateWebhookURLs controls whether webhook URLs can point to
// private/loopback addresses. Defaults to false (production).
// Set to true for development and testing.
var AllowPrivateWebhookURLs = false

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
	if err := validateWebhookURL(p.URL); err != nil {
		return err
	}
	if len(p.Events) == 0 {
		return &ValidationError{Field: "events", Message: "must not be empty"}
	}
	if p.Secret == "" {
		return &ValidationError{Field: "secret", Message: "must not be empty"}
	}
	return nil
}

// validateWebhookURL checks that the URL is a valid http/https URL.
// In production mode (default), private and loopback addresses are blocked.
func validateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return &ValidationError{Field: "url", Message: "must be a valid URL"}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return &ValidationError{Field: "url", Message: "must use http or https scheme"}
	}
	if u.Host == "" {
		return &ValidationError{Field: "url", Message: "must include a host"}
	}
	if !AllowPrivateWebhookURLs {
		host := u.Hostname()
		if host == "localhost" {
			return &ValidationError{Field: "url", Message: "must not point to localhost"}
		}
		if ip := net.ParseIP(host); ip != nil {
			if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
				return &ValidationError{Field: "url", Message: "must not point to a private or loopback address"}
			}
		}
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
