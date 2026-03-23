# AEX Agent Certification Authority (ACA) - Implementation Plan

## Context

AEX (Agent Exchange) is a Go-based microservices marketplace ("NASDAQ for AI Agents") with 10 services. We are building an **Agent Certification Authority** - X.509-style cryptographic certificates for AI agents proving their capabilities, backed by real transaction reputation data.

**Product thesis**: Agents self-declare capabilities (e.g., "I can sell Delta tickets"). ACA issues cryptographic certificates for those claims. Then, as agents complete real work through AEX, transaction outcomes (success/failure/dispute) build an **evidence-based reputation score** attached to the certificate. The certificate evolves from "this agent claims X" to "this agent claims X and has proven it 200 times with 95% success."

**Why evidence-from-transactions, not standalone evaluation**: Behavioral testing requires domain-specific test suites and access to agent internals (knowledge base, APIs, tools) - impractical for a startup. But AEX already tracks real transaction outcomes in the trust-broker. That IS behavioral evidence, generated organically from actual marketplace usage.

**Why now**: Only 6% of companies fully trust AI agents. Agent marketplace is $7.63B (2025) growing rapidly. $400M+ flowing into non-human identity space. Multiple competitors validating the category (Vouched $22M, 7AI $130M, Descope $88M).

---

## Phase 1: Fix AEX Foundation (Must Do First)

Critical production issues found in the existing codebase that must be fixed before building ACA.

### 1.1 Settlement Race Condition (CRITICAL - Data Loss)

**Problem**: `settleExecution()` performs non-atomic read-modify-write on balances.

**Files**:
- [mongo.go:184-195](src/aex-settlement/internal/store/mongo.go#L184-L195) - `UpdateBalance()` uses `ReplaceOne()` without transactions
- [service.go:281-359](src/aex-settlement/internal/service/service.go#L281-L359) - `settleExecution()` does GetBalance → Calculate → UpdateBalance separately

**Fix**: Two changes needed:
1. **Change `Balance` field from `string` to `int64` (cents)** in [model.go:59](src/aex-settlement/internal/model/model.go#L59) - currently stored as string via `shopspring/decimal`, which prevents atomic `$inc`. Store as integer cents instead.
2. **Wrap `settleExecution()` in a MongoDB session transaction** covering all 4 operations (consumer debit, consumer ledger, provider credit, provider ledger). This ensures atomicity - if provider credit fails after consumer debit, the whole thing rolls back.
3. **Also fix `ProcessDeposit()` at lines 424-482** - has the same read-modify-write race (not just settlement).

Requires MongoDB replica set (see 1.5) for transaction support.

### 1.2 Event System - Replace Stub with NATS JetStream

**Problem**: Events are logged but never published. No event-driven architecture exists.

**File**: [publisher.go:54-68](src/internal/events/publisher.go#L54-L68) - `Publish()` just calls `slog.InfoContext()` with comment "In the future, this will publish to Pub/Sub"

**Fix**: Replace the in-memory publisher with NATS JetStream:
- Add NATS client to `src/internal/events/`
- Create streams for event categories (work, bid, contract, settlement, trust, certificate)
- Preserve the existing `Publish()` interface so callers don't change
- Add dead-letter queue for failed deliveries
- Use the existing `IdempotencyKey` field for deduplication

**Why NATS**: Lightweight, Go-native, supports JetStream for persistence, easy to deploy in K8s. Better fit than Kafka for this scale.

### 1.3 Circuit Breakers for Inter-Service Calls

**Problem**: 7 HTTP client files have no circuit breakers. Failures cascade. Trust-broker client returns hardcoded 0.5 on error (silent failure).

**Files to modify**:
- [client.go](src/internal/httpclient/client.go) - Add circuit breaker to the shared HTTP client
- All `src/*/internal/clients/*.go` files use this shared client

**Fix**: Add `sony/gobreaker` circuit breaker to the shared `httpclient.Client`:
- Trip after 5 consecutive failures
- Half-open after 10 seconds
- Each downstream service gets its own breaker instance
- Return explicit errors instead of default values (fix trust-broker's hardcoded 0.5)
- **Also fix**: Trust-broker client in bid-evaluator creates its own `http.Client` directly (doesn't use shared client). Must refactor it to use the shared client, or add breaker independently

### 1.4 Rate Limiting - Move to Redis

**Problem**: Rate limiting is in-memory only. Breaks in multi-instance K8s deployment.

**File**: [ratelimit.go:12-49](src/aex-gateway/internal/middleware/ratelimit.go#L12-L49) - `buckets map[string]*bucket` stored in local process memory

**Fix**: Replace with Redis-backed rate limiter using `INCR` + `EXPIRE`:
- Add Redis client to gateway config
- Use sliding window algorithm with Redis sorted sets
- Per-tenant rate limits from existing quota system

### 1.5 MongoDB - Replica Set + Production Config

**Problem**: Single MongoDB instance (replicas: 1), 1GB storage, 256Mi memory, no backups, no transaction support.

**File**: [statefulset.yaml](deploy/k8s/services/mongodb/statefulset.yaml)

**Fix**:
- Scale to 3 replicas with replica set configuration
- Increase storage to 50GB with auto-expansion
- Increase resources to 1CPU/2Gi memory minimum
- Add backup CronJob with `mongodump` and PV snapshots
- Replica set enables multi-document transactions (needed for settlement fix option 2)

### 1.6 Authentication Gaps

**Problem**: Bearer token auth accepts ANY non-empty token. Some routes bypass auth.

**Files**:
- [router.go:20-26](src/aex-gateway/internal/httpapi/router.go#L20-L26) - `/v1/info` bypasses auth
- [auth.go:162-174](src/aex-gateway/internal/middleware/auth.go#L162-L174) - Bearer token accepts any value

**Fix**:
- Implement JWT validation (using a JWKS endpoint or shared secret)
- Remove development API keys from `InMemoryAPIKeyValidator`
- Move `/v1/info` inside the auth middleware chain
- Add scope-based authorization checks

### 1.7 Observability

**Add to all services**:
- OpenTelemetry distributed tracing (spans for HTTP handlers, DB calls, inter-service calls)
- Prometheus metrics endpoint (`/metrics`) on each service
- Structured logging with trace IDs (slog already used, add trace context)

---

## Phase 2: Agent Certification Authority (ACA)

### New Service: `aex-certauth` (Port 8091)

Follows existing AEX service pattern (`internal/{config,model,store,service,httpapi,clients}`).

### Architecture Decision: Use `smallstep/crypto` Library

Per architect review, building a custom CA from scratch is unnecessary. Use [`smallstep/crypto`](https://github.com/smallstep/crypto) (NOT the full `smallstep/certificates` which is a complete ACME server with massive deps):
- Provides the signing engine and KMS backend integration
- Lightweight - won't bloat AEX's lean go.mod files
- We build the AEX-specific layer on top: capability claims, reputation, certificate lifecycle
- Significantly reduces crypto implementation risk

### Data Models

**AgentCertificate** (core):
- `certificate_id`, `tenant_id`, `provider_id`, `agent_name`
- X.509 fields: `issuer_id`, `not_before`, `not_after`, `status` (PENDING/ACTIVE/SUSPENDED/REVOKED/EXPIRED)
- `claims []CapabilityClaim` - the capability attestations
- `certificate_type`: CAPABILITY, IDENTITY, REPUTATION, RESELLER
- Crypto: `public_key_pem`, `signature_alg` (ECDSA-SHA256), `signature`
- W3C DID binding: `subject_did`, `issuer_did` (optional, for interop)
- Revocation: `revoked_at`, `revocation_reason`
- Renewal chain: `previous_cert_id`, `renewal_count`

**CapabilityClaim** (machine-verifiable capability descriptor):
- `category`: COMMERCE, FINANCE, TRAVEL, ENTERTAINMENT, etc.
- `capability`: e.g., "ticket.sell"
- `scope`: e.g., "Delta Air Lines"
- `authorization`: SELF_ASSERTED, PROVIDER_ATTESTED, THIRD_PARTY, AEX_VERIFIED
- `authorization_ref`: external proof URL
- `constraints`: map (e.g., `{"max_price": 5000, "regions": ["US","EU"]}`)

**ReputationScore** (aggregated from trust-broker):
- Composite: `overall_score` (0.0-1.0), `reputation_tier` (BRONZE/SILVER/GOLD/PLATINUM)
- Components: `transaction_score`, `success_rate`, `volume_score`, `consistency_score`, `certification_bonus`
- Raw metrics from trust-broker: `total_contracts`, `successful_contracts`, etc.
- Per-category breakdown: `category_stats map[string]CategoryStat`

**CRL** (Certificate Revocation List): entries with `certificate_id`, `revoked_at`, `reason`, signed by CA.

### Reputation Calculation

```
OverallScore = (0.35 * TransactionScore) +    // from trust-broker
               (0.25 * SuccessRate) +          // successful / total
               (0.15 * VolumeScore) +          // min(1.0, total/500)
               (0.15 * ConsistencyScore) +     // 1.0 - stddev(30-day rates)
               (0.10 * CertificationBonus)     // active_certs * 0.05, cap 0.10

Tiers:
  PLATINUM: score >= 0.9, contracts >= 200
  GOLD:     score >= 0.75, contracts >= 50
  SILVER:   score >= 0.5, contracts >= 10
  BRONZE:   everything else
```

### API Endpoints

External (`/v1/`):
- `POST /v1/certificates/request` - Submit CSR
- `GET /v1/certificates/{cert_id}` - Get certificate
- `POST /v1/certificates/{cert_id}/renew` - Renew
- `DELETE /v1/certificates/{cert_id}` - Revoke
- `POST /v1/certificates/verify` - Verify a certificate
- `GET /v1/providers/{id}/certificates` - List provider's certs
- `GET /v1/providers/{id}/reputation` - Get reputation
- `GET /v1/crl` - Current CRL
- `GET /v1/reputation/leaderboard` - Top agents
- `GET /v1/certificates/search` - Search by capability/category/tier

Internal (`/internal/v1/`):
- `POST /internal/v1/certificates/batch-verify` - For bid-evaluator
- `GET /internal/v1/providers/{id}/can-perform` - For contract-engine
- `POST /internal/v1/certificates/{id}/approve` - Admin approve CSR
- `GET /.well-known/aex-ca.json` - CA public key (like JWKS)
- `GET /.well-known/did.json` - W3C DID Document for the CA

### W3C Verifiable Credential Export

Certificates serializable as W3C VCs for interoperability:
```json
{
  "@context": ["https://www.w3.org/2018/credentials/v1", "https://aex.exchange/credentials/v1"],
  "type": ["VerifiableCredential", "AgentCapabilityCertificate"],
  "issuer": {"id": "did:aex:ca_root"},
  "credentialSubject": {
    "id": "did:aex:prov_abc123",
    "capabilities": [{"category": "TRAVEL", "capability": "ticket.sell", "scope": "Delta Air Lines"}]
  },
  "proof": {"type": "EcdsaSecp256r1Signature2019", "jws": "..."}
}
```

### Key Management
- Smallstep CA with Google Cloud KMS backend for CA private key
- Key rotation support with versioned key IDs
- No plaintext key storage - all crypto operations via KMS API

### Certificate Event Types

Add to [types.go](src/internal/events/types.go):
- `certificate.requested`, `certificate.issued`, `certificate.renewed`
- `certificate.revoked`, `certificate.expired`
- `crl.updated`, `reputation.updated`

---

## Phase 3: ACA Platform Integration

### Existing Files to Modify

**[proxy.go](src/aex-gateway/internal/proxy/proxy.go)** - Add certauth routes:
```
"/v1/certificates": cfg.CertAuthURL,
"/v1/crl":          cfg.CertAuthURL,
"/v1/reputation":   cfg.CertAuthURL,
```

**[evaluator.go](src/aex-bid-evaluator/internal/service/evaluator.go)** - Add `Certification` weight to `strategyWeights`:
```
Balanced: {Price: 0.25, Trust: 0.25, Confidence: 0.15, MVPSample: 0.1, SLA: 0.1, Certification: 0.15}
```
Add certauth HTTP client to fetch cert score per provider during bid evaluation.

**[model.go](src/aex-trust-broker/internal/model/model.go)** - Extend `TrustRecord`:
- Add `certification_bonus float64`, `active_certificates int`, `reputation_tier string`

**[model.go](src/aex-provider-registry/internal/model/model.go)** - Extend search:
- Add `require_certification bool`, `min_reputation_tier string`, `required_capabilities []string` to search filters

**[types.go](src/internal/events/types.go)** - Add certificate event types

**`Makefile`** - Add `aex-certauth` to `SERVICES`

**`hack/docker-compose.yml`** - Add `aex-certauth` service + Redis + NATS

**K8s manifests** - Add `deploy/k8s/services/aex-certauth/` (deployment.yaml, service.yaml)

---

## New Files to Create

```
src/aex-certauth/                          # Port 8091
  src/main.go
  internal/config/config.go
  internal/model/certificate.go
  internal/model/reputation.go
  internal/model/crl.go
  internal/store/store.go                  # Interface
  internal/store/mongo.go                  # MongoDB (production only)
  internal/service/ca.go                   # Smallstep CA wrapper + KMS
  internal/service/certificate.go          # Certificate CRUD + CSR workflow
  internal/service/reputation.go           # Reputation calculation engine
  internal/service/verification.go         # Certificate verification logic
  internal/httpapi/router.go
  internal/clients/trustbroker.go          # HTTP client for trust-broker
  internal/clients/providerregistry.go     # HTTP client for provider-registry
  internal/clients/identity.go             # HTTP client for identity service
  hack/tests/certauth_http_test.go
  Dockerfile
  go.mod

src/internal/certauth/                     # Shared cert verification package
  types.go                                 # Certificate, CapabilityClaim types
  verifier.go                              # Signature check, expiry, revocation

src/internal/nats/                         # Shared NATS client (replaces event stub)
  client.go                                # JetStream publisher/subscriber
  streams.go                               # Stream definitions

deploy/k8s/services/aex-certauth/
  deployment.yaml
  service.yaml

deploy/k8s/services/redis/
  deployment.yaml
  service.yaml

deploy/k8s/services/nats/
  statefulset.yaml
  service.yaml
```

---

## Revenue Model (ACA Only)

| Stream | Pricing | Target |
|--------|---------|--------|
| **Explorer (Free)** | Free (1 agent, basic identity) | Drive adoption - "Let's Encrypt" model |
| **Professional** | $99/agent/year | 5 capabilities, API verification endpoint |
| **Business** | $499/agent/year | Unlimited agents, continuous monitoring, SLA |
| **Enterprise** | $2,999/year (unlimited agents) | Root cert delegation, SSO, audit logs |
| **Verification API** | $0.001/call | Third parties verifying certificates |
| **AEX Platform Fee** | 15% of GMV (existing) | Unchanged |

Conservative projections (per VC feedback):
- Year 1: $50K-$150K ARR (realistic with new CA brand)
- Year 2: $300K-$800K ARR (with framework partnerships)
- Year 3: $2M-$5M ARR (with NIST standard alignment)

---

## Go-To-Market

**Priority: Ship product, get users, raise funding.**

1. **Open-source the verification SDK** (Go + Python) - the cert format spec and verification library are open, the CA service is commercial
2. **Free tier for first 1,000 agents** to build network density
3. **Partner with 1-2 agent frameworks** (start with LangChain - largest ecosystem) for SDK integration
4. **Target AI agent builders** who need trust signals to get enterprise adoption
5. **Developer-first distribution**: GitHub, blog posts, framework plugin registries
6. **Use AEX marketplace traction** as proof - agents certified through ACA get higher bid rankings, creating a natural adoption incentive

---

## Implementation Sequence

### Step 1: Foundation Fixes
1. Fix settlement race condition with atomic `$inc` operations
2. Deploy NATS JetStream and replace event publisher stub
3. Add `sony/gobreaker` circuit breakers to shared HTTP client
4. Deploy Redis and migrate rate limiter from in-memory
5. Scale MongoDB to 3-replica set with production config
6. Fix authentication gaps (JWT validation, remove dev API keys)
7. Add OpenTelemetry tracing to all services

### Step 2: ACA Core Service
1. Create `aex-certauth` service structure following AEX conventions
2. Integrate Smallstep CA library with Cloud KMS backend
3. Implement certificate models and MongoDB store with proper indexes
4. Implement CSR workflow: request → review → approve/reject → issue
5. Implement certificate lifecycle: issuance, renewal, revocation, expiry
6. Implement CRL generation and OCSP-like quick-check endpoint
7. Create shared `src/internal/certauth/` verifier package

### Step 3: Reputation Engine
1. Build trust-broker HTTP client in certauth service
2. Implement weighted reputation calculation
3. Implement tier assignment (BRONZE/SILVER/GOLD/PLATINUM)
4. Build leaderboard and search APIs
5. Build per-category reputation breakdown

### Step 4: Platform Integration
1. Add certauth routes to gateway proxy
2. Add `Certification` weight to bid-evaluator scoring
3. Extend provider-registry search with cert filters
4. Extend trust-broker model with certification bonus
5. Add certificate event types to event system
6. Implement W3C Verifiable Credential export endpoint

### Step 5: Deployment & Testing
1. Create K8s manifests for certauth, Redis, NATS
2. Update docker-compose for local development
3. Update Makefile with new service
4. Write integration tests against real MongoDB
5. E2E test: register → certify → bid → verify cert affects ranking
6. Load test certificate verification endpoint

---

## Verification

- **Unit tests**: Business logic tests for CA operations, reputation calculation, certificate verification
- **Integration tests**: `hack/tests/certauth_http_test.go` against real MongoDB
- **E2E flows**:
  1. Register provider → Request certificate → Admin approves → Verify certificate cryptographically → Check CRL → Query reputation
  2. Submit work → Bid with certified agent → Verify cert score boosts bid ranking → Contract awarded to higher-cert agent
  3. Revoke certificate → Verify CRL updated → Verify bid-evaluator no longer boosts revoked cert
- **Foundation verification**:
  1. Concurrent settlement test → Verify no balance loss with atomic operations
  2. Event publication → Verify events flow through NATS to subscribers
  3. Circuit breaker → Kill a downstream service → Verify fast-fail instead of 15s timeout
  4. Rate limiting → Send from multiple gateway pods → Verify global limit enforced via Redis
- **Build**: `make build` includes `aex-certauth`
- **Deploy**: `docker-compose up` starts all services including certauth, Redis, NATS
- **Monitoring**: Prometheus metrics for cert issuance rate, verification latency, reputation recalculations

---

## Competitive Landscape

| Competitor | What They Do | Our Advantage |
|-----------|-------------|--------------|
| Vouched ($22M) | Agent Checkpoint - agent identity governance | We're marketplace-integrated, they're standalone detection |
| Keyfactor | General PKI infrastructure | We're agent-native with capability claims + reputation |
| GoDaddy ANS | Agent naming (FQDN-based) | We add capabilities + reputation, not just names |
| 7AI ($130M) | Agentic security for enterprises | Security-only, no marketplace integration |
| Descope ($35M) | Agent security tools | Identity-focused, no capability certification |
| t54 Labs ($5M) | Agent trust for finance | Vertical-only (finance), we're horizontal |

**Our moat**: Deep integration with AEX marketplace (cert score directly affects bid ranking - no competitor has this), evidence-based reputation from real transactions (not self-reported), and open-source verification SDK for ecosystem adoption.

---

## Complete Task Breakdown (16 weeks, 2-person team)

### Phase 1: Foundation Fixes (Weeks 1-6)

#### Week 1-2: Infrastructure + Spike
| # | Task | Owner | Files | Est |
|---|------|-------|-------|-----|
| 1.1 | ~~Deploy MongoDB 3-replica set (decide migration strategy: parallel vs in-place)~~ | Eng-1 | `deploy/k8s/services/mongodb/statefulset.yaml` | 3d | DONE |
| 1.2 | ~~Configure MongoDB replica set for transaction support~~ | Eng-1 | `deploy/k8s/services/mongodb/statefulset.yaml` | 1d | DONE |
| 1.3 | ~~**Smallstep/crypto spike** - superseded: CA built with Go stdlib crypto/ecdsa~~ | Eng-2 | `src/aex-certauth/internal/service/ca.go` | - | DONE |
| 1.4 | ~~Increase MongoDB storage to 50GB, resources to 1CPU/2Gi~~ | Eng-1 | `deploy/k8s/services/mongodb/statefulset.yaml` | 0.5d | DONE |
| 1.5 | ~~Add MongoDB backup CronJob with `mongodump` + PV snapshots~~ | Eng-1 | `deploy/k8s/services/mongodb/backup-cronjob.yaml` (new) | 1d | DONE |

#### Week 2-3: Settlement Race Condition Fix
| # | Task | Owner | Files | Est |
|---|------|-------|-------|-----|
| 2.1 | ~~Change `Balance` field from `string` to `int64` (cents) in TenantBalance model~~ | Eng-1 | `src/aex-settlement/internal/model/model.go:59` | 0.5d | DONE |
| 2.2 | ~~Update all Balance serialization (shopspring/decimal → int64 cents)~~ | Eng-1 | `src/aex-settlement/internal/service/service.go` | 1d | DONE |
| 2.3 | ~~Replace `ReplaceOne()` with `FindOneAndUpdate()` + `$inc` in UpdateBalance~~ | Eng-1 | `src/aex-settlement/internal/store/mongo.go:184-195` | 1d | DONE |
| 2.4 | ~~Wrap `settleExecution()` in MongoDB session transaction (4 ops atomic)~~ | Eng-1 | `src/aex-settlement/internal/service/service.go:281-359` | 1d | DONE |
| 2.5 | ~~Fix `ProcessDeposit()` same race condition~~ | Eng-1 | `src/aex-settlement/internal/service/service.go:424-482` | 1d | DONE |
| 2.6 | ~~Update SettlementStore interface for transaction support~~ | Eng-1 | `src/aex-settlement/internal/store/store.go` | 0.5d | DONE |
| 2.7 | ~~Load test settlement with concurrent transactions (test written)~~ | Eng-2 | `hack/tests/settlement_load_test.go` (new) | 2d | DONE |

#### Week 3-4: Event System + Circuit Breakers
| # | Task | Owner | Files | Est |
|---|------|-------|-------|-----|
| 3.1 | ~~Deploy NATS JetStream (3 replicas) in K8s~~ | Eng-2 | `deploy/k8s/services/nats/` (new) | 1d | DONE |
| 3.2 | ~~Create shared NATS client package~~ | Eng-2 | `src/internal/nats/client.go` (new) | 2d | DONE |
| 3.3 | ~~Define JetStream streams for event categories~~ | Eng-2 | `src/internal/nats/streams.go` (new) | 1d | DONE |
| 3.4 | ~~Replace event publisher stub with NATS publisher~~ | Eng-2 | `src/internal/events/publisher.go:54-68` | 1d | DONE |
| 3.5 | ~~Add dead-letter queue for failed event deliveries~~ | Eng-2 | `src/internal/nats/client.go` | 0.5d | DONE |
| 3.6 | ~~Add `sony/gobreaker` to shared HTTP client~~ | Eng-1 | `src/internal/httpclient/client.go` | 1d | DONE |
| 3.7 | ~~Refactor trust-broker client to use shared HTTP client~~ | Eng-1 | `src/aex-bid-evaluator/internal/clients/trustbroker.go` | 1d | DONE |
| 3.8 | ~~Remove hardcoded 0.5 default - return explicit errors~~ | Eng-1 | `src/aex-bid-evaluator/internal/clients/trustbroker.go:39` | 0.5d | DONE |
| 3.9 | ~~Add circuit breakers to all 7 inter-service clients~~ | Eng-1 | All `src/*/internal/clients/*.go` | 1d | DONE |

#### Week 4-5: Rate Limiting + Auth + Observability
| # | Task | Owner | Files | Est |
|---|------|-------|-------|-----|
| 4.1 | ~~Deploy Redis in K8s~~ | Eng-2 | `deploy/k8s/services/redis/` (new) | 0.5d | DONE |
| 4.2 | ~~Replace in-memory rate limiter with Redis-backed (INCR + EXPIRE)~~ | Eng-1 | `src/aex-gateway/internal/middleware/ratelimit.go:12-49` | 2d | DONE |
| 4.3 | ~~Implement JWT validation (replace "ANY non-empty token" acceptance)~~ | Eng-1 | `src/aex-gateway/internal/middleware/auth.go:162-174` | 2d | DONE |
| 4.4 | ~~Remove development API keys from InMemoryAPIKeyValidator~~ | Eng-1 | `src/aex-gateway/internal/middleware/auth.go:34-45` | 0.5d | DONE |
| 4.5 | ~~Move `/v1/info` inside auth middleware chain~~ | Eng-1 | `src/aex-gateway/internal/httpapi/router.go:20-26` | 0.5d | DONE |
| 4.6 | ~~Add OpenTelemetry tracing to all 13 services~~ | Eng-2 | All `src/*/src/main.go` | 3d | DONE |
| 4.7 | ~~Add Prometheus metrics endpoint to all services~~ | Eng-2 | All `src/*/src/main.go` (GET /metrics) | 1d | DONE |
| 4.8 | ~~Add trace context propagation to slog structured logging + httpclient~~ | Eng-2 | `src/internal/telemetry/slog.go`, `src/internal/httpclient/client.go` | 1d | DONE |

#### Week 6: Integration Testing + Buffer
| # | Task | Owner | Files | Est |
|---|------|-------|-------|-----|
| 5.1 | ~~Test concurrent settlement (verify no balance loss)~~ | Both | `hack/tests/settlement_load_test.go` | 1d | DONE |
| 5.2 | Test event propagation through NATS | Both | `hack/tests/` | 1d |
| 5.3 | Test circuit breaker (kill downstream → verify fast-fail) | Both | `hack/tests/` | 1d |
| 5.4 | Test rate limiting across multiple gateway pods | Both | `hack/tests/` | 1d |
| 5.5 | ~~Buffer for infrastructure surprises~~ | Both | - | 1d | DONE |

---

### Phase 2: ACA Core Service (Weeks 7-12)

#### Week 7-8: Service Skeleton + CA Engine
| # | Task | Owner | Files | Est |
|---|------|-------|-------|-----|
| 6.1 | ~~Create `aex-certauth` directory structure (following AEX conventions)~~ | Eng-1 | `src/aex-certauth/` | 0.5d | DONE |
| 6.2 | ~~Implement config, main.go, Dockerfile, go.mod~~ | Eng-1 | `src/aex-certauth/src/main.go`, `internal/config/config.go` | 1d | DONE |
| 6.3 | ~~Implement AgentCertificate, CapabilityClaim, CertificateRequest models~~ | Eng-1 | `src/aex-certauth/internal/model/certificate.go` | 1d | DONE |
| 6.4 | ~~Implement ReputationScore, CategoryStat models~~ | Eng-1 | `src/aex-certauth/internal/model/reputation.go` | 0.5d | DONE |
| 6.5 | ~~Implement CRL, CRLEntry models~~ | Eng-1 | `src/aex-certauth/internal/model/crl.go` | 0.5d | DONE |
| 6.6 | ~~Integrate CA signing engine (ECDSA P-256, Go stdlib crypto)~~ | Eng-2 | `src/aex-certauth/internal/service/ca.go` | 3d | DONE |
| 6.7 | ~~Implement Store interface~~ | Eng-1 | `src/aex-certauth/internal/store/store.go` | 0.5d | DONE |
| 6.8 | ~~Implement MongoDB store with indexes~~ | Eng-1 | `src/aex-certauth/internal/store/mongo.go` | 2d | DONE |
| 6.9 | ~~Implement HTTP router with /health endpoint~~ | Eng-1 | `src/aex-certauth/internal/httpapi/router.go` | 0.5d | DONE |

#### Week 9-10: Certificate Lifecycle
| # | Task | Owner | Files | Est |
|---|------|-------|-------|-----|
| 7.1 | ~~Implement CSR submission (POST /v1/certificates/request)~~ | Eng-1 | `src/aex-certauth/internal/service/certificate.go` | 1d | DONE |
| 7.2 | ~~Implement CSR review/approve/reject workflow~~ | Eng-1 | `src/aex-certauth/internal/service/certificate.go` | 1d | DONE |
| 7.3 | ~~Implement certificate issuance (sign with CA key)~~ | Eng-2 | `src/aex-certauth/internal/service/ca.go` | 2d | DONE |
| 7.4 | ~~Implement certificate renewal~~ | Eng-1 | `src/aex-certauth/internal/service/certificate.go` | 1d | DONE |
| 7.5 | ~~Implement certificate revocation~~ | Eng-1 | `src/aex-certauth/internal/service/certificate.go` | 1d | DONE |
| 7.6 | ~~Implement CRL generation~~ | Eng-2 | `src/aex-certauth/internal/service/verification.go` | 1d | DONE |
| 7.7 | ~~Implement OCSP-like quick-check endpoint (GET /v1/crl/check/{cert_id})~~ | Eng-2 | `src/aex-certauth/internal/service/verification.go` | 0.5d | DONE |
| 7.8 | ~~Implement certificate verification (POST /v1/certificates/verify)~~ | Eng-2 | `src/aex-certauth/internal/service/verification.go` | 1d | DONE |
| 7.9 | ~~Create shared verifier package~~ | Eng-2 | `src/internal/certauth/types.go`, `verifier.go` | 1d | DONE |
| 7.10 | ~~Publish certificate lifecycle events to NATS~~ | Eng-1 | `src/aex-certauth/internal/service/certificate.go` | 0.5d | DONE |

#### Week 11-12: Reputation Engine
| # | Task | Owner | Files | Est |
|---|------|-------|-------|-----|
| 8.1 | ~~Build trust-broker HTTP client~~ | Eng-1 | `src/aex-certauth/internal/clients/trustbroker.go` | 1d | DONE |
| 8.2 | ~~Build provider-registry HTTP client~~ | Eng-1 | `src/aex-certauth/internal/clients/providerregistry.go` | 0.5d | DONE |
| 8.3 | ~~Build identity HTTP client~~ | Eng-1 | `src/aex-certauth/internal/clients/identity.go` | 0.5d | DONE |
| 8.4 | ~~Implement weighted reputation calculation (35/25/15/15/10 formula)~~ | Eng-2 | `src/aex-certauth/internal/service/reputation.go` | 2d | DONE |
| 8.5 | ~~Implement tier assignment (BRONZE/SILVER/GOLD/PLATINUM)~~ | Eng-2 | `src/aex-certauth/internal/service/reputation.go` | 0.5d | DONE |
| 8.6 | ~~Implement per-category reputation breakdown~~ | Eng-2 | `src/aex-certauth/internal/service/reputation.go` | 1d | DONE |
| 8.7 | ~~Implement leaderboard API (GET /v1/reputation/leaderboard)~~ | Eng-1 | `src/aex-certauth/internal/httpapi/router.go` | 1d | DONE |
| 8.8 | ~~Implement search by capability/category/tier (GET /v1/certificates/search)~~ | Eng-1 | `src/aex-certauth/internal/httpapi/router.go` | 1d | DONE |
| 8.9 | ~~Add anti-gaming safeguards (anomaly detection for volume/collusion)~~ | Eng-2 | `src/aex-certauth/internal/service/reputation.go` | 2d | DONE |

---

### Phase 3: Platform Integration (Weeks 13-14)

| # | Task | Owner | Files | Est |
|---|------|-------|-------|-----|
| 9.1 | ~~Add certauth routes to gateway proxy~~ | Eng-1 | `src/aex-gateway/internal/proxy/proxy.go` | 0.5d | DONE |
| 9.2 | ~~Add certauth config URL to gateway~~ | Eng-1 | `src/aex-gateway/internal/config/config.go` | 0.5d | DONE |
| 9.3 | ~~Create certauth HTTP client in bid-evaluator~~ | Eng-2 | `src/aex-bid-evaluator/internal/clients/certauth.go` (new) | 1d | DONE |
| 9.4 | ~~Add `Certification` weight to all 3 bid strategies~~ | Eng-2 | `src/aex-bid-evaluator/internal/service/evaluator.go` | 1d | DONE |
| 9.5 | ~~Add graceful degradation: cert score = 0 when certauth is down~~ | Eng-2 | `src/aex-bid-evaluator/internal/service/evaluator.go` | 0.5d | DONE |
| 9.6 | ~~Extend TrustRecord with certification_bonus, active_certificates~~ | Eng-1 | `src/aex-trust-broker/internal/model/model.go` | 0.5d | DONE |
| 9.7 | ~~Extend provider-registry search with cert filters~~ | Eng-1 | `src/aex-provider-registry/internal/model/model.go` | 1d | DONE |
| 9.8 | ~~Add certificate event types to events package~~ | Eng-1 | `src/internal/events/types.go` | 0.5d | DONE |
| 9.9 | ~~Add `aex-certauth` to Makefile SERVICES~~ | Eng-2 | `Makefile` | 0.5d | DONE |
| 9.10 | ~~Add certauth + Redis + NATS to docker-compose~~ | Eng-2 | `hack/docker-compose.yml` | 1d | DONE |

---

### Phase 4: Deployment & Testing (Weeks 15-16)

| # | Task | Owner | Files | Est |
|---|------|-------|-------|-----|
| 10.1 | ~~Create K8s deployment + service for aex-certauth~~ | Eng-1 | `deploy/k8s/services/aex-certauth/` | 1d | DONE |
| 10.2 | ~~Update K8s configmap with CERTAUTH_URL~~ | Eng-1 | `deploy/k8s/base/configmap.yaml` | 0.5d | DONE |
| 10.3 | ~~Update kustomization.yaml~~ | Eng-1 | `deploy/k8s/base/kustomization.yaml` | 0.5d | DONE |
| 10.4 | ~~Write certauth integration tests~~ | Eng-2 | `hack/tests/certauth_http_test.go`, `hack/tests/e2e_test.sh` | 2d | DONE |
| 10.5 | ~~E2E: register → certify → bid → verify cert boosts ranking (44/44 passed)~~ | Both | `hack/tests/e2e_test.sh` | 2d | DONE |
| 10.6 | ~~E2E: revoke cert → verify CRL → verify bid-evaluator stops boost~~ | Both | `hack/tests/e2e_test.sh` | 1d | DONE |
| 10.7 | Load test certificate verification (<100ms P95 target) | Eng-2 | `hack/tests/` | 1d |
| 10.8 | Load test certificate issuance (<1s P95 target) | Eng-2 | `hack/tests/` | 0.5d |
| 10.9 | Validate reputation formula against historical AEX data | Eng-1 | Analysis script | 2d |
| 10.10 | ~~Final smoke test: full E2E on docker-compose (44/44 passed)~~ | Both | `hack/tests/e2e_test.sh` | 1d | DONE |

---

## Total: 72 tasks across 16 weeks

| Phase | Tasks | Weeks | Focus |
|-------|-------|-------|-------|
| Phase 1: Foundation | 28 tasks | 6 weeks | Settlement fix, NATS, circuit breakers, Redis, auth, observability |
| Phase 2: ACA Core | 27 tasks | 6 weeks | Service, CA engine, certificate lifecycle, reputation |
| Phase 3: Integration | 10 tasks | 2 weeks | Gateway, bid-evaluator, trust-broker, provider-registry |
| Phase 4: Deploy & Test | 10 tasks | 2 weeks | K8s, E2E tests, load tests, validation |
