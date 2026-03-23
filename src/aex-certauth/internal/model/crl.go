package model

import (
	"time"
)

// CRL represents a Certificate Revocation List
type CRL struct {
	CRLID      string     `json:"crl_id" bson:"crl_id"`
	IssuerID   string     `json:"issuer_id" bson:"issuer_id"`
	ThisUpdate time.Time  `json:"this_update" bson:"this_update"`
	NextUpdate time.Time  `json:"next_update" bson:"next_update"`
	Entries    []CRLEntry `json:"entries" bson:"entries"`
	Signature  string     `json:"signature" bson:"signature"`
	CreatedAt  time.Time  `json:"created_at" bson:"created_at"`
}

// CRLEntry represents a single entry in a Certificate Revocation List
type CRLEntry struct {
	CertificateID    string    `json:"certificate_id" bson:"certificate_id"`
	RevokedAt        time.Time `json:"revoked_at" bson:"revoked_at"`
	RevocationReason string    `json:"revocation_reason" bson:"revocation_reason"`
}
