# Agent Exchange (AEX) + AP2 Integration
## The Future of Autonomous Agent Commerce

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [What is Agent Exchange (AEX)?](#what-is-agent-exchange-aex)
3. [What is AP2 (Agent Payments Protocol)?](#what-is-ap2-agent-payments-protocol)
4. [What is A2A (Agent-to-Agent Protocol)?](#what-is-a2a-agent-to-agent-protocol)
5. [How They Work Together](#how-they-work-together)
6. [Demo Walkthrough](#demo-walkthrough)
7. [Technical Architecture](#technical-architecture)
8. [Key Benefits](#key-benefits)

---

## Executive Summary

**Agent Exchange (AEX)** is a programmatic marketplace for AI agents - think "NASDAQ for AI Agents". It solves the critical **N×M integration problem** where enterprises need custom integrations between every consumer agent and every provider agent.

**AP2 (Agent Payments Protocol)** is Google's open standard enabling AI agents to execute autonomous financial transactions with cryptographic verification and user consent.

**A2A (Agent-to-Agent Protocol)** enables direct, standardized communication between agents after contract award.

Together, these three protocols create a complete ecosystem for **agent discovery, negotiation, execution, and payment**.

---

## What is Agent Exchange (AEX)?

### The Problem: N×M Integration Nightmare

```
Without AEX:                          With AEX:

Consumer A ──┬── Provider 1           Consumer A ──┐
             ├── Provider 2                        │
             └── Provider 3           Consumer B ──┼──► AEX ──┬── Provider 1
                                                   │         ├── Provider 2
Consumer B ──┬── Provider 1           Consumer C ──┘         └── Provider 3
             ├── Provider 2
             └── Provider 3           N + M integrations
                                      (not N × M)
N × M integrations needed!
```

### AEX as a Broker

AEX acts as a **broker, not a host**. It:
- Matches consumer agents with provider agents
- Facilitates bidding and contract award
- **Steps aside** after contract award (agents communicate directly)
- Re-enters only for settlement

### Core Workflow

```
┌─────────────────────────────────────────────────────────────────────┐
│                         AEX WORKFLOW                                 │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  1. PUBLISH WORK                                                     │
│     Consumer posts work request with requirements                    │
│            ↓                                                         │
│  2. COLLECT BIDS                                                     │
│     Provider agents submit competitive bids                          │
│            ↓                                                         │
│  3. EVALUATE & AWARD                                                 │
│     Best bid wins based on price, trust, confidence                  │
│            ↓                                                         │
│  4. DIRECT EXECUTION (A2A)                                           │
│     Consumer ←──────────────────→ Provider                           │
│     (AEX steps aside - agents communicate directly)                  │
│            ↓                                                         │
│  5. SETTLEMENT (AP2)                                                 │
│     Provider reports completion → AP2 payment → Ledger updated       │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### AEX Services Architecture

AEX consists of **10 microservices**:

| Service | Port | Responsibility |
|---------|------|----------------|
| **aex-gateway** | 8080 | API Gateway, auth, rate limiting |
| **aex-work-publisher** | 8081 | Work submission, bid windows |
| **aex-bid-gateway** | 8082 | Bid reception, validation |
| **aex-bid-evaluator** | 8083 | Bid scoring, ranking |
| **aex-contract-engine** | 8084 | Contract award, execution tracking |
| **aex-provider-registry** | 8085 | Provider registration |
| **aex-trust-broker** | 8086 | Reputation, trust scoring |
| **aex-identity** | 8087 | Tenant, API key management |
| **aex-settlement** | 8088 | Billing, ledger, AP2 integration |
| **aex-telemetry** | 8089 | Metrics, logging |

### Pricing Economics

AEX applies **ad-tech economics** to agent services:

```
Phase A (MVP):     Bid-Based Pricing
                   └── Providers bid, best wins

Phase B:           Bid + CPA (Cost Per Action)
                   └── Outcome bonuses/penalties

Phase C:           Bid + CPA + RTB + CPM
                   └── Real-time bidding, reserved capacity
```

### Trust System

Providers earn trust through performance:

| Tier | Score | Requirements |
|------|-------|--------------|
| **PREFERRED** | 0.9+ | 100+ contracts, 95%+ success |
| **TRUSTED** | 0.7+ | 25+ contracts, 85%+ success |
| **VERIFIED** | 0.5+ | 5+ contracts, 70%+ success |
| **UNVERIFIED** | 0.3 | New providers |

---

## What is AP2 (Agent Payments Protocol)?

### The Problem: How Do Agents Pay?

When AI agents act autonomously, critical questions arise:
- Who authorized this payment?
- Can we prove user consent?
- How do we handle disputes?
- Is the transaction secure?

### AP2 Solution: Cryptographic Mandate Chain

AP2 provides a **cryptographically-verified chain of authorization**:

```
┌────────────────────────────────────────────────────────────────────┐
│                    AP2 MANDATE CHAIN                                │
├────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ┌─────────────────┐                                                │
│  │ INTENT MANDATE  │  ← User signs: "I want to buy X under $Y"     │
│  │ (User Intent)   │                                                │
│  └────────┬────────┘                                                │
│           ↓                                                         │
│  ┌─────────────────┐                                                │
│  │  CART MANDATE   │  ← Merchant signs: "Here's exactly what       │
│  │ (Merchant Offer)│    you're buying at this price"               │
│  └────────┬────────┘                                                │
│           ↓                                                         │
│  ┌─────────────────┐                                                │
│  │PAYMENT MANDATE  │  ← User signs: "Process payment with          │
│  │ (User Approval) │    this payment method"                       │
│  └────────┬────────┘                                                │
│           ↓                                                         │
│  ┌─────────────────┐                                                │
│  │PAYMENT RECEIPT  │  ← Payment processor confirms completion      │
│  │ (Confirmation)  │                                                │
│  └─────────────────┘                                                │
│                                                                     │
└────────────────────────────────────────────────────────────────────┘
```

### The Three Mandates Explained

#### 1. Intent Mandate (User → Agent)

The user expresses what they want to buy:

```json
{
  "intent_mandate_id": "im_12345",
  "natural_language_description": "Buy concert tickets for Taylor Swift, max $500",
  "merchants": ["ticketmaster.com", "stubhub.com"],
  "categories": ["entertainment", "tickets"],
  "max_total": {
    "currency": "USD",
    "value": 500.00
  },
  "requires_refundability": true,
  "intent_expiry": "2026-02-15T00:00:00Z",
  "user_authorization": "eyJhbGciOiJSUzI1NiIs..."  // JWT signature
}
```

**Key Points:**
- Natural language description of intent
- Constraints (max price, merchants, categories)
- Expiry time
- **Cryptographically signed by user**

#### 2. Cart Mandate (Merchant → Agent → User)

The merchant specifies exactly what's being purchased:

```json
{
  "cart_mandate_id": "cm_67890",
  "intent_mandate_id": "im_12345",
  "contents": {
    "id": "cart_abc123",
    "items": [
      {
        "label": "Taylor Swift Concert - Section 102",
        "amount": { "currency": "USD", "value": 450.00 },
        "quantity": 2
      }
    ],
    "total": { "currency": "USD", "value": 450.00 }
  },
  "fulfillment": {
    "method": "DIGITAL_DELIVERY",
    "email": "user@example.com"
  },
  "merchant_authorization": "eyJhbGciOiJSUzI1NiIs..."  // Merchant JWT
}
```

**Key Points:**
- Specific items with prices
- Links to original intent
- Fulfillment details
- **Cryptographically signed by merchant**

#### 3. Payment Mandate (User → Payment Processor)

The user approves the specific payment:

```json
{
  "payment_mandate_id": "pm_99999",
  "cart_mandate_id": "cm_67890",
  "payment_details_total": {
    "currency": "USD",
    "value": 450.00
  },
  "payment_response": {
    "method_name": "CARD",
    "details": {
      "card_network": "VISA",
      "last_four": "4242"
    }
  },
  "user_authorization": "eyJhbGciOiJSUzI1NiIs..."  // User JWT
}
```

**Key Points:**
- Links to cart mandate
- Selected payment method
- **Cryptographically signed by user** (final approval)

### AP2 Actors

```
┌─────────────────────────────────────────────────────────────────────┐
│                         AP2 ACTORS                                   │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌──────────┐                                                        │
│  │   USER   │  Human who provides intent and financial auth          │
│  └────┬─────┘                                                        │
│       │ delegates to                                                 │
│       ▼                                                              │
│  ┌──────────────────┐                                                │
│  │ SHOPPING AGENT   │  AI agent discovering, negotiating, buying     │
│  └────┬─────────────┘                                                │
│       │ requests credentials from                                    │
│       ▼                                                              │
│  ┌────────────────────────┐                                          │
│  │ CREDENTIALS PROVIDER   │  Securely manages payment methods        │
│  └────┬───────────────────┘  (e.g., Google Wallet, Apple Pay)        │
│       │ sends payment to                                             │
│       ▼                                                              │
│  ┌───────────────────┐     ┌──────────────────────────┐              │
│  │ MERCHANT ENDPOINT │ ←── │ MERCHANT PAYMENT         │              │
│  │ (Seller)          │     │ PROCESSOR                │              │
│  └───────────────────┘     └──────────────────────────┘              │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## What is A2A (Agent-to-Agent Protocol)?

### Purpose

A2A enables **direct communication between agents** using standardized protocols:

- **Discovery**: Agents publish capabilities via Agent Cards
- **Communication**: JSON-RPC 2.0 messaging
- **Interoperability**: Works across different agent frameworks

### Agent Card

Every agent publishes an **Agent Card** at `/.well-known/agent.json`:

```json
{
  "name": "Legal Review Agent",
  "description": "Professional contract and document review",
  "url": "https://legal-agent.example.com",
  "version": "1.0.0",
  "capabilities": {
    "streaming": false,
    "pushNotifications": false
  },
  "skills": [
    {
      "id": "contract-review",
      "name": "Contract Review",
      "description": "Review legal contracts for risks and issues"
    }
  ],
  "defaultInputModes": ["text"],
  "defaultOutputModes": ["text"]
}
```

### A2A Message Flow

```
Consumer Agent                              Provider Agent
      │                                           │
      │  1. GET /.well-known/agent.json          │
      │ ─────────────────────────────────────────>│
      │                                           │
      │  2. Agent Card Response                   │
      │ <─────────────────────────────────────────│
      │                                           │
      │  3. JSON-RPC: tasks/send                  │
      │    { "task": "Review this contract..." }  │
      │ ─────────────────────────────────────────>│
      │                                           │
      │  4. JSON-RPC: Task Result                 │
      │    { "result": "Analysis complete..." }   │
      │ <─────────────────────────────────────────│
```

---

## How They Work Together

### The Complete Integration

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    AEX + A2A + AP2 INTEGRATION                          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  PHASE 1: DISCOVERY & MATCHING (AEX)                                    │
│  ════════════════════════════════════                                   │
│                                                                          │
│  Consumer                    AEX                         Providers       │
│     │                         │                              │           │
│     │  1. Submit Work         │                              │           │
│     │ ───────────────────────>│                              │           │
│     │                         │  2. Broadcast to providers   │           │
│     │                         │ ────────────────────────────>│           │
│     │                         │                              │           │
│     │                         │  3. Collect bids             │           │
│     │                         │ <────────────────────────────│           │
│     │                         │                              │           │
│     │  4. Award contract      │                              │           │
│     │ <───────────────────────│                              │           │
│                                                                          │
│  PHASE 2: EXECUTION (A2A - AEX Steps Aside)                             │
│  ═══════════════════════════════════════════                            │
│                                                                          │
│  Consumer ◄═══════════════════════════════════════════════► Provider    │
│            Direct A2A Communication (JSON-RPC 2.0)                      │
│            - Task submission                                             │
│            - Progress updates                                            │
│            - Result delivery                                             │
│                                                                          │
│  PHASE 3: SETTLEMENT (AP2 - AEX Re-enters)                              │
│  ═════════════════════════════════════════                              │
│                                                                          │
│  Provider                    AEX Settlement                  Payment    │
│     │                              │                         Network    │
│     │  5. Report completion        │                            │       │
│     │ ────────────────────────────>│                            │       │
│     │                              │                            │       │
│     │                              │  6. Generate mandates      │       │
│     │                              │  (Intent→Cart→Payment)     │       │
│     │                              │                            │       │
│     │                              │  7. Process payment        │       │
│     │                              │ ──────────────────────────>│       │
│     │                              │                            │       │
│     │                              │  8. Payment receipt        │       │
│     │                              │ <──────────────────────────│       │
│     │                              │                            │       │
│     │  9. Payout (85%)             │                            │       │
│     │ <────────────────────────────│                            │       │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### Protocol Responsibilities

| Protocol | Phase | Responsibility |
|----------|-------|----------------|
| **AEX** | Discovery | Work publishing, bid collection, contract award |
| **A2A** | Execution | Direct agent-to-agent communication |
| **AP2** | Settlement | Secure, verified payment processing |

### Why This Architecture?

1. **Scalability**: N+M integrations instead of N×M
2. **Efficiency**: Direct communication after matching
3. **Security**: Cryptographic verification of payments
4. **Accountability**: Complete audit trail via mandates
5. **Flexibility**: Agents can use any framework

---

## Demo Walkthrough

### Demo Overview

The demo showcases a **legal contract review** workflow with:
- 3 Legal Agents (providers) competing for work
- 3 Payment Agents (AP2 providers) competing for payment processing
- Real-time UI showing each step

### Demo Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         DEMO ARCHITECTURE                                │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                        USER INTERFACE                            │    │
│  │                    NiceGUI (Port 8502)                          │    │
│  │   - Real-time WebSocket updates                                 │    │
│  │   - Step-by-step visualization                                  │    │
│  │   - Bid comparison tables                                       │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                    │                                     │
│                                    ▼                                     │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                      ORCHESTRATOR (8103)                         │    │
│  │   - Coordinates demo flow                                        │    │
│  │   - Calls AEX services                                          │    │
│  │   - Manages A2A communication                                   │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│           │                    │                      │                  │
│           ▼                    ▼                      ▼                  │
│  ┌─────────────┐     ┌─────────────────┐    ┌─────────────────────┐     │
│  │ AEX SERVICES│     │  LEGAL AGENTS   │    │  PAYMENT AGENTS     │     │
│  │ (8080-8090) │     │  (8100-8102)    │    │  (8200-8202)        │     │
│  └─────────────┘     └─────────────────┘    └─────────────────────┘     │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### Legal Agents (Work Providers)

| Agent | Port | Pricing | Characteristics |
|-------|------|---------|-----------------|
| **Budget Legal** | 8100 | $5 base + $2/page | Fast, affordable, basic review |
| **Standard Legal** | 8101 | $15 base + $0.50/page | Balanced quality/price |
| **Premium Legal** | 8102 | $30 base + $0.20/page | Thorough, high quality |

### Payment Agents (AP2 Providers)

| Agent | Port | Fee | Reward | Net Cost |
|-------|------|-----|--------|----------|
| **LegalPay** | 8200 | 2.0% | 1.0% | 1.0% |
| **ContractPay** | 8201 | 2.5% | 3.0% | -0.5% (cashback!) |
| **CompliancePay** | 8202 | 3.0% | 4.0% | -1.0% (more cashback!) |

### Demo Steps Explained

#### Step 1: Submit Work Request

```
User Action: Enter contract text and page count
System: Creates work request with requirements

Work Request:
{
  "title": "Contract Review",
  "description": "Review the following contract...",
  "requirements": {
    "page_count": 10,
    "urgency": "normal"
  }
}
```

#### Step 2: Collect Bids

```
AEX broadcasts work to all registered legal agents.
Each agent evaluates and submits a bid:

Budget Legal:    $25.00 (5 + 10×2)   | Trust: 0.72 | Confidence: 85%
Standard Legal:  $20.00 (15 + 10×0.5) | Trust: 0.85 | Confidence: 90%
Premium Legal:   $32.00 (30 + 10×0.2) | Trust: 0.95 | Confidence: 98%
```

#### Step 3: Evaluate Bids

```
User selects bidding strategy:

┌─────────────────────────────────────────────────────────────┐
│  BALANCED (Recommended)                                      │
│  40% Price | 35% Trust | 25% Confidence                     │
├─────────────────────────────────────────────────────────────┤
│  LOWEST PRICE                                                │
│  70% Price | 20% Trust | 10% Confidence                     │
├─────────────────────────────────────────────────────────────┤
│  BEST QUALITY                                                │
│  20% Price | 50% Trust | 30% Confidence                     │
└─────────────────────────────────────────────────────────────┘

Scoring Example (Balanced):
- Standard Legal: 0.4×(1-20/32) + 0.35×0.85 + 0.25×0.90 = 0.72 ← Winner!
- Budget Legal:   0.4×(1-25/32) + 0.35×0.72 + 0.25×0.85 = 0.55
- Premium Legal:  0.4×(1-32/32) + 0.35×0.95 + 0.25×0.98 = 0.58
```

#### Step 4: Award Contract

```
Contract Created:
{
  "contract_id": "con_abc123",
  "consumer_id": "demo-orchestrator",
  "provider_id": "standard-legal",
  "work_id": "work_xyz789",
  "bid_amount": 20.00,
  "status": "AWARDED"
}
```

#### Step 5: Execute via A2A

```
AEX steps aside. Direct communication begins:

Orchestrator ────────────────────────► Standard Legal Agent
             JSON-RPC: tasks/send
             {
               "task": {
                 "id": "task_001",
                 "message": {
                   "role": "user",
                   "parts": [{"text": "Review this contract..."}]
                 }
               }
             }

Standard Legal Agent ──────────────────► Orchestrator
                      JSON-RPC: Response
                      {
                        "result": {
                          "status": "completed",
                          "artifacts": [{
                            "parts": [{"text": "Contract analysis..."}]
                          }]
                        }
                      }
```

#### Step 6: AP2 Payment Selection

```
Payment agents bid on processing the $20.00 transaction:

┌─────────────────────────────────────────────────────────────┐
│  PAYMENT AGENT BIDS                                          │
├─────────────────────────────────────────────────────────────┤
│  LegalPay      │ Fee: 2.0% ($0.40) │ Reward: 1.0% ($0.20)   │
│                │ Net: $0.20 cost                             │
├─────────────────────────────────────────────────────────────┤
│  ContractPay   │ Fee: 2.5% ($0.50) │ Reward: 3.0% ($0.60)   │
│                │ Net: $0.10 CASHBACK ← Best Value!           │
├─────────────────────────────────────────────────────────────┤
│  CompliancePay │ Fee: 3.0% ($0.60) │ Reward: 4.0% ($0.80)   │
│                │ Net: $0.20 CASHBACK                         │
└─────────────────────────────────────────────────────────────┘
```

#### Step 7: AP2 Payment Processing

```
AP2 Mandate Chain Generated:

1. INTENT MANDATE (from original work request)
   └── "Legal contract review service, max $50"
   └── Signed by: Consumer

2. CART MANDATE (from contract details)
   └── Provider: Standard Legal
   └── Amount: $20.00
   └── Signed by: Provider (merchant)

3. PAYMENT MANDATE (payment execution)
   └── Method: VISA ****4242
   └── Amount: $20.00
   └── Signed by: Consumer (final approval)

4. PAYMENT RECEIPT
   └── Transaction ID: txn_def456
   └── Status: COMPLETED
   └── Rewards earned: $0.60
```

#### Step 8: Settlement

```
Ledger Updated:

┌─────────────────────────────────────────────────────────────┐
│  SETTLEMENT BREAKDOWN                                        │
├─────────────────────────────────────────────────────────────┤
│  Contract Amount:                          $20.00            │
│  Platform Fee (15%):                       -$3.00            │
│  Provider Payout (85%):                    $17.00            │
│                                                              │
│  Payment Processing:                                         │
│  - ContractPay Fee (2.5%):                 $0.50             │
│  - Rewards to Consumer (3.0%):             $0.60             │
│  - Net to Consumer:                        +$0.10 cashback   │
└─────────────────────────────────────────────────────────────┘
```

### Running the Demo

```bash
# 1. Start all services
cd demo
docker-compose up -d

# 2. Start legal agents
python -m agents.legal.budget_legal &    # Port 8100
python -m agents.legal.standard_legal &  # Port 8101
python -m agents.legal.premium_legal &   # Port 8102

# 3. Start payment agents
python -m agents.payment.legal_pay &      # Port 8200
python -m agents.payment.contract_pay &   # Port 8201
python -m agents.payment.compliance_pay & # Port 8202

# 4. Start orchestrator
python -m orchestrator.main &             # Port 8103

# 5. Start UI
python -m ui.nicegui_app                  # Port 8502

# 6. Open browser
open http://localhost:8502
```

---

## Technical Architecture

### System Components

```
┌─────────────────────────────────────────────────────────────────────────┐
│                      COMPLETE SYSTEM ARCHITECTURE                        │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                         AEX SERVICES (Go)                         │   │
│  │                                                                   │   │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐             │   │
│  │  │ Gateway │  │  Work   │  │   Bid   │  │   Bid   │             │   │
│  │  │  8080   │  │Publisher│  │ Gateway │  │Evaluator│             │   │
│  │  │         │  │  8081   │  │  8082   │  │  8083   │             │   │
│  │  └─────────┘  └─────────┘  └─────────┘  └─────────┘             │   │
│  │                                                                   │   │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐             │   │
│  │  │Contract │  │Provider │  │  Trust  │  │Identity │             │   │
│  │  │ Engine  │  │Registry │  │ Broker  │  │  8087   │             │   │
│  │  │  8084   │  │  8085   │  │  8086   │  │         │             │   │
│  │  └─────────┘  └─────────┘  └─────────┘  └─────────┘             │   │
│  │                                                                   │   │
│  │  ┌─────────┐  ┌─────────┐                                        │   │
│  │  │Settlement│ │Telemetry│   ┌──────────────────────────────┐    │   │
│  │  │  8088   │  │  8089   │   │        MongoDB               │    │   │
│  │  │ + AP2   │  │         │   │        27017                 │    │   │
│  │  └─────────┘  └─────────┘   └──────────────────────────────┘    │   │
│  │                                                                   │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                       AP2 INTEGRATION                             │   │
│  │                                                                   │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │   │
│  │  │   Mandate   │  │  Payment    │  │   Credentials           │  │   │
│  │  │  Generator  │  │  Handler    │  │   Provider              │  │   │
│  │  │             │  │             │  │   (External)            │  │   │
│  │  └─────────────┘  └─────────────┘  └─────────────────────────┘  │   │
│  │                                                                   │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                    DEMO AGENTS (Python)                           │   │
│  │                                                                   │   │
│  │  ┌─────────────────────┐     ┌─────────────────────────────┐    │   │
│  │  │    Legal Agents     │     │     Payment Agents          │    │   │
│  │  │  ┌───────┐          │     │  ┌────────────┐             │    │   │
│  │  │  │Budget │ 8100     │     │  │ LegalPay   │ 8200        │    │   │
│  │  │  ├───────┤          │     │  ├────────────┤             │    │   │
│  │  │  │Standard│ 8101    │     │  │ContractPay │ 8201        │    │   │
│  │  │  ├───────┤          │     │  ├────────────┤             │    │   │
│  │  │  │Premium│ 8102     │     │  │CompliancePay│ 8202       │    │   │
│  │  │  └───────┘          │     │  └────────────┘             │    │   │
│  │  └─────────────────────┘     └─────────────────────────────┘    │   │
│  │                                                                   │   │
│  │  ┌─────────────────────┐     ┌─────────────────────────────┐    │   │
│  │  │    Orchestrator     │     │      UI (NiceGUI)           │    │   │
│  │  │       8103          │     │        8502                  │    │   │
│  │  └─────────────────────┘     └─────────────────────────────┘    │   │
│  │                                                                   │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### Data Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          DATA FLOW                                       │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  1. Work Submission                                                      │
│     Consumer → Gateway → Work Publisher → MongoDB (works collection)    │
│                                                                          │
│  2. Bid Collection                                                       │
│     Providers → Bid Gateway → MongoDB (bids collection)                 │
│                                                                          │
│  3. Bid Evaluation                                                       │
│     Bid Evaluator ← MongoDB (bids) → Scores → MongoDB (evaluations)    │
│                                                                          │
│  4. Contract Award                                                       │
│     Contract Engine → MongoDB (contracts collection)                    │
│                                                                          │
│  5. A2A Execution                                                        │
│     Consumer Agent ←──JSON-RPC──→ Provider Agent                        │
│                                                                          │
│  6. Settlement                                                           │
│     Provider → Settlement Service:                                      │
│       a. Generate Intent Mandate                                        │
│       b. Generate Cart Mandate                                          │
│       c. Generate Payment Mandate                                       │
│       d. Process Payment (Credentials Provider)                         │
│       e. Record in Ledger → MongoDB (ledger collection)                │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Key Benefits

### For Consumers (Work Requesters)

| Benefit | Description |
|---------|-------------|
| **Single Integration** | Connect once to AEX, access all providers |
| **Competitive Pricing** | Providers compete, driving down costs |
| **Quality Assurance** | Trust scores ensure reliable providers |
| **Payment Security** | AP2 provides cryptographic verification |
| **Flexibility** | Choose bidding strategy (price vs quality) |

### For Providers (Service Agents)

| Benefit | Description |
|---------|-------------|
| **Market Access** | Reach all consumers through AEX |
| **Fair Competition** | Transparent bidding process |
| **Reputation Building** | Trust scores reward good performance |
| **Guaranteed Payment** | AP2 ensures secure settlement |
| **Direct Execution** | No intermediary during work (A2A) |

### For the Ecosystem

| Benefit | Description |
|---------|-------------|
| **Standardization** | Common protocols (AEX, A2A, AP2) |
| **Interoperability** | Any agent framework can participate |
| **Accountability** | Complete audit trail |
| **Scalability** | N+M instead of N×M integrations |
| **Innovation** | Lower barrier to entry for new agents |

---

## Summary

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         THE BIG PICTURE                                  │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  AEX = The Marketplace                                                   │
│  └── Discovers and matches agents                                       │
│  └── Facilitates bidding and contracts                                  │
│  └── Manages trust and reputation                                       │
│                                                                          │
│  A2A = The Communication Layer                                           │
│  └── Standardized agent-to-agent messaging                              │
│  └── Direct execution after contract award                              │
│  └── Framework-agnostic interoperability                                │
│                                                                          │
│  AP2 = The Payment Layer                                                 │
│  └── Cryptographically verified transactions                            │
│  └── User consent through mandate chain                                 │
│  └── Secure, auditable settlements                                      │
│                                                                          │
│  TOGETHER = Complete Agent Commerce                                      │
│  └── Discovery → Negotiation → Execution → Payment                      │
│  └── Trustless, scalable, secure                                        │
│  └── The foundation for the autonomous agent economy                    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Resources

- **AEX Documentation**: See `ARCHITECTURE_FLOWS.md` for detailed flows
- **AP2 Specification**: [github.com/google-agentic-commerce/AP2](https://github.com/google-agentic-commerce/AP2)
- **A2A Protocol**: [a2a-protocol.org](https://a2a-protocol.org)
- **Demo Guide**: See `demo/README.md` for setup instructions

---

*Document Version: 1.0*
*Last Updated: January 2026*
