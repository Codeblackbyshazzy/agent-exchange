package clients

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/parlakisik/agent-exchange/internal/httpclient"
)

type CertAuthClient struct {
	baseURL string
	client  *httpclient.Client
}

func NewCertAuthClient(baseURL string) *CertAuthClient {
	return &CertAuthClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  httpclient.NewClient("certauth", 10*time.Second),
	}
}

func (c *CertAuthClient) GetCertScore(ctx context.Context, providerID string) (float64, error) {
	if c.baseURL == "" {
		return 0, fmt.Errorf("certauth base URL is not configured")
	}

	var out struct {
		OverallScore float64 `json:"overall_score"`
	}

	err := httpclient.NewRequest("GET", c.baseURL).
		Path("/v1/providers/" + providerID + "/reputation").
		Context(ctx).
		ExecuteJSON(c.client, &out)
	if err != nil {
		return 0, fmt.Errorf("certauth request for provider %s: %w", providerID, err)
	}

	return out.OverallScore, nil
}
