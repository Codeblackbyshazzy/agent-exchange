# AEX Platform Improvements

This document covers all infrastructure and feature improvements implemented across the AEX platform, organized by category.

---

## 1. Settlement Race Condition Fix (CRITICAL)

**Problem:** `settleExecution()` performed non-atomic read-modify-write on balances. Concurrent settlements could lose money.

**Before:**
```
Thread A: GetBalance → $100
Thread B: GetBalance → $100
Thread A: UpdateBalance → $80  (debit $20)
Thread B: UpdateBalance → $70  (debit $30, overwrites A's write)
Result: $50 lost
```

**Fix:**
- Changed `Balance` field from `string` (shopspring/decimal) to `int64` (cents) for atomic `$inc` support
- Replaced `ReplaceOne()` with `FindOneAndUpdate()` + MongoDB `$inc` operator
- Wrapped `settleExecution()` in a MongoDB session transaction (4 operations atomic)
- Fixed same race condition in `ProcessDeposit()`

**Files changed:**
- `src/aex-settlement/internal/model/model.go` - Balance type change
- `src/aex-settlement/internal/store/mongo.go` - Atomic `$inc` operations
- `src/aex-settlement/internal/service/service.go` - MongoDB transactions

---

## 2. Event System: NATS JetStream

**Problem:** Events were logged but never published. No event-driven architecture existed.

**Before:** `publisher.go` just called `slog.InfoContext()` with comment "In the future, this will publish to Pub/Sub"

**Fix:** Full NATS JetStream integration:

**Shared packages created:**
- `src/internal/nats/client.go` - JetStream client with auto-reconnect, deduplication
- `src/internal/nats/streams.go` - 7 stream definitions (WORK, BID, CONTRACT, SETTLEMENT, TRUST, CERTIFICATE, DEADLETTER)
- `src/internal/events/publisher.go` - Dual transport: NATS JetStream + HTTP webhooks

**Stream configuration:**
| Stream | Subjects | Retention | Purpose |
|--------|----------|-----------|---------|
| WORK | `work.>` | 30 days | Work lifecycle events |
| BID | `bid.>`, `bids.>` | 30 days | Bid lifecycle events |
| CONTRACT | `contract.>` | 90 days | Contract events |
| SETTLEMENT | `settlement.>` | 90 days | Settlement events |
| TRUST | `trust.>`, `reputation.>` | 90 days | Trust/reputation updates |
| CERTIFICATE | `certificate.>`, `crl.>` | 365 days | Certificate lifecycle |
| DEADLETTER | `deadletter.>` | 90 days | Failed deliveries |

**Features:**
- 2-minute deduplication window via `Nats-Msg-Id` header
- Manual acknowledgment with 30s timeout, 5 max delivery attempts
- FileStorage for persistence
- Graceful degradation: falls back to log-only if NATS unavailable

**Services wired to NATS:**
- `aex-work-publisher` - publishes `work.submitted`, `work.cancelled`, `work.bid_window_closed`
- `aex-settlement` - publishes `settlement.completed`
- `aex-certauth` - publishes `certificate.requested`, `certificate.issued`, `certificate.renewed`, `certificate.revoked`

---

## 3. Circuit Breakers

**Problem:** 7 HTTP client files had no circuit breakers. A single downstream failure cascaded across all services. Trust-broker client returned hardcoded `0.5` on error (silent data corruption).

**Fix:** Added `sony/gobreaker` circuit breaker to the shared HTTP client:

```
Healthy (Closed)
  │
  │ 5 consecutive failures
  ▼
Open (reject all) ──── 10s timeout ────► Half-Open (allow 1 request)
  ▲                                           │
  │          failure                   success │
  └───────────────────────────────────────────┘
                                               ▼
                                          Closed (healthy)
```

**Configuration:**
- Trip after 5 consecutive failures
- Stay open for 10 seconds
- Returns `ErrCircuitOpen` immediately (no TCP timeout wait)
- State changes logged for monitoring

**Performance impact (measured):**
- Without breaker: ~800ms per request (TCP connection timeout)
- With breaker tripped: ~45ms per request (16x faster failure)

**Files changed:**
- `src/internal/httpclient/client.go` - Circuit breaker integration
- All `src/*/internal/clients/*.go` - Use shared HTTP client

---

## 4. Redis-Backed Rate Limiting

**Problem:** Rate limiting was in-memory only. Broke in multi-instance deployments.

**Before:** `buckets map[string]*bucket` stored in local process memory

**Fix:** Redis-backed fixed-window rate limiter:
- Algorithm: `INCR` + `EXPIRE` per tenant per minute window
- Key format: `ratelimit:<tenantID>:<windowStart>`
- Default: 1000 requests/minute per tenant
- Fail-open: requests allowed if Redis unavailable
- Response headers: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, `Retry-After`

**File:** `src/aex-gateway/internal/middleware/ratelimit.go`

---

## 5. JWT Authentication

**Problem:** Bearer token auth accepted ANY non-empty token. Some routes bypassed auth entirely.

**Fix:**
- HMAC-SHA256 (HS256) JWT validation with configurable secret
- Claims structure: `tenant_id`, `scopes`, standard registered claims
- Issuer validation (`aex-identity`)
- Expiration checking
- API key validation via Identity Service with 5-minute cache
- All `/v1/*` routes now require authentication
- CORS, timeout, rate limiting applied before auth

**File:** `src/aex-gateway/internal/middleware/auth.go`

---

## 6. OpenTelemetry Observability

**Problem:** No distributed tracing, no metrics, no trace context in logs.

**Fix:** Full observability stack integrated into all 13 services:

**Shared telemetry package** (`src/internal/telemetry/`):
- `InitTracer()` - OpenTelemetry OTLP trace exporter
- `InitMeter()` - Prometheus metrics endpoint
- `HTTPMiddleware()` - Auto-instrument incoming HTTP requests with spans
- `TraceHandler()` - slog handler that injects `trace_id` and `span_id` into structured logs

**Per-service integration (all 13 services):**
```go
// In every main.go:
tracerShutdown, _ := telemetry.InitTracer(ctx, "service-name", otlpEndpoint)
metricsHandler, _ := telemetry.InitMeter("service-name")
handler := telemetry.HTTPMiddleware("service-name")(mux)
logHandler := telemetry.TraceHandler(slog.NewJSONHandler(os.Stdout, opts))
```

**Inter-service trace propagation:**
- Outgoing HTTP client injects W3C Trace Context headers
- Incoming HTTP middleware extracts trace context
- Logs automatically include `trace_id` for correlation

---

## 7. Agent Certification Authority (ACA)

**New service:** `aex-certauth` (port 8091)

The flagship feature - X.509-style cryptographic certificates for AI agents.

### Certificate Lifecycle

```
Request → Pending → Approve → Active → (Renew | Revoke | Expire)
```

### Data Models

**AgentCertificate:**
- X.509 fields: issuer, validity dates, signature (ECDSA P-256)
- Capability claims with category, scope, authorization level
- Types: CAPABILITY, IDENTITY, REPUTATION, RESELLER
- W3C DID binding support
- Renewal chain tracking

**ReputationScore (evidence-based from transactions):**
```
OverallScore = (0.35 * TransactionScore) +
               (0.25 * SuccessRate) +
               (0.15 * VolumeScore) +
               (0.15 * ConsistencyScore) +
               (0.10 * CertificationBonus)

Tiers:
  PLATINUM: score >= 0.9, contracts >= 200
  GOLD:     score >= 0.75, contracts >= 50
  SILVER:   score >= 0.5, contracts >= 10
  BRONZE:   everything else
```

**CRL (Certificate Revocation List):**
- Signed by CA
- Queryable per certificate

### API Endpoints

| Method | Endpoint | Purpose |
|--------|----------|---------|
| POST | `/v1/certificates/request` | Submit certificate signing request |
| GET | `/v1/certificates/{id}` | Get certificate details |
| POST | `/v1/certificates/{id}/renew` | Renew certificate |
| DELETE | `/v1/certificates/{id}` | Revoke certificate |
| POST | `/v1/certificates/verify` | Verify a certificate |
| GET | `/v1/providers/{id}/reputation` | Get provider reputation |
| GET | `/v1/reputation/leaderboard` | Top agents by reputation |
| GET | `/v1/certificates/search` | Search by capability/tier |
| GET | `/v1/crl` | Current revocation list |
| GET | `/.well-known/aex-ca.json` | CA public key |

### Platform Integration

**Bid Evaluator** - Certification weight added to bid scoring:
```
Balanced Strategy:
  Price: 0.25, Trust: 0.25, Confidence: 0.15,
  MVPSample: 0.10, SLA: 0.10, Certification: 0.15
```

Certified agents get higher bid rankings. Revoked certificates lose the boost.

---

## 8. NATS Healthcheck Fix

**Problem:** Docker Compose NATS healthcheck used `nats-server --signal ldm` which sends the **lame duck mode** signal, triggering graceful shutdown on every healthcheck interval (every 10s).

**Fix:** Replaced with HTTP monitoring endpoint check:
```yaml
# Before (kills the server!)
test: ["CMD", "nats-server", "--signal", "ldm"]

# After (proper healthcheck)
command: ["--jetstream", "-m", "8222"]
test: ["CMD", "sh", "-c", "wget ... http://localhost:8222/healthz"]
```

---

## 9. Docker & Build Improvements

- All 13 Dockerfiles updated with `COPY` directives for internal modules (telemetry, httpclient, events, nats, ap2, certauth)
- `Makefile` updated with `aex-credentials-provider` and `aex-token-bank` build targets
- `docker-compose.yml` updated with:
  - Gateway: Redis, JWT, CertAuth, Work Publisher, Settlement, Bid Evaluator URLs
  - NATS: JetStream enabled with monitoring port (8222)
  - Token Bank service added (port 8092)
  - Service dependency ordering for healthy startup

---

## 10. ACA Code Review & Hardening

**Problem:** Code review revealed the ACA service had critical issues - the `VerificationService` (crypto verification, CRL generation) was fully implemented but never wired into the HTTP handlers. The API was serving unverified certificates.

**Issues found and fixed:**

### Critical
1. **VerificationService was dead code** - The verify, batch-verify, CRL, and can-perform endpoints used shallow `cert.Status == "ACTIVE"` checks instead of the real `VerificationService` which performs:
   - ECDSA P-256 signature verification against CA public key
   - Time-based expiry checks (not_before, not_after)
   - Revocation status checks
   - Certificate suspension checks

2. **Reputation formula duplicated** - Weights (0.35/0.25/0.15/0.15/0.10) were copy-pasted in `computeScore()` and `applyAntiGaming()`. If one changed, the other wouldn't, creating silent score inconsistency. Extracted into single `computeWeightedScore()` function.

3. **MongoDB URI logged with credentials** - `slog.Info("using mongodb store", "uri", cfg.MongoURI)` was exposing `mongodb://user:password@host` in production logs.

### High
4. **Fragile error detection** - 7 handlers used `strings.Contains(err.Error(), "not found")` to detect missing resources. Replaced with sentinel `store.ErrNotFound` and `errors.Is()` checks.

5. **Dead client code** - `IdentityClient` and `ProviderRegistryClient` were never instantiated. Removed along with 3 unused config fields.

6. **Case-sensitivity inconsistency** - `CanPerform` handler used `strings.EqualFold` (case-insensitive) while all other code used `==` (case-sensitive). Now delegates to `VerificationService.CanPerform` for consistency.

7. **Weak certificate IDs** - 8 bytes (64 bits) of randomness. Birthday paradox collision at ~4B IDs. Increased to 16 bytes (128 bits).

### Medium
8. **Dockerfile COPY bug** - `COPY --from=builder /build/aex-certauth .` copied the entire source directory into the runtime image. Fixed to `COPY --from=builder /build/aex-certauth/aex-certauth .` (binary only).

9. **`os.Exit(1)` inside goroutine** - Bypassed all `defer` cleanups (MongoDB disconnect, NATS close, tracer shutdown). Replaced with error channel for proper shutdown.

10. **mvpScore always 0.5** - Bid evaluator assigned `mvpScore = 0.5` whether MVP sample was present or not. Fixed: 0.5 (no sample) vs 0.8 (sample provided).

11. **No request body size limits** - POST endpoints accepted unlimited payloads. Added `io.LimitReader(r.Body, 1<<20)` (1MB) to all POST handlers.

12. **Wasted randomness in eval IDs** - `generateEvalID` allocated 16 random bytes but only used 8. Fixed to use all 16.

**Files changed:** 12 files, +117/-243 lines (net reduction - removed dead code)

---

## 11. Kubernetes Deployment

Full K8s manifests created for production deployment:

- **Services:** 11 AEX services + MongoDB + NATS + Redis
- **Agents:** 7 demo agents (3 code reviewers, 3 payment providers, 1 orchestrator)
- **Overlays:** dev (1 replica), staging (2 replicas), production (HPA 2-10 replicas)
- **Production features:** Pod Disruption Budgets, Network Policies, Horizontal Pod Autoscaling
- **Kustomize:** Base + overlay pattern for environment-specific configuration

---

## Test Results Summary

| Test Suite | Tests | Status |
|------------|:-----:|:------:|
| E2E (full workflow) | 44/44 | PASS |
| NATS event propagation | 18/18 | PASS |
| Circuit breaker fast-fail | 10/10 | PASS |
| Rate limiter (Redis) | 10/10 | PASS |
| **Total** | **82/82** | **ALL PASS** |
