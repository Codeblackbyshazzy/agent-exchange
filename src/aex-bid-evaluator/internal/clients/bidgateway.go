package clients

import (
	"context"
	"time"

	"github.com/parlakisik/agent-exchange/aex-bid-evaluator/internal/model"
	"github.com/parlakisik/agent-exchange/internal/httpclient"
)

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

func (c *BidGatewayClient) GetBids(ctx context.Context, workID string) ([]model.BidPacket, error) {
	var out struct {
		WorkID    string            `json:"work_id"`
		Bids      []model.BidPacket `json:"bids"`
		TotalBids int               `json:"total_bids"`
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
