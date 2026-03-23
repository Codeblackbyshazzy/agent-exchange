#!/usr/bin/env bash
#
# Task 5.2: Test event propagation through NATS JetStream
#
# Verifies that events published by AEX services are delivered through NATS
# JetStream streams. Uses the NATS monitoring HTTP API to check stream
# message counts before and after triggering actions via service HTTP APIs.
#
# Prerequisites:
#   - docker-compose services running (hack/docker-compose.yml)
#   - NATS monitoring accessible on localhost:8222
#   - curl, jq installed
#
set -euo pipefail

NATS_MONITOR_URL="${NATS_MONITOR_URL:-http://localhost:8222}"
WORK_PUBLISHER_URL="${WORK_PUBLISHER_URL:-http://localhost:8081}"
PROVIDER_REGISTRY_URL="${PROVIDER_REGISTRY_URL:-http://localhost:8085}"
CERTAUTH_URL="${CERTAUTH_URL:-http://localhost:8091}"

PASS_COUNT=0
FAIL_COUNT=0

if [ -t 1 ]; then
  GREEN='\033[0;32m'; RED='\033[0;31m'; CYAN='\033[0;36m'; NC='\033[0m'
else
  GREEN='' RED='' CYAN='' NC=''
fi

log_header() { echo -e "\n${CYAN}====== $1 ======${NC}"; }
log_pass()   { echo -e "  ${GREEN}PASS${NC}: $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
log_fail()   { echo -e "  ${RED}FAIL${NC}: $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
log_info()   { echo -e "  ${CYAN}INFO${NC}: $1"; }

RUN_ID="$(date +%s)-$$"

# ---------------------------------------------------------------------------
# Helper: get stream message count via NATS HTTP monitoring API
# ---------------------------------------------------------------------------
get_stream_msgs() {
  local stream="$1"
  curl -s "${NATS_MONITOR_URL}/jsz?streams=true" 2>/dev/null \
    | jq -r --arg s "$stream" '[.account_details[]?.stream_detail[]? | select(.name == $s) | .state.messages] | .[0] // 0' 2>/dev/null \
    || echo "0"
}

# ---------------------------------------------------------------------------
# Helper: get list of all stream names
# ---------------------------------------------------------------------------
get_stream_names() {
  curl -s "${NATS_MONITOR_URL}/jsz?streams=true" 2>/dev/null \
    | jq -r '[.account_details[]?.stream_detail[]?.name] | .[]' 2>/dev/null \
    || echo ""
}

# ---------------------------------------------------------------------------
# Test 1: Verify NATS JetStream is running
# ---------------------------------------------------------------------------
log_header "NATS JetStream Health"

NATS_HEALTH=$(curl -s "${NATS_MONITOR_URL}/healthz" 2>/dev/null || echo "{}")
if echo "$NATS_HEALTH" | jq -e '.status == "ok"' >/dev/null 2>&1; then
  log_pass "NATS server is healthy"
else
  log_fail "NATS server is not healthy: $NATS_HEALTH"
fi

JSZ=$(curl -s "${NATS_MONITOR_URL}/jsz" 2>/dev/null || echo "{}")
if echo "$JSZ" | jq -e '.streams >= 0' >/dev/null 2>&1; then
  STREAM_COUNT=$(echo "$JSZ" | jq -r '.streams')
  log_pass "JetStream is enabled ($STREAM_COUNT streams)"
else
  log_fail "JetStream may not be enabled"
fi

# ---------------------------------------------------------------------------
# Test 2: Verify expected streams exist
# ---------------------------------------------------------------------------
log_header "JetStream Streams"

EXPECTED_STREAMS="WORK BID CONTRACT SETTLEMENT TRUST CERTIFICATE DEADLETTER"
ACTUAL_STREAMS=$(get_stream_names)

for stream in $EXPECTED_STREAMS; do
  if echo "$ACTUAL_STREAMS" | grep -q "^${stream}$"; then
    log_pass "Stream $stream exists"
  else
    log_fail "Stream $stream not found"
  fi
done

# ---------------------------------------------------------------------------
# Test 3: Publish a work spec and verify event appears in WORK stream
# ---------------------------------------------------------------------------
log_header "Event Propagation: Work Submission"

INITIAL_WORK_MSGS=$(get_stream_msgs "WORK")
log_info "Initial WORK stream messages: $INITIAL_WORK_MSGS"

# Register a provider first (needed for work submission)
REG_RESP=$(curl -s -X POST "$PROVIDER_REGISTRY_URL/v1/providers" \
  -H 'Content-Type: application/json' \
  -d "{
    \"name\": \"nats-test-provider-${RUN_ID}\",
    \"type\": \"AI_AGENT\",
    \"capabilities\": [\"testing\"],
    \"category\": \"TESTING\"
  }" 2>/dev/null || echo "{}")

PROVIDER_ID=$(echo "$REG_RESP" | jq -r '.provider_id // .id // empty' 2>/dev/null || echo "")
log_info "Provider registered: $PROVIDER_ID"

# Submit a work spec
WORK_RESP=$(curl -s -X POST "$WORK_PUBLISHER_URL/v1/work" \
  -H 'Content-Type: application/json' \
  -H "X-Tenant-ID: tenant-nats-test-${RUN_ID}" \
  -d "{
    \"title\": \"NATS Event Test ${RUN_ID}\",
    \"description\": \"Testing event propagation through NATS\",
    \"category\": \"TESTING\",
    \"budget\": {\"min_price\": 10, \"max_price\": 100, \"currency\": \"USD\"},
    \"requirements\": {\"test\": true},
    \"success_criteria\": [{\"metric\": \"completion\", \"threshold\": 1.0}]
  }" 2>/dev/null || echo "{}")

WORK_ID=$(echo "$WORK_RESP" | jq -r '.work_id // .id // empty' 2>/dev/null || echo "")

if [ -n "$WORK_ID" ] && [ "$WORK_ID" != "null" ]; then
  log_pass "Work spec submitted: $WORK_ID"

  # Wait for async event propagation
  sleep 2

  NEW_WORK_MSGS=$(get_stream_msgs "WORK")
  log_info "New WORK stream messages: $NEW_WORK_MSGS"

  if [ "$NEW_WORK_MSGS" -gt "$INITIAL_WORK_MSGS" ] 2>/dev/null; then
    log_pass "work.submitted event propagated to WORK stream (count: $INITIAL_WORK_MSGS -> $NEW_WORK_MSGS)"
  else
    log_fail "work.submitted event NOT detected in WORK stream"
  fi
else
  log_fail "Failed to submit work spec for event test"
  log_info "Response: $WORK_RESP"
fi

# ---------------------------------------------------------------------------
# Test 4: Request a certificate and verify event in CERTIFICATE stream
# ---------------------------------------------------------------------------
log_header "Event Propagation: Certificate Request"

INITIAL_CERT_MSGS=$(get_stream_msgs "CERTIFICATE")
log_info "Initial CERTIFICATE stream messages: $INITIAL_CERT_MSGS"

CERT_RESP=$(curl -s -X POST "$CERTAUTH_URL/v1/certificates/request" \
  -H 'Content-Type: application/json' \
  -d "{
    \"tenant_id\": \"tenant-nats-test-${RUN_ID}\",
    \"provider_id\": \"prov-nats-test-${RUN_ID}\",
    \"agent_name\": \"nats-test-agent\",
    \"claims\": [{
      \"category\": \"TESTING\",
      \"capability\": \"event.propagation\",
      \"scope\": \"nats-test\",
      \"authorization\": \"SELF_ASSERTED\"
    }],
    \"public_key_pem\": \"test-key-nats-${RUN_ID}\"
  }" 2>/dev/null || echo "{}")

CERT_ID=$(echo "$CERT_RESP" | jq -r '.request_id // .certificate_id // empty' 2>/dev/null || echo "")

if [ -n "$CERT_ID" ] && [ "$CERT_ID" != "null" ]; then
  log_pass "Certificate requested: $CERT_ID"

  sleep 2

  NEW_CERT_MSGS=$(get_stream_msgs "CERTIFICATE")
  log_info "New CERTIFICATE stream messages: $NEW_CERT_MSGS"

  if [ "$NEW_CERT_MSGS" -gt "$INITIAL_CERT_MSGS" ] 2>/dev/null; then
    log_pass "certificate.requested event propagated to CERTIFICATE stream (count: $INITIAL_CERT_MSGS -> $NEW_CERT_MSGS)"
  else
    log_fail "certificate.requested event NOT detected in CERTIFICATE stream"
  fi

  # Approve the certificate and check for certificate.issued event
  APPROVE_RESP=$(curl -s -X POST "$CERTAUTH_URL/internal/v1/certificates/${CERT_ID}/approve" \
    -H 'Content-Type: application/json' 2>/dev/null || echo "{}")

  APPROVED_CERT_ID=$(echo "$APPROVE_RESP" | jq -r '.certificate_id // empty' 2>/dev/null || echo "")

  if [ -n "$APPROVED_CERT_ID" ] && [ "$APPROVED_CERT_ID" != "null" ]; then
    log_pass "Certificate approved: $APPROVED_CERT_ID"

    sleep 2

    ISSUED_CERT_MSGS=$(get_stream_msgs "CERTIFICATE")
    log_info "CERTIFICATE stream messages after issuance: $ISSUED_CERT_MSGS"

    if [ "$ISSUED_CERT_MSGS" -gt "$NEW_CERT_MSGS" ] 2>/dev/null; then
      log_pass "certificate.issued event propagated to CERTIFICATE stream"
    else
      log_fail "certificate.issued event NOT detected in CERTIFICATE stream"
    fi
  else
    log_fail "Certificate approval failed"
    log_info "Response: $APPROVE_RESP"
  fi
else
  log_fail "Failed to request certificate for event test"
  log_info "Response: $CERT_RESP"
fi

# ---------------------------------------------------------------------------
# Test 5: Verify stream message counts are consistent
# ---------------------------------------------------------------------------
log_header "Stream Message Counts"

ALL_STREAMS=$(curl -s "${NATS_MONITOR_URL}/jsz?streams=true" 2>/dev/null)
echo "$ALL_STREAMS" | jq -r '.account_details[]?.stream_detail[]? | "\(.name): \(.state.messages) messages"' 2>/dev/null | while read -r line; do
  log_info "$line"
done

WORK_FINAL=$(get_stream_msgs "WORK")
CERT_FINAL=$(get_stream_msgs "CERTIFICATE")

if [ "$WORK_FINAL" -gt 0 ] 2>/dev/null; then
  log_pass "WORK stream has messages ($WORK_FINAL)"
else
  log_fail "WORK stream is empty"
fi

if [ "$CERT_FINAL" -gt 0 ] 2>/dev/null; then
  log_pass "CERTIFICATE stream has messages ($CERT_FINAL)"
else
  log_fail "CERTIFICATE stream is empty"
fi

# ---------------------------------------------------------------------------
# Test 6: Revoke certificate and verify event
# ---------------------------------------------------------------------------
log_header "Event Propagation: Certificate Revocation"

REVOKE_INITIAL=$(get_stream_msgs "CERTIFICATE")

if [ -n "${APPROVED_CERT_ID:-}" ] && [ "${APPROVED_CERT_ID:-}" != "null" ]; then
  curl -s -X DELETE "$CERTAUTH_URL/v1/certificates/${APPROVED_CERT_ID}" \
    -H 'Content-Type: application/json' \
    -d '{"reason": "TESTING"}' >/dev/null 2>&1
  sleep 2
  REVOKE_AFTER=$(get_stream_msgs "CERTIFICATE")

  if [ "$REVOKE_AFTER" -gt "$REVOKE_INITIAL" ] 2>/dev/null; then
    log_pass "certificate.revoked event propagated ($REVOKE_INITIAL -> $REVOKE_AFTER)"
  else
    log_fail "certificate.revoked event not detected"
  fi
else
  log_info "No certificate to revoke, skipping"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo ""
echo -e "${CYAN}========================================${NC}"
echo -e "${CYAN}  NATS Event Propagation Test Results${NC}"
echo -e "${CYAN}========================================${NC}"
echo -e "  ${GREEN}PASSED${NC}: $PASS_COUNT"
echo -e "  ${RED}FAILED${NC}: $FAIL_COUNT"
TOTAL=$((PASS_COUNT + FAIL_COUNT))
echo -e "  TOTAL:  $TOTAL"
echo ""

if [ "$FAIL_COUNT" -gt 0 ]; then
  echo -e "${RED}Some tests failed!${NC}"
  exit 1
else
  echo -e "${GREEN}All NATS event propagation tests passed!${NC}"
  exit 0
fi
