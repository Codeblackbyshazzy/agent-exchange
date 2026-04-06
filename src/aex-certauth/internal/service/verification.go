package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/parlakisik/agent-exchange/aex-certauth/internal/model"
	"github.com/parlakisik/agent-exchange/aex-certauth/internal/store"
	"github.com/parlakisik/agent-exchange/internal/events"
)

// VerificationResult contains the outcome of a certificate verification check.
type VerificationResult struct {
	Valid       bool                   `json:"valid"`
	Errors      []string               `json:"errors,omitempty"`
	Certificate model.AgentCertificate `json:"certificate"`
}

// VerificationService handles certificate verification, CRL management, and
// capability lookups.
type VerificationService struct {
	store     store.CertAuthStore
	ca        *CAEngine
	publisher *events.Publisher
}

// NewVerificationService creates a new VerificationService.
func NewVerificationService(st store.CertAuthStore, ca *CAEngine, publisher *events.Publisher) *VerificationService {
	return &VerificationService{
		store:     st,
		ca:        ca,
		publisher: publisher,
	}
}

// VerifyCertificate performs full verification on a certificate:
//  1. Certificate exists in the store
//  2. Cryptographic signature is valid (signed by CA)
//  3. Not expired (not_before <= now <= not_after)
//  4. Not revoked (status is not REVOKED)
func (v *VerificationService) VerifyCertificate(ctx context.Context, certID string) (VerificationResult, error) {
	cert, err := v.store.GetCertificate(ctx, certID)
	if err != nil {
		return VerificationResult{}, fmt.Errorf("get certificate: %w", err)
	}

	result := VerificationResult{
		Valid:       true,
		Certificate: cert,
	}

	// 1. Verify cryptographic signature.
	if sigErr := v.verifySignature(cert); sigErr != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("signature: %s", sigErr.Error()))
	}

	// 2. Check expiration.
	now := time.Now().UTC()
	if now.Before(cert.NotBefore) {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("certificate not yet valid (not_before: %s)", cert.NotBefore.Format(time.RFC3339)))
	}
	if now.After(cert.NotAfter) {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("certificate expired (not_after: %s)", cert.NotAfter.Format(time.RFC3339)))
	}

	// 3. Check revocation status.
	if cert.Status == model.CertStatusRevoked {
		result.Valid = false
		reason := cert.RevocationReason
		if reason == "" {
			reason = "unspecified"
		}
		result.Errors = append(result.Errors, fmt.Sprintf("certificate revoked (reason: %s)", reason))
	}

	// 4. Check suspension.
	if cert.Status == model.CertStatusSuspended {
		result.Valid = false
		result.Errors = append(result.Errors, "certificate is suspended")
	}

	slog.InfoContext(ctx, "certificate_verified",
		"certificate_id", certID,
		"valid", result.Valid,
		"errors", len(result.Errors),
	)

	return result, nil
}

// BatchVerify verifies multiple certificates at once. This is used by the
// bid-evaluator to verify provider certificates during bid evaluation.
func (v *VerificationService) BatchVerify(ctx context.Context, certIDs []string) ([]VerificationResult, error) {
	results := make([]VerificationResult, 0, len(certIDs))

	for _, certID := range certIDs {
		result, err := v.VerifyCertificate(ctx, certID)
		if err != nil {
			// Include the error as a verification failure rather than aborting
			// the entire batch.
			results = append(results, VerificationResult{
				Valid:  false,
				Errors: []string{fmt.Sprintf("verification error: %s", err.Error())},
			})
			continue
		}
		results = append(results, result)
	}

	slog.InfoContext(ctx, "batch_verify_completed",
		"total", len(certIDs),
		"results", len(results),
	)

	return results, nil
}

// CanPerform checks whether a provider has an active, valid certificate that
// grants a specific capability within a category. Returns true only when a
// matching certificate passes full verification.
func (v *VerificationService) CanPerform(ctx context.Context, providerID, category, capability string) (bool, error) {
	certs, err := v.store.GetCertificatesByProvider(ctx, providerID)
	if err != nil {
		return false, fmt.Errorf("get provider certificates: %w", err)
	}

	now := time.Now().UTC()

	for _, cert := range certs {
		// Skip non-active certificates.
		if cert.Status != model.CertStatusActive {
			continue
		}

		// Skip expired certificates.
		if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
			continue
		}

		// Check if any claim matches.
		for _, claim := range cert.Claims {
			if claim.Category == category && claim.Capability == capability {
				// Verify signature to ensure integrity.
				if sigErr := v.verifySignature(cert); sigErr != nil {
					slog.WarnContext(ctx, "certificate has invalid signature during CanPerform",
						"certificate_id", cert.CertificateID,
						"error", sigErr,
					)
					continue
				}
				return true, nil
			}
		}
	}

	return false, nil
}

// GenerateCRL builds a new Certificate Revocation List from all revoked
// certificates, signs it with the CA engine, and stores it.
func (v *VerificationService) GenerateCRL(ctx context.Context) (model.CRL, error) {
	revokedCerts, err := v.store.SearchCertificates(ctx, store.CertificateFilters{
		Status: model.CertStatusRevoked,
		Limit:  10000, // Reasonable upper bound for CRL generation
	})
	if err != nil {
		return model.CRL{}, fmt.Errorf("list revoked certificates: %w", err)
	}

	entries := make([]model.CRLEntry, 0, len(revokedCerts))
	for _, cert := range revokedCerts {
		revokedAt := cert.CreatedAt
		if cert.RevokedAt != nil {
			revokedAt = *cert.RevokedAt
		}
		entries = append(entries, model.CRLEntry{
			CertificateID:    cert.CertificateID,
			RevokedAt:        revokedAt,
			RevocationReason: cert.RevocationReason,
		})
	}

	// Sign the CRL.
	signature, err := v.ca.SignCRL(entries)
	if err != nil {
		return model.CRL{}, fmt.Errorf("sign CRL: %w", err)
	}

	now := time.Now().UTC()
	crl := model.CRL{
		CRLID:      generateID("crl"),
		IssuerID:   "aex-certauth",
		ThisUpdate: now,
		NextUpdate: now.Add(24 * time.Hour), // CRL valid for 24 hours
		Entries:    entries,
		Signature:  signature,
		CreatedAt:  now,
	}

	if err := v.store.SaveCRL(ctx, crl); err != nil {
		return model.CRL{}, fmt.Errorf("save CRL: %w", err)
	}

	// Publish CRL update event.
	_ = v.publisher.Publish(ctx, events.EventCRLUpdated, map[string]any{
		"crl_id":        crl.CRLID,
		"entries_added": len(entries),
		"total_entries": len(entries),
		"updated_at":    now,
	})

	slog.InfoContext(ctx, "crl_generated",
		"crl_id", crl.CRLID,
		"entries", len(entries),
	)

	return crl, nil
}

// GetCRL returns the latest CRL from the store. If no CRL exists, a new one
// is generated.
func (v *VerificationService) GetCRL(ctx context.Context) (model.CRL, error) {
	crl, err := v.store.GetLatestCRL(ctx)
	if err != nil {
		// If no CRL exists yet, generate one.
		slog.InfoContext(ctx, "no existing CRL found, generating new one")
		return v.GenerateCRL(ctx)
	}

	// If the CRL has expired, regenerate.
	if time.Now().UTC().After(crl.NextUpdate) {
		slog.InfoContext(ctx, "CRL expired, regenerating",
			"crl_id", crl.CRLID,
			"next_update", crl.NextUpdate,
		)
		return v.GenerateCRL(ctx)
	}

	return crl, nil
}

// ---------- internal helpers ----------

// verifySignature verifies a certificate's signature against the CA engine.
// If SignedData is stored (SchemaVersion >= 1), use it directly to avoid
// reconstruction ambiguity. Falls back to reconstruction for legacy certs.
func (v *VerificationService) verifySignature(cert model.AgentCertificate) error {
	// Prefer stored signed data (avoids reconstruction ambiguity on format changes).
	if cert.SignedData != "" {
		return v.ca.VerifyCertificate([]byte(cert.SignedData), cert.Signature)
	}

	// Legacy fallback: reconstruct the canonical signing data.
	csr := CertificateSigningData{
		CertificateID:   cert.CertificateID,
		ProviderID:      cert.ProviderID,
		AgentName:       cert.AgentName,
		CertificateType: cert.CertificateType,
		Claims:          cert.Claims,
		PublicKeyPEM:    cert.PublicKeyPEM,
		NotBefore:       cert.NotBefore,
		NotAfter:        cert.NotAfter,
	}

	canonicalJSON, err := json.Marshal(csr)
	if err != nil {
		return fmt.Errorf("marshal certificate data: %w", err)
	}

	return v.ca.VerifyCertificate(canonicalJSON, cert.Signature)
}
