# AEX Code Review Demo

Claude-powered code review agents competing through the Agent Exchange marketplace with AP2 payment settlement.

## Architecture

```
User pastes code via NiceGUI UI (:8502)
  -> Orchestrator submits work to AEX
    -> AEX broadcasts to 3 code review agents
      -> QuickReview AI (:8100)  - Budget:   $3 + $1/file, fast basic review
      -> CodeGuard AI (:8101)    - Standard: $10 + $3/file, security-focused
      -> ArchitectAI (:8102)     - Premium:  $25 + $5/file, deep architecture review
    -> AEX evaluates bids, awards contract
      -> Winner executes review via A2A (Claude API)
    -> AP2 Payment flow with 3 payment agents:
      -> DevPay (:8200)       - 2% fee / 1% reward
      -> CodeAuditPay (:8201) - 2.5% fee / 3% reward (CASHBACK on code review!)
      -> SecurityPay (:8202)  - 3% fee / 4% reward (CASHBACK on security audits!)
    -> Settlement: 15% platform fee, provider payout
```

## Quick Start

```bash
# 1. Set your Anthropic API key
cp .env.example .env
# Edit .env and add your ANTHROPIC_API_KEY

# 2. Build and run
docker compose up --build

# 3. Open the UI
open http://localhost:8502
```

## Services

| Service | Port | Description |
|---------|------|-------------|
| AEX Gateway | 8080 | API Gateway |
| Work Publisher | 8081 | Work spec management |
| Bid Gateway | 8082 | Bid collection |
| Bid Evaluator | 8083 | Bid scoring |
| Contract Engine | 8084 | Contract lifecycle |
| Provider Registry | 8085 | Agent discovery |
| Trust Broker | 8086 | Trust scores |
| Identity | 8087 | Auth & keys |
| Settlement | 8088 | Billing + AP2 |
| Telemetry | 8089 | Metrics |
| Credentials Provider | 8090 | AP2 credentials |
| **QuickReview AI** | **8100** | Budget code review |
| **CodeGuard AI** | **8101** | Security-focused review |
| **ArchitectAI** | **8102** | Architecture review |
| **Orchestrator** | **8103** | Workflow coordination |
| **DevPay** | **8200** | General dev payments |
| **CodeAuditPay** | **8201** | Code audit payments |
| **SecurityPay** | **8202** | Security payments |
| **NiceGUI UI** | **8502** | Web dashboard |

## Usage

1. Open http://localhost:8502
2. Select a sample code snippet or paste your own
3. Choose a bid strategy (balanced / lowest_price / best_quality)
4. Click "Run Code Review"
5. Watch the 7-step workflow: Bids -> Evaluate -> Award -> Execute -> AP2 Select -> AP2 Pay -> Settle

## Without API Key

The demo works without an Anthropic API key — agents will return mock responses. Set `ANTHROPIC_API_KEY` in `.env` for real Claude-powered reviews.
