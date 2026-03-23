package service

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"time"

	"github.com/parlakisik/agent-exchange/aex-certauth/internal/model"
)

// CAEngine wraps cryptographic operations for the AEX certificate authority.
// It uses ECDSA P-256 for signing agent capability certificates. The initial
// implementation relies on Go's standard crypto library; a KMS-backed signer
// (smallstep/crypto) can be swapped in later without changing the interface.
type CAEngine struct {
	privateKey  crypto.PrivateKey  // ECDSA P-256
	publicKey   crypto.PublicKey
	certificate *x509.Certificate // CA's own self-signed certificate
	keyID       string
}

// CertificateSigningData contains the data to be signed for an agent certificate.
type CertificateSigningData struct {
	CertificateID   string                  `json:"certificate_id"`
	ProviderID      string                  `json:"provider_id"`
	AgentName       string                  `json:"agent_name"`
	CertificateType string                  `json:"certificate_type"`
	Claims          []model.CapabilityClaim `json:"claims"`
	PublicKeyPEM    string                  `json:"public_key_pem"`
	NotBefore       time.Time               `json:"not_before"`
	NotAfter        time.Time               `json:"not_after"`
}

// NewCAEngine loads or generates an ECDSA P-256 CA key pair from the given path.
// If keyPath is empty, an ephemeral in-memory key pair is generated (suitable
// for development/testing). When keyPath points to a non-existent file the CA
// key and self-signed certificate are generated and written to disk.
func NewCAEngine(keyPath string) (*CAEngine, error) {
	engine := &CAEngine{}

	if keyPath == "" {
		slog.Info("ca_engine: no key path specified, generating ephemeral CA key pair")
		if err := engine.generateCA(); err != nil {
			return nil, fmt.Errorf("generate ephemeral CA: %w", err)
		}
		return engine, nil
	}

	// Attempt to load existing key; generate if missing.
	if err := engine.GenerateCAIfNotExists(keyPath); err != nil {
		return nil, err
	}

	return engine, nil
}

// GenerateCAIfNotExists generates a self-signed CA cert and key at keyPath if
// the file does not already exist. If the key file is present it is loaded
// instead.
func (e *CAEngine) GenerateCAIfNotExists(keyPath string) error {
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		slog.Info("ca_engine: key file not found, generating new CA", "path", keyPath)
		if err := e.generateCA(); err != nil {
			return fmt.Errorf("generate CA: %w", err)
		}
		if err := e.saveKey(keyPath); err != nil {
			return fmt.Errorf("save CA key: %w", err)
		}
		// Save the certificate alongside the key.
		certPath := keyPath + ".crt"
		if err := e.saveCert(certPath); err != nil {
			return fmt.Errorf("save CA cert: %w", err)
		}
		slog.Info("ca_engine: CA key and certificate generated",
			"key_path", keyPath,
			"cert_path", certPath,
			"key_id", e.keyID,
		)
		return nil
	}

	slog.Info("ca_engine: loading existing CA key", "path", keyPath)
	if err := e.loadKey(keyPath); err != nil {
		return fmt.Errorf("load CA key: %w", err)
	}

	// Try loading the certificate as well.
	certPath := keyPath + ".crt"
	if _, err := os.Stat(certPath); err == nil {
		if err := e.loadCert(certPath); err != nil {
			slog.Warn("ca_engine: failed to load cert, regenerating", "error", err)
			if err := e.generateSelfSignedCert(); err != nil {
				return fmt.Errorf("regenerate self-signed cert: %w", err)
			}
		}
	} else {
		// No cert file; generate one from the loaded key.
		if err := e.generateSelfSignedCert(); err != nil {
			return fmt.Errorf("generate self-signed cert from existing key: %w", err)
		}
		if err := e.saveCert(certPath); err != nil {
			return fmt.Errorf("save generated cert: %w", err)
		}
	}

	return nil
}

// SignCertificate signs the given certificate data and returns the DER-encoded
// signed data and a base64-encoded ECDSA signature.
//
// Signing process:
//  1. Marshal the certificate data to canonical JSON
//  2. Hash with SHA-256
//  3. Sign with ECDSA P-256
//  4. Return base64-encoded signature
func (e *CAEngine) SignCertificate(csr CertificateSigningData) ([]byte, string, error) {
	canonicalJSON, err := json.Marshal(csr)
	if err != nil {
		return nil, "", fmt.Errorf("marshal certificate data: %w", err)
	}

	hash := sha256.Sum256(canonicalJSON)

	ecKey, ok := e.privateKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, "", fmt.Errorf("private key is not ECDSA")
	}

	sig, err := ecdsa.SignASN1(rand.Reader, ecKey, hash[:])
	if err != nil {
		return nil, "", fmt.Errorf("ECDSA sign: %w", err)
	}

	signature := base64.StdEncoding.EncodeToString(sig)

	slog.Info("ca_engine: certificate signed",
		"certificate_id", csr.CertificateID,
		"provider_id", csr.ProviderID,
	)

	return canonicalJSON, signature, nil
}

// VerifyCertificate verifies the ECDSA signature over the given certificate
// data using the CA's public key.
func (e *CAEngine) VerifyCertificate(certData []byte, signature string) error {
	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	hash := sha256.Sum256(certData)

	ecKey, ok := e.publicKey.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("public key is not ECDSA")
	}

	if !ecdsa.VerifyASN1(ecKey, hash[:], sigBytes) {
		return fmt.Errorf("ECDSA signature verification failed")
	}

	return nil
}

// GetCACertificatePEM returns the CA's self-signed certificate in PEM format.
func (e *CAEngine) GetCACertificatePEM() string {
	if e.certificate == nil {
		return ""
	}
	block := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: e.certificate.Raw,
	}
	return string(pem.EncodeToMemory(block))
}

// GetCAPublicKeyPEM returns the CA's public key in PEM format.
func (e *CAEngine) GetCAPublicKeyPEM() string {
	ecKey, ok := e.publicKey.(*ecdsa.PublicKey)
	if !ok {
		return ""
	}
	derBytes, err := x509.MarshalPKIXPublicKey(ecKey)
	if err != nil {
		return ""
	}
	block := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: derBytes,
	}
	return string(pem.EncodeToMemory(block))
}

// SignCRL signs a Certificate Revocation List and returns a base64-encoded
// ECDSA signature over the canonical JSON representation of the entries.
func (e *CAEngine) SignCRL(entries []model.CRLEntry) (string, error) {
	canonicalJSON, err := json.Marshal(entries)
	if err != nil {
		return "", fmt.Errorf("marshal CRL entries: %w", err)
	}

	hash := sha256.Sum256(canonicalJSON)

	ecKey, ok := e.privateKey.(*ecdsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("private key is not ECDSA")
	}

	sig, err := ecdsa.SignASN1(rand.Reader, ecKey, hash[:])
	if err != nil {
		return "", fmt.Errorf("ECDSA sign CRL: %w", err)
	}

	signature := base64.StdEncoding.EncodeToString(sig)

	slog.Info("ca_engine: CRL signed", "entries", len(entries))

	return signature, nil
}

// ---------- internal helpers ----------

// generateCA creates a fresh ECDSA P-256 key pair and self-signed certificate.
func (e *CAEngine) generateCA() error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate ECDSA key: %w", err)
	}

	e.privateKey = key
	e.publicKey = &key.PublicKey

	// Derive a stable key ID from the public key hash.
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return fmt.Errorf("marshal public key: %w", err)
	}
	keyHash := sha256.Sum256(pubDER)
	e.keyID = base64.RawURLEncoding.EncodeToString(keyHash[:8])

	return e.generateSelfSignedCert()
}

// generateSelfSignedCert creates a self-signed X.509 CA certificate from the
// currently loaded key pair.
func (e *CAEngine) generateSelfSignedCert() error {
	ecKey, ok := e.privateKey.(*ecdsa.PrivateKey)
	if !ok {
		return fmt.Errorf("private key is not ECDSA")
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generate serial number: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "AEX Certificate Authority",
			Organization: []string{"Agent Exchange"},
		},
		NotBefore:             time.Now().UTC().Add(-1 * time.Hour), // Allow for clock skew
		NotAfter:              time.Now().UTC().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &ecKey.PublicKey, ecKey)
	if err != nil {
		return fmt.Errorf("create self-signed certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return fmt.Errorf("parse generated certificate: %w", err)
	}

	e.certificate = cert
	return nil
}

// saveKey writes the ECDSA private key to disk in PEM format.
func (e *CAEngine) saveKey(path string) error {
	ecKey, ok := e.privateKey.(*ecdsa.PrivateKey)
	if !ok {
		return fmt.Errorf("private key is not ECDSA")
	}

	der, err := x509.MarshalECPrivateKey(ecKey)
	if err != nil {
		return fmt.Errorf("marshal EC private key: %w", err)
	}

	block := &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: der,
	}

	return os.WriteFile(path, pem.EncodeToMemory(block), 0600)
}

// saveCert writes the CA certificate to disk in PEM format.
func (e *CAEngine) saveCert(path string) error {
	if e.certificate == nil {
		return fmt.Errorf("no certificate to save")
	}

	block := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: e.certificate.Raw,
	}

	return os.WriteFile(path, pem.EncodeToMemory(block), 0644)
}

// loadKey reads an ECDSA private key from a PEM file.
func (e *CAEngine) loadKey(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read key file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("no PEM block found in %s", path)
	}

	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse EC private key: %w", err)
	}

	e.privateKey = key
	e.publicKey = &key.PublicKey

	// Derive key ID from public key.
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return fmt.Errorf("marshal public key for key ID: %w", err)
	}
	keyHash := sha256.Sum256(pubDER)
	e.keyID = base64.RawURLEncoding.EncodeToString(keyHash[:8])

	return nil
}

// loadCert reads an X.509 certificate from a PEM file.
func (e *CAEngine) loadCert(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read cert file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("no PEM block found in %s", path)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse certificate: %w", err)
	}

	e.certificate = cert
	return nil
}
