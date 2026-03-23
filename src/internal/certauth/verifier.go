package certauth

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"time"
)

// VerifySignature verifies an ECDSA signature over the given data using the
// provided PEM-encoded public key. This is the primary function used by
// consuming services (e.g. bid-evaluator, contract-engine) to verify that a
// certificate was signed by the AEX CA without needing a direct dependency on
// the certauth service.
//
// The data parameter should be the canonical JSON representation of the
// certificate signing data, and signature should be the base64-encoded ECDSA
// ASN.1 signature produced by the CA engine.
func VerifySignature(data []byte, signature string, publicKeyPEM string) error {
	// Decode the PEM public key.
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return fmt.Errorf("failed to decode PEM public key")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}

	ecKey, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("public key is not ECDSA (got %T)", pub)
	}

	// Decode the base64 signature.
	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("decode base64 signature: %w", err)
	}

	// Hash the data with SHA-256 and verify.
	hash := sha256.Sum256(data)

	if !ecdsa.VerifyASN1(ecKey, hash[:], sigBytes) {
		return fmt.Errorf("ECDSA signature verification failed")
	}

	return nil
}

// IsCertificateValid checks whether a certificate is currently valid based on
// its status and expiry dates. This performs only the non-cryptographic checks;
// use VerifySignature separately if a full cryptographic verification is
// required.
func IsCertificateValid(cert CertificateInfo) error {
	// Check status.
	switch cert.Status {
	case StatusRevoked:
		return fmt.Errorf("certificate %s is revoked", cert.CertificateID)
	case StatusSuspended:
		return fmt.Errorf("certificate %s is suspended", cert.CertificateID)
	case StatusExpired:
		return fmt.Errorf("certificate %s is expired", cert.CertificateID)
	case StatusActive:
		// Continue to time-based checks below.
	default:
		return fmt.Errorf("certificate %s has unknown status: %s", cert.CertificateID, cert.Status)
	}

	// Check time validity.
	now := time.Now().UTC()
	if now.Before(cert.NotBefore) {
		return fmt.Errorf("certificate %s is not yet valid (not_before: %s)", cert.CertificateID, cert.NotBefore.Format(time.RFC3339))
	}
	if now.After(cert.NotAfter) {
		return fmt.Errorf("certificate %s has expired (not_after: %s)", cert.CertificateID, cert.NotAfter.Format(time.RFC3339))
	}

	return nil
}

// HasCapability checks whether a CertificateInfo contains a claim matching the
// given category and capability.
func HasCapability(cert CertificateInfo, category, capability string) bool {
	for _, claim := range cert.Claims {
		if claim.Category == category && claim.Capability == capability {
			return true
		}
	}
	return false
}
