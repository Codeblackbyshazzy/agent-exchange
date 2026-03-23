# AEX + AP2 Integration
## 10-Minute Presentation Guide

---

## Presentation Overview

| Section | Duration | Content |
|---------|----------|---------|
| 1. Hook & Problem | 1 min | The N×M problem |
| 2. AEX Solution | 2 min | Marketplace concept |
| 3. AP2 Payments | 2 min | Secure agent payments |
| 4. Integration | 1 min | How it all connects |
| 5. Live Demo | 3 min | End-to-end workflow |
| 6. Wrap-up | 1 min | Key takeaways |
| **Total** | **10 min** | |

---

# DEMO SETUP: Step-by-Step Commands

## Phase 1: Start AEX Infrastructure (Before Presentation)

```bash
# Navigate to demo directory
cd demo

# Start ONLY AEX core services (no agents)
docker-compose up -d mongo aex-identity aex-provider-registry aex-trust-broker \
  aex-bid-gateway aex-bid-evaluator aex-contract-engine aex-work-publisher \
  aex-settlement aex-credentials-provider aex-telemetry aex-gateway
```

**Expected Output:**
```
[+] Running 12/12
 ✔ Container aex-mongo                 Started
 ✔ Container aex-identity              Started
 ✔ Container aex-provider-registry     Started
 ✔ Container aex-trust-broker          Started
 ✔ Container aex-bid-gateway           Started
 ✔ Container aex-bid-evaluator         Started
 ✔ Container aex-contract-engine       Started
 ✔ Container aex-work-publisher        Started
 ✔ Container aex-credentials-provider  Started
 ✔ Container aex-settlement            Started
 ✔ Container aex-telemetry             Started
 ✔ Container aex-gateway               Started
```

**Verify AEX is running:**
```bash
curl http://localhost:8080/health
```
**Expected:** `{"status":"healthy"}`

**Check registered providers (should be empty):**
```bash
curl http://localhost:8085/providers | jq
```
**Expected:**
```json
{
  "providers": [],
  "total": 0
}
```

---

## Phase 2: Start UI (Before Presentation)

```bash
# Start the NiceGUI demo interface
docker-compose up -d demo-ui-nicegui
```

**Expected Output:**
```
[+] Running 1/1
 ✔ Container aex-demo-ui-nicegui  Started
```

**Open browser:** http://localhost:8502

**What you'll see:** Empty provider list, no agents registered yet

---

## Phase 3: Add Agents One-by-One (During Demo)

### 3.1 Add First Legal Agent: Budget Legal (Port 8100)

```bash
docker-compose up -d legal-agent-a
```

**Expected Output:**
```
[+] Running 1/1
 ✔ Container aex-legal-agent-a  Started
```

**Verify agent is running:**
```bash
curl http://localhost:8100/.well-known/agent.json | jq '.name'
```
**Expected:** `"Budget Legal Agent"`

**Check AEX registry - now has 1 provider:**
```bash
curl http://localhost:8085/providers | jq '.total'
```
**Expected:** `1`

**Show agent card:**
```bash
curl http://localhost:8100/.well-known/agent.json | jq
```
**Expected:**
```json
{
  "name": "Budget Legal Agent",
  "description": "Fast, affordable contract review. $5 base + $2/page.",
  "url": "http://legal-agent-a:8100",
  "version": "1.0.0",
  "capabilities": {
    "streaming": false,
    "pushNotifications": false
  },
  "skills": [
    {
      "id": "contract-review",
      "name": "Contract Review",
      "description": "Quick legal contract review and risk identification"
    }
  ]
}
```

**UI Update:** Refresh browser - Budget Legal appears in provider list

---

### 3.2 Add Second Legal Agent: Standard Legal (Port 8101)

```bash
docker-compose up -d legal-agent-b
```

**Expected Output:**
```
[+] Running 1/1
 ✔ Container aex-legal-agent-b  Started
```

**Verify:**
```bash
curl http://localhost:8101/.well-known/agent.json | jq '.name'
```
**Expected:** `"Standard Legal Agent"`

**Check AEX registry - now has 2 providers:**
```bash
curl http://localhost:8085/providers | jq '.total'
```
**Expected:** `2`

**UI Update:** Standard Legal appears in provider list

---

### 3.3 Add Third Legal Agent: Premium Legal (Port 8102)

```bash
docker-compose up -d legal-agent-c
```

**Expected Output:**
```
[+] Running 1/1
 ✔ Container aex-legal-agent-c  Started
```

**Verify:**
```bash
curl http://localhost:8102/.well-known/agent.json | jq '.name'
```
**Expected:** `"Premium Legal Agent"`

**Check AEX registry - now has 3 providers:**
```bash
curl http://localhost:8085/providers | jq '.total'
```
**Expected:** `3`

**List all providers:**
```bash
curl http://localhost:8085/providers | jq '.providers[].name'
```
**Expected:**
```
"Budget Legal Agent"
"Standard Legal Agent"
"Premium Legal Agent"
```

**UI Update:** All 3 legal agents now visible, ready for bidding

---

### 3.4 Add Payment Agents (for AP2 demo)

```bash
# Add all 3 payment agents
docker-compose up -d payment-legalpay payment-contractpay payment-compliancepay
```

**Expected Output:**
```
[+] Running 3/3
 ✔ Container aex-payment-legalpay      Started
 ✔ Container aex-payment-contractpay   Started
 ✔ Container aex-payment-compliancepay Started
```

**Verify payment agents:**
```bash
curl http://localhost:8200/.well-known/agent.json | jq '.name'
curl http://localhost:8201/.well-known/agent.json | jq '.name'
curl http://localhost:8202/.well-known/agent.json | jq '.name'
```
**Expected:**
```
"LegalPay"
"ContractPay"
"CompliancePay"
```

---

### 3.5 Add Orchestrator (Consumer Agent)

```bash
docker-compose up -d orchestrator
```

**Expected Output:**
```
[+] Running 1/1
 ✔ Container aex-orchestrator  Started
```

**Verify:**
```bash
curl http://localhost:8103/.well-known/agent.json | jq '.name'
```
**Expected:** `"Orchestrator Agent"`

---

## Phase 4: Run the Demo Flow

Now all components are running. Use the UI to:

1. **Submit Work** - Enter contract text
2. **Watch Bids** - See 3 legal agents compete
3. **Award Contract** - Best bid wins
4. **A2A Execution** - Direct communication
5. **AP2 Payment** - Payment agents bid
6. **Settlement** - Ledger updated

---

## Quick Commands Reference

| Action | Command | Expected Result |
|--------|---------|-----------------|
| Start AEX only | `docker-compose up -d mongo aex-*` | 12 containers |
| Check health | `curl localhost:8080/health` | `{"status":"healthy"}` |
| Count providers | `curl localhost:8085/providers \| jq '.total'` | Number of agents |
| Add Budget Legal | `docker-compose up -d legal-agent-a` | Port 8100 |
| Add Standard Legal | `docker-compose up -d legal-agent-b` | Port 8101 |
| Add Premium Legal | `docker-compose up -d legal-agent-c` | Port 8102 |
| Add Payment Agents | `docker-compose up -d payment-*` | Ports 8200-8202 |
| Add Orchestrator | `docker-compose up -d orchestrator` | Port 8103 |
| Start UI | `docker-compose up -d demo-ui-nicegui` | Port 8502 |
| View logs | `docker-compose logs -f <service>` | Live logs |
| Stop all | `docker-compose down` | All stopped |

---

## Cleanup After Demo

```bash
# Stop all containers
docker-compose down

# Remove volumes (fresh start next time)
docker-compose down -v
```

---

# SLIDE 1: Title (30 seconds)

## Agent Exchange + AP2
### The Future of Autonomous Agent Commerce

**Subtitle:** Solving the N×M integration problem with programmatic agent marketplaces

**Talking Points:**
- Welcome everyone
- Today: How AI agents can discover, negotiate, and pay each other autonomously
- Real working demo at the end

---

# SLIDE 2: The Problem (1 minute)

## The N×M Integration Nightmare

```
Without a Marketplace:

Consumer A ──┬── Provider 1      Every consumer needs
             ├── Provider 2      custom integration to
             └── Provider 3      every provider

Consumer B ──┬── Provider 1
             ├── Provider 2      3 consumers × 3 providers
             └── Provider 3      = 9 integrations!

Consumer C ──┬── Provider 1      100 × 100 = 10,000 integrations!
             ├── Provider 2
             └── Provider 3
```

**Talking Points:**
- Imagine every AI assistant needing custom code for every service
- Doesn't scale
- Like before ad exchanges - advertisers called every publisher manually

---

# SLIDE 3: AEX Solution (1 minute)

## Agent Exchange: The NASDAQ for AI Agents

```
With AEX:

Consumer A ──┐                  ┌── Provider 1
Consumer B ──┼──►   AEX    ◄───┼── Provider 2
Consumer C ──┘   (Broker)       └── Provider 3

                N + M integrations
                (not N × M)
```

**Key Concept:** AEX is a **broker, not a host**
- Matches buyers and sellers
- Steps aside during execution
- Re-enters for payment

**Talking Points:**
- Just like stock exchange - AEX matches, doesn't hold
- One integration gives access to entire marketplace
- Applies ad-tech economics to AI services

---

# SLIDE 4: AEX Workflow (1 minute)

## How AEX Works

```
1. PUBLISH      Consumer posts work request
      ↓
2. BID          Providers compete with offers
      ↓
3. AWARD        Best bid wins contract
      ↓
4. EXECUTE      Direct agent-to-agent (A2A)
      ↓
5. SETTLE       Secure payment (AP2)
```

**Talking Points:**
- Competitive bidding drives efficiency
- Trust scores ensure quality
- After award, agents communicate directly
- Platform only takes 15% fee on settlement

---

# SLIDE 5: The Payment Problem (30 seconds)

## But Wait... How Do Agents Pay?

**Critical Questions:**
- Who authorized this $500 purchase?
- Can we prove the user consented?
- What if there's a dispute?
- How do we prevent fraud?

**Answer:** AP2 - Agent Payments Protocol by Google

**Talking Points:**
- Autonomous agents need autonomous payments
- But with accountability
- AP2 solves this with cryptographic proofs

---

# SLIDE 6: AP2 Mandate Chain (1.5 minutes)

## AP2: Three Signed Mandates

```
┌─────────────────────────────────────────────┐
│  1. INTENT MANDATE (User Signs)             │
│     "I want concert tickets under $500"     │
│     ✓ Signed by user                        │
└─────────────────┬───────────────────────────┘
                  ↓
┌─────────────────────────────────────────────┐
│  2. CART MANDATE (Merchant Signs)           │
│     "2 tickets, Section 102, $450 total"    │
│     ✓ Signed by merchant                    │
└─────────────────┬───────────────────────────┘
                  ↓
┌─────────────────────────────────────────────┐
│  3. PAYMENT MANDATE (User Signs Again)      │
│     "Pay $450 with Visa ending 4242"        │
│     ✓ Signed by user (final approval)       │
└─────────────────────────────────────────────┘
```

**Talking Points:**
- Three cryptographic signatures create audit trail
- User intent → Merchant offer → User approval
- Non-repudiable proof of authorization
- Any dispute? Check the mandate chain

---

# SLIDE 7: Integration Overview (1 minute)

## AEX + A2A + AP2: The Complete Picture

```
┌─────────────────────────────────────────────────────────┐
│                                                          │
│  DISCOVERY & MATCHING           AEX handles              │
│  (Work → Bid → Contract)        marketplace              │
│           │                                              │
│           ↓                                              │
│  EXECUTION                      A2A protocol             │
│  (Direct Agent ↔ Agent)         (AEX steps aside)        │
│           │                                              │
│           ↓                                              │
│  SETTLEMENT                     AP2 protocol             │
│  (Secure Payment)               (AEX re-enters)          │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

**Talking Points:**
- Three protocols, three responsibilities
- AEX: Marketplace (discovery, matching, settlement)
- A2A: Communication (direct execution)
- AP2: Payments (secure, verified)

---

# SLIDE 8: Demo Introduction (30 seconds)

## Live Demo: Legal Contract Review

**Scenario:** User needs a contract reviewed

**Players:**
- 3 Legal Agents (competing providers)
- 3 Payment Agents (competing for transaction)
- Real-time UI showing each step

**What you'll see:**
1. Agents bidding competitively
2. Contract awarded to best bid
3. Direct A2A execution
4. AP2 payment with rewards

---

# SLIDES 9-15: Demo Walkthrough (3 minutes)

## Demo Step 1: Submit Work

```
┌────────────────────────────────────────┐
│  CONTRACT REVIEW REQUEST               │
│                                        │
│  Document: Employment Agreement        │
│  Pages: 10                             │
│  Urgency: Normal                       │
│                                        │
│  [Submit for Bids]                     │
└────────────────────────────────────────┘
```

---

## Demo Step 2: Collect Bids

```
┌─────────────────────────────────────────────────────────┐
│  BIDS RECEIVED                                          │
├─────────────────────────────────────────────────────────┤
│  Agent          │ Price   │ Trust │ Confidence │ Time  │
├─────────────────────────────────────────────────────────┤
│  Budget Legal   │ $25.00  │ 72%   │ 85%        │ 1 hr  │
│  Standard Legal │ $20.00  │ 85%   │ 90%        │ 2 hr  │
│  Premium Legal  │ $32.00  │ 95%   │ 98%        │ 3 hr  │
└─────────────────────────────────────────────────────────┘
```

**Talking Points:**
- Different providers, different trade-offs
- Market competition in action

---

## Demo Step 3: Evaluate & Award

```
Bidding Strategy: BALANCED
(40% Price | 35% Trust | 25% Confidence)

Scores:
├── Standard Legal: 0.72 ← WINNER
├── Premium Legal:  0.58
└── Budget Legal:   0.55

CONTRACT AWARDED to Standard Legal
```

**Talking Points:**
- Configurable evaluation criteria
- Transparent scoring

---

## Demo Step 4: A2A Execution

```
AEX Steps Aside

Consumer ◄══════════════════════► Standard Legal
          Direct JSON-RPC 2.0

Task: "Review this employment contract..."
Result: "Analysis complete. Found 3 issues..."
```

**Talking Points:**
- No middleman during execution
- Direct, efficient communication

---

## Demo Step 5: AP2 Payment Selection

```
┌─────────────────────────────────────────────────────────┐
│  PAYMENT AGENTS BIDDING                                 │
├─────────────────────────────────────────────────────────┤
│  Agent          │ Fee   │ Reward │ Net Cost             │
├─────────────────────────────────────────────────────────┤
│  LegalPay       │ 2.0%  │ 1.0%   │ 1.0% cost           │
│  ContractPay    │ 2.5%  │ 3.0%   │ 0.5% CASHBACK ←     │
│  CompliancePay  │ 3.0%  │ 4.0%   │ 1.0% CASHBACK       │
└─────────────────────────────────────────────────────────┘
```

**Talking Points:**
- Even payment processing is a marketplace!
- Competition benefits consumers

---

## Demo Step 6: Settlement

```
┌─────────────────────────────────────────┐
│  SETTLEMENT COMPLETE                    │
├─────────────────────────────────────────┤
│  Contract Amount:      $20.00           │
│  Platform Fee (15%):   -$3.00           │
│  Provider Payout:      $17.00           │
│                                         │
│  Payment Processing:                    │
│  - Fee (2.5%):         $0.50            │
│  - Rewards (3.0%):     $0.60            │
│  - Consumer Cashback:  +$0.10           │
│                                         │
│  AP2 Mandates:         ✓ Verified       │
│  Ledger:               ✓ Updated        │
└─────────────────────────────────────────┘
```

---

# SLIDE 16: Key Takeaways (1 minute)

## What We've Built

```
┌─────────────────────────────────────────────────────────┐
│                                                          │
│  AEX    = Marketplace for AI agents                     │
│           (discovery, matching, settlement)              │
│                                                          │
│  A2A    = Direct agent communication                    │
│           (efficient execution)                          │
│                                                          │
│  AP2    = Secure agent payments                         │
│           (cryptographic verification)                   │
│                                                          │
│  Together = Complete autonomous commerce                │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

**Benefits:**
- N+M integrations (not N×M)
- Competitive pricing through bidding
- Trust scores ensure quality
- Cryptographic payment security

---

# SLIDE 17: Closing

## The Future is Autonomous

**Today:** Agents need human approval for every action

**Tomorrow:** Agents discover, negotiate, execute, and pay - autonomously and securely

**AEX + AP2 makes this possible.**

---

## Questions?

**Resources:**
- Demo: `localhost:8502`
- Docs: `PRESENTATION.md`
- AP2 Spec: `github.com/google-agentic-commerce/AP2`

---

# LIVE DEMO SCRIPT (3 minutes)

## Pre-Demo Setup (Do Before Presentation Starts)

```bash
# Terminal 1: Start AEX infrastructure only
cd demo
docker-compose up -d mongo aex-identity aex-provider-registry aex-trust-broker \
  aex-bid-gateway aex-bid-evaluator aex-contract-engine aex-work-publisher \
  aex-settlement aex-credentials-provider aex-telemetry aex-gateway

# Wait for services to be healthy (~30 seconds)
sleep 30

# Start UI
docker-compose up -d demo-ui-nicegui

# Open browser
open http://localhost:8502
```

**Keep Terminal ready for live commands during demo**

---

## Live Demo Script

### Scene 1: Empty Marketplace (30 seconds)

**[Show browser with UI]**

**SAY:** "Here's our Agent Exchange marketplace. Notice it's empty - no providers registered yet. Let me show you how agents join the marketplace."

**[Show terminal]**

```bash
# Check providers - should be empty
curl -s http://localhost:8085/providers | jq '.total'
```
**Expected output:** `0`

**SAY:** "Zero providers. Let's add our first agent."

---

### Scene 2: Add Budget Legal Agent (30 seconds)

**[Type in terminal]**

```bash
docker-compose up -d legal-agent-a
```

**SAY:** "I'm starting Budget Legal - a fast, affordable contract reviewer charging $5 base plus $2 per page."

**[Wait 5 seconds, then verify]**

```bash
curl -s http://localhost:8100/.well-known/agent.json | jq '{name, description}'
```

**Expected output:**
```json
{
  "name": "Budget Legal Agent",
  "description": "Fast, affordable contract review. $5 base + $2/page."
}
```

**[Refresh browser]**

**SAY:** "And there it is in the marketplace! The agent automatically registered with AEX."

---

### Scene 3: Add More Legal Agents (30 seconds)

**[Type in terminal]**

```bash
docker-compose up -d legal-agent-b legal-agent-c
```

**SAY:** "Now let's add two more competitors - Standard Legal at $15 base, and Premium Legal at $30 base but with better quality."

**[Wait 5 seconds, verify]**

```bash
curl -s http://localhost:8085/providers | jq '.providers[].name'
```

**Expected output:**
```
"Budget Legal Agent"
"Standard Legal Agent"
"Premium Legal Agent"
```

**[Refresh browser]**

**SAY:** "Now we have three legal agents competing for work. This is the power of the marketplace - consumers get choices."

---

### Scene 4: Add Payment Agents + Orchestrator (20 seconds)

**[Type in terminal]**

```bash
docker-compose up -d payment-legalpay payment-contractpay payment-compliancepay orchestrator
```

**SAY:** "I'm also adding three payment processors - they'll compete to process payments with different reward structures - and the orchestrator which acts as our consumer agent."

---

### Scene 5: Submit Work Request (20 seconds)

**[In browser UI]**

**SAY:** "Now let's submit a contract for review."

**[Enter in UI form:]**
- Contract text: "Employment agreement with non-compete clause..."
- Pages: 10

**[Click Submit]**

**SAY:** "Work request submitted. Watch as the agents compete to win this contract."

---

### Scene 6: Watch Bidding (20 seconds)

**[Watch UI update with bids]**

**SAY:** "Here come the bids!
- Budget Legal: $25 with 72% trust score
- Standard Legal: $20 with 85% trust - interesting, cheaper AND more trusted
- Premium Legal: $32 with 95% trust - premium price, premium quality"

**SAY:** "The marketplace uses a scoring algorithm considering price, trust, AND confidence."

---

### Scene 7: Award Contract (15 seconds)

**[Select bidding strategy in UI - "Balanced"]**

**SAY:** "Using balanced scoring... Standard Legal wins! Best value for money."

**[Contract awarded]**

**SAY:** "Contract awarded. Now AEX steps aside - the agents communicate directly."

---

### Scene 8: A2A Execution (15 seconds)

**[Show execution happening in UI]**

**SAY:** "This is A2A protocol - Agent-to-Agent direct communication. No middleman. The consumer and provider agents are talking directly via JSON-RPC."

**[Wait for result]**

**SAY:** "Work complete! Contract analyzed, issues identified."

---

### Scene 9: AP2 Payment (20 seconds)

**[Show payment agents competing]**

**SAY:** "Now for payment. Watch - even payment processing is a marketplace!"

**[UI shows payment bids]**

**SAY:** "ContractPay offers 3% rewards on a 2.5% fee - that's actually cashback! The consumer EARNS money by choosing this processor."

**[Payment completes]**

---

### Scene 10: Settlement (10 seconds)

**[Show settlement summary]**

**SAY:** "Settlement complete:
- $20 contract amount
- Platform takes 15% fee
- Provider gets $17
- Consumer earned $0.10 cashback
- And all of this is cryptographically verified with AP2 mandates."

---

### Wrap-up (10 seconds)

**SAY:** "That's the complete flow - from empty marketplace to executed contract with payment - all autonomous, all secure, all verified."

---

# Presentation Tips

## Before the Presentation

- [ ] Start AEX services (without agents)
- [ ] Start UI and open browser
- [ ] Test the health endpoint: `curl localhost:8080/health`
- [ ] Have terminal ready with commands
- [ ] Practice the timing

## During the Presentation

- **Pacing:** Don't rush the demo - let each step complete
- **Narration:** Explain what's happening at each step
- **Focus:** Emphasize the "aha moments":
  - Competitive bidding in action
  - AEX stepping aside during A2A
  - Payment agents competing with rewards
  - Mandate chain verification

## Timing Checkpoints

| Checkpoint | Time |
|------------|------|
| Finish problem/solution | 3:00 |
| Start demo | 5:00 |
| Demo complete | 8:00 |
| Q&A begins | 9:00 |

## Backup Plan

If demo fails:
- Show pre-recorded video
- Walk through screenshots in `PRESENTATION.md`
- Explain architecture diagrams

---

# Quick Reference: Service Ports

| Service | Port | Purpose |
|---------|------|---------|
| AEX Gateway | 8080 | API entry point |
| Work Publisher | 8081 | Submit work |
| Bid Gateway | 8082 | Receive bids |
| Contract Engine | 8084 | Award contracts |
| Settlement | 8088 | AP2 payments |
| Budget Legal | 8100 | Legal agent |
| Standard Legal | 8101 | Legal agent |
| Premium Legal | 8102 | Legal agent |
| LegalPay | 8200 | Payment agent |
| ContractPay | 8201 | Payment agent |
| CompliancePay | 8202 | Payment agent |
| Orchestrator | 8103 | Demo coordinator |
| NiceGUI UI | 8502 | Demo interface |
| MongoDB | 27017 | Database |
