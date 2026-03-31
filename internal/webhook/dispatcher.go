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
)

// WebhookLister provides access to webhook registrations.
type WebhookLister interface {
	ListWebhooks(ctx context.Context) ([]model.Webhook, error)
}

// Dispatcher subscribes to an event bus and delivers matching events
// to webhook endpoints. Delivery is asynchronous and non-blocking.
type Dispatcher struct {
	bus     *eventbus.EventBus
	lister  WebhookLister
	sub     *eventbus.Subscription
	cancel  context.CancelFunc
	client  *http.Client
}

// NewDispatcher creates and starts a webhook dispatcher.
// Call Stop() to shut it down.
func NewDispatcher(bus *eventbus.EventBus, lister WebhookLister) *Dispatcher {
	ctx, cancel := context.WithCancel(context.Background())
	d := &Dispatcher{
		bus:    bus,
		lister: lister,
		cancel: cancel,
		client: &http.Client{Timeout: deliveryTimeout},
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
		// Deliver asynchronously so we don't block on slow endpoints.
		go d.deliver(wh, evt)
	}
}

func (d *Dispatcher) deliver(wh model.Webhook, evt eventbus.Event) {
	payload, err := json.Marshal(evt)
	if err != nil {
		log.Printf("webhook: failed to marshal event for %s: %v", wh.URL, err)
		return
	}

	signature := sign(payload, wh.Secret)

	ctx, cancel := context.WithTimeout(context.Background(), deliveryTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", wh.URL, bytes.NewReader(payload))
	if err != nil {
		log.Printf("webhook: failed to create request for %s: %v", wh.URL, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(signatureHeader, signature)
	req.Header.Set(eventHeader, evt.Type)

	resp, err := d.client.Do(req)
	if err != nil {
		log.Printf("webhook: delivery to %s failed: %v", wh.URL, err)
		return
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("webhook: delivery to %s returned %d", wh.URL, resp.StatusCode)
		return
	}

	log.Printf("webhook: delivered %s to %s (%d)", evt.Type, wh.URL, resp.StatusCode)
}

// sign computes an HMAC-SHA256 signature for the payload using the given secret.
func sign(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return fmt.Sprintf("sha256=%s", hex.EncodeToString(mac.Sum(nil)))
}
