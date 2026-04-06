package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/model"
	"github.com/parlakisik/agent-exchange/aex-work-publisher/internal/service"
)

type Handlers struct {
	svc *service.Service
}

func NewHandlers(svc *service.Service) *Handlers {
	return &Handlers{svc: svc}
}

// HandleSubmitWork handles POST /v1/work
func (h *Handlers) HandleSubmitWork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// In production, extract from JWT token
	// For now, use header or query param
	consumerID := r.Header.Get("X-Consumer-ID")
	if consumerID == "" {
		consumerID = "default_consumer" // TODO: Replace with actual auth
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "failed to read request")
		return
	}
	defer func() { _ = r.Body.Close() }()

	var req model.WorkSubmission
	if err := json.Unmarshal(body, &req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	resp, err := h.svc.PublishWork(ctx, consumerID, req)
	if err != nil {
		slog.ErrorContext(ctx, "failed to publish work", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to publish work")
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

// HandleGetWork handles GET /v1/work/{work_id}
func (h *Handlers) HandleGetWork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workID := extractWorkID(r.URL.Path)
	if workID == "" {
		respondError(w, http.StatusBadRequest, "WORK_ID_REQUIRED", "work_id is required")
		return
	}

	work, err := h.svc.GetWork(ctx, workID)
	if err != nil {
		if err == service.ErrWorkNotFound {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "work not found")
			return
		}
		slog.ErrorContext(ctx, "failed to get work", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get work")
		return
	}

	writeJSON(w, http.StatusOK, work)
}

// HandleCancelWork handles POST /v1/work/{work_id}/cancel
func (h *Handlers) HandleCancelWork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	consumerID := r.Header.Get("X-Consumer-ID")
	if consumerID == "" {
		consumerID = "default_consumer" // TODO: Replace with actual auth
	}

	workID := extractWorkID(r.URL.Path)
	if workID == "" {
		respondError(w, http.StatusBadRequest, "WORK_ID_REQUIRED", "work_id is required")
		return
	}

	work, err := h.svc.CancelWork(ctx, workID, consumerID)
	if err != nil {
		if err == service.ErrWorkNotFound {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "work not found")
			return
		}
		slog.ErrorContext(ctx, "failed to cancel work", "error", err)
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, work)
}

// HandleBidSubmitted handles POST /internal/work/{work_id}/bids (internal endpoint)
func (h *Handlers) HandleBidSubmitted(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workID := extractWorkID(r.URL.Path)
	if workID == "" {
		respondError(w, http.StatusBadRequest, "WORK_ID_REQUIRED", "work_id is required")
		return
	}

	var req struct {
		BidID string `json:"bid_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if err := h.svc.OnBidSubmitted(ctx, workID, req.BidID); err != nil {
		slog.ErrorContext(ctx, "failed to record bid", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to record bid")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleCloseBidWindow handles POST /internal/work/{work_id}/close-bids (internal endpoint)
func (h *Handlers) HandleCloseBidWindow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workID := extractWorkID(r.URL.Path)
	if workID == "" {
		respondError(w, http.StatusBadRequest, "WORK_ID_REQUIRED", "work_id is required")
		return
	}

	if err := h.svc.CloseBidWindow(ctx, workID); err != nil {
		slog.ErrorContext(ctx, "failed to close bid window", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to close bid window")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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

func extractWorkID(path string) string {
	// Extract work_id from paths like:
	// /v1/work/{work_id}
	// /v1/work/{work_id}/cancel
	// /internal/work/{work_id}/bids
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 3 {
		return ""
	}
	// parts[0] = "v1" or "internal"
	// parts[1] = "work"
	// parts[2] = work_id
	return parts[2]
}
