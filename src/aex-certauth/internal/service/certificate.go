package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/parlakisik/agent-exchange/aex-certauth/internal/config"
	"github.com/parlakisik/agent-exchange/aex-certauth/internal/model"
	"github.com/parlakisik/agent-exchange/aex-certauth/internal/store"
	"github.com/parlakisik/agent-exchange/internal/events"
)

var (
	ErrMissingProviderID  = errors.New("provider_id is required")
	ErrMissingClaims      = errors.New("at least one capability claim is required")
	ErrMissingPublicKey   = errors.New("public_key_pem is required")
	ErrRequestNotPending  = errors.New("certificate request is not in PENDING status")
	ErrCertificateNotFound = errors.New("certificate not found")
	ErrCertNotActive      = errors.New("certificate is not in ACTIVE status")
	ErrAlreadyRevoked     = errors.New("certificate is already revoked")
)

// CreateCertRequest holds the parameters for requesting a new certificate.
type CreateCertRequest struct {
	TenantID        string                  `json:"tenant_id"`
	ProviderID      string                  `json:"provider_id"`
	AgentName       string                  `json:"agent_name"`
	CertificateType string                  `json:"certificate_type"`
	Claims          []model.CapabilityClaim `json:"claims"`
	PublicKeyPEM    string                  `json:"public_key_pem"`
}

// SearchFilters defines filter criteria passed through the API layer.
type SearchFilters struct {
	ProviderID      string `json:"provider_id,omitempty"`
	TenantID        string `json:"tenant_id,omitempty"`
	CertificateType string `json:"certificate_type,omitempty"`
	Status          string `json:"status,omitempty"`
	Category        string `json:"category,omitempty"`
	Capability      string `json:"capability,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	Offset          int    `json:"offset,omitempty"`
}

// CertificateService handles the certificate lifecycle: request, approve,
// reject, renew, and revoke.
type CertificateService struct {
	store     store.CertAuthStore
	ca        *CAEngine
	publisher *events.Publisher
	config    *config.Config
}

// NewCertificateService creates a new CertificateService.
func NewCertificateService(
	st store.CertAuthStore,
	ca *CAEngine,
	publisher *events.Publisher,
	cfg *config.Config,
) *CertificateService {
	return &CertificateService{
		store:     st,
		ca:        ca,
		publisher: publisher,
		config:    cfg,
	}
}

// RequestCertificate validates and stores a certificate signing request. The
// request is stored in PENDING status and must be approved before a certificate
// is issued.
func (s *CertificateService) RequestCertificate(ctx context.Context, req CreateCertRequest) (model.CertificateRequest, error) {
	// Validate required fields.
	if req.ProviderID == "" {
		return model.CertificateRequest{}, ErrMissingProviderID
	}
	if len(req.Claims) == 0 {
		return model.CertificateRequest{}, ErrMissingClaims
	}
	if req.PublicKeyPEM == "" {
		return model.CertificateRequest{}, ErrMissingPublicKey
	}

	// Default certificate type.
	certType := req.CertificateType
	if certType == "" {
		certType = model.CertTypeCapability
	}

	now := time.Now().UTC()
	certReq := model.CertificateRequest{
		RequestID:    generateID("req"),
		TenantID:     req.TenantID,
		ProviderID:   req.ProviderID,
		AgentName:    req.AgentName,
		CertType:     certType,
		Claims:       req.Claims,
		PublicKeyPEM: req.PublicKeyPEM,
		Status:       model.CertStatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.store.CreateCertificateRequest(ctx, certReq); err != nil {
		return model.CertificateRequest{}, fmt.Errorf("store certificate request: %w", err)
	}

	// Publish event.
	_ = s.publisher.Publish(ctx, events.EventCertificateRequested, map[string]any{
		"certificate_id": certReq.RequestID,
		"provider_id":    certReq.ProviderID,
		"agent_id":       certReq.AgentName,
		"domain":         primaryCategory(certReq.Claims),
		"requested_at":   now,
	})

	slog.InfoContext(ctx, "certificate_requested",
		"request_id", certReq.RequestID,
		"provider_id", certReq.ProviderID,
		"agent_name", certReq.AgentName,
		"claims", len(certReq.Claims),
	)

	return certReq, nil
}

// ApproveCertificateRequest approves a pending request and issues a signed
// agent certificate. The certificate is signed with the CA engine and stored
// with status ACTIVE.
func (s *CertificateService) ApproveCertificateRequest(ctx context.Context, requestID, reviewedBy string) (model.AgentCertificate, error) {
	certReq, err := s.store.GetCertificateRequest(ctx, requestID)
	if err != nil {
		return model.AgentCertificate{}, fmt.Errorf("get certificate request: %w", err)
	}

	if certReq.Status != model.CertStatusPending {
		return model.AgentCertificate{}, ErrRequestNotPending
	}

	now := time.Now().UTC()
	notBefore := now
	notAfter := now.Add(time.Duration(s.config.CertValidityDays) * 24 * time.Hour)

	certID := generateID("cert")

	// Build the signing data.
	csr := CertificateSigningData{
		CertificateID:   certID,
		ProviderID:      certReq.ProviderID,
		AgentName:       certReq.AgentName,
		CertificateType: certReq.CertType,
		Claims:          certReq.Claims,
		PublicKeyPEM:    certReq.PublicKeyPEM,
		NotBefore:       notBefore,
		NotAfter:        notAfter,
	}

	// Sign with the CA engine. Store the canonical signed bytes to avoid
	// reconstruction ambiguity if the signing format changes in the future.
	signedBytes, signature, err := s.ca.SignCertificate(csr)
	if err != nil {
		return model.AgentCertificate{}, fmt.Errorf("sign certificate: %w", err)
	}

	// Compute a fingerprint (SHA-256 of the signature for quick lookup).
	fingerprint := computeFingerprint(signature)

	cert := model.AgentCertificate{
		CertificateID:   certID,
		TenantID:        certReq.TenantID,
		ProviderID:      certReq.ProviderID,
		AgentName:       certReq.AgentName,
		CertificateType: certReq.CertType,
		Status:          model.CertStatusActive,
		Claims:          certReq.Claims,
		PublicKeyPEM:    certReq.PublicKeyPEM,
		SignatureAlg:    "ECDSA-P256-SHA256",
		Signature:       signature,
		SignedData:      string(signedBytes),
		SchemaVersion:   1,
		IssuerID:        "aex-certauth",
		NotBefore:       notBefore,
		NotAfter:        notAfter,
		RenewalCount:    0,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.store.CreateCertificate(ctx, cert); err != nil {
		return model.AgentCertificate{}, fmt.Errorf("store certificate: %w", err)
	}

	// Update the request status to APPROVED.
	if err := s.store.UpdateCertificateRequest(ctx, requestID, "APPROVED", reviewedBy, ""); err != nil {
		slog.WarnContext(ctx, "failed to update request status after certificate creation",
			"request_id", requestID,
			"error", err,
		)
	}

	// Publish issued event.
	_ = s.publisher.Publish(ctx, events.EventCertificateIssued, map[string]any{
		"certificate_id": certID,
		"provider_id":    cert.ProviderID,
		"agent_id":       cert.AgentName,
		"domain":         primaryCategory(cert.Claims),
		"issued_at":      now,
		"expires_at":     notAfter,
		"fingerprint":    fingerprint,
	})

	slog.InfoContext(ctx, "certificate_issued",
		"certificate_id", certID,
		"request_id", requestID,
		"provider_id", cert.ProviderID,
		"not_after", notAfter,
	)

	return cert, nil
}

// RejectCertificateRequest rejects a pending certificate request.
func (s *CertificateService) RejectCertificateRequest(ctx context.Context, requestID, reviewedBy, reason string) error {
	certReq, err := s.store.GetCertificateRequest(ctx, requestID)
	if err != nil {
		return fmt.Errorf("get certificate request: %w", err)
	}

	if certReq.Status != model.CertStatusPending {
		return ErrRequestNotPending
	}

	if err := s.store.UpdateCertificateRequest(ctx, requestID, "REJECTED", reviewedBy, reason); err != nil {
		return fmt.Errorf("update request status: %w", err)
	}

	slog.InfoContext(ctx, "certificate_request_rejected",
		"request_id", requestID,
		"reviewed_by", reviewedBy,
		"reason", reason,
	)

	return nil
}

// GetCertificate retrieves a certificate by its ID.
func (s *CertificateService) GetCertificate(ctx context.Context, certID string) (model.AgentCertificate, error) {
	cert, err := s.store.GetCertificate(ctx, certID)
	if err != nil {
		return model.AgentCertificate{}, fmt.Errorf("get certificate: %w", err)
	}
	return cert, nil
}

// GetProviderCertificates retrieves all certificates for a given provider.
func (s *CertificateService) GetProviderCertificates(ctx context.Context, providerID string) ([]model.AgentCertificate, error) {
	certs, err := s.store.GetCertificatesByProvider(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("get provider certificates: %w", err)
	}
	return certs, nil
}

// RenewCertificate issues a new certificate linked to the previous one. The
// old certificate remains ACTIVE until it naturally expires (or can be revoked
// separately). The new certificate inherits the same claims but gets fresh
// validity dates and an incremented renewal count.
func (s *CertificateService) RenewCertificate(ctx context.Context, certID string) (model.AgentCertificate, error) {
	existing, err := s.store.GetCertificate(ctx, certID)
	if err != nil {
		return model.AgentCertificate{}, fmt.Errorf("get certificate for renewal: %w", err)
	}

	if existing.Status != model.CertStatusActive {
		return model.AgentCertificate{}, ErrCertNotActive
	}

	now := time.Now().UTC()
	notBefore := now
	notAfter := now.Add(time.Duration(s.config.CertValidityDays) * 24 * time.Hour)
	newCertID := generateID("cert")

	// Build signing data for the renewed certificate.
	csr := CertificateSigningData{
		CertificateID:   newCertID,
		ProviderID:      existing.ProviderID,
		AgentName:       existing.AgentName,
		CertificateType: existing.CertificateType,
		Claims:          existing.Claims,
		PublicKeyPEM:    existing.PublicKeyPEM,
		NotBefore:       notBefore,
		NotAfter:        notAfter,
	}

	signedBytes, signature, err := s.ca.SignCertificate(csr)
	if err != nil {
		return model.AgentCertificate{}, fmt.Errorf("sign renewed certificate: %w", err)
	}

	fingerprint := computeFingerprint(signature)

	newCert := model.AgentCertificate{
		CertificateID:   newCertID,
		TenantID:        existing.TenantID,
		ProviderID:      existing.ProviderID,
		AgentName:       existing.AgentName,
		CertificateType: existing.CertificateType,
		Status:          model.CertStatusActive,
		Claims:          existing.Claims,
		PublicKeyPEM:    existing.PublicKeyPEM,
		SignatureAlg:    "ECDSA-P256-SHA256",
		Signature:       signature,
		SignedData:      string(signedBytes),
		SchemaVersion:   1,
		IssuerID:        "aex-certauth",
		NotBefore:       notBefore,
		NotAfter:        notAfter,
		PreviousCertID:  certID,
		RenewalCount:    existing.RenewalCount + 1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.store.CreateCertificate(ctx, newCert); err != nil {
		return model.AgentCertificate{}, fmt.Errorf("store renewed certificate: %w", err)
	}

	// Mark the old certificate as EXPIRED so only the new one is active.
	if err := s.store.UpdateCertificateStatus(ctx, certID, model.CertStatusExpired); err != nil {
		slog.WarnContext(ctx, "failed to expire previous certificate during renewal",
			"previous_cert_id", certID,
			"error", err,
		)
	}

	// Publish renewal event.
	_ = s.publisher.Publish(ctx, events.EventCertificateRenewed, map[string]any{
		"certificate_id":  newCertID,
		"provider_id":     newCert.ProviderID,
		"agent_id":        newCert.AgentName,
		"previous_expiry": existing.NotAfter,
		"new_expiry":      notAfter,
		"renewed_at":      now,
		"new_fingerprint": fingerprint,
	})

	slog.InfoContext(ctx, "certificate_renewed",
		"new_certificate_id", newCertID,
		"previous_certificate_id", certID,
		"provider_id", newCert.ProviderID,
		"renewal_count", newCert.RenewalCount,
	)

	return newCert, nil
}

// RevokeCertificate revokes an active certificate. The certificate status is
// set to REVOKED, the revocation timestamp is recorded, and a new CRL is
// generated. A certificate.revoked event is published.
func (s *CertificateService) RevokeCertificate(ctx context.Context, certID, reason string) error {
	cert, err := s.store.GetCertificate(ctx, certID)
	if err != nil {
		return fmt.Errorf("get certificate for revocation: %w", err)
	}

	if cert.Status == model.CertStatusRevoked {
		return ErrAlreadyRevoked
	}

	if err := s.store.RevokeCertificate(ctx, certID, reason); err != nil {
		return fmt.Errorf("revoke certificate in store: %w", err)
	}

	now := time.Now().UTC()

	// Publish revocation event.
	_ = s.publisher.Publish(ctx, events.EventCertificateRevoked, map[string]any{
		"certificate_id": certID,
		"provider_id":    cert.ProviderID,
		"agent_id":       cert.AgentName,
		"reason":         reason,
		"revoked_at":     now,
	})

	slog.InfoContext(ctx, "certificate_revoked",
		"certificate_id", certID,
		"provider_id", cert.ProviderID,
		"reason", reason,
	)

	return nil
}

// SearchCertificates searches for certificates matching the given filters.
func (s *CertificateService) SearchCertificates(ctx context.Context, filters SearchFilters) ([]model.AgentCertificate, error) {
	storeFilters := store.CertificateFilters{
		ProviderID:      filters.ProviderID,
		TenantID:        filters.TenantID,
		CertificateType: filters.CertificateType,
		Status:          filters.Status,
		Category:        filters.Category,
		Capability:      filters.Capability,
		Limit:           filters.Limit,
		Offset:          filters.Offset,
	}

	if storeFilters.Limit <= 0 {
		storeFilters.Limit = 50
	}

	certs, err := s.store.SearchCertificates(ctx, storeFilters)
	if err != nil {
		return nil, fmt.Errorf("search certificates: %w", err)
	}

	return certs, nil
}

// ---------- helpers ----------

// generateID produces a prefixed random hex ID (e.g. "cert_a1b2c3d4e5f6g7h8").
func generateID(prefix string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}

// computeFingerprint returns the hex-encoded SHA-256 hash of the input string.
func computeFingerprint(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

// primaryCategory extracts the first claim's category (used as "domain" in events).
func primaryCategory(claims []model.CapabilityClaim) string {
	if len(claims) == 0 {
		return ""
	}
	return claims[0].Category
}
