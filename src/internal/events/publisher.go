package events

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	aexnats "github.com/parlakisik/agent-exchange/internal/nats"
)

// Publisher handles event publishing via NATS JetStream, with optional HTTP
// webhook fallback. When a NATS client is configured, events are published to
// JetStream using the event type as the NATS subject and the IdempotencyKey
// for server-side deduplication. If no NATS client is present the publisher
// falls back to logging only (useful for tests and local development).
type Publisher struct {
	source     string
	natsClient *aexnats.Client
	httpClient *http.Client
	endpoints  map[string]string // eventType -> webhook URL
}

// NewPublisher creates a new event publisher that logs events only. Use
// WithNATS to attach a JetStream backend.
func NewPublisher(source string) *Publisher {
	return &Publisher{
		source: source,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		endpoints: make(map[string]string),
	}
}

// NewPublisherWithNATS creates a publisher that publishes events to NATS
// JetStream. HTTP webhook endpoints can still be registered as a secondary
// delivery mechanism.
func NewPublisherWithNATS(source string, nc *aexnats.Client) *Publisher {
	return &Publisher{
		source:     source,
		natsClient: nc,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		endpoints: make(map[string]string),
	}
}

// WithNATS attaches a NATS client to an existing publisher.
func (p *Publisher) WithNATS(nc *aexnats.Client) {
	p.natsClient = nc
}

// RegisterEndpoint registers a webhook endpoint for an event type.
func (p *Publisher) RegisterEndpoint(eventType, webhookURL string) {
	p.endpoints[eventType] = webhookURL
}

// Publish publishes an event. When a NATS client is configured the event
// envelope is published to JetStream with the IdempotencyKey set as the
// Nats-Msg-Id header for deduplication. The event type is mapped directly to
// a NATS subject (e.g. "work.submitted" -> subject "work.submitted").
//
// If a webhook endpoint is registered for the event type, the event is also
// delivered via HTTP POST as a secondary channel.
func (p *Publisher) Publish(ctx context.Context, eventType string, data map[string]any) error {
	envelope := Envelope{
		EventID:        generateEventID(),
		EventType:      eventType,
		SchemaVersion:  "1.0",
		IdempotencyKey: fmt.Sprintf("%s_%s_%d", eventType, data["work_id"], time.Now().Unix()),
		Timestamp:      time.Now().UTC(),
		Source:         p.source,
		Data:           data,
	}

	if tenantID, ok := data["tenant_id"].(string); ok {
		envelope.TenantID = tenantID
	}

	// Publish to NATS JetStream when a client is configured.
	if p.natsClient != nil {
		subject := aexnats.SubjectForEvent(eventType)

		if err := p.natsClient.Publish(ctx, subject, envelope.IdempotencyKey, envelope); err != nil {
			slog.ErrorContext(ctx, "nats_publish_failed",
				"event_id", envelope.EventID,
				"event_type", envelope.EventType,
				"subject", subject,
				"error", err,
			)
			return fmt.Errorf("publish event %s to nats: %w", eventType, err)
		}

		slog.InfoContext(ctx, "event_published",
			"event_id", envelope.EventID,
			"event_type", envelope.EventType,
			"source", envelope.Source,
			"subject", subject,
			"transport", "nats",
		)
	} else {
		// Fallback: log only (no NATS client configured).
		slog.InfoContext(ctx, "event_published",
			"event_id", envelope.EventID,
			"event_type", envelope.EventType,
			"source", envelope.Source,
			"transport", "log",
		)
	}

	// If webhook endpoint registered, also send HTTP POST.
	if webhookURL, ok := p.endpoints[eventType]; ok {
		if err := p.sendWebhook(ctx, webhookURL, envelope); err != nil {
			// Log but don't fail; webhook delivery is best-effort.
			slog.WarnContext(ctx, "webhook_delivery_error",
				"event_type", eventType,
				"error", err,
			)
		}
	}

	return nil
}

func (p *Publisher) sendWebhook(ctx context.Context, url string, envelope Envelope) error {
	body, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-ID", envelope.EventID)
	req.Header.Set("X-Event-Type", envelope.EventType)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		slog.WarnContext(ctx, "webhook_failed",
			"url", url,
			"event_type", envelope.EventType,
			"error", err,
		)
		return nil // Don't fail on webhook errors
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		slog.WarnContext(ctx, "webhook_error",
			"url", url,
			"event_type", envelope.EventType,
			"status", resp.StatusCode,
		)
	}

	return nil
}

func generateEventID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "evt_" + hex.EncodeToString(b[:])
}
