package clients

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/parlakisik/agent-exchange/internal/httpclient"
)

// TrustData contains trust and transaction data fetched from the trust-broker
// service, used as input for the reputation scoring formula.
type TrustData struct {
	ProviderID          string                    `json:"provider_id"`
	TotalContracts      int64                     `json:"total_contracts"`
	SuccessfulContracts int64                     `json:"successful_contracts"`
	FailedContracts     int64                     `json:"failed_contracts"`
	DisputedContracts   int64                     `json:"disputed_contracts"`
	AverageScore        float64                   `json:"average_score"`
	CategoryBreakdowns  map[string]CategoryDetail `json:"category_breakdowns"`
}

// CategoryDetail represents per-category trust breakdown from the trust-broker.
type CategoryDetail struct {
	Category            string  `json:"category"`
	TotalContracts      int64   `json:"total_contracts"`
	SuccessfulContracts int64   `json:"successful_contracts"`
	SuccessRate         float64 `json:"success_rate"`
	AverageRating       float64 `json:"average_rating"`
}

// TrustBrokerClient communicates with the trust-broker service to fetch
// trust records and transaction data for providers.
type TrustBrokerClient struct {
	baseURL string
	client  *httpclient.Client
}

// NewTrustBrokerClient creates a new trust-broker client.
func NewTrustBrokerClient(baseURL string) *TrustBrokerClient {
	return &TrustBrokerClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  httpclient.NewClient("trust-broker", 10*time.Second),
	}
}

// GetTrustRecord fetches the trust record for a provider, including contract
// counts, average score, and per-category breakdowns.
func (c *TrustBrokerClient) GetTrustRecord(ctx context.Context, providerID string) (TrustData, error) {
	if c.baseURL == "" {
		return TrustData{}, fmt.Errorf("trust-broker base URL is not configured")
	}

	var out struct {
		ProviderID          string                    `json:"provider_id"`
		TrustScore          float64                   `json:"trust_score"`
		TotalContracts      int64                     `json:"total_contracts"`
		SuccessfulContracts int64                     `json:"successful_contracts"`
		FailedContracts     int64                     `json:"failed_contracts"`
		DisputedContracts   int64                     `json:"disputed_contracts"`
		CategoryBreakdowns  map[string]CategoryDetail `json:"category_breakdowns"`
	}

	err := httpclient.NewRequest("GET", c.baseURL).
		Path("/v1/providers/" + providerID + "/trust").
		Context(ctx).
		ExecuteJSON(c.client, &out)
	if err != nil {
		return TrustData{}, fmt.Errorf("trust-broker request for provider %s: %w", providerID, err)
	}

	return TrustData{
		ProviderID:          out.ProviderID,
		TotalContracts:      out.TotalContracts,
		SuccessfulContracts: out.SuccessfulContracts,
		FailedContracts:     out.FailedContracts,
		DisputedContracts:   out.DisputedContracts,
		AverageScore:        out.TrustScore,
		CategoryBreakdowns:  out.CategoryBreakdowns,
	}, nil
}
