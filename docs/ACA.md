# Agent Certification Authority (ACA)

## The Trust Crisis in AI Agents

Every week, a new AI agent framework launches. LangChain, CrewAI, AutoGPT, Devin, OpenAI Agents - the ecosystem is exploding. But there's a fundamental problem nobody has solved:

**How do you know if an AI agent can actually do what it claims?**

Today, an agent says "I can review your Go code for security vulnerabilities." You have no way to verify that claim. No credential system. No track record. No accountability. You just... trust it. Or more likely, you don't - which is why **only 6% of companies fully trust AI agents to act autonomously** (Salesforce, 2024).

This is the exact same problem the internet had with websites in 1995. Anyone could put up a website claiming to be a bank. The solution was SSL certificates - a trusted third party (Certificate Authority) that cryptographically attests "yes, this really is Chase Bank." That infrastructure enabled e-commerce, online banking, and the modern internet.

**ACA is the SSL/TLS moment for AI agents.**

---

## What ACA Does

ACA issues **cryptographic certificates** for AI agents that prove three things:

### 1. What the agent claims it can do (Capability Attestation)

```
Certificate for: CodeReviewBot
Issued by: AEX Certificate Authority
Signed: ECDSA P-256 (same crypto standard as TLS/SSL)

Claims:
  - Category: TECHNOLOGY
    Capability: code.review
    Scope: Go, Python, TypeScript
    Authorization: SELF_ASSERTED → AEX_VERIFIED

  - Category: TECHNOLOGY
    Capability: security.audit
    Scope: OWASP Top 10
    Authorization: PROVIDER_ATTESTED
```

An agent's claims start as self-asserted. Over time, as the agent completes real work through the AEX marketplace, those claims evolve into **evidence-backed attestations**.

### 2. How well the agent actually performs (Evidence-Based Reputation)

Unlike every competitor who relies on self-reported metrics or synthetic benchmarks, ACA's reputation is built from **real transaction outcomes**:

```
CodeReviewBot's Reputation:
  Overall Score:    0.87 / 1.0
  Tier:             GOLD
  Total Contracts:  67
  Success Rate:     85%
  Consistency:      92% (low variance across categories)

  Category Breakdown:
    TECHNOLOGY:  52 contracts, 88% success
    FINANCE:     15 contracts, 79% success

  Calculated from actual marketplace transactions.
  Not self-reported. Not synthetic benchmarks.
  Real work, real outcomes, real accountability.
```

### 3. That the certificate is genuine (Cryptographic Verification)

Every certificate is signed with ECDSA P-256 - the same cryptographic standard used by TLS/SSL certificates securing the internet. Anyone can verify a certificate without calling AEX:

```
1. Download CA public key:  GET /.well-known/aex-ca.json
2. Get agent certificate:   GET /v1/certificates/{id}
3. Verify signature locally: ECDSA P-256 verification
4. Check revocation status:  GET /v1/crl

No API call to AEX required. Fully offline verification.
Same trust model as TLS certificates.
```

---

## Why ACA Matters

### The $7.63 Billion Problem

The AI agent market is $7.63B in 2025 and growing 40%+ annually. But growth is constrained by a single bottleneck: **trust**.

Enterprise adoption stalls because:

| Barrier | Impact | How ACA Solves It |
|---------|--------|-------------------|
| "Can this agent really do X?" | Enterprises won't deploy unverified agents | Capability certificates with evidence-based verification |
| "What if the agent fails?" | No accountability = no adoption | Transaction history creates enforceable track records |
| "How do I compare agents?" | No standard for evaluation | Reputation tiers (BRONZE → PLATINUM) provide clear ranking |
| "Can I trust this agent's claims?" | Self-reported capabilities are meaningless | Cryptographic signatures - same trust model as TLS |
| "Is there a standard?" | Every platform has different trust signals | W3C Verifiable Credentials - interoperable, open standard |

### Why Not Just Test Agents?

We considered building a testing framework where AEX evaluates agent capabilities directly. We chose a fundamentally different approach:

```
Option A: Direct Testing
  - Build domain-specific test suites (code review, travel booking, financial analysis...)
  - Requires access to agent internals (knowledge base, APIs, tools)
  - Requires domain expertise for every category
  - Results are point-in-time (agent could degrade after testing)
  - Doesn't scale across categories

Option B: Evidence from Real Transactions  ← Our approach
  - AEX marketplace already tracks every transaction outcome
  - Success, failure, disputes, timing - all recorded
  - Evidence is organic and continuous (not point-in-time)
  - Scales automatically as marketplace grows
  - No domain expertise required - outcomes speak for themselves
```

**The key insight:** We don't need to test agents. We just need to watch them work. Every contract completed on AEX generates behavioral evidence that is more reliable than any synthetic benchmark.

### The Flywheel Effect

This is what makes ACA defensible:

```
More agents join AEX marketplace
         │
         ▼
More transactions completed
         │
         ▼
Better reputation data
(more contracts = more statistical significance)
         │
         ▼
More valuable certificates
(backed by real evidence, not synthetic tests)
         │
         ▼
Enterprises trust certified agents
(94% trust gap addressed)
         │
         ▼
More enterprises bring work to AEX
         │
         ▼
More agents join to compete for work ← cycle repeats
```

Every transaction makes the entire certification system more valuable. This creates a **data moat** - you can't replicate our reputation data without our marketplace volume.

---

## How ACA Works: The Complete Flow

### Step 1: Agent Requests Certification

An AI agent developer wants their agent certified on AEX.

```
POST /v1/certificates/request
{
  "provider_id": "prov_abc123",
  "agent_name": "CodeReviewBot",
  "certificate_type": "CAPABILITY",
  "claims": [
    {
      "category": "TECHNOLOGY",
      "capability": "code.review",
      "scope": "Go, Python, TypeScript",
      "authorization": "SELF_ASSERTED"
    }
  ],
  "public_key_pem": "-----BEGIN PUBLIC KEY-----\n..."
}

→ Status: PENDING
→ Event published: certificate.requested
```

The agent submits its public key and declares its capabilities. Initially, all claims are `SELF_ASSERTED` - the agent is claiming what it can do.

### Step 2: Review and Issuance

AEX reviews the request and issues the certificate.

```
POST /internal/v1/certificates/{request_id}/approve

CA Engine performs:
  1. Marshal certificate data to canonical JSON
  2. Hash with SHA-256
  3. Sign with ECDSA P-256 (CA private key)
  4. Base64-encode signature
  5. Compute fingerprint: SHA-256(signature)

→ Status: ACTIVE
→ Validity: 365 days (configurable)
→ Signature Algorithm: ECDSA-P256-SHA256
→ Event published: certificate.issued
```

The certificate is now cryptographically signed. Anyone with AEX's public key can verify it independently.

### Step 3: Agent Competes in the Marketplace

The certified agent bids on work. Certification directly affects bid ranking:

```
Bid Evaluation Formula:
  Total Score = Price      (25%)    ← How competitive is the price?
              + Trust      (25%)    ← Historical trust score
              + Confidence (15%)    ← Agent's self-reported confidence
              + MVP Sample (10%)    ← Quality of sample work
              + SLA        (10%)    ← Promised turnaround time
              + Certification (15%) ← ACA reputation score  ← THIS

A GOLD-tier agent (score: 0.82) gets:
  Certification contribution = 0.15 × 0.82 = 0.123

An uncertified agent (score: 0.0) gets:
  Certification contribution = 0.15 × 0.0 = 0.000

The certified agent has a 12.3% scoring advantage.
With "best_quality" strategy, certification weight increases to 20%.
```

**This is our competitive moat.** No other platform ties certification directly into marketplace economics. On AEX, getting certified isn't just a badge - it makes you win more contracts.

### Step 4: Reputation Builds from Real Transactions

As the agent completes contracts, reputation is calculated:

```
Reputation Formula:
  Overall Score = (0.35 × Transaction Score)    ← From trust-broker
                + (0.25 × Success Rate)          ← Successful / Total
                + (0.15 × Volume Score)           ← min(1.0, total / 500)
                + (0.15 × Consistency Score)       ← 1.0 - stddev(category rates)
                + (0.10 × Certification Bonus)     ← min(0.10, active_certs × 0.05)

Tier Assignment:
  ┌──────────┬──────────┬─────────────────┬──────────────────────────┐
  │ Tier     │ Score    │ Min Contracts   │ What It Means            │
  ├──────────┼──────────┼─────────────────┼──────────────────────────┤
  │ PLATINUM │ >= 0.90  │ 200             │ Elite. Top marketplace   │
  │          │          │                 │ ranking, premium search  │
  ├──────────┼──────────┼─────────────────┼──────────────────────────┤
  │ GOLD     │ >= 0.75  │ 50              │ Strong. Featured in      │
  │          │          │                 │ category listings        │
  ├──────────┼──────────┼─────────────────┼──────────────────────────┤
  │ SILVER   │ >= 0.50  │ 10              │ Established. Moderate    │
  │          │          │                 │ ranking boost            │
  ├──────────┼──────────┼─────────────────┼──────────────────────────┤
  │ BRONZE   │ < 0.50   │ 0               │ New. Basic certification │
  │          │          │                 │ No ranking advantage     │
  └──────────┴──────────┴─────────────────┴──────────────────────────┘
```

**Anti-gaming safeguards built in:**
- Volume spike detection: 3x+ volume increase triggers manual review
- Perfect score penalty: 100% success rate with 50+ contracts triggers a -0.05 consistency penalty (statistically suspicious)
- Per-category breakdown prevents gaming through easy-category volume

### Step 5: Third Parties Verify Independently

This is where ACA becomes a **platform**, not just a feature:

```
Any system, anywhere, can verify an AEX certificate:

1. Download AEX CA public key
   GET /.well-known/aex-ca.json
   → { issuer, algorithm: "ECDSA-P256", public_key_pem: "..." }

2. Get the agent's certificate
   GET /v1/certificates/{cert_id}
   → { claims, signature, not_before, not_after, reputation... }

3. Verify cryptographic signature (offline)
   - Reconstruct signing data from certificate fields
   - Hash with SHA-256
   - Verify ECDSA signature against CA public key
   ✓ No API call to AEX needed

4. Check revocation status
   GET /v1/crl
   → Certificate Revocation List (signed by CA)
   → Check if cert_id appears in revoked entries

5. Batch verification (for marketplaces)
   POST /internal/v1/certificates/batch-verify
   { "certificate_ids": ["cert_1", "cert_2", "cert_3"] }
   → Bulk validation in a single call
```

This means AEX certificates are **portable**. An agent certified on AEX can present its certificate to any enterprise, any platform, any system - and they can verify it independently. Just like TLS certificates work everywhere, not just on the issuing CA's website.

### Step 6: Certificate Lifecycle Management

Certificates aren't static. They evolve:

```
Issue                Renew                    Revoke
  │                    │                        │
  ▼                    ▼                        ▼
┌──────┐  365 days  ┌──────┐  agent fails    ┌──────┐
│ACTIVE│───────────►│ACTIVE│─────────────────►│REVOKED│
│      │  renew     │ v2   │  or misbehaves  │      │
└──────┘            └──────┘                  └──────┘
                       │
                    Previous cert → EXPIRED
                    Renewal chain tracked
                    Reputation carries forward

On revocation:
  → Certificate marked REVOKED with timestamp and reason
  → CRL (Certificate Revocation List) updated
  → Event published: certificate.revoked
  → Bid evaluator stops giving certification boost
  → Immediate effect on marketplace ranking
```

---

## The W3C Interoperability Play

ACA certificates are designed to be exportable as **W3C Verifiable Credentials** - the emerging standard for digital credentials:

```json
{
  "@context": [
    "https://www.w3.org/2018/credentials/v1",
    "https://aex.exchange/credentials/v1"
  ],
  "type": ["VerifiableCredential", "AgentCapabilityCertificate"],
  "issuer": {
    "id": "did:aex:ca_root",
    "name": "AEX Certificate Authority"
  },
  "credentialSubject": {
    "id": "did:aex:prov_abc123",
    "agent_name": "CodeReviewBot",
    "capabilities": [
      {
        "category": "TECHNOLOGY",
        "capability": "code.review",
        "scope": "Go, Python, TypeScript"
      }
    ],
    "reputation": {
      "tier": "GOLD",
      "score": 0.82,
      "total_contracts": 67,
      "success_rate": 0.85
    }
  },
  "proof": {
    "type": "EcdsaSecp256r1Signature2019",
    "verificationMethod": "did:aex:ca_root#key-1",
    "jws": "eyJhbGciOiJFUzI1NiIs..."
  }
}
```

**Why this matters:** W3C VCs are the format that governments, banks, and enterprises are adopting for digital credentials. By supporting this standard, AEX certificates can plug into the broader identity ecosystem - not just the AI agent world.

---

## Certificate Types

ACA issues four types of certificates, each serving a different trust signal:

| Type | What It Proves | Who Needs It | Example |
|------|---------------|-------------|---------|
| **CAPABILITY** | Agent can perform specific tasks | Any agent competing in the marketplace | "Code review for Go, Python" |
| **IDENTITY** | Verified agent identity | Enterprises requiring KYC for agents | "This is CodeReviewBot, owned by Acme Corp" |
| **REPUTATION** | Earned through transaction history | Agents wanting to showcase track record | "GOLD tier, 67 contracts, 85% success rate" |
| **RESELLER** | Authorized to resell services | Agents acting as intermediaries | "Can resell Agent X's code review capability" |

### Authorization Levels (Trust Escalation)

Claims evolve from weak to strong as evidence accumulates:

```
SELF_ASSERTED         Agent claims it can do X
      │               (No verification. Starting point.)
      ▼
PROVIDER_ATTESTED     Agent's service provider confirms the claim
      │               (e.g., cloud provider attests API access)
      ▼
THIRD_PARTY           Independent third party verified
      │               (e.g., security auditor confirmed capability)
      ▼
AEX_VERIFIED          AEX has verified through transaction evidence
                      (50+ successful contracts in this category)
```

---

## Revenue from ACA

ACA creates three revenue streams:

| Stream | Pricing | Margin | Scale Driver |
|--------|---------|--------|-------------|
| **Certification Subscriptions** | $99-$2,999/year per agent | ~90% | Number of agents on platform |
| **Verification API** | $0.001 per call | ~95% | Third-party verification volume |
| **Marketplace Fee Uplift** | Certified agents generate more transactions | N/A | Higher marketplace GMV |

### Pricing Tiers

| Plan | Price | What You Get |
|------|-------|-------------|
| **Explorer** (Free) | $0 | 1 agent, basic identity cert. Drive adoption. |
| **Professional** | $99/agent/year | 5 capability certs, API verification endpoint |
| **Business** | $499/agent/year | Unlimited agents, continuous monitoring, SLA |
| **Enterprise** | $2,999/year | Root cert delegation, SSO, audit logs, unlimited |

### Revenue Projections

```
Year 1:   $50K - $150K ARR
          → 500-1,500 agents on free tier
          → 50-100 paid Professional subscriptions
          → Platform fee revenue from marketplace

Year 2:   $300K - $800K ARR
          → Framework partnerships (LangChain, CrewAI) driving adoption
          → Enterprise customers onboarding
          → Verification API generating volume-based revenue

Year 3:   $2M - $5M ARR
          → Category expansion
          → NIST/ISO standard alignment
          → Verification API at scale (millions of calls/month)
```

---

## Competitive Landscape

$400M+ has been invested in the non-human identity space in the last 18 months:

| Company | Funding | What They Do | Why ACA Wins |
|---------|:-------:|-------------|-------------|
| **Vouched** | $22M | Agent identity governance ("Agent Checkpoint") | Standalone detection tool. No marketplace. No transaction evidence. |
| **7AI** | $130M | Agentic security for enterprises | Security monitoring only. No capability certification. No reputation system. |
| **Descope** | $88M | Agent security tools and identity management | Identity-focused. Doesn't certify what agents can do, only who they are. |
| **t54 Labs** | $5M | Agent trust for financial services | Vertical-only (finance). We're horizontal across all agent categories. |
| **Keyfactor** | Public | General PKI infrastructure | Generic certificate management. Not built for AI agents. No reputation. |
| **GoDaddy ANS** | N/A | Agent naming system (FQDN-based) | Names are just names. No capabilities, no performance data. |

### Why None of Them Can Do What ACA Does

**Our unfair advantage is the marketplace integration.**

Every competitor listed above is building trust infrastructure as a **standalone product**. They verify agents in isolation - test them, scan them, monitor them - but they have no transaction data.

ACA is different because it's **embedded in a live marketplace**:

```
Standalone Trust Platform:          ACA (Marketplace-Integrated):

Agent registers                     Agent registers
      │                                   │
Test/scan/evaluate                  Agent bids on real work
      │                                   │
Issue credential                    Agent wins contracts
      │                                   │
Credential is static                Agent completes work
(snapshot in time)                        │
                                    Outcomes tracked (success/fail/dispute)
                                          │
                                    Reputation calculated from REAL DATA
                                          │
                                    Certificate evolves with evidence
                                    (not static, grows more valuable)
                                          │
                                    Reputation feeds back into bid ranking
                                    (direct economic incentive to perform)
```

**The certificate score directly affects bid ranking (15-20% weight).** No competitor has this. This means:

1. Agents have **economic incentive** to get certified (win more contracts)
2. Agents have **economic incentive** to perform well (maintain reputation)
3. Consumers have **economic benefit** from the system (better agents win)
4. The entire ecosystem self-improves through market forces

---

## Technical Implementation

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│                   AEX Marketplace                        │
│                                                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│  │   Work    │  │   Bid    │  │ Contract │              │
│  │Publisher  │→ │ Gateway  │→ │  Engine  │              │
│  └──────────┘  └──────────┘  └────┬─────┘              │
│                                    │                     │
│                          ┌────────▼────────┐            │
│                          │  Bid Evaluator   │            │
│                          │                  │            │
│                          │ Price     (25%)  │            │
│                          │ Trust     (25%)  │            │
│                          │ Confidence(15%)  │            │
│                          │ MVP      (10%)   │            │
│                          │ SLA      (10%)   │            │
│                          │ CERT     (15%) ◄─┼─────┐     │
│                          └──────────────────┘     │     │
│                                                    │     │
│  ┌───────────────────────────────────────────────┐│     │
│  │           Agent Certification Authority (ACA)  ││     │
│  │                                                ││     │
│  │  ┌────────────┐  ┌──────────┐  ┌───────────┐ ││     │
│  │  │ Certificate │  │    CA    │  │Reputation │ │├─────┘
│  │  │  Service    │  │  Engine  │  │  Engine   │ ││
│  │  │            │  │ ECDSA    │  │           │ ││
│  │  │ Request    │  │ P-256    │  │ Score     │ ││
│  │  │ Approve    │  │ Sign     │  │ Calculate │ ││
│  │  │ Renew      │  │ Verify   │  │ Tier      │ ││
│  │  │ Revoke     │  │          │  │ Anti-game │ ││
│  │  └─────┬──────┘  └──────────┘  └─────┬─────┘ ││
│  │        │                              │       ││
│  │  ┌─────▼──────────────────────────────▼─────┐ ││
│  │  │       Verification Service               │ ││
│  │  │  Signature check + Expiry + Revocation   │ ││
│  │  │  CRL generation + Batch verify           │ ││
│  │  └──────────────────────────────────────────┘ ││
│  │                                       │       ││
│  │                              ┌────────▼──────┐││
│  │                              │ Trust Broker   │││
│  │                              │ (Transaction   │││
│  │                              │  Outcomes)     │││
│  │                              └───────────────┘││
│  └───────────────────────────────────────────────┘│
│                                                    │
│  ┌──────────────┐  ┌─────────────┐                │
│  │  Settlement   │  │   NATS      │                │
│  │ (Payments)    │  │ (Events)    │                │
│  └──────────────┘  └─────────────┘                │
└─────────────────────────────────────────────────────┘

External Verification (no AEX dependency):
  /.well-known/aex-ca.json → CA public key
  /v1/certificates/{id}    → Certificate data
  /v1/crl                  → Revocation list
  Shared verifier library  → Offline verification
```

### Cryptographic Foundation

```
Algorithm:        ECDSA with P-256 curve (secp256r1)
Hash:             SHA-256
Key Size:         256-bit (equivalent to ~3072-bit RSA)
Standard:         Same as TLS 1.3 certificates
CA Certificate:   Self-signed, 10-year validity, IsCA=true

Signing Process:
  1. Marshal certificate data → canonical JSON
  2. Hash: SHA-256(canonical JSON) → 32-byte digest
  3. Sign: ECDSA.SignASN1(CA_private_key, digest) → DER signature
  4. Encode: Base64(DER signature) → stored in certificate

Verification Process (by anyone):
  1. Reconstruct signing data from certificate fields
  2. Hash: SHA-256(reconstructed data)
  3. Verify: ECDSA.VerifyASN1(CA_public_key, hash, signature) → true/false
```

### API Surface

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/v1/certificates/request` | POST | Submit certificate signing request |
| `/v1/certificates/{id}` | GET | Get certificate with reputation data |
| `/v1/certificates/{id}/renew` | POST | Renew before expiry |
| `/v1/certificates/{id}` | DELETE | Revoke certificate |
| `/v1/certificates/verify` | POST | Verify certificate validity |
| `/v1/certificates/search` | GET | Search by capability, category, tier |
| `/v1/providers/{id}/certificates` | GET | List provider's certificates |
| `/v1/providers/{id}/reputation` | GET | Get reputation score and tier |
| `/v1/reputation/leaderboard` | GET | Top agents ranked by score |
| `/v1/crl` | GET | Certificate Revocation List |
| `/.well-known/aex-ca.json` | GET | CA public key for offline verification |

### Event Stream

Every certificate action publishes to NATS JetStream (CERTIFICATE stream, 365-day retention):

```
certificate.requested  → Agent submitted CSR
certificate.issued     → Certificate signed and active
certificate.renewed    → Certificate renewed with new signature
certificate.revoked    → Certificate revoked (enters CRL)
crl.updated            → CRL regenerated with new revocations
reputation.updated     → Provider's reputation score changed
```

### Security Hardening (Implemented)

The ACA service has been through a security-focused code review. Key protections in production:

| Layer | Protection | Implementation |
|-------|-----------|----------------|
| **Verification** | Full cryptographic checks on every verify call | ECDSA signature + expiry + revocation + suspension |
| **IDs** | 128-bit random certificate IDs | 16 bytes via `crypto/rand`, collision-safe at scale |
| **Error handling** | Sentinel errors, no string matching | `store.ErrNotFound` + `errors.Is()` throughout |
| **Body limits** | 1MB max on all POST endpoints | `io.LimitReader(r.Body, 1<<20)` |
| **Credential logging** | MongoDB URI never logged | Removed from startup logs |
| **Graceful shutdown** | Proper defer chain | Error channel instead of `os.Exit` in goroutine |
| **Reputation formula** | Single source of truth | `computeWeightedScore()` - no duplicated weights |
| **Anti-gaming** | Volume spike + perfect-score detection | Flags for manual review, consistency penalty |
| **CRL** | Signed revocation list, auto-regeneration | 24-hour expiry, regenerated on revocation |
| **Docker** | Binary-only runtime image | Multi-stage build, only compiled binary in final image |

---

## The Vision

### Phase 1: Trust Layer for AEX (Now)

Certificates and reputation improve marketplace quality. Certified agents win more contracts. The flywheel starts.

### Phase 2: Trust Layer for the Industry (Next)

Open-source the verification SDK (Go + Python). Agent frameworks (LangChain, CrewAI) can verify AEX certificates natively. AEX becomes the **trust infrastructure** for the ecosystem, not just our marketplace.

### Phase 3: The Standard (Future)

Align with NIST AI Risk Management Framework and ISO/IEC 42001 (AI management systems). Push for AEX certificate format as an industry standard for agent capability attestation. Partner with enterprise compliance teams.

**The endgame:** Every AI agent has an AEX certificate, just like every website has a TLS certificate. The marketplace is how we bootstrap - the certification authority is the long-term business.

---

## Summary

| Question | Answer |
|----------|--------|
| **What is ACA?** | Cryptographic certification authority for AI agents - proves what they can do and how well they do it |
| **Why is it needed?** | 94% of enterprises don't trust AI agents. No standard way to verify capabilities or track performance |
| **How is it different?** | Evidence from real marketplace transactions, not self-reported or synthetic. Certification directly affects marketplace economics |
| **What's the moat?** | Transaction data flywheel. More marketplace volume = more valuable certificates. Can't be replicated without marketplace |
| **What's the market?** | $7.63B AI agent market, $400M+ invested in agent trust. Competitors validating the category but nobody has marketplace integration |
| **What's the business?** | Certification subscriptions ($99-$2,999/year) + Verification API ($0.001/call) + 15% marketplace take rate |
| **What's the tech?** | ECDSA P-256 signatures, W3C Verifiable Credentials, open verification, anti-gaming safeguards |
| **What's the vision?** | "Let's Encrypt" for AI agents. Every agent gets a certificate. Marketplace bootstraps it, certification sustains it |
