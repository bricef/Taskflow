// Package webhook implements event-driven webhook dispatch.
// A Dispatcher subscribes to the event bus and delivers matching
// events to registered webhook endpoints as signed HTTP POST requests.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"time"

	"github.com/bricef/taskflow/internal/eventbus"
	"github.com/bricef/taskflow/internal/model"
)

const (
	deliveryTimeout = 10 * time.Second
	signatureHeader = "X-TaskFlow-Signature"
	eventHeader     = "X-TaskFlow-Event"
	maxRetries      = 3
)

// Default retry delays: immediate, 30s, 5min.
var defaultRetryDelays = []time.Duration{0, 30 * time.Second, 5 * time.Minute}

// WebhookLister provides access to webhook registrations.
type WebhookLister interface {
	ListWebhooks(ctx context.Context) ([]model.Webhook, error)
}

// DeliveryLogger records webhook delivery attempts.
type DeliveryLogger interface {
	WebhookDeliveryInsert(ctx context.Context, d model.WebhookDelivery) (model.WebhookDelivery, error)
}

// Dispatcher subscribes to an event bus and delivers matching events
// to webhook endpoints. Delivery is asynchronous and non-blocking.
type Dispatcher struct {
	bus         *eventbus.EventBus
	lister      WebhookLister
	logger      DeliveryLogger
	sub         *eventbus.Subscription
	cancel      context.CancelFunc
	client      *http.Client
	retryDelays []time.Duration
}

// NewDispatcher creates and starts a webhook dispatcher.
// Call Stop() to shut it down.
// Option configures a Dispatcher.
type Option func(*Dispatcher)

// WithRetryDelays overrides the default retry delays (useful for testing).
func WithRetryDelays(delays []time.Duration) Option {
	return func(d *Dispatcher) {
		d.retryDelays = delays
	}
}

// NewDispatcher creates and starts a webhook dispatcher.
// Call Stop() to shut it down.
func NewDispatcher(bus *eventbus.EventBus, lister WebhookLister, logger DeliveryLogger, opts ...Option) *Dispatcher {
	ctx, cancel := context.WithCancel(context.Background())
	d := &Dispatcher{
		bus:         bus,
		lister:      lister,
		logger:      logger,
		cancel:      cancel,
		client:      &http.Client{Timeout: deliveryTimeout},
		retryDelays: defaultRetryDelays,
	}
	for _, opt := range opts {
		opt(d)
	}
	d.sub = bus.Subscribe()
	go d.run(ctx)
	return d
}

// Stop shuts down the dispatcher.
func (d *Dispatcher) Stop() {
	d.cancel()
	d.sub.Cancel()
}

func (d *Dispatcher) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-d.sub.C:
			if !ok {
				return
			}
			d.dispatch(ctx, evt)
		}
	}
}

func (d *Dispatcher) dispatch(ctx context.Context, evt eventbus.Event) {
	webhooks, err := d.lister.ListWebhooks(ctx)
	if err != nil {
		log.Printf("webhook: failed to list webhooks: %v", err)
		return
	}

	payload, err := json.Marshal(evt)
	if err != nil {
		log.Printf("webhook: failed to marshal event: %v", err)
		return
	}

	eventID := evt.Timestamp.Format(time.RFC3339Nano)

	for _, wh := range webhooks {
		if !wh.Active {
			continue
		}
		if !slices.Contains(wh.Events, evt.Type) {
			continue
		}
		if wh.BoardSlug != nil && *wh.BoardSlug != evt.Board.Slug {
			continue
		}
		go d.deliverWithRetry(wh, evt.Type, eventID, payload)
	}
}

func (d *Dispatcher) deliverWithRetry(wh model.Webhook, eventType, eventID string, payload []byte) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 && attempt-1 < len(d.retryDelays) {
			time.Sleep(d.retryDelays[attempt-1])
		}

		statusCode, duration, err := d.deliver(wh, eventType, payload)

		// Log the delivery attempt.
		delivery := model.WebhookDelivery{
			WebhookID:   wh.ID,
			EventType:   eventType,
			EventID:     eventID,
			Attempt:     attempt,
			StatusCode:  statusCode,
			RequestBody: string(payload),
			DurationMs:  duration,
		}
		if err != nil {
			errStr := err.Error()
			delivery.Error = &errStr
		}

		if d.logger != nil {
			if _, logErr := d.logger.WebhookDeliveryInsert(context.Background(), delivery); logErr != nil {
				log.Printf("webhook: failed to log delivery: %v", logErr)
			}
		}

		if err == nil {
			log.Printf("webhook: delivered %s to %s (attempt %d, %d)", eventType, wh.URL, attempt, *statusCode)
			return
		}

		log.Printf("webhook: delivery to %s failed (attempt %d/%d): %v", wh.URL, attempt, maxRetries, err)
	}
}

// deliver sends a single delivery attempt and returns the status code, duration in ms, and error.
func (d *Dispatcher) deliver(wh model.Webhook, eventType string, payload []byte) (*int, *int, error) {
	signature := sign(payload, wh.Secret)

	ctx, cancel := context.WithTimeout(context.Background(), deliveryTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", wh.URL, bytes.NewReader(payload))
	if err != nil {
		return nil, nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(signatureHeader, signature)
	req.Header.Set(eventHeader, eventType)

	start := time.Now()
	resp, err := d.client.Do(req)
	durationMs := int(time.Since(start).Milliseconds())

	if err != nil {
		return nil, &durationMs, fmt.Errorf("request failed: %w", err)
	}
	resp.Body.Close()

	code := resp.StatusCode
	if code >= 400 {
		return &code, &durationMs, fmt.Errorf("server returned %d", code)
	}

	return &code, &durationMs, nil
}

// sign computes an HMAC-SHA256 signature for the payload using the given secret.
func sign(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return fmt.Sprintf("sha256=%s", hex.EncodeToString(mac.Sum(nil)))
}
