package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// Client wraps a NATS connection with JetStream support.
type Client struct {
	conn *nats.Conn
	js   nats.JetStreamContext

	mu     sync.RWMutex
	closed bool
}

// Config holds NATS connection configuration.
type Config struct {
	// URL is the NATS server URL (e.g. "nats://localhost:4222").
	URL string

	// Name identifies this client in NATS server logs.
	Name string

	// Token is an optional auth token.
	Token string

	// MaxReconnects controls the number of reconnect attempts (-1 for infinite).
	MaxReconnects int

	// ReconnectWait is the delay between reconnect attempts.
	ReconnectWait time.Duration
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		URL:           nats.DefaultURL,
		Name:          "aex-client",
		MaxReconnects: -1, // infinite reconnects
		ReconnectWait: 2 * time.Second,
	}
}

// Connect establishes a NATS connection and initialises JetStream.
func Connect(cfg Config) (*Client, error) {
	opts := []nats.Option{
		nats.Name(cfg.Name),
		nats.MaxReconnects(cfg.MaxReconnects),
		nats.ReconnectWait(cfg.ReconnectWait),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			slog.Warn("nats disconnected", "error", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			slog.Info("nats reconnected", "url", nc.ConnectedUrl())
		}),
		nats.ClosedHandler(func(_ *nats.Conn) {
			slog.Info("nats connection closed")
		}),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			slog.Error("nats async error", "error", err)
		}),
	}

	if cfg.Token != "" {
		opts = append(opts, nats.Token(cfg.Token))
	}

	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream init: %w", err)
	}

	slog.Info("nats connected",
		"url", nc.ConnectedUrl(),
		"name", cfg.Name,
	)

	return &Client{conn: nc, js: js}, nil
}

// JetStream returns the underlying JetStream context.
func (c *Client) JetStream() nats.JetStreamContext {
	return c.js
}

// Conn returns the underlying NATS connection.
func (c *Client) Conn() *nats.Conn {
	return c.conn
}

// EnsureStreams creates or updates all AEX streams. This is safe to call on
// every startup; existing streams are updated in place.
func (c *Client) EnsureStreams() error {
	for _, def := range AllStreams() {
		cfg := def.Config()
		existing, err := c.js.StreamInfo(cfg.Name)
		if err != nil && err != nats.ErrStreamNotFound {
			return fmt.Errorf("stream info %s: %w", cfg.Name, err)
		}

		if existing == nil {
			if _, err := c.js.AddStream(cfg); err != nil {
				return fmt.Errorf("add stream %s: %w", cfg.Name, err)
			}
			slog.Info("nats stream created", "stream", cfg.Name, "subjects", cfg.Subjects)
		} else {
			if _, err := c.js.UpdateStream(cfg); err != nil {
				return fmt.Errorf("update stream %s: %w", cfg.Name, err)
			}
			slog.Info("nats stream updated", "stream", cfg.Name, "subjects", cfg.Subjects)
		}
	}
	return nil
}

// Publish serialises payload as JSON and publishes it to the given subject.
// The msgID is used as the Nats-Msg-Id header for server-side deduplication.
func (c *Client) Publish(ctx context.Context, subject, msgID string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  nats.Header{},
	}
	msg.Header.Set(nats.MsgIdHdr, msgID)

	ack, err := c.js.PublishMsg(msg, nats.Context(ctx))
	if err != nil {
		return fmt.Errorf("jetstream publish %s: %w", subject, err)
	}

	slog.DebugContext(ctx, "nats message published",
		"subject", subject,
		"msg_id", msgID,
		"stream", ack.Stream,
		"seq", ack.Sequence,
	)

	return nil
}

// Subscribe creates a durable pull-based consumer on the given stream for the
// specified subject filter. Messages are delivered to handler. The consumer
// uses the durable name for resumption across restarts.
func (c *Client) Subscribe(
	ctx context.Context,
	stream string,
	subject string,
	durable string,
	handler func(msg *nats.Msg),
) (*nats.Subscription, error) {
	sub, err := c.js.Subscribe(
		subject,
		handler,
		nats.Durable(durable),
		nats.ManualAck(),
		nats.AckWait(30*time.Second),
		nats.MaxDeliver(5),
		nats.DeliverNew(),
		nats.BindStream(stream),
		nats.Context(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("subscribe %s/%s: %w", stream, subject, err)
	}

	slog.Info("nats subscription created",
		"stream", stream,
		"subject", subject,
		"durable", durable,
	)

	return sub, nil
}

// SubjectForEvent converts a dot-delimited event type (e.g. "work.submitted")
// into a NATS subject. This is a direct mapping since AEX event types already
// follow the dot-delimited convention.
func SubjectForEvent(eventType string) string {
	return strings.ToLower(eventType)
}

// Close drains the connection and waits for in-flight messages before closing.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	if err := c.conn.Drain(); err != nil {
		slog.Warn("nats drain error, forcing close", "error", err)
		c.conn.Close()
		return err
	}

	slog.Info("nats connection drained and closed")
	return nil
}
