package store

import (
	"context"
	"errors"

	"github.com/parlakisik/agent-exchange/aex-certauth/internal/model"
)

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = errors.New("not found")

// CertificateFilters defines the filter criteria for searching certificates
type CertificateFilters struct {
	ProviderID      string
	TenantID        string
	CertificateType string
	Status          string
	Category        string
	Capability      string
	Limit           int
	Offset          int
}

// CertAuthStore defines the interface for certificate authority persistence
type CertAuthStore interface {
	// Certificate Requests
	CreateCertificateRequest(ctx context.Context, req model.CertificateRequest) error
	GetCertificateRequest(ctx context.Context, requestID string) (model.CertificateRequest, error)
	UpdateCertificateRequest(ctx context.Context, requestID string, status string, reviewedBy string, reviewNote string) error

	// Certificates
	CreateCertificate(ctx context.Context, cert model.AgentCertificate) error
	GetCertificate(ctx context.Context, certID string) (model.AgentCertificate, error)
	GetCertificatesByProvider(ctx context.Context, providerID string) ([]model.AgentCertificate, error)
	UpdateCertificateStatus(ctx context.Context, certID string, status string) error
	RevokeCertificate(ctx context.Context, certID string, reason string) error
	SearchCertificates(ctx context.Context, filters CertificateFilters) ([]model.AgentCertificate, error)
	CountActiveCertificates(ctx context.Context, providerID string) (int, error)

	// Reputation
	UpsertReputation(ctx context.Context, reputation model.ReputationScore) error
	GetReputation(ctx context.Context, providerID string) (model.ReputationScore, error)
	GetLeaderboard(ctx context.Context, limit int, offset int) ([]model.ReputationScore, error)
	ListAllProviderIDs(ctx context.Context) ([]string, error)

	// CRL
	SaveCRL(ctx context.Context, crl model.CRL) error
	GetLatestCRL(ctx context.Context) (model.CRL, error)

	Close() error
}
