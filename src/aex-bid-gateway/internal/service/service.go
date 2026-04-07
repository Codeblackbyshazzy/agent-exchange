package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/parlakisik/agent-exchange/aex-bid-gateway/internal/clients"
	"github.com/parlakisik/agent-exchange/aex-bid-gateway/internal/model"
	"github.com/parlakisik/agent-exchange/aex-bid-gateway/internal/store"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrBadRequest   = errors.New("bad_request")
	ErrInvalidBid   = errors.New("invalid_bid")
)

// ProviderKeyValidator validates provider API keys
type ProviderKeyValidator interface {
	ValidateAPIKey(ctx context.Context, apiKey string) (string, error)
}

type Service struct {
	store store.BidStore

	// Static fallback: apiKey -> providerID
	providerKeys map[string]string

	// Dynamic validation via provider registry
	providerRegistry ProviderKeyValidator
}

func New(store store.BidStore, providerKeys map[string]string) *Service {
	return &Service{
		store:        store,
		providerKeys: providerKeys,
	}
}

// NewWithProviderRegistry creates a service that validates API keys against the provider registry
func NewWithProviderRegistry(store store.BidStore, providerRegistryURL string) *Service {
	return &Service{
		store:            store,
		providerKeys:     map[string]string{},
		providerRegistry: clients.NewProviderRegistryClient(providerRegistryURL),
	}
}

func (s *Service) HandleSubmitBid(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	providerID, err := s.validateProviderAuth(r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Bad request")
		return
	}
	defer func() { _ = r.Body.Close() }()

	var req model.SubmitBidRequest
	if err := json.Unmarshal(body, &req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid bid format")
		return
	}

	now := time.Now().UTC()
	bid := model.BidPacket{
		BidID:            generateBidID(),
		WorkID:           req.WorkID,
		ProviderID:       providerID,
		Price:            req.Price,
		PriceBreakdown:   req.PriceBreakdown,
		Confidence:       req.Confidence,
		Approach:         req.Approach,
		EstimatedLatency: req.EstimatedLatency,
		MVPSample:        req.MVPSample,
		SLA:              req.SLA,
		A2AEndpoint:      req.A2AEndpoint,
		ExpiresAt:        req.ExpiresAt,
		ReceivedAt:       now,
	}

	if err := validateBid(now, bid); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_BID", err.Error())
		return
	}

	if err := s.store.Save(ctx, bid); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to store bid")
		return
	}

	resp := model.SubmitBidResponse{
		BidID:      bid.BidID,
		WorkID:     bid.WorkID,
		Status:     "RECEIVED",
		ReceivedAt: bid.ReceivedAt,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Service) HandleInternalListBids(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workID := strings.TrimSpace(r.URL.Query().Get("work_id"))
	if workID == "" {
		respondError(w, http.StatusBadRequest, "WORK_ID_REQUIRED", "work_id is required")
		return
	}

	limit, offset := parsePagination(r)
	bids, err := s.store.ListByWorkID(ctx, workID, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load bids")
		return
	}

	out := map[string]any{
		"work_id":    workID,
		"bids":       bids,
		"total_bids": len(bids),
		"limit":      limit,
		"offset":     offset,
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Service) validateProviderAuth(r *http.Request) (string, error) {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", ErrUnauthorized
	}
	apiKey := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if apiKey == "" {
		return "", ErrUnauthorized
	}

	// First, try static keys (for backwards compatibility/testing)
	if providerID, ok := s.providerKeys[apiKey]; ok && providerID != "" {
		return providerID, nil
	}

	// If provider registry client is configured, validate dynamically
	if s.providerRegistry != nil {
		providerID, err := s.providerRegistry.ValidateAPIKey(r.Context(), apiKey)
		if err == nil && providerID != "" {
			return providerID, nil
		}
	}

	return "", ErrUnauthorized
}

func validateBid(now time.Time, bid model.BidPacket) error {
	if bid.WorkID == "" || bid.Price <= 0 || bid.A2AEndpoint == "" {
		return errors.New("missing required fields")
	}
	if err := validateEndpointURL(bid.A2AEndpoint); err != nil {
		return fmt.Errorf("invalid A2AEndpoint: %w", err)
	}
	if bid.Confidence < 0 || bid.Confidence > 1 {
		return errors.New("confidence must be between 0 and 1")
	}
	if bid.ExpiresAt.IsZero() {
		return errors.New("expires_at is required")
	}
	if bid.ExpiresAt.Before(now) {
		return errors.New("bid already expired")
	}
	return nil
}

// validateEndpointURL checks that the A2AEndpoint is a valid HTTPS URL and
// does not point to private/internal IP ranges (SSRF prevention).
func validateEndpointURL(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return errors.New("malformed URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("scheme must be http or https")
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("missing host")
	}
	// Block localhost variants
	lower := strings.ToLower(host)
	if lower == "localhost" || lower == "127.0.0.1" || lower == "::1" || lower == "0.0.0.0" {
		return errors.New("localhost endpoints not allowed")
	}
	// Block cloud metadata endpoints
	if lower == "169.254.169.254" || lower == "metadata.google.internal" {
		return errors.New("metadata endpoints not allowed")
	}
	// Block private IP ranges
	ip := net.ParseIP(host)
	if ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
		return errors.New("private IP addresses not allowed")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func respondError(w http.ResponseWriter, statusCode int, code string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":      code,
			"message":   message,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	})
}

func parsePagination(r *http.Request) (limit, offset int) {
	limit = 100
	offset = 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

func generateBidID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "bid_" + hex.EncodeToString(b[:8])
}
