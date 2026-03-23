package nats

import (
	"time"

	"github.com/nats-io/nats.go"
)

// StreamDef describes a JetStream stream and its configuration.
type StreamDef struct {
	// Name is the stream name (e.g. "WORK").
	Name string

	// Subjects lists the subject patterns this stream captures.
	Subjects []string

	// Description is a human-readable summary.
	Description string

	// MaxAge is how long messages are retained.
	MaxAge time.Duration

	// Replicas controls R-factor for the stream. Use 1 for dev, 3 for prod.
	Replicas int
}

// Config converts the definition into a nats.StreamConfig.
func (d StreamDef) Config() *nats.StreamConfig {
	replicas := d.Replicas
	if replicas < 1 {
		replicas = 1
	}

	return &nats.StreamConfig{
		Name:        d.Name,
		Description: d.Description,
		Subjects:    d.Subjects,
		Retention:   nats.LimitsPolicy,
		MaxAge:      d.MaxAge,
		Storage:     nats.FileStorage,
		Replicas:    replicas,
		Discard:     nats.DiscardOld,
		Duplicates:  2 * time.Minute, // dedup window for Nats-Msg-Id
	}
}

// AllStreams returns the full set of AEX JetStream stream definitions.
func AllStreams() []StreamDef {
	return []StreamDef{
		{
			Name:        "WORK",
			Subjects:    []string{"work.>"},
			Description: "Work lifecycle events (submitted, bid_window_closed, cancelled)",
			MaxAge:      30 * 24 * time.Hour, // 30 days
			Replicas:    1,
		},
		{
			Name:        "BID",
			Subjects:    []string{"bid.>", "bids.>"},
			Description: "Bid lifecycle events (submitted, evaluated)",
			MaxAge:      30 * 24 * time.Hour,
			Replicas:    1,
		},
		{
			Name:        "CONTRACT",
			Subjects:    []string{"contract.>"},
			Description: "Contract lifecycle events (awarded, completed, failed)",
			MaxAge:      90 * 24 * time.Hour, // 90 days
			Replicas:    1,
		},
		{
			Name:        "SETTLEMENT",
			Subjects:    []string{"settlement.>"},
			Description: "Settlement events (completed)",
			MaxAge:      90 * 24 * time.Hour,
			Replicas:    1,
		},
		{
			Name:        "TRUST",
			Subjects:    []string{"trust.>", "reputation.>"},
			Description: "Trust and reputation events (score_updated, tier_changed, reputation.updated)",
			MaxAge:      90 * 24 * time.Hour,
			Replicas:    1,
		},
		{
			Name:        "CERTIFICATE",
			Subjects:    []string{"certificate.>", "crl.>"},
			Description: "Certificate lifecycle events (requested, issued, renewed, revoked, expired, crl.updated)",
			MaxAge:      365 * 24 * time.Hour, // 1 year
			Replicas:    1,
		},
		{
			Name:        "DEADLETTER",
			Subjects:    []string{"deadletter.>"},
			Description: "Dead-letter stream for messages that exceeded max delivery attempts",
			MaxAge:      90 * 24 * time.Hour,
			Replicas:    1,
		},
	}
}

// StreamForSubject returns the stream name that owns the given subject prefix.
// Returns an empty string if no stream matches.
func StreamForSubject(subject string) string {
	// Match on the first token of the subject.
	prefix := subject
	for i, c := range subject {
		if c == '.' {
			prefix = subject[:i]
			break
		}
	}

	switch prefix {
	case "work":
		return "WORK"
	case "bid", "bids":
		return "BID"
	case "contract":
		return "CONTRACT"
	case "settlement":
		return "SETTLEMENT"
	case "trust", "reputation":
		return "TRUST"
	case "certificate", "crl":
		return "CERTIFICATE"
	case "deadletter":
		return "DEADLETTER"
	default:
		return ""
	}
}
