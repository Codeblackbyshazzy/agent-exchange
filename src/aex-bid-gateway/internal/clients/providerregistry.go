package clients

import (
	"context"
	"fmt"
	"time"

	"github.com/parlakisik/agent-exchange/internal/httpclient"
)

// ProviderRegistryClient validates provider API keys against the provider registry
type ProviderRegistryClient struct {
	baseURL string
	client  *httpclient.Client
}

// NewProviderRegistryClient creates a new provider registry client
func NewProviderRegistryClient(baseURL string) *ProviderRegistryClient {
	return &ProviderRegistryClient{
		baseURL: baseURL,
		client:  httpclient.NewClient("provider-registry", 5*time.Second),
	}
}

// ValidateAPIKeyResponse is the response from the provider registry
type ValidateAPIKeyResponse struct {
	ProviderID string `json:"provider_id"`
	Valid      bool   `json:"valid"`
	Status     string `json:"status"`
}

// ValidateAPIKey validates an API key against the provider registry
func (c *ProviderRegistryClient) ValidateAPIKey(ctx context.Context, apiKey string) (string, error) {
	var result ValidateAPIKeyResponse

	err := httpclient.NewRequest("GET", c.baseURL).
		Path("/internal/v1/providers/validate-key").
		Query("api_key", apiKey).
		Context(ctx).
		ExecuteJSON(c.client, &result)
	if err != nil {
		return "", err
	}

	if !result.Valid {
		return "", fmt.Errorf("invalid API key")
	}

	return result.ProviderID, nil
}
