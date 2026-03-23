package model

import (
	"time"
)

// CertificateStatus constants
const (
	CertStatusPending   = "PENDING"
	CertStatusActive    = "ACTIVE"
	CertStatusSuspended = "SUSPENDED"
	CertStatusRevoked   = "REVOKED"
	CertStatusExpired   = "EXPIRED"
)

// CertificateType constants
const (
	CertTypeCapability = "CAPABILITY"
	CertTypeIdentity   = "IDENTITY"
	CertTypeReputation = "REPUTATION"
	CertTypeReseller   = "RESELLER"
)

// AuthorizationType constants
const (
	AuthSelfAsserted     = "SELF_ASSERTED"
	AuthProviderAttested = "PROVIDER_ATTESTED"
	AuthThirdParty       = "THIRD_PARTY"
	AuthAEXVerified      = "AEX_VERIFIED"
)

// CapabilityCategory constants
const (
	CategoryCommerce      = "COMMERCE"
	CategoryFinance       = "FINANCE"
	CategoryTravel        = "TRAVEL"
	CategoryEntertainment = "ENTERTAINMENT"
	CategoryHealthcare    = "HEALTHCARE"
	CategoryEducation     = "EDUCATION"
	CategoryTechnology    = "TECHNOLOGY"
	CategoryGeneral       = "GENERAL"
)

// AgentCertificate represents an X.509-style cryptographic certificate for an AI agent
type AgentCertificate struct {
	CertificateID   string            `json:"certificate_id" bson:"certificate_id"`
	TenantID        string            `json:"tenant_id" bson:"tenant_id"`
	ProviderID      string            `json:"provider_id" bson:"provider_id"`
	AgentName       string            `json:"agent_name" bson:"agent_name"`
	CertificateType string            `json:"certificate_type" bson:"certificate_type"`
	Status          string            `json:"status" bson:"status"`
	Claims          []CapabilityClaim `json:"claims" bson:"claims"`

	// Crypto
	PublicKeyPEM string `json:"public_key_pem" bson:"public_key_pem"`
	SignatureAlg string `json:"signature_alg" bson:"signature_alg"`
	Signature    string `json:"signature" bson:"signature"`

	// X.509 fields
	IssuerID  string    `json:"issuer_id" bson:"issuer_id"`
	NotBefore time.Time `json:"not_before" bson:"not_before"`
	NotAfter  time.Time `json:"not_after" bson:"not_after"`

	// W3C DID binding (optional)
	SubjectDID string `json:"subject_did,omitempty" bson:"subject_did,omitempty"`
	IssuerDID  string `json:"issuer_did,omitempty" bson:"issuer_did,omitempty"`

	// Revocation
	RevokedAt        *time.Time `json:"revoked_at,omitempty" bson:"revoked_at,omitempty"`
	RevocationReason string     `json:"revocation_reason,omitempty" bson:"revocation_reason,omitempty"`

	// Renewal chain
	PreviousCertID string `json:"previous_cert_id,omitempty" bson:"previous_cert_id,omitempty"`
	RenewalCount   int    `json:"renewal_count" bson:"renewal_count"`

	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

// CapabilityClaim represents a single capability claim within a certificate
type CapabilityClaim struct {
	Category         string         `json:"category" bson:"category"`
	Capability       string         `json:"capability" bson:"capability"`
	Scope            string         `json:"scope,omitempty" bson:"scope,omitempty"`
	Authorization    string         `json:"authorization" bson:"authorization"`
	AuthorizationRef string         `json:"authorization_ref,omitempty" bson:"authorization_ref,omitempty"`
	Constraints      map[string]any `json:"constraints,omitempty" bson:"constraints,omitempty"`
}

// CertificateRequest represents a pending request for a new certificate
type CertificateRequest struct {
	RequestID    string            `json:"request_id" bson:"request_id"`
	TenantID     string            `json:"tenant_id" bson:"tenant_id"`
	ProviderID   string            `json:"provider_id" bson:"provider_id"`
	AgentName    string            `json:"agent_name" bson:"agent_name"`
	CertType     string            `json:"certificate_type" bson:"certificate_type"`
	Claims       []CapabilityClaim `json:"claims" bson:"claims"`
	PublicKeyPEM string            `json:"public_key_pem" bson:"public_key_pem"`
	Status       string            `json:"status" bson:"status"` // PENDING, APPROVED, REJECTED
	ReviewedBy   string            `json:"reviewed_by,omitempty" bson:"reviewed_by,omitempty"`
	ReviewNote   string            `json:"review_note,omitempty" bson:"review_note,omitempty"`
	CreatedAt    time.Time         `json:"created_at" bson:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at" bson:"updated_at"`
}
