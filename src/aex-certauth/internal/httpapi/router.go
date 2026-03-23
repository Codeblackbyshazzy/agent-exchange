package httpapi

import (
	"net/http"

	"github.com/parlakisik/agent-exchange/aex-certauth/internal/service"
)

func NewRouter(certSvc *service.CertificateService, repSvc *service.ReputationService, verifySvc *service.VerificationService, ca *service.CAEngine) http.Handler {
	h := NewHandlers(certSvc, repSvc, verifySvc, ca)
	mux := http.NewServeMux()

	// External API - Certificates
	mux.HandleFunc("/v1/certificates/request", dispatchCertRequest(h))
	mux.HandleFunc("/v1/certificates/verify", h.VerifyCertificate)
	mux.HandleFunc("/v1/certificates/search", h.SearchCertificates)
	mux.HandleFunc("/v1/certificates/", dispatchCertByID(h))

	// External API - Providers
	mux.HandleFunc("/v1/providers/", dispatchProviders(h))

	// External API - CRL
	mux.HandleFunc("/v1/crl", h.GetCRL)

	// External API - Reputation Leaderboard
	mux.HandleFunc("/v1/reputation/leaderboard", h.GetLeaderboard)

	// Internal API
	mux.HandleFunc("/internal/v1/certificates/batch-verify", h.BatchVerify)
	mux.HandleFunc("/internal/v1/providers/", dispatchInternalProviders(h))
	mux.HandleFunc("/internal/v1/certificates/", dispatchInternalCertByID(h))

	// Well-known
	mux.HandleFunc("/.well-known/aex-ca.json", h.GetCAPublicKey)
	mux.HandleFunc("/health", h.Health)

	return mux
}

// dispatchCertRequest routes POST /v1/certificates/request
func dispatchCertRequest(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			h.RequestCertificate(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// dispatchCertByID routes requests to /v1/certificates/{cert_id}
// and /v1/certificates/{cert_id}/renew
func dispatchCertByID(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the path to extract cert_id and optional sub-path.
		// Pattern: /v1/certificates/{cert_id}[/renew]
		path := r.URL.Path
		const prefix = "/v1/certificates/"
		if len(path) <= len(prefix) {
			http.Error(w, "certificate ID is required", http.StatusBadRequest)
			return
		}

		rest := path[len(prefix):]
		certID, subPath := splitPath(rest)

		if certID == "" {
			http.Error(w, "certificate ID is required", http.StatusBadRequest)
			return
		}

		switch {
		case subPath == "renew" && r.Method == http.MethodPost:
			h.RenewCertificate(w, r, certID)
		case subPath == "" && r.Method == http.MethodGet:
			h.GetCertificate(w, r, certID)
		case subPath == "" && r.Method == http.MethodDelete:
			h.RevokeCertificate(w, r, certID)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// dispatchProviders routes /v1/providers/{id}/certificates and
// /v1/providers/{id}/reputation
func dispatchProviders(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		const prefix = "/v1/providers/"
		if len(path) <= len(prefix) {
			http.Error(w, "provider ID is required", http.StatusBadRequest)
			return
		}

		rest := path[len(prefix):]
		providerID, subPath := splitPath(rest)

		if providerID == "" {
			http.Error(w, "provider ID is required", http.StatusBadRequest)
			return
		}

		switch {
		case subPath == "certificates" && r.Method == http.MethodGet:
			h.GetProviderCertificates(w, r, providerID)
		case subPath == "reputation" && r.Method == http.MethodGet:
			h.GetProviderReputation(w, r, providerID)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}
}

// dispatchInternalProviders routes /internal/v1/providers/{id}/can-perform
func dispatchInternalProviders(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		const prefix = "/internal/v1/providers/"
		if len(path) <= len(prefix) {
			http.Error(w, "provider ID is required", http.StatusBadRequest)
			return
		}

		rest := path[len(prefix):]
		providerID, subPath := splitPath(rest)

		if providerID == "" {
			http.Error(w, "provider ID is required", http.StatusBadRequest)
			return
		}

		switch {
		case subPath == "can-perform" && r.Method == http.MethodGet:
			h.CanPerform(w, r, providerID)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}
}

// dispatchInternalCertByID routes /internal/v1/certificates/{id}/approve and
// /internal/v1/certificates/{id}/reject
func dispatchInternalCertByID(h *Handlers) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		const prefix = "/internal/v1/certificates/"
		if len(path) <= len(prefix) {
			http.Error(w, "certificate/request ID is required", http.StatusBadRequest)
			return
		}

		rest := path[len(prefix):]
		id, subPath := splitPath(rest)

		if id == "" {
			http.Error(w, "certificate/request ID is required", http.StatusBadRequest)
			return
		}

		switch {
		case subPath == "approve" && r.Method == http.MethodPost:
			h.ApproveCertificate(w, r, id)
		case subPath == "reject" && r.Method == http.MethodPost:
			h.RejectCertificate(w, r, id)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}
}

// splitPath splits "abc/def" into ("abc", "def") and "abc" into ("abc", "").
func splitPath(s string) (first, rest string) {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}
