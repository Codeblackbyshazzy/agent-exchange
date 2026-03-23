package clients

import (
	"context"
	"time"

	"github.com/parlakisik/agent-exchange/internal/httpclient"
)

type Bid struct {
	BidID       string    `json:"bid_id"`
	WorkID      string    `json:"work_id"`
	ProviderID  string    `json:"provider_id"`
	Price       float64   `json:"price"`
	A2AEndpoint string    `json:"a2a_endpoint"`
	ExpiresAt   time.Time `json:"expires_at"`
	ReceivedAt  time.Time `json:"received_at"`
	SLA         any       `json:"sla"`
}

type BidGatewayClient struct {
	baseURL string
	client  *httpclient.Client
}

func NewBidGatewayClient(baseURL string) *BidGatewayClient {
	return &BidGatewayClient{
		baseURL: baseURL,
		client:  httpclient.NewClient("bid-gateway", 10*time.Second),
	}
}

func (c *BidGatewayClient) ListBids(ctx context.Context, workID string) ([]Bid, error) {
	var out struct {
		Bids []Bid `json:"bids"`
	}

	err := httpclient.NewRequest("GET", c.baseURL).
		Path("/internal/v1/bids").
		Query("work_id", workID).
		Context(ctx).
		ExecuteJSON(c.client, &out)
	if err != nil {
		return nil, err
	}

	return out.Bids, nil
}
