package clients

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/parlakisik/agent-exchange/internal/httpclient"
)

type TrustBrokerClient struct {
	baseURL string
	client  *httpclient.Client
}

func NewTrustBrokerClient(baseURL string) *TrustBrokerClient {
	return &TrustBrokerClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  httpclient.NewClient("trust-broker", 10*time.Second),
	}
}

func (c *TrustBrokerClient) GetScore(ctx context.Context, providerID string) (float64, error) {
	if c.baseURL == "" {
		return 0, fmt.Errorf("trust-broker base URL is not configured")
	}

	var out struct {
		TrustScore float64 `json:"trust_score"`
	}

	err := httpclient.NewRequest("GET", c.baseURL).
		Path("/v1/providers/" + providerID + "/trust").
		Context(ctx).
		ExecuteJSON(c.client, &out)
	if err != nil {
		return 0, fmt.Errorf("trust-broker request for provider %s: %w", providerID, err)
	}

	return out.TrustScore, nil
}
