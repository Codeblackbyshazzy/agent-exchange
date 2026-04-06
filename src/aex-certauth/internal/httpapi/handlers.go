package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/parlakisik/agent-exchange/aex-certauth/internal/service"
	"github.com/parlakisik/agent-exchange/aex-certauth/internal/store"
)

// Handlers holds the HTTP handler methods for the certauth API.
type Handlers struct {
	certSvc    *service.CertificateService
	repSvc     *service.ReputationService
	verifySvc  *service.VerificationService
	ca         *service.CAEngine
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(certSvc *service.CertificateService, repSvc *service.ReputationService, verifySvc *service.VerificationService, ca *service.CAEngine) *Handlers {
	return &Handlers{
		certSvc:   certSvc,
		repSvc:    repSvc,
		verifySvc: verifySvc,
		ca:        ca,
	}
}

// --- Certificate Endpoints ---

// RequestCertificate handles POST /v1/certificates/request
func (h *Handlers) RequestCertificate(w http.ResponseWriter, r *http.Request) {
	var req service.CreateCertRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	certReq, err := h.certSvc.RequestCertificate(r.Context(), req)
	if err != nil {
		switch err {
		case service.ErrMissingProviderID:
			respondError(w, http.StatusBadRequest, "MISSING_PROVIDER_ID", err.Error())
		case service.ErrMissingClaims:
			respondError(w, http.StatusBadRequest, "MISSING_CLAIMS", err.Error())
		case service.ErrMissingPublicKey:
			respondError(w, http.StatusBadRequest, "MISSING_PUBLIC_KEY", err.Error())
		default:
			slog.ErrorContext(r.Context(), "request certificate failed", "error", err)
			respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		}
		return
	}

	respondJSON(w, http.StatusCreated, certReq)
}

// GetCertificate handles GET /v1/certificates/{cert_id}
func (h *Handlers) GetCertificate(w http.ResponseWriter, r *http.Request, certID string) {
	cert, err := h.certSvc.GetCertificate(r.Context(), certID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "certificate not found")
			return
		}
		slog.ErrorContext(r.Context(), "get certificate failed", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	respondJSON(w, http.StatusOK, cert)
}

// RenewCertificate handles POST /v1/certificates/{cert_id}/renew
func (h *Handlers) RenewCertificate(w http.ResponseWriter, r *http.Request, certID string) {
	cert, err := h.certSvc.RenewCertificate(r.Context(), certID)
	if err != nil {
		switch err {
		case service.ErrCertNotActive:
			respondError(w, http.StatusConflict, "CERT_NOT_ACTIVE", err.Error())
		case service.ErrCertificateNotFound:
			respondError(w, http.StatusNotFound, "NOT_FOUND", "certificate not found")
		default:
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusNotFound, "NOT_FOUND", "certificate not found")
				return
			}
			slog.ErrorContext(r.Context(), "renew certificate failed", "error", err)
			respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		}
		return
	}

	respondJSON(w, http.StatusOK, cert)
}

// RevokeCertificate handles DELETE /v1/certificates/{cert_id}
func (h *Handlers) RevokeCertificate(w http.ResponseWriter, r *http.Request, certID string) {
	var req struct {
		Reason string `json:"reason"`
	}
	// Body is optional for DELETE; default reason if empty.
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req)
	if req.Reason == "" {
		req.Reason = "revoked via API"
	}

	err := h.certSvc.RevokeCertificate(r.Context(), certID, req.Reason)
	if err != nil {
		switch err {
		case service.ErrAlreadyRevoked:
			respondError(w, http.StatusConflict, "ALREADY_REVOKED", err.Error())
		default:
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusNotFound, "NOT_FOUND", "certificate not found")
				return
			}
			slog.ErrorContext(r.Context(), "revoke certificate failed", "error", err)
			respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		}
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// VerifyCertificate handles POST /v1/certificates/verify
func (h *Handlers) VerifyCertificate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	var req struct {
		CertificateID string `json:"certificate_id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.CertificateID == "" {
		respondError(w, http.StatusBadRequest, "CERTIFICATE_ID_REQUIRED", "certificate_id is required")
		return
	}

	result, err := h.verifySvc.VerifyCertificate(r.Context(), req.CertificateID)
	if err != nil {
		if errors.Is(err, service.ErrCertificateNotFound) {
			respondJSON(w, http.StatusOK, map[string]any{
				"valid":          false,
				"certificate_id": req.CertificateID,
				"reason":         "certificate not found",
			})
			return
		}
		slog.ErrorContext(r.Context(), "verify certificate failed", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	reason := ""
	if !result.Valid && len(result.Errors) > 0 {
		reason = result.Errors[0]
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"valid":          result.Valid,
		"certificate_id": result.Certificate.CertificateID,
		"provider_id":    result.Certificate.ProviderID,
		"status":         result.Certificate.Status,
		"not_before":     result.Certificate.NotBefore,
		"not_after":      result.Certificate.NotAfter,
		"reason":         reason,
		"errors":         result.Errors,
	})
}

// SearchCertificates handles GET /v1/certificates/search
func (h *Handlers) SearchCertificates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	q := r.URL.Query()
	filters := service.SearchFilters{
		ProviderID:      q.Get("provider_id"),
		TenantID:        q.Get("tenant_id"),
		CertificateType: q.Get("certificate_type"),
		Status:          q.Get("status"),
		Category:        q.Get("category"),
		Capability:      q.Get("capability"),
		Limit:           parseIntDefault(q.Get("limit"), 50),
		Offset:          parseIntDefault(q.Get("offset"), 0),
	}

	certs, err := h.certSvc.SearchCertificates(r.Context(), filters)
	if err != nil {
		slog.ErrorContext(r.Context(), "search certificates failed", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"certificates": certs,
		"total":        len(certs),
	})
}

// GetProviderCertificates handles GET /v1/providers/{id}/certificates
func (h *Handlers) GetProviderCertificates(w http.ResponseWriter, r *http.Request, providerID string) {
	certs, err := h.certSvc.GetProviderCertificates(r.Context(), providerID)
	if err != nil {
		slog.ErrorContext(r.Context(), "get provider certificates failed", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"provider_id":  providerID,
		"certificates": certs,
		"total":        len(certs),
	})
}

// --- Reputation Endpoints ---

// GetProviderReputation handles GET /v1/providers/{id}/reputation
func (h *Handlers) GetProviderReputation(w http.ResponseWriter, r *http.Request, providerID string) {
	rep, err := h.repSvc.GetReputation(r.Context(), providerID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Attempt a fresh calculation if no cached reputation exists.
			rep, err = h.repSvc.CalculateReputation(r.Context(), providerID)
			if err != nil {
				slog.ErrorContext(r.Context(), "calculate reputation failed", "error", err)
				respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
				return
			}
		} else {
			slog.ErrorContext(r.Context(), "get reputation failed", "error", err)
			respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
			return
		}
	}

	respondJSON(w, http.StatusOK, rep)
}

// GetLeaderboard handles GET /v1/reputation/leaderboard
func (h *Handlers) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	q := r.URL.Query()
	limit := parseIntDefault(q.Get("limit"), 20)
	offset := parseIntDefault(q.Get("offset"), 0)

	scores, err := h.repSvc.GetLeaderboard(r.Context(), limit, offset)
	if err != nil {
		slog.ErrorContext(r.Context(), "get leaderboard failed", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"leaderboard": scores,
		"total":       len(scores),
		"limit":       limit,
		"offset":      offset,
	})
}

// --- CRL Endpoint ---

// GetCRL handles GET /v1/crl
func (h *Handlers) GetCRL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	crl, err := h.verifySvc.GetCRL(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "get CRL failed", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	respondJSON(w, http.StatusOK, crl)
}

// --- Internal Endpoints ---

// BatchVerify handles POST /internal/v1/certificates/batch-verify
func (h *Handlers) BatchVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	var req struct {
		CertificateIDs []string `json:"certificate_ids"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if len(req.CertificateIDs) == 0 {
		respondError(w, http.StatusBadRequest, "MISSING_CERTIFICATE_IDS", "certificate_ids is required")
		return
	}

	verifyResults, err := h.verifySvc.BatchVerify(r.Context(), req.CertificateIDs)
	if err != nil {
		slog.ErrorContext(r.Context(), "batch verify failed", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	results := make(map[string]any, len(req.CertificateIDs))
	for i, certID := range req.CertificateIDs {
		if i >= len(verifyResults) {
			break
		}
		vr := verifyResults[i]
		entry := map[string]any{
			"valid":       vr.Valid,
			"provider_id": vr.Certificate.ProviderID,
			"status":      vr.Certificate.Status,
			"not_after":   vr.Certificate.NotAfter,
		}
		if !vr.Valid && len(vr.Errors) > 0 {
			entry["reason"] = vr.Errors[0]
		}
		results[certID] = entry
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"results": results,
	})
}

// CanPerform handles GET /internal/v1/providers/{id}/can-perform
func (h *Handlers) CanPerform(w http.ResponseWriter, r *http.Request, providerID string) {
	capability := r.URL.Query().Get("capability")
	category := r.URL.Query().Get("category")

	if capability == "" {
		respondError(w, http.StatusBadRequest, "MISSING_CAPABILITY", "capability query parameter is required")
		return
	}

	canPerform, err := h.verifySvc.CanPerform(r.Context(), providerID, category, capability)
	if err != nil {
		slog.ErrorContext(r.Context(), "can-perform lookup failed", "error", err)
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"provider_id": providerID,
		"capability":  capability,
		"category":    category,
		"can_perform": canPerform,
	})
}

// ApproveCertificate handles POST /internal/v1/certificates/{id}/approve
func (h *Handlers) ApproveCertificate(w http.ResponseWriter, r *http.Request, requestID string) {
	var req struct {
		ReviewedBy string `json:"reviewed_by"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req)

	if req.ReviewedBy == "" {
		req.ReviewedBy = "admin"
	}

	cert, err := h.certSvc.ApproveCertificateRequest(r.Context(), requestID, req.ReviewedBy)
	if err != nil {
		switch err {
		case service.ErrRequestNotPending:
			respondError(w, http.StatusConflict, "REQUEST_NOT_PENDING", err.Error())
		default:
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusNotFound, "NOT_FOUND", "certificate request not found")
				return
			}
			slog.ErrorContext(r.Context(), "approve certificate failed", "error", err)
			respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		}
		return
	}

	respondJSON(w, http.StatusOK, cert)
}

// RejectCertificate handles POST /internal/v1/certificates/{id}/reject
func (h *Handlers) RejectCertificate(w http.ResponseWriter, r *http.Request, requestID string) {
	var req struct {
		ReviewedBy string `json:"reviewed_by"`
		Reason     string `json:"reason"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	if req.ReviewedBy == "" {
		req.ReviewedBy = "admin"
	}

	err := h.certSvc.RejectCertificateRequest(r.Context(), requestID, req.ReviewedBy, req.Reason)
	if err != nil {
		switch err {
		case service.ErrRequestNotPending:
			respondError(w, http.StatusConflict, "REQUEST_NOT_PENDING", err.Error())
		default:
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusNotFound, "NOT_FOUND", "certificate request not found")
				return
			}
			slog.ErrorContext(r.Context(), "reject certificate failed", "error", err)
			respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		}
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

// --- Well-known Endpoints ---

// GetCAPublicKey handles GET /.well-known/aex-ca.json
func (h *Handlers) GetCAPublicKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	publicKeyPEM := h.ca.GetCAPublicKeyPEM()
	certPEM := h.ca.GetCACertificatePEM()

	respondJSON(w, http.StatusOK, map[string]any{
		"issuer":         "aex-certauth",
		"algorithm":      "ECDSA-P256-SHA256",
		"public_key_pem": publicKeyPEM,
		"certificate":    certPEM,
	})
}

// Health handles GET /health
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

// --- Helpers ---

func respondJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
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

func parseIntDefault(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defaultVal
	}
	return v
}
