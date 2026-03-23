//go:build e2e

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// certauthBaseURL returns the base URL for the certauth service.
func certauthBaseURL() string {
	if u := os.Getenv("CERTAUTH_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://localhost:8091"
}

// httpDo is a small helper that executes an HTTP request and returns the
// status code, decoded JSON body (as map), and raw body bytes.
func httpDo(t *testing.T, method, url string, body interface{}) (int, map[string]interface{}, []byte) {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("execute request %s %s: %v", method, url, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	var result map[string]interface{}
	// Tolerate non-JSON responses (e.g. health check returning just 200).
	_ = json.Unmarshal(raw, &result)

	return resp.StatusCode, result, raw
}

// uniqueID returns a unique suffix for test isolation.
func uniqueID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// ---------------------------------------------------------------------------
// Test: Health check
// ---------------------------------------------------------------------------

func TestCertAuth_Health(t *testing.T) {
	base := certauthBaseURL()
	status, body, _ := httpDo(t, http.MethodGet, base+"/health", nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if s, ok := body["status"].(string); !ok || s != "healthy" {
		t.Errorf("expected status=healthy, got %v", body["status"])
	}
}

// ---------------------------------------------------------------------------
// Test: Request certificate -- valid and invalid payloads
// ---------------------------------------------------------------------------

func TestCertAuth_RequestCertificate_Valid(t *testing.T) {
	base := certauthBaseURL()
	uid := uniqueID()

	payload := map[string]interface{}{
		"tenant_id":        "tenant-" + uid,
		"provider_id":      "provider-" + uid,
		"agent_name":       "agent-" + uid,
		"certificate_type": "CAPABILITY",
		"claims": []map[string]interface{}{
			{
				"category":      "TECHNOLOGY",
				"capability":    "code-review",
				"authorization": "SELF_ASSERTED",
			},
		},
		"public_key_pem": "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEtest" + uid + "\n-----END PUBLIC KEY-----",
	}

	status, body, _ := httpDo(t, http.MethodPost, base+"/v1/certificates/request", payload)
	if status != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %v", status, body)
	}

	reqID, ok := body["request_id"].(string)
	if !ok || reqID == "" {
		t.Fatal("response missing request_id")
	}

	if s := body["status"]; s != "PENDING" {
		t.Errorf("expected status PENDING, got %v", s)
	}
	t.Logf("request_id=%s", reqID)
}

func TestCertAuth_RequestCertificate_MissingProviderID(t *testing.T) {
	base := certauthBaseURL()
	payload := map[string]interface{}{
		"claims": []map[string]interface{}{
			{"category": "GENERAL", "capability": "test", "authorization": "SELF_ASSERTED"},
		},
		"public_key_pem": "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----",
	}

	status, body, _ := httpDo(t, http.MethodPost, base+"/v1/certificates/request", payload)
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %v", status, body)
	}
}

func TestCertAuth_RequestCertificate_MissingClaims(t *testing.T) {
	base := certauthBaseURL()
	payload := map[string]interface{}{
		"provider_id":    "provider-test",
		"public_key_pem": "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----",
	}

	status, body, _ := httpDo(t, http.MethodPost, base+"/v1/certificates/request", payload)
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %v", status, body)
	}
}

func TestCertAuth_RequestCertificate_MissingPublicKey(t *testing.T) {
	base := certauthBaseURL()
	payload := map[string]interface{}{
		"provider_id": "provider-test",
		"claims": []map[string]interface{}{
			{"category": "GENERAL", "capability": "test", "authorization": "SELF_ASSERTED"},
		},
	}

	status, body, _ := httpDo(t, http.MethodPost, base+"/v1/certificates/request", payload)
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %v", status, body)
	}
}

// ---------------------------------------------------------------------------
// Test: Full lifecycle -- request -> approve -> get -> verify -> renew ->
//       revoke -> verify revoked -> CRL
// ---------------------------------------------------------------------------

func TestCertAuth_FullLifecycle(t *testing.T) {
	base := certauthBaseURL()
	uid := uniqueID()

	// ---- Step 1: Request certificate ----
	t.Log("Step 1: Request certificate")
	reqPayload := map[string]interface{}{
		"tenant_id":        "tenant-lifecycle-" + uid,
		"provider_id":      "provider-lifecycle-" + uid,
		"agent_name":       "lifecycle-agent",
		"certificate_type": "CAPABILITY",
		"claims": []map[string]interface{}{
			{
				"category":      "FINANCE",
				"capability":    "risk-analysis",
				"authorization": "SELF_ASSERTED",
			},
			{
				"category":      "FINANCE",
				"capability":    "portfolio-management",
				"authorization": "PROVIDER_ATTESTED",
			},
		},
		"public_key_pem": "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAElifecycle" + uid + "\n-----END PUBLIC KEY-----",
	}

	status, body, _ := httpDo(t, http.MethodPost, base+"/v1/certificates/request", reqPayload)
	if status != http.StatusCreated {
		t.Fatalf("request cert: expected 201, got %d: %v", status, body)
	}
	requestID := body["request_id"].(string)
	t.Logf("  request_id=%s", requestID)

	// ---- Step 2: Approve certificate ----
	t.Log("Step 2: Approve certificate")
	status, body, _ = httpDo(t, http.MethodPost,
		base+"/internal/v1/certificates/"+requestID+"/approve",
		map[string]interface{}{"reviewed_by": "e2e-lifecycle-test"},
	)
	if status != http.StatusOK {
		t.Fatalf("approve cert: expected 200, got %d: %v", status, body)
	}
	certID := body["certificate_id"].(string)
	if certID == "" {
		t.Fatal("approve response missing certificate_id")
	}
	if body["status"] != "ACTIVE" {
		t.Errorf("expected ACTIVE status, got %v", body["status"])
	}
	t.Logf("  certificate_id=%s", certID)

	// ---- Step 3: Get certificate ----
	t.Log("Step 3: Get certificate")
	status, body, _ = httpDo(t, http.MethodGet, base+"/v1/certificates/"+certID, nil)
	if status != http.StatusOK {
		t.Fatalf("get cert: expected 200, got %d", status)
	}
	if body["certificate_id"] != certID {
		t.Errorf("certificate_id mismatch: got %v, want %s", body["certificate_id"], certID)
	}
	if body["status"] != "ACTIVE" {
		t.Errorf("expected ACTIVE, got %v", body["status"])
	}
	if body["signature"] == nil || body["signature"] == "" {
		t.Error("certificate should have a signature")
	}
	if body["issuer_id"] != "aex-certauth" {
		t.Errorf("expected issuer_id=aex-certauth, got %v", body["issuer_id"])
	}

	// ---- Step 4: Verify certificate ----
	t.Log("Step 4: Verify certificate")
	status, body, _ = httpDo(t, http.MethodPost, base+"/v1/certificates/verify",
		map[string]interface{}{"certificate_id": certID},
	)
	if status != http.StatusOK {
		t.Fatalf("verify cert: expected 200, got %d", status)
	}
	if body["valid"] != true {
		t.Errorf("expected valid=true, got %v", body["valid"])
	}

	// ---- Step 5: Renew certificate ----
	t.Log("Step 5: Renew certificate")
	status, body, _ = httpDo(t, http.MethodPost, base+"/v1/certificates/"+certID+"/renew", nil)
	if status != http.StatusOK {
		t.Fatalf("renew cert: expected 200, got %d: %v", status, body)
	}
	renewedCertID := body["certificate_id"].(string)
	if renewedCertID == "" {
		t.Fatal("renewed cert missing certificate_id")
	}
	if renewedCertID == certID {
		t.Error("renewed cert should have a different ID")
	}
	if body["status"] != "ACTIVE" {
		t.Errorf("renewed cert status should be ACTIVE, got %v", body["status"])
	}
	renewalCount, _ := body["renewal_count"].(float64)
	if renewalCount < 1 {
		t.Errorf("expected renewal_count >= 1, got %v", renewalCount)
	}
	if body["previous_cert_id"] != certID {
		t.Errorf("expected previous_cert_id=%s, got %v", certID, body["previous_cert_id"])
	}
	t.Logf("  renewed_certificate_id=%s, renewal_count=%d", renewedCertID, int(renewalCount))

	// ---- Step 6: Revoke the renewed certificate ----
	t.Log("Step 6: Revoke certificate")
	status, body, _ = httpDo(t, http.MethodDelete, base+"/v1/certificates/"+renewedCertID,
		map[string]interface{}{"reason": "e2e lifecycle test revocation"},
	)
	if status != http.StatusOK {
		t.Fatalf("revoke cert: expected 200, got %d: %v", status, body)
	}

	// ---- Step 7: Verify revoked certificate ----
	t.Log("Step 7: Verify revoked certificate")
	status, body, _ = httpDo(t, http.MethodPost, base+"/v1/certificates/verify",
		map[string]interface{}{"certificate_id": renewedCertID},
	)
	if status != http.StatusOK {
		t.Fatalf("verify revoked: expected 200, got %d", status)
	}
	if body["valid"] != false {
		t.Errorf("revoked cert should be invalid, got valid=%v", body["valid"])
	}

	// ---- Step 8: Double-revoke should return 409 ----
	t.Log("Step 8: Double-revoke returns conflict")
	status, _, _ = httpDo(t, http.MethodDelete, base+"/v1/certificates/"+renewedCertID,
		map[string]interface{}{"reason": "double revoke"},
	)
	if status != http.StatusConflict {
		t.Errorf("double-revoke: expected 409, got %d", status)
	}

	// ---- Step 9: CRL ----
	t.Log("Step 9: Check CRL")
	status, body, _ = httpDo(t, http.MethodGet, base+"/v1/crl", nil)
	if status != http.StatusOK {
		t.Fatalf("CRL: expected 200, got %d", status)
	}
	if body["issuer"] != "aex-certauth" {
		t.Errorf("CRL issuer should be aex-certauth, got %v", body["issuer"])
	}
}

// ---------------------------------------------------------------------------
// Test: Search certificates by capability
// ---------------------------------------------------------------------------

func TestCertAuth_SearchCertificates(t *testing.T) {
	base := certauthBaseURL()
	uid := uniqueID()

	// Create and approve a certificate with a specific capability
	reqPayload := map[string]interface{}{
		"provider_id": "search-provider-" + uid,
		"agent_name":  "search-agent",
		"claims": []map[string]interface{}{
			{
				"category":      "HEALTHCARE",
				"capability":    "diagnosis-support-" + uid,
				"authorization": "SELF_ASSERTED",
			},
		},
		"public_key_pem": "-----BEGIN PUBLIC KEY-----\nsearch" + uid + "\n-----END PUBLIC KEY-----",
	}

	status, body, _ := httpDo(t, http.MethodPost, base+"/v1/certificates/request", reqPayload)
	if status != http.StatusCreated {
		t.Fatalf("create cert: expected 201, got %d", status)
	}
	requestID := body["request_id"].(string)

	status, body, _ = httpDo(t, http.MethodPost,
		base+"/internal/v1/certificates/"+requestID+"/approve",
		map[string]interface{}{"reviewed_by": "e2e-search"},
	)
	if status != http.StatusOK {
		t.Fatalf("approve cert: expected 200, got %d", status)
	}

	// Search by capability
	searchURL := base + "/v1/certificates/search?capability=diagnosis-support-" + uid + "&status=ACTIVE"
	status, body, _ = httpDo(t, http.MethodGet, searchURL, nil)
	if status != http.StatusOK {
		t.Fatalf("search: expected 200, got %d: %v", status, body)
	}

	total, _ := body["total"].(float64)
	if total < 1 {
		t.Errorf("expected at least 1 result, got %v", total)
	}
	t.Logf("search returned %d certificate(s)", int(total))
}

// ---------------------------------------------------------------------------
// Test: Get reputation
// ---------------------------------------------------------------------------

func TestCertAuth_GetReputation(t *testing.T) {
	base := certauthBaseURL()
	uid := uniqueID()

	// Create a provider with a certificate so reputation can be calculated
	reqPayload := map[string]interface{}{
		"provider_id": "rep-provider-" + uid,
		"agent_name":  "rep-agent",
		"claims": []map[string]interface{}{
			{
				"category":      "GENERAL",
				"capability":    "generic-task",
				"authorization": "SELF_ASSERTED",
			},
		},
		"public_key_pem": "-----BEGIN PUBLIC KEY-----\nrep" + uid + "\n-----END PUBLIC KEY-----",
	}

	status, body, _ := httpDo(t, http.MethodPost, base+"/v1/certificates/request", reqPayload)
	if status != http.StatusCreated {
		t.Fatalf("create cert: expected 201, got %d", status)
	}
	requestID := body["request_id"].(string)

	status, _, _ = httpDo(t, http.MethodPost,
		base+"/internal/v1/certificates/"+requestID+"/approve",
		map[string]interface{}{"reviewed_by": "e2e-rep"},
	)
	if status != http.StatusOK {
		t.Fatalf("approve cert: expected 200, got %d", status)
	}

	// Get reputation
	status, body, _ = httpDo(t, http.MethodGet, base+"/v1/providers/rep-provider-"+uid+"/reputation", nil)
	if status != http.StatusOK {
		t.Fatalf("get reputation: expected 200, got %d: %v", status, body)
	}

	if body["provider_id"] != "rep-provider-"+uid {
		t.Errorf("provider_id mismatch: %v", body["provider_id"])
	}
	if _, ok := body["overall_score"]; !ok {
		t.Error("response missing overall_score")
	}
	if _, ok := body["reputation_tier"]; !ok {
		t.Error("response missing reputation_tier")
	}

	t.Logf("reputation: score=%.4f, tier=%v, active_certs=%v",
		body["overall_score"], body["reputation_tier"], body["active_certificates"])
}

// ---------------------------------------------------------------------------
// Test: Leaderboard
// ---------------------------------------------------------------------------

func TestCertAuth_Leaderboard(t *testing.T) {
	base := certauthBaseURL()
	status, body, _ := httpDo(t, http.MethodGet, base+"/v1/reputation/leaderboard?limit=5", nil)
	if status != http.StatusOK {
		t.Fatalf("leaderboard: expected 200, got %d", status)
	}

	if _, ok := body["leaderboard"]; !ok {
		t.Error("response missing leaderboard field")
	}
	if _, ok := body["total"]; !ok {
		t.Error("response missing total field")
	}
	t.Logf("leaderboard: total=%v", body["total"])
}

// ---------------------------------------------------------------------------
// Test: Well-known endpoint
// ---------------------------------------------------------------------------

func TestCertAuth_WellKnown(t *testing.T) {
	base := certauthBaseURL()
	status, body, _ := httpDo(t, http.MethodGet, base+"/.well-known/aex-ca.json", nil)
	if status != http.StatusOK {
		t.Fatalf("well-known: expected 200, got %d", status)
	}

	if body["issuer"] != "aex-certauth" {
		t.Errorf("expected issuer=aex-certauth, got %v", body["issuer"])
	}
	if body["algorithm"] != "ECDSA-P256-SHA256" {
		t.Errorf("expected algorithm=ECDSA-P256-SHA256, got %v", body["algorithm"])
	}
	if body["public_key_pem"] == nil || body["public_key_pem"] == "" {
		t.Error("expected non-empty public_key_pem")
	}
	if body["certificate"] == nil || body["certificate"] == "" {
		t.Error("expected non-empty certificate")
	}
	t.Logf("CA algorithm=%v, has public key and certificate", body["algorithm"])
}

// ---------------------------------------------------------------------------
// Test: Get non-existent certificate returns 404
// ---------------------------------------------------------------------------

func TestCertAuth_GetCertificate_NotFound(t *testing.T) {
	base := certauthBaseURL()
	status, _, _ := httpDo(t, http.MethodGet, base+"/v1/certificates/nonexistent-cert-id-12345", nil)
	if status != http.StatusNotFound {
		t.Errorf("expected 404, got %d", status)
	}
}

// ---------------------------------------------------------------------------
// Test: Verify non-existent certificate
// ---------------------------------------------------------------------------

func TestCertAuth_VerifyCertificate_NotFound(t *testing.T) {
	base := certauthBaseURL()
	status, body, _ := httpDo(t, http.MethodPost, base+"/v1/certificates/verify",
		map[string]interface{}{"certificate_id": "nonexistent-cert-id-99999"},
	)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if body["valid"] != false {
		t.Errorf("non-existent cert should be invalid, got valid=%v", body["valid"])
	}
}

// ---------------------------------------------------------------------------
// Test: Renew a non-active certificate fails
// ---------------------------------------------------------------------------

func TestCertAuth_RenewRevokedCertificate(t *testing.T) {
	base := certauthBaseURL()
	uid := uniqueID()

	// Create, approve, revoke
	status, body, _ := httpDo(t, http.MethodPost, base+"/v1/certificates/request", map[string]interface{}{
		"provider_id": "renew-revoke-" + uid,
		"agent_name":  "renew-revoke-agent",
		"claims": []map[string]interface{}{
			{"category": "GENERAL", "capability": "test-renew-" + uid, "authorization": "SELF_ASSERTED"},
		},
		"public_key_pem": "-----BEGIN PUBLIC KEY-----\nrenew-revoke" + uid + "\n-----END PUBLIC KEY-----",
	})
	if status != http.StatusCreated {
		t.Fatalf("request cert: expected 201, got %d", status)
	}
	requestID := body["request_id"].(string)

	status, body, _ = httpDo(t, http.MethodPost,
		base+"/internal/v1/certificates/"+requestID+"/approve",
		map[string]interface{}{"reviewed_by": "e2e"},
	)
	if status != http.StatusOK {
		t.Fatalf("approve: expected 200, got %d", status)
	}
	certID := body["certificate_id"].(string)

	// Revoke
	status, _, _ = httpDo(t, http.MethodDelete, base+"/v1/certificates/"+certID,
		map[string]interface{}{"reason": "test"},
	)
	if status != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d", status)
	}

	// Try to renew revoked cert -- should fail
	status, _, _ = httpDo(t, http.MethodPost, base+"/v1/certificates/"+certID+"/renew", nil)
	if status != http.StatusConflict {
		t.Errorf("renew revoked: expected 409, got %d", status)
	}
}

// ---------------------------------------------------------------------------
// Test: Approve already-approved request returns conflict
// ---------------------------------------------------------------------------

func TestCertAuth_DoubleApprove(t *testing.T) {
	base := certauthBaseURL()
	uid := uniqueID()

	status, body, _ := httpDo(t, http.MethodPost, base+"/v1/certificates/request", map[string]interface{}{
		"provider_id": "double-approve-" + uid,
		"agent_name":  "double-approve-agent",
		"claims": []map[string]interface{}{
			{"category": "GENERAL", "capability": "test-double-" + uid, "authorization": "SELF_ASSERTED"},
		},
		"public_key_pem": "-----BEGIN PUBLIC KEY-----\ndouble" + uid + "\n-----END PUBLIC KEY-----",
	})
	if status != http.StatusCreated {
		t.Fatalf("request: expected 201, got %d", status)
	}
	requestID := body["request_id"].(string)

	// First approve
	status, _, _ = httpDo(t, http.MethodPost,
		base+"/internal/v1/certificates/"+requestID+"/approve",
		map[string]interface{}{"reviewed_by": "e2e"},
	)
	if status != http.StatusOK {
		t.Fatalf("first approve: expected 200, got %d", status)
	}

	// Second approve -- should conflict
	status, _, _ = httpDo(t, http.MethodPost,
		base+"/internal/v1/certificates/"+requestID+"/approve",
		map[string]interface{}{"reviewed_by": "e2e"},
	)
	if status != http.StatusConflict {
		t.Errorf("double approve: expected 409, got %d", status)
	}
}

// ---------------------------------------------------------------------------
// Test: Reject certificate request
// ---------------------------------------------------------------------------

func TestCertAuth_RejectCertificateRequest(t *testing.T) {
	base := certauthBaseURL()
	uid := uniqueID()

	status, body, _ := httpDo(t, http.MethodPost, base+"/v1/certificates/request", map[string]interface{}{
		"provider_id": "reject-test-" + uid,
		"agent_name":  "reject-agent",
		"claims": []map[string]interface{}{
			{"category": "GENERAL", "capability": "test-reject-" + uid, "authorization": "SELF_ASSERTED"},
		},
		"public_key_pem": "-----BEGIN PUBLIC KEY-----\nreject" + uid + "\n-----END PUBLIC KEY-----",
	})
	if status != http.StatusCreated {
		t.Fatalf("request: expected 201, got %d", status)
	}
	requestID := body["request_id"].(string)

	// Reject
	status, body, _ = httpDo(t, http.MethodPost,
		base+"/internal/v1/certificates/"+requestID+"/reject",
		map[string]interface{}{"reviewed_by": "e2e", "reason": "test rejection"},
	)
	if status != http.StatusOK {
		t.Fatalf("reject: expected 200, got %d: %v", status, body)
	}

	// Trying to approve after reject should conflict
	status, _, _ = httpDo(t, http.MethodPost,
		base+"/internal/v1/certificates/"+requestID+"/approve",
		map[string]interface{}{"reviewed_by": "e2e"},
	)
	if status != http.StatusConflict {
		t.Errorf("approve after reject: expected 409, got %d", status)
	}
}

// ---------------------------------------------------------------------------
// Test: Batch verify (internal API)
// ---------------------------------------------------------------------------

func TestCertAuth_BatchVerify(t *testing.T) {
	base := certauthBaseURL()
	uid := uniqueID()

	// Create and approve a cert
	status, body, _ := httpDo(t, http.MethodPost, base+"/v1/certificates/request", map[string]interface{}{
		"provider_id": "batch-verify-" + uid,
		"agent_name":  "batch-agent",
		"claims": []map[string]interface{}{
			{"category": "GENERAL", "capability": "batch-test", "authorization": "SELF_ASSERTED"},
		},
		"public_key_pem": "-----BEGIN PUBLIC KEY-----\nbatch" + uid + "\n-----END PUBLIC KEY-----",
	})
	if status != http.StatusCreated {
		t.Fatalf("request: expected 201, got %d", status)
	}

	status, body, _ = httpDo(t, http.MethodPost,
		base+"/internal/v1/certificates/"+body["request_id"].(string)+"/approve",
		map[string]interface{}{"reviewed_by": "e2e"},
	)
	if status != http.StatusOK {
		t.Fatalf("approve: expected 200, got %d", status)
	}
	certID := body["certificate_id"].(string)

	// Batch verify with one real and one fake ID
	status, body, _ = httpDo(t, http.MethodPost, base+"/internal/v1/certificates/batch-verify",
		map[string]interface{}{
			"certificate_ids": []string{certID, "fake-cert-id-does-not-exist"},
		},
	)
	if status != http.StatusOK {
		t.Fatalf("batch verify: expected 200, got %d", status)
	}

	results, ok := body["results"].(map[string]interface{})
	if !ok {
		t.Fatal("batch verify response missing results map")
	}

	// Check real cert
	realResult, ok := results[certID].(map[string]interface{})
	if !ok {
		t.Fatal("missing result for real cert")
	}
	if realResult["valid"] != true {
		t.Errorf("real cert should be valid, got %v", realResult["valid"])
	}

	// Check fake cert
	fakeResult, ok := results["fake-cert-id-does-not-exist"].(map[string]interface{})
	if !ok {
		t.Fatal("missing result for fake cert")
	}
	if fakeResult["valid"] != false {
		t.Errorf("fake cert should be invalid, got %v", fakeResult["valid"])
	}
}

// ---------------------------------------------------------------------------
// Test: Can-perform internal API
// ---------------------------------------------------------------------------

func TestCertAuth_CanPerform(t *testing.T) {
	base := certauthBaseURL()
	uid := uniqueID()

	capability := "can-perform-capability-" + uid

	// Create and approve a certificate with the capability
	status, body, _ := httpDo(t, http.MethodPost, base+"/v1/certificates/request", map[string]interface{}{
		"provider_id": "canperform-provider-" + uid,
		"agent_name":  "canperform-agent",
		"claims": []map[string]interface{}{
			{"category": "TECHNOLOGY", "capability": capability, "authorization": "SELF_ASSERTED"},
		},
		"public_key_pem": "-----BEGIN PUBLIC KEY-----\ncanperform" + uid + "\n-----END PUBLIC KEY-----",
	})
	if status != http.StatusCreated {
		t.Fatalf("request: expected 201, got %d", status)
	}

	status, _, _ = httpDo(t, http.MethodPost,
		base+"/internal/v1/certificates/"+body["request_id"].(string)+"/approve",
		map[string]interface{}{"reviewed_by": "e2e"},
	)
	if status != http.StatusOK {
		t.Fatalf("approve: expected 200, got %d", status)
	}

	// Check can-perform: should be true
	status, body, _ = httpDo(t, http.MethodGet,
		base+"/internal/v1/providers/canperform-provider-"+uid+"/can-perform?capability="+capability,
		nil,
	)
	if status != http.StatusOK {
		t.Fatalf("can-perform: expected 200, got %d", status)
	}
	if body["can_perform"] != true {
		t.Errorf("expected can_perform=true, got %v", body["can_perform"])
	}

	// Check can-perform with wrong capability: should be false
	status, body, _ = httpDo(t, http.MethodGet,
		base+"/internal/v1/providers/canperform-provider-"+uid+"/can-perform?capability=nonexistent-capability",
		nil,
	)
	if status != http.StatusOK {
		t.Fatalf("can-perform: expected 200, got %d", status)
	}
	if body["can_perform"] != false {
		t.Errorf("expected can_perform=false for wrong capability, got %v", body["can_perform"])
	}
}

// ---------------------------------------------------------------------------
// Test: Provider certificates list
// ---------------------------------------------------------------------------

func TestCertAuth_GetProviderCertificates(t *testing.T) {
	base := certauthBaseURL()
	uid := uniqueID()
	providerID := "list-certs-provider-" + uid

	// Create and approve two certificates for the same provider
	for i := 0; i < 2; i++ {
		status, body, _ := httpDo(t, http.MethodPost, base+"/v1/certificates/request", map[string]interface{}{
			"provider_id": providerID,
			"agent_name":  fmt.Sprintf("list-agent-%d", i),
			"claims": []map[string]interface{}{
				{"category": "GENERAL", "capability": fmt.Sprintf("list-cap-%d-%s", i, uid), "authorization": "SELF_ASSERTED"},
			},
			"public_key_pem": fmt.Sprintf("-----BEGIN PUBLIC KEY-----\nlistcerts%d%s\n-----END PUBLIC KEY-----", i, uid),
		})
		if status != http.StatusCreated {
			t.Fatalf("request %d: expected 201, got %d", i, status)
		}
		reqID := body["request_id"].(string)

		status, _, _ = httpDo(t, http.MethodPost,
			base+"/internal/v1/certificates/"+reqID+"/approve",
			map[string]interface{}{"reviewed_by": "e2e"},
		)
		if status != http.StatusOK {
			t.Fatalf("approve %d: expected 200, got %d", i, status)
		}
	}

	// List all certs for the provider
	status, body, _ := httpDo(t, http.MethodGet,
		base+"/v1/providers/"+providerID+"/certificates",
		nil,
	)
	if status != http.StatusOK {
		t.Fatalf("list provider certs: expected 200, got %d", status)
	}

	total, _ := body["total"].(float64)
	if total < 2 {
		t.Errorf("expected at least 2 certificates, got %v", total)
	}
	t.Logf("provider %s has %d certificates", providerID, int(total))
}
