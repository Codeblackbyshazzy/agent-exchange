package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/parlakisik/agent-exchange/aex-contract-engine/internal/clients"
	"github.com/parlakisik/agent-exchange/aex-contract-engine/internal/model"
	"github.com/parlakisik/agent-exchange/aex-contract-engine/internal/store"
)

type Service struct {
	store store.ContractStore
	bg    *clients.BidGatewayClient
}

func New(store store.ContractStore, bidGatewayURL string) (*Service, error) {
	if strings.TrimSpace(bidGatewayURL) == "" {
		return nil, errors.New("BID_GATEWAY_URL is required")
	}
	return &Service{
		store: store,
		bg:    clients.NewBidGatewayClient(bidGatewayURL),
	}, nil
}

func (s *Service) HandleAward(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workID := pathParam(r.URL.Path, "/v1/work/", "/award")
	if workID == "" {
		respondError(w, http.StatusBadRequest, "WORK_ID_REQUIRED", "work_id is required")
		return
	}

	var req model.AwardRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "bad request")
		return
	}

	bids, err := s.bg.ListBids(ctx, workID)
	if err != nil {
		respondError(w, http.StatusBadGateway, "BAD_GATEWAY", "failed to fetch bids")
		return
	}

	now := time.Now().UTC()
	var chosen *clients.Bid
	if req.AutoAward {
		// Simplest policy for local use: choose the lowest price among unexpired bids.
		for i := range bids {
			if bids[i].ExpiresAt.Before(now) {
				continue
			}
			if chosen == nil || bids[i].Price < chosen.Price {
				chosen = &bids[i]
			}
		}
		if chosen == nil {
			respondError(w, http.StatusBadRequest, "NO_VALID_BIDS", "no valid bids to award")
			return
		}
		req.BidID = chosen.BidID
	} else {
		for i := range bids {
			if bids[i].BidID == req.BidID {
				chosen = &bids[i]
				break
			}
		}
		if chosen == nil {
			respondError(w, http.StatusBadRequest, "INVALID_BID_ID", "invalid bid_id")
			return
		}
		if chosen.ExpiresAt.Before(now) {
			respondError(w, http.StatusConflict, "BID_EXPIRED", "bid expired")
			return
		}
	}

	contractID := generateID("contract_")
	execToken := generateID("exec_")
	consumerToken := generateID("cons_")
	expiresAt := now.Add(1 * time.Hour)

	// ConsumerID is unknown until identity/gateway integration; keep placeholder.
	contract := model.Contract{
		ContractID:       contractID,
		WorkID:           workID,
		ConsumerID:       "unknown",
		ProviderID:       chosen.ProviderID,
		BidID:            chosen.BidID,
		AgreedPrice:      chosen.Price,
		SLA:              model.SLACommitment{},
		ProviderEndpoint: chosen.A2AEndpoint,
		ExecutionToken:   execToken,
		ConsumerToken:    consumerToken,
		Status:           model.ContractStatusAwarded,
		ExpiresAt:        expiresAt,
		AwardedAt:        now,
	}

	if err := s.store.Save(ctx, contract); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save contract")
		return
	}

	resp := model.AwardResponse{
		ContractID:       contract.ContractID,
		WorkID:           contract.WorkID,
		ProviderID:       contract.ProviderID,
		AgreedPrice:      contract.AgreedPrice,
		Status:           contract.Status,
		ProviderEndpoint: contract.ProviderEndpoint,
		ExecutionToken:   contract.ExecutionToken,
		ExpiresAt:        contract.ExpiresAt,
		AwardedAt:        contract.AwardedAt,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Service) HandleGetContract(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	contractID := pathParam(r.URL.Path, "/v1/contracts/", "")
	if contractID == "" {
		respondError(w, http.StatusBadRequest, "CONTRACT_ID_REQUIRED", "contract_id is required")
		return
	}
	c, err := s.store.Get(ctx, contractID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if c == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Service) HandleProgress(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	contractID := pathParam(r.URL.Path, "/v1/contracts/", "/progress")
	if contractID == "" {
		respondError(w, http.StatusBadRequest, "CONTRACT_ID_REQUIRED", "contract_id is required")
		return
	}
	token := bearerToken(r)
	if token == "" {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	var req model.ProgressRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "bad request")
		return
	}

	c, err := s.store.Get(ctx, contractID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if c == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	if c.ExecutionToken != token {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}

	now := time.Now().UTC()
	if now.After(c.ExpiresAt) {
		c.Status = model.ContractStatusExpired
		_ = s.store.Update(ctx, *c)
		respondError(w, http.StatusGone, "CONTRACT_EXPIRED", "contract expired")
		return
	}
	c.ExecutionUpdates = append(c.ExecutionUpdates, model.ExecutionUpdate{
		Status:    req.Status,
		Percent:   req.Percent,
		Message:   req.Message,
		Timestamp: now,
	})
	if c.Status == model.ContractStatusAwarded {
		c.Status = model.ContractStatusExecuting
		c.StartedAt = &now
	}
	if err := s.store.Update(ctx, *c); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"acknowledged": true, "contract_id": contractID})
}

func (s *Service) HandleComplete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	contractID := pathParam(r.URL.Path, "/v1/contracts/", "/complete")
	if contractID == "" {
		respondError(w, http.StatusBadRequest, "CONTRACT_ID_REQUIRED", "contract_id is required")
		return
	}
	token := bearerToken(r)
	if token == "" {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	var req model.CompleteRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "bad request")
		return
	}

	c, err := s.store.Get(ctx, contractID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if c == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	if c.ExecutionToken != token {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}

	now := time.Now().UTC()
	if now.After(c.ExpiresAt) {
		c.Status = model.ContractStatusExpired
		_ = s.store.Update(ctx, *c)
		respondError(w, http.StatusGone, "CONTRACT_EXPIRED", "contract expired")
		return
	}
	c.Status = model.ContractStatusCompleted
	c.CompletedAt = &now
	c.Outcome = &model.OutcomeReport{
		Success:        req.Success,
		ResultSummary:  req.ResultSummary,
		Metrics:        req.Metrics,
		ResultLocation: req.ResultLocation,
		ReportedAt:     now,
	}
	if err := s.store.Update(ctx, *c); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"contract_id":  contractID,
		"status":       c.Status,
		"completed_at": now,
	})
}

func (s *Service) HandleFail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	contractID := pathParam(r.URL.Path, "/v1/contracts/", "/fail")
	if contractID == "" {
		respondError(w, http.StatusBadRequest, "CONTRACT_ID_REQUIRED", "contract_id is required")
		return
	}
	// For local: allow either execution token or consumer token; both are Bearer.
	token := bearerToken(r)
	if token == "" {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	var req model.FailRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "bad request")
		return
	}

	c, err := s.store.Get(ctx, contractID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	if c == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "not found")
		return
	}
	if c.ExecutionToken != token && c.ConsumerToken != token {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}

	now := time.Now().UTC()
	if now.After(c.ExpiresAt) {
		c.Status = model.ContractStatusExpired
		_ = s.store.Update(ctx, *c)
		respondError(w, http.StatusGone, "CONTRACT_EXPIRED", "contract expired")
		return
	}
	c.Status = model.ContractStatusFailed
	c.FailedAt = &now
	c.FailureReason = &req.Reason
	if err := s.store.Update(ctx, *c); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"contract_id":    contractID,
		"status":         c.Status,
		"failure_reason": req.Reason,
		"failed_at":      now,
	})
}

func decodeJSON(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	defer func() { _ = r.Body.Close() }()
	return json.Unmarshal(body, v)
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

func bearerToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
}

func generateID(prefix string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:8])
}

func pathParam(path string, prefix string, suffix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	if suffix != "" {
		if !strings.HasSuffix(rest, suffix) {
			return ""
		}
		rest = strings.TrimSuffix(rest, suffix)
	}
	rest = strings.Trim(rest, "/")
	// take first segment
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	return strings.TrimSpace(rest)
}
