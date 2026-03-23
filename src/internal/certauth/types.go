package certauth

import "time"

// CertificateInfo is a lightweight subset of the full AgentCertificate,
// designed for cross-service use where the consuming service only needs to
// verify validity and check capabilities without pulling in the full certauth
// model dependency.
type CertificateInfo struct {
	CertificateID   string           `json:"certificate_id"`
	ProviderID      string           `json:"provider_id"`
	AgentName       string           `json:"agent_name"`
	CertificateType string           `json:"certificate_type"`
	Status          string           `json:"status"`
	Claims          []CapabilityInfo `json:"claims"`
	PublicKeyPEM    string           `json:"public_key_pem"`
	SignatureAlg    string           `json:"signature_alg"`
	Signature       string           `json:"signature"`
	IssuerID        string           `json:"issuer_id"`
	NotBefore       time.Time        `json:"not_before"`
	NotAfter        time.Time        `json:"not_after"`
}

// CapabilityInfo is a lightweight representation of a capability claim for
// cross-service verification.
type CapabilityInfo struct {
	Category     string `json:"category"`
	Capability   string `json:"capability"`
	Scope        string `json:"scope,omitempty"`
	Authorization string `json:"authorization"`
}

// VerificationResult represents the outcome of a certificate verification
// performed by a consuming service using the shared verifier.
type VerificationResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}

// Certificate status constants. These mirror the certauth service's model
// constants so consuming services do not need to import the full model.
const (
	StatusActive    = "ACTIVE"
	StatusSuspended = "SUSPENDED"
	StatusRevoked   = "REVOKED"
	StatusExpired   = "EXPIRED"
)
