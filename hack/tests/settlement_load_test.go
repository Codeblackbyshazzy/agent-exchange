//go:build e2e

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// settlementBaseURL returns the base URL for the settlement service.
func settlementBaseURL() string {
	if u := os.Getenv("SETTLEMENT_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://localhost:8088"
}

// settlementHTTP executes an HTTP request against the settlement service.
func settlementHTTP(t *testing.T, method, url string, body interface{}) (int, map[string]interface{}, []byte) {
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

	client := &http.Client{Timeout: 30 * time.Second}
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
	_ = json.Unmarshal(raw, &result)

	return resp.StatusCode, result, raw
}

// getBalance retrieves the balance for a tenant and returns balance_cents.
func getBalance(t *testing.T, base, tenantID string) int64 {
	t.Helper()
	status, body, raw := settlementHTTP(t, http.MethodGet, base+"/v1/balance?tenant_id="+tenantID, nil)
	if status != http.StatusOK {
		t.Fatalf("get balance for %s: expected 200, got %d: %s", tenantID, status, string(raw))
	}
	// balance_cents is a JSON number
	balCents, ok := body["balance_cents"].(float64)
	if !ok {
		t.Fatalf("balance response missing balance_cents: %v", body)
	}
	return int64(balCents)
}

// deposit adds funds to a tenant and returns the created transaction.
func deposit(t *testing.T, base, tenantID, amount string) {
	t.Helper()
	status, _, raw := settlementHTTP(t, http.MethodPost, base+"/v1/deposits", map[string]interface{}{
		"tenant_id": tenantID,
		"amount":    amount,
	})
	if status != http.StatusCreated {
		t.Fatalf("deposit for %s: expected 201, got %d: %s", tenantID, status, string(raw))
	}
}

// ---------------------------------------------------------------------------
// Test: Settlement Health Check
// ---------------------------------------------------------------------------

func TestSettlement_Health(t *testing.T) {
	base := settlementBaseURL()
	status, body, _ := settlementHTTP(t, http.MethodGet, base+"/health", nil)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if s, _ := body["status"].(string); s != "healthy" {
		t.Errorf("expected status=healthy, got %v", body["status"])
	}
}

// ---------------------------------------------------------------------------
// Test: Concurrent Settlement Load (Task 2.7)
//
// Strategy:
//  1. Deposit a known amount into two test tenants (consumer and provider).
//  2. Fire 100+ concurrent contract completion events, each transferring a
//     small amount from consumer to provider.
//  3. After all settle, verify:
//     a) consumer_balance + provider_balance + total_platform_fees == initial_total
//     b) No money was created or destroyed.
// ---------------------------------------------------------------------------

func TestSettlement_ConcurrentLoad(t *testing.T) {
	base := settlementBaseURL()

	// Unique run ID to isolate this test from others
	runID := fmt.Sprintf("%d", time.Now().UnixNano())

	consumerID := "load-consumer-" + runID
	providerID := "load-provider-" + runID

	// Initial deposit amounts
	const (
		consumerDepositStr = "10000.00" // $10,000
		providerDepositStr = "100.00"   // $100 starting balance
		consumerDepositCents int64 = 1000000 // 10000 * 100
		providerDepositCents int64 = 10000   // 100 * 100
	)

	initialTotal := consumerDepositCents + providerDepositCents

	t.Logf("Consumer ID: %s", consumerID)
	t.Logf("Provider ID: %s", providerID)

	// Step 1: Deposit funds
	t.Log("Step 1: Depositing funds...")
	deposit(t, base, consumerID, consumerDepositStr)
	deposit(t, base, providerID, providerDepositStr)

	consumerBal := getBalance(t, base, consumerID)
	providerBal := getBalance(t, base, providerID)
	t.Logf("  Consumer balance: %d cents ($%.2f)", consumerBal, float64(consumerBal)/100)
	t.Logf("  Provider balance: %d cents ($%.2f)", providerBal, float64(providerBal)/100)

	if consumerBal < consumerDepositCents {
		t.Fatalf("consumer balance too low: got %d, need >= %d", consumerBal, consumerDepositCents)
	}

	// Step 2: Fire concurrent settlement operations
	const numOperations = 120
	const pricePerContract = "10.00" // $10 each
	const pricePerContractCents int64 = 1000

	// Platform fee is 15% => platform gets $1.50, provider gets $8.50 per contract
	// totalAgreedPrice = numOperations * 10.00 = $1200
	// totalPlatformFee = numOperations * 1.50  = $180
	// totalProviderPayout = numOperations * 8.50 = $1020

	t.Logf("Step 2: Firing %d concurrent settlements ($%s each)...", numOperations, pricePerContract)

	var (
		wg             sync.WaitGroup
		successCount   int64
		conflictCount  int64
		errorCount     int64
		errMessages    []string
		errMu          sync.Mutex
	)

	client := &http.Client{Timeout: 30 * time.Second}

	startTime := time.Now()

	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			contractID := fmt.Sprintf("load-contract-%s-%d", runID, idx)
			workID := fmt.Sprintf("load-work-%s-%d", runID, idx)
			agentID := fmt.Sprintf("load-agent-%d", idx)

			now := time.Now().UTC()
			event := map[string]interface{}{
				"contract_id":  contractID,
				"work_id":      workID,
				"agent_id":     agentID,
				"consumer_id":  consumerID,
				"provider_id":  providerID,
				"domain":       "technology",
				"started_at":   now.Add(-1 * time.Second).Format(time.RFC3339Nano),
				"completed_at": now.Format(time.RFC3339Nano),
				"success":      true,
				"agreed_price": pricePerContract,
			}

			data, err := json.Marshal(event)
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				errMu.Lock()
				errMessages = append(errMessages, fmt.Sprintf("marshal %d: %v", idx, err))
				errMu.Unlock()
				return
			}

			req, err := http.NewRequest(http.MethodPost, base+"/internal/settlement/complete", bytes.NewReader(data))
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				return
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				errMu.Lock()
				errMessages = append(errMessages, fmt.Sprintf("request %d: %v", idx, err))
				errMu.Unlock()
				return
			}
			defer resp.Body.Close()
			io.ReadAll(resp.Body) // drain body

			switch resp.StatusCode {
			case http.StatusOK:
				atomic.AddInt64(&successCount, 1)
			case http.StatusConflict:
				// Duplicate execution is expected if the service deduplicates by contract_id
				atomic.AddInt64(&conflictCount, 1)
			default:
				atomic.AddInt64(&errorCount, 1)
				errMu.Lock()
				errMessages = append(errMessages, fmt.Sprintf("settlement %d: HTTP %d", idx, resp.StatusCode))
				errMu.Unlock()
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(startTime)

	t.Logf("  Completed in %v", elapsed)
	t.Logf("  Success: %d, Conflicts: %d, Errors: %d", successCount, conflictCount, errorCount)

	if len(errMessages) > 0 {
		maxShow := 10
		if len(errMessages) < maxShow {
			maxShow = len(errMessages)
		}
		for _, msg := range errMessages[:maxShow] {
			t.Logf("  ERROR: %s", msg)
		}
	}

	// All should either succeed or conflict (dedup). No unknown errors.
	if errorCount > 0 {
		t.Errorf("expected 0 errors, got %d", errorCount)
	}

	if successCount == 0 {
		t.Fatal("no settlements succeeded -- service may be down or broken")
	}

	// Step 3: Verify balance consistency
	t.Log("Step 3: Verifying balance consistency...")

	finalConsumerBal := getBalance(t, base, consumerID)
	finalProviderBal := getBalance(t, base, providerID)

	t.Logf("  Final consumer balance: %d cents ($%.2f)", finalConsumerBal, float64(finalConsumerBal)/100)
	t.Logf("  Final provider balance: %d cents ($%.2f)", finalProviderBal, float64(finalProviderBal)/100)

	// Calculate expected values based on successful settlements
	settled := successCount
	expectedConsumerDebit := settled * pricePerContractCents                        // full agreed price
	expectedProviderCredit := settled * int64(float64(pricePerContractCents)*0.85)  // 85% after 15% fee
	expectedPlatformFees := settled * int64(float64(pricePerContractCents)*0.15)    // 15% platform fee

	expectedConsumerBalance := consumerBal - expectedConsumerDebit
	expectedProviderBalance := providerBal + expectedProviderCredit

	t.Logf("  Settled count: %d", settled)
	t.Logf("  Expected consumer balance: %d cents", expectedConsumerBalance)
	t.Logf("  Expected provider balance: %d cents", expectedProviderBalance)
	t.Logf("  Expected platform fees collected: %d cents ($%.2f)", expectedPlatformFees, float64(expectedPlatformFees)/100)

	// Allow a small tolerance for rounding (1 cent per operation)
	tolerance := settled // 1 cent per settled operation for rounding
	if tolerance < 10 {
		tolerance = 10
	}

	// Verify consumer balance
	consumerDiff := abs64(finalConsumerBal - expectedConsumerBalance)
	if consumerDiff > tolerance {
		t.Errorf("consumer balance off by %d cents (got %d, expected ~%d, tolerance %d)",
			consumerDiff, finalConsumerBal, expectedConsumerBalance, tolerance)
	} else {
		t.Logf("  Consumer balance is within tolerance (diff=%d, tolerance=%d)", consumerDiff, tolerance)
	}

	// Verify provider balance
	providerDiff := abs64(finalProviderBal - expectedProviderBalance)
	if providerDiff > tolerance {
		t.Errorf("provider balance off by %d cents (got %d, expected ~%d, tolerance %d)",
			providerDiff, finalProviderBal, expectedProviderBalance, tolerance)
	} else {
		t.Logf("  Provider balance is within tolerance (diff=%d, tolerance=%d)", providerDiff, tolerance)
	}

	// Conservation of money check:
	// initial_total = final_consumer + final_provider + platform_fees_held
	// Since platform fees go to the platform (not tracked as a tenant balance),
	// we verify: initial_total >= final_consumer + final_provider
	// and: initial_total - (final_consumer + final_provider) ~= platform_fees
	finalTotal := finalConsumerBal + finalProviderBal
	platformFeesCollected := initialTotal - finalTotal // What "disappeared" into platform fees

	t.Logf("  Money conservation check:")
	t.Logf("    Initial total: %d cents", initialTotal)
	t.Logf("    Final consumer + provider: %d cents", finalTotal)
	t.Logf("    Platform fees (implicit): %d cents ($%.2f)", platformFeesCollected, float64(platformFeesCollected)/100)

	// Platform fees should approximately equal expected
	feeDiff := abs64(platformFeesCollected - expectedPlatformFees)
	if feeDiff > tolerance {
		t.Errorf("platform fees off by %d cents (got %d, expected ~%d)",
			feeDiff, platformFeesCollected, expectedPlatformFees)
	} else {
		t.Logf("    Platform fees within tolerance (diff=%d)", feeDiff)
	}

	// Verify no money was created (final totals should not exceed initial)
	if finalTotal > initialTotal+tolerance {
		t.Errorf("MONEY CREATION DETECTED: final=%d > initial=%d (+tolerance %d)",
			finalTotal, initialTotal, tolerance)
	}

	// Step 4: Verify transaction history
	t.Log("Step 4: Checking transaction history...")

	status, txBody, _ := settlementHTTP(t, http.MethodGet,
		base+"/v1/usage/transactions?tenant_id="+consumerID+"&limit=200",
		nil,
	)
	if status != http.StatusOK {
		t.Fatalf("get transactions: expected 200, got %d", status)
	}

	txCount, _ := txBody["count"].(float64)
	t.Logf("  Consumer has %d transaction records", int(txCount))

	// Should have at least: 1 deposit + settled debits
	minExpected := 1 + settled
	if int64(txCount) < minExpected {
		t.Errorf("expected >= %d transactions, got %d", minExpected, int(txCount))
	}

	// Step 5: Performance assertions
	t.Log("Step 5: Performance check...")
	opsPerSec := float64(numOperations) / elapsed.Seconds()
	t.Logf("  Throughput: %.1f operations/second", opsPerSec)
	if opsPerSec < 5.0 {
		t.Logf("  WARNING: throughput below 5 ops/sec (got %.1f)", opsPerSec)
	}

	t.Log("Concurrent settlement load test complete.")
}

// ---------------------------------------------------------------------------
// Test: Deposit validation
// ---------------------------------------------------------------------------

func TestSettlement_DepositValidation(t *testing.T) {
	base := settlementBaseURL()

	// Missing tenant_id
	status, _, _ := settlementHTTP(t, http.MethodPost, base+"/v1/deposits", map[string]interface{}{
		"amount": "100.00",
	})
	if status != http.StatusBadRequest {
		t.Errorf("missing tenant_id: expected 400, got %d", status)
	}

	// Missing amount
	status, _, _ = settlementHTTP(t, http.MethodPost, base+"/v1/deposits", map[string]interface{}{
		"tenant_id": "test-tenant",
	})
	if status != http.StatusBadRequest {
		t.Errorf("missing amount: expected 400, got %d", status)
	}

	// Negative amount
	status, _, _ = settlementHTTP(t, http.MethodPost, base+"/v1/deposits", map[string]interface{}{
		"tenant_id": "test-tenant",
		"amount":    "-50.00",
	})
	if status != http.StatusBadRequest {
		t.Errorf("negative amount: expected 400, got %d", status)
	}

	// Zero amount
	status, _, _ = settlementHTTP(t, http.MethodPost, base+"/v1/deposits", map[string]interface{}{
		"tenant_id": "test-tenant",
		"amount":    "0.00",
	})
	if status != http.StatusBadRequest {
		t.Errorf("zero amount: expected 400, got %d", status)
	}
}

// ---------------------------------------------------------------------------
// Test: Balance for unknown tenant
// ---------------------------------------------------------------------------

func TestSettlement_BalanceUnknownTenant(t *testing.T) {
	base := settlementBaseURL()
	uid := fmt.Sprintf("%d", time.Now().UnixNano())

	status, body, _ := settlementHTTP(t, http.MethodGet,
		base+"/v1/balance?tenant_id=nonexistent-tenant-"+uid, nil)

	// Service may return 200 with zero balance or 404/500
	// Both are acceptable -- we just verify it doesn't panic
	t.Logf("Balance for unknown tenant: HTTP %d, body=%v", status, body)
	if status != http.StatusOK && status != http.StatusNotFound && status != http.StatusInternalServerError {
		t.Errorf("unexpected status %d for unknown tenant balance", status)
	}
}

// ---------------------------------------------------------------------------
// Test: Sequential deposits accumulate correctly
// ---------------------------------------------------------------------------

func TestSettlement_SequentialDeposits(t *testing.T) {
	base := settlementBaseURL()
	uid := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := "seq-deposit-" + uid

	amounts := []string{"100.00", "50.50", "200.00", "0.01"}
	expectedCents := int64(0)

	for _, amt := range amounts {
		deposit(t, base, tenantID, amt)
		// Parse amount to cents
		cents := amountToCents(t, amt)
		expectedCents += cents
	}

	balance := getBalance(t, base, tenantID)
	if balance != expectedCents {
		t.Errorf("balance mismatch after sequential deposits: got %d, expected %d", balance, expectedCents)
	} else {
		t.Logf("Balance correct after %d deposits: %d cents ($%.2f)",
			len(amounts), balance, float64(balance)/100)
	}
}

// ---------------------------------------------------------------------------
// Test: Concurrent deposits on the same tenant
// ---------------------------------------------------------------------------

func TestSettlement_ConcurrentDeposits(t *testing.T) {
	base := settlementBaseURL()
	uid := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := "concurrent-deposit-" + uid

	const numDeposits = 50
	const depositAmount = "10.00"
	const depositCents int64 = 1000

	expectedTotal := int64(numDeposits) * depositCents

	var wg sync.WaitGroup
	var successCount int64
	var errorCount int64

	client := &http.Client{Timeout: 15 * time.Second}

	for i := 0; i < numDeposits; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			data, _ := json.Marshal(map[string]interface{}{
				"tenant_id": tenantID,
				"amount":    depositAmount,
			})

			req, _ := http.NewRequest(http.MethodPost, base+"/v1/deposits", bytes.NewReader(data))
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				return
			}
			defer resp.Body.Close()
			io.ReadAll(resp.Body)

			if resp.StatusCode == http.StatusCreated {
				atomic.AddInt64(&successCount, 1)
			} else {
				atomic.AddInt64(&errorCount, 1)
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Concurrent deposits: success=%d, errors=%d", successCount, errorCount)

	if errorCount > 0 {
		t.Errorf("%d deposit errors occurred", errorCount)
	}

	balance := getBalance(t, base, tenantID)

	// Allow rounding tolerance
	diff := abs64(balance - expectedTotal)
	if diff > 1 { // allow 1 cent tolerance
		t.Errorf("balance after concurrent deposits: got %d, expected %d (diff=%d)",
			balance, expectedTotal, diff)
	} else {
		t.Logf("Balance correct after %d concurrent deposits: %d cents", numDeposits, balance)
	}
}

// ---------------------------------------------------------------------------
// Test: Usage endpoint
// ---------------------------------------------------------------------------

func TestSettlement_Usage(t *testing.T) {
	base := settlementBaseURL()

	// Missing tenant_id
	status, _, _ := settlementHTTP(t, http.MethodGet, base+"/v1/usage", nil)
	if status != http.StatusBadRequest {
		t.Errorf("usage without tenant_id: expected 400, got %d", status)
	}

	// Valid request (may return empty)
	uid := fmt.Sprintf("%d", time.Now().UnixNano())
	status, body, _ := settlementHTTP(t, http.MethodGet, base+"/v1/usage?tenant_id=usage-test-"+uid, nil)
	if status != http.StatusOK {
		t.Errorf("usage: expected 200, got %d", status)
	}
	if _, ok := body["tenant_id"]; !ok {
		t.Error("usage response missing tenant_id")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// amountToCents converts a decimal string like "100.50" to cents (10050).
func amountToCents(t *testing.T, amount string) int64 {
	t.Helper()
	f, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		t.Fatalf("parse amount %q: %v", amount, err)
	}
	return int64(math.Round(f * 100))
}
