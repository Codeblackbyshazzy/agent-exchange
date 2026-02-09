"""AEX Code Review Demo UI - NiceGUI with Real-Time WebSocket Updates."""

import asyncio
import json
import os
import time
import httpx
from dataclasses import dataclass, field
from datetime import datetime
from typing import Optional
from nicegui import ui, app

# Configuration
AEX_GATEWAY_URL = os.environ.get("AEX_GATEWAY_URL", "http://localhost:8080")
AEX_SETTLEMENT_URL = os.environ.get("AEX_SETTLEMENT_URL", "http://localhost:8088")
AEX_PROVIDER_REGISTRY_URL = os.environ.get("AEX_PROVIDER_REGISTRY_URL", "http://localhost:8085")
CODE_REVIEWER_A_URL = os.environ.get("CODE_REVIEWER_A_URL", "http://localhost:8100")
CODE_REVIEWER_B_URL = os.environ.get("CODE_REVIEWER_B_URL", "http://localhost:8101")
CODE_REVIEWER_C_URL = os.environ.get("CODE_REVIEWER_C_URL", "http://localhost:8102")
DEVPAY_URL = os.environ.get("DEVPAY_URL", "http://localhost:8200")
CODEAUDITPAY_URL = os.environ.get("CODEAUDITPAY_URL", "http://localhost:8201")
SECURITYPAY_URL = os.environ.get("SECURITYPAY_URL", "http://localhost:8202")

# Provider URL mapping
PROVIDER_URL_MAP = {
    "quickreview-ai": CODE_REVIEWER_A_URL,
    "codeguard-ai": CODE_REVIEWER_B_URL,
    "architectai": CODE_REVIEWER_C_URL,
    "code-reviewer-a": CODE_REVIEWER_A_URL,
    "code-reviewer-b": CODE_REVIEWER_B_URL,
    "code-reviewer-c": CODE_REVIEWER_C_URL,
}

# Theme colors
COLORS = {
    "bg_dark": "#0f172a",
    "bg_card": "#1e293b",
    "border": "#334155",
    "text_primary": "#f1f5f9",
    "text_secondary": "#94a3b8",
    "accent_green": "#22c55e",
    "accent_blue": "#3b82f6",
    "accent_cyan": "#06b6d4",
    "accent_orange": "#f97316",
    "accent_purple": "#a855f7",
}

TIER_COLORS = {
    "VERIFIED": COLORS["accent_orange"],
    "TRUSTED": COLORS["accent_blue"],
    "PREFERRED": COLORS["accent_green"],
    "UNVERIFIED": "#666",
}

SAMPLE_CODE = {
    "Python - Auth Handler": '''def login(username, password):
    query = "SELECT * FROM users WHERE username='" + username + "' AND password='" + password + "'"
    result = db.execute(query)
    if result:
        token = str(result[0]["id"])
        return {"token": token, "status": "ok"}
    return {"status": "fail"}

def get_user_data(user_id):
    data = db.execute(f"SELECT * FROM users WHERE id={user_id}")
    return data
''',
    "JavaScript - API Server": '''const express = require('express');
const app = express();

app.get('/api/users/:id', (req, res) => {
    const userId = req.params.id;
    const query = `SELECT * FROM users WHERE id = ${userId}`;
    db.query(query, (err, results) => {
        if (err) throw err;
        res.json(results);
    });
});

app.post('/api/upload', (req, res) => {
    const file = req.body.file;
    fs.writeFileSync('/uploads/' + req.body.filename, file);
    res.json({ status: 'uploaded' });
});

app.listen(3000);
''',
    "Go - HTTP Handler": '''package main

import (
    "database/sql"
    "fmt"
    "net/http"
    "os/exec"
)

func handleRequest(w http.ResponseWriter, r *http.Request) {
    userInput := r.URL.Query().Get("cmd")
    out, _ := exec.Command("sh", "-c", userInput).Output()
    fmt.Fprintf(w, string(out))
}

func getUser(w http.ResponseWriter, r *http.Request) {
    id := r.URL.Query().Get("id")
    query := fmt.Sprintf("SELECT * FROM users WHERE id = %s", id)
    rows, _ := db.Query(query)
    defer rows.Close()
}

func main() {
    http.HandleFunc("/run", handleRequest)
    http.HandleFunc("/user", getUser)
    http.ListenAndServe(":8080", nil)
}
''',
    "Custom Code": "",
}


@dataclass
class TaskResult:
    """Result of a code review task execution."""
    task_id: str = ""
    description: str = ""
    code_files: int = 3
    bid_strategy: str = "balanced"
    bids: list = field(default_factory=list)
    winner_name: str = ""
    winner_tier: str = ""
    winner_price: float = 0.0
    winner_score: float = 0.0
    contract_id: str = ""
    agent_response: str = ""
    execution_time_ms: int = 0
    platform_fee: float = 0.0
    provider_payout: float = 0.0
    timestamp: str = ""
    status: str = "pending"
    current_step: int = 0
    # AP2 Payment fields
    ap2_payment_provider: str = ""
    ap2_payment_method: str = ""
    ap2_cart_mandate_id: str = ""
    ap2_payment_receipt_id: str = ""
    ap2_base_fee: float = 0.0
    ap2_reward: float = 0.0
    ap2_net_fee: float = 0.0


class AppState:
    def __init__(self):
        self.tasks: list[TaskResult] = []
        self.logs: list[str] = []
        self.is_running: bool = False
        self.current_task: Optional[TaskResult] = None
        self.stats = {
            "total_reviews": 0,
            "platform_revenue": 0.0,
            "avg_response_time": 0,
        }


def get_state() -> AppState:
    if not hasattr(app.storage.user, 'state'):
        app.storage.user.state = AppState()
    return app.storage.user.state


def add_log(message: str, log_container=None):
    """Add a log message with timestamp."""
    state = get_state()
    timestamp = datetime.now().strftime("%H:%M:%S")
    log_entry = f"[{timestamp}] {message}"
    state.logs.append(log_entry)
    if log_container:
        with log_container:
            ui.label(log_entry).classes('font-mono text-xs text-slate-400')


async def fetch_registered_agents() -> list[dict]:
    """Fetch registered agents from provider registry."""
    agents = []

    try:
        async with httpx.AsyncClient(timeout=5.0) as client:
            resp = await client.get(f"{AEX_PROVIDER_REGISTRY_URL}/v1/providers")
            if resp.status_code == 200:
                data = resp.json()
                providers = data.get("providers", []) if isinstance(data, dict) else data
                for p in providers:
                    capabilities = p.get("capabilities", [])
                    agent_type = "payment" if "payment" in capabilities or "payment_processing" in capabilities else "reviewer"

                    agents.append({
                        "id": p.get("provider_id", ""),
                        "name": p.get("name", "Unknown"),
                        "description": p.get("description", ""),
                        "endpoint": p.get("endpoint", ""),
                        "status": p.get("status", "ACTIVE"),
                        "tier": p.get("trust_tier", p.get("metadata", {}).get("trust_tier", "UNVERIFIED")),
                        "trust_score": p.get("trust_score", p.get("metadata", {}).get("trust_score", 0.5)),
                        "capabilities": capabilities,
                        "type": agent_type,
                    })
    except Exception as e:
        print(f"Error fetching from provider registry: {e}")

    # Probe known agent endpoints
    known_agents = [
        {"name": "QuickReview AI", "url": CODE_REVIEWER_A_URL, "type": "reviewer"},
        {"name": "CodeGuard AI", "url": CODE_REVIEWER_B_URL, "type": "reviewer"},
        {"name": "ArchitectAI", "url": CODE_REVIEWER_C_URL, "type": "reviewer"},
        {"name": "DevPay", "url": DEVPAY_URL, "type": "payment"},
        {"name": "CodeAuditPay", "url": CODEAUDITPAY_URL, "type": "payment"},
        {"name": "SecurityPay", "url": SECURITYPAY_URL, "type": "payment"},
    ]

    async with httpx.AsyncClient(timeout=3.0) as client:
        for agent in known_agents:
            existing = [a for a in agents if a.get("name") == agent["name"]]
            if existing:
                continue
            try:
                health_resp = await client.get(f"{agent['url']}/health")
                if health_resp.status_code != 200:
                    continue

                agent_card = {}
                try:
                    card_resp = await client.get(f"{agent['url']}/.well-known/agent.json")
                    if card_resp.status_code == 200:
                        agent_card = card_resp.json()
                except Exception:
                    pass

                agents.append({
                    "id": agent_card.get("name", agent["name"]).lower().replace(" ", "-"),
                    "name": agent_card.get("name", agent["name"]),
                    "description": agent_card.get("description", ""),
                    "endpoint": agent["url"],
                    "status": "ACTIVE",
                    "tier": "VERIFIED",
                    "trust_score": 0.5,
                    "capabilities": [s.get("id", "") for s in agent_card.get("skills", [])],
                    "type": agent["type"],
                })
            except Exception:
                pass

    return agents


async def fetch_real_bids(code_files: int) -> list[dict]:
    """Fetch bids from code review agents via A2A protocol."""
    agents = [
        {"name": "QuickReview AI", "url": CODE_REVIEWER_A_URL},
        {"name": "CodeGuard AI", "url": CODE_REVIEWER_B_URL},
        {"name": "ArchitectAI", "url": CODE_REVIEWER_C_URL},
    ]

    bids = []
    bid_request = json.dumps({"action": "get_bid", "document_pages": code_files})

    async with httpx.AsyncClient(timeout=10.0) as client:
        for agent in agents:
            try:
                payload = {
                    "jsonrpc": "2.0",
                    "method": "message/send",
                    "id": f"bid-{agent['name']}-{int(time.time())}",
                    "params": {
                        "message": {
                            "role": "user",
                            "parts": [{"type": "text", "text": bid_request}],
                        }
                    },
                }
                resp = await client.post(f"{agent['url']}/a2a", json=payload)
                if resp.status_code == 200:
                    data = resp.json()
                    result = data.get("result", {})
                    for msg in result.get("history", []):
                        if msg.get("role") == "agent":
                            for part in msg.get("parts", []):
                                if part.get("type") == "text":
                                    try:
                                        bid_resp = json.loads(part.get("text", "{}"))
                                        if bid_resp.get("action") == "bid_response":
                                            bid_data = bid_resp.get("bid", {})
                                            bids.append({
                                                "provider_id": bid_data.get("provider_id"),
                                                "provider_name": bid_data.get("provider_name"),
                                                "price": bid_data.get("price", 0),
                                                "confidence": bid_data.get("confidence", 0.8),
                                                "estimated_minutes": bid_data.get("estimated_minutes", 10),
                                                "trust_score": bid_data.get("trust_score", 0.5),
                                                "tier": bid_data.get("tier", "UNVERIFIED"),
                                                "a2a_endpoint": f"{agent['url']}/a2a",
                                            })
                                    except json.JSONDecodeError:
                                        pass
            except Exception as e:
                print(f"Error fetching bid from {agent['name']}: {e}")

    # Fallback simulated bids if agents not reachable
    if not bids:
        bids = [
            {"provider_id": "quickreview-ai", "provider_name": "QuickReview AI", "price": 6.0,
             "confidence": 0.75, "estimated_minutes": 2, "trust_score": 0.70, "tier": "VERIFIED",
             "a2a_endpoint": f"{CODE_REVIEWER_A_URL}/a2a"},
            {"provider_id": "codeguard-ai", "provider_name": "CodeGuard AI", "price": 19.0,
             "confidence": 0.85, "estimated_minutes": 5, "trust_score": 0.85, "tier": "TRUSTED",
             "a2a_endpoint": f"{CODE_REVIEWER_B_URL}/a2a"},
            {"provider_id": "architectai", "provider_name": "ArchitectAI", "price": 40.0,
             "confidence": 0.95, "estimated_minutes": 10, "trust_score": 0.95, "tier": "PREFERRED",
             "a2a_endpoint": f"{CODE_REVIEWER_C_URL}/a2a"},
        ]
    return bids


async def fetch_payment_bids(amount: float, category: str) -> list[dict]:
    """Fetch payment provider bids via A2A."""
    providers = [
        {"name": "DevPay", "url": DEVPAY_URL},
        {"name": "CodeAuditPay", "url": CODEAUDITPAY_URL},
        {"name": "SecurityPay", "url": SECURITYPAY_URL},
    ]

    bids = []
    async with httpx.AsyncClient(timeout=10.0) as client:
        for provider in providers:
            try:
                payload = {
                    "jsonrpc": "2.0",
                    "method": "message/send",
                    "id": f"payment-bid-{int(time.time())}",
                    "params": {
                        "message": {
                            "role": "user",
                            "parts": [{"type": "text", "text": json.dumps({
                                "action": "bid",
                                "amount": amount,
                                "work_category": category,
                                "currency": "USD",
                            })}],
                        }
                    },
                }
                resp = await client.post(f"{provider['url']}/a2a", json=payload)
                if resp.status_code == 200:
                    data = resp.json()
                    for msg in data.get("result", {}).get("history", []):
                        if msg.get("role") == "agent":
                            for part in msg.get("parts", []):
                                if part.get("type") == "text":
                                    try:
                                        bid_resp = json.loads(part.get("text", "{}"))
                                        if bid_resp.get("action") == "bid_response":
                                            bid_data = bid_resp.get("bid", {})
                                            bids.append({
                                                "provider_name": bid_data.get("provider_name"),
                                                "base_fee_percent": bid_data.get("base_fee_percent", 2.0),
                                                "reward_percent": bid_data.get("reward_percent", 1.0),
                                                "net_fee_percent": bid_data.get("net_fee_percent", 1.0),
                                            })
                                    except json.JSONDecodeError:
                                        pass
            except Exception as e:
                print(f"Error fetching payment bid from {provider['name']}: {e}")

    if not bids:
        bids = [
            {"provider_name": "DevPay", "base_fee_percent": 2.0, "reward_percent": 1.0, "net_fee_percent": 1.0},
            {"provider_name": "CodeAuditPay", "base_fee_percent": 2.5, "reward_percent": 3.0, "net_fee_percent": -0.5},
            {"provider_name": "SecurityPay", "base_fee_percent": 3.0, "reward_percent": 4.0, "net_fee_percent": -1.0},
        ]
    return bids


def evaluate_bids(bids: list[dict], strategy: str) -> list[dict]:
    """Evaluate and score bids based on strategy."""
    weights = {
        "balanced": {"price": 0.4, "trust": 0.35, "confidence": 0.25},
        "lowest_price": {"price": 0.7, "trust": 0.2, "confidence": 0.1},
        "best_quality": {"price": 0.2, "trust": 0.5, "confidence": 0.3},
    }
    w = weights.get(strategy, weights["balanced"])

    if not bids:
        return []

    max_price = max(b["price"] for b in bids) or 1

    for bid in bids:
        price_score = 1 - (bid["price"] / max_price)
        trust_score = bid.get("trust_score", 0.5)
        confidence = bid.get("confidence", 0.8)
        bid["score"] = (
            w["price"] * price_score +
            w["trust"] * trust_score +
            w["confidence"] * confidence
        )

    return sorted(bids, key=lambda x: x["score"], reverse=True)


async def call_agent_a2a(url: str, task: str) -> tuple[str, int]:
    """Call an agent via A2A protocol."""
    start = time.time()
    payload = {
        "jsonrpc": "2.0",
        "method": "message/send",
        "id": f"task-{int(time.time())}",
        "params": {
            "message": {
                "role": "user",
                "parts": [{"type": "text", "text": task}],
            }
        },
    }

    try:
        async with httpx.AsyncClient(timeout=120.0) as client:
            resp = await client.post(f"{url}/a2a", json=payload)
            elapsed_ms = int((time.time() - start) * 1000)

            if resp.status_code == 200:
                data = resp.json()
                for msg in data.get("result", {}).get("history", []):
                    if msg.get("role") == "agent":
                        for part in msg.get("parts", []):
                            if part.get("type") == "text":
                                return part.get("text", "No response"), elapsed_ms
            return "No response from agent", elapsed_ms
    except Exception as e:
        elapsed_ms = int((time.time() - start) * 1000)
        return f"Error: {str(e)}", elapsed_ms


def create_task_card(task: TaskResult):
    """Create a task result card with real-time updates."""
    status_config = {
        "pending": {"color": "gray", "label": "Pending", "icon": "hourglass_empty"},
        "bidding": {"color": "cyan", "label": "Collecting Bids", "icon": "gavel"},
        "evaluating": {"color": "orange", "label": "Evaluating", "icon": "analytics"},
        "awarded": {"color": "blue", "label": "Contract Awarded", "icon": "assignment_turned_in"},
        "executing": {"color": "green", "label": "Executing Review", "icon": "play_circle"},
        "paying": {"color": "purple", "label": "Processing Payment", "icon": "payment"},
        "settling": {"color": "amber", "label": "Settling", "icon": "account_balance"},
        "completed": {"color": "green", "label": "Completed", "icon": "check_circle"},
        "failed": {"color": "red", "label": "Failed", "icon": "error"},
    }

    status = status_config.get(task.status, status_config["pending"])
    is_active = task.status not in ["completed", "failed"]

    with ui.card().classes(f'w-full bg-slate-800 border {"border-2 border-" + status["color"] + "-500" if is_active else "border-slate-700"}'):
        with ui.row().classes('w-full justify-between items-center'):
            with ui.row().classes('items-center gap-2'):
                ui.label(f"Review: {task.task_id}").classes('font-bold text-white')
                ui.badge(status["label"], color=status["color"]).props(f'icon={status["icon"]}')
            ui.label(task.timestamp or "In progress...").classes('text-xs text-slate-400')

        # Progress bar (7 steps)
        if task.current_step > 0:
            steps = ["Bids", "Eval", "Award", "Execute", "AP2 Select", "AP2 Pay", "Settle"]
            with ui.row().classes('w-full gap-1 my-2'):
                for i, step in enumerate(steps):
                    completed = i < task.current_step
                    active = i == task.current_step - 1
                    color = "green" if completed else (status["color"] if active else "slate-600")
                    with ui.column().classes('flex-1'):
                        ui.linear_progress(value=1 if completed else (0.5 if active else 0), color=color).classes('h-1')
                        ui.label(step).classes(f'text-xs {"text-white" if completed else "text-slate-500"}')

        # Code snippet preview
        desc = task.description
        if len(desc) > 120:
            desc = desc[:120] + "..."
        ui.label(f'"{desc}"').classes('text-slate-400 italic my-2 text-xs font-mono')

        # Bids section
        if task.bids:
            ui.label("Bids Received:").classes('font-bold text-white mt-2')
            for bid in task.bids:
                is_winner = bid.get("provider_name") == task.winner_name
                with ui.row().classes(f'w-full justify-between items-center p-2 rounded {"bg-slate-700" if is_winner else ""}'):
                    with ui.row().classes('items-center gap-2'):
                        if is_winner:
                            ui.icon("star", color="green").classes('text-sm')
                        ui.label(bid.get("provider_name", "Unknown")).classes(f'{"text-white" if is_winner else "text-slate-400"}')
                        tier = bid.get("tier", "UNVERIFIED")
                        ui.badge(tier, color={"VERIFIED": "orange", "TRUSTED": "blue", "PREFERRED": "green"}.get(tier, "gray"))
                    with ui.row().classes('gap-4'):
                        ui.label(f"${bid.get('price', 0):.2f}").classes(f'{"text-white" if is_winner else "text-slate-400"}')
                        if bid.get("score", 0) > 0:
                            ui.label(f"score: {bid.get('score', 0):.3f}").classes('text-xs text-slate-500')

        # Winner info
        if task.winner_name:
            with ui.card().classes('w-full bg-slate-700 mt-2'):
                with ui.row().classes('w-full justify-between items-center'):
                    ui.label(f"Winner: {task.winner_name}").classes('font-bold text-green-400')
                    ui.label(f"${task.winner_price:.2f}").classes('text-xl font-bold text-green-400')
                with ui.row().classes('gap-4 text-xs text-slate-400'):
                    if task.contract_id:
                        ui.label(f"Contract: {task.contract_id[:20]}...")
                    if task.execution_time_ms > 0:
                        ui.label(f"Execution: {task.execution_time_ms}ms")
                    if task.platform_fee > 0:
                        ui.label(f"Platform fee: ${task.platform_fee:.2f}")

        # AP2 Payment info
        if task.ap2_payment_provider:
            with ui.card().classes('w-full bg-gradient-to-r from-purple-900 to-indigo-900 border border-purple-500 mt-2'):
                with ui.row().classes('w-full justify-between items-center'):
                    ui.label("AP2 Payment").classes('font-bold text-purple-300')
                    ui.badge("PAID", color="green")
                with ui.column().classes('gap-1'):
                    ui.label(f"Provider: {task.ap2_payment_provider}").classes('text-white text-sm')
                    if task.ap2_cart_mandate_id:
                        ui.label(f"Cart Mandate: {task.ap2_cart_mandate_id}").classes('text-slate-400 text-xs')
                    if task.ap2_payment_receipt_id:
                        ui.label(f"Receipt: {task.ap2_payment_receipt_id}").classes('text-slate-400 text-xs')
                    if task.ap2_net_fee != 0:
                        if task.ap2_net_fee < 0:
                            ui.label(f"You earned ${abs(task.ap2_net_fee):.2f} CASHBACK!").classes('text-green-400 font-bold')
                        else:
                            ui.label(f"Net fee: ${task.ap2_net_fee:.2f}").classes('text-slate-300')

        # Agent response
        if task.agent_response:
            with ui.expansion("Code Review Results", icon="code").classes('w-full mt-2'):
                ui.markdown(task.agent_response).classes('text-sm bg-slate-900 p-4 rounded max-h-96 overflow-y-auto')


def create_agent_card(agent: dict):
    """Create a card for displaying an agent."""
    is_online = agent.get("status") == "ACTIVE"
    agent_type = agent.get("type", "reviewer")
    tier = agent.get("tier", "UNVERIFIED")
    tier_color = {"VERIFIED": "orange", "TRUSTED": "blue", "PREFERRED": "green", "UNVERIFIED": "gray"}.get(tier, "gray")

    with ui.card().classes(f'w-full bg-slate-800 border {"border-green-500" if is_online else "border-red-500"} mb-2'):
        with ui.row().classes('w-full justify-between items-center'):
            with ui.row().classes('items-center gap-2'):
                ui.icon("circle", color="green" if is_online else "red").classes('text-xs')
                ui.label(agent.get("name", "Unknown")).classes('font-bold text-white text-sm')
            type_color = "cyan" if agent_type == "reviewer" else "purple"
            ui.badge(agent_type.upper(), color=type_color).props('dense')

        with ui.row().classes('items-center gap-2 mt-1'):
            ui.badge(tier, color=tier_color).props('dense')
            trust_score = agent.get("trust_score", 0)
            ui.label(f"Trust: {trust_score:.0%}").classes('text-xs text-slate-400')

        desc = agent.get("description", "")
        if desc:
            ui.label(desc[:60] + "..." if len(desc) > 60 else desc).classes('text-xs text-slate-500 mt-1')

        capabilities = agent.get("capabilities", [])
        if capabilities:
            with ui.row().classes('gap-1 mt-1 flex-wrap'):
                for cap in capabilities[:3]:
                    ui.badge(cap, color="gray").props('dense outline')
                if len(capabilities) > 3:
                    ui.label(f"+{len(capabilities) - 3}").classes('text-xs text-slate-500')


@ui.page('/')
async def main_page():
    """Main dashboard page."""
    state = get_state()

    ui.dark_mode().enable()
    ui.add_head_html('''
        <style>
            body { background: linear-gradient(180deg, #0f172a 0%, #1e1b4b 100%); }
            .nicegui-content { max-width: 1600px; margin: auto; }
        </style>
    ''')

    agents_data = await fetch_registered_agents()

    with ui.column().classes('w-full p-6 gap-6'):
        # Header
        with ui.row().classes('w-full justify-between items-center'):
            with ui.column():
                ui.label("Agent Exchange - Code Review").classes('text-3xl font-bold text-white')
                ui.label("Claude-Powered Code Review Agents + AEX Marketplace + AP2 Payments").classes('text-slate-400')
            with ui.row().classes('gap-2'):
                reviewer_count = len([a for a in agents_data if a.get("type") == "reviewer" and a.get("status") == "ACTIVE"])
                payment_count = len([a for a in agents_data if a.get("type") == "payment" and a.get("status") == "ACTIVE"])
                ui.badge(f"Review Agents: {reviewer_count}", color="cyan")
                ui.badge(f"Payment Agents: {payment_count}", color="purple")
                ui.badge(f"Reviews: {len(state.tasks)}", color="blue")
                ui.badge(f"Revenue: ${state.stats['platform_revenue']:.2f}", color="green")

        # Main content - three columns
        with ui.row().classes('w-full gap-4'):
            # Left column - Registered Agents
            with ui.column().classes('w-72'):
                with ui.card().classes('w-full bg-slate-800 border border-slate-700'):
                    with ui.row().classes('w-full justify-between items-center mb-3'):
                        ui.label("Registered Agents").classes('text-lg font-bold text-white')
                        async def refresh_agents():
                            nonlocal agents_data
                            agents_data = await fetch_registered_agents()
                            agents_container.refresh()
                        ui.button(icon="refresh", on_click=refresh_agents).props('flat dense round').classes('text-slate-400')

                    @ui.refreshable
                    def agents_container():
                        reviewer_agents = [a for a in agents_data if a.get("type") == "reviewer"]
                        payment_agents = [a for a in agents_data if a.get("type") == "payment"]

                        with ui.expansion(value=True).classes('w-full bg-slate-700 mb-2').props('dense header-class="bg-slate-700"') as rev_exp:
                            with rev_exp.add_slot('header'):
                                with ui.row().classes('items-center gap-2 w-full'):
                                    ui.icon("code", color="cyan").classes('text-lg')
                                    ui.label("Code Review Agents").classes('font-bold text-cyan-400')
                                    ui.badge(f"{len(reviewer_agents)}", color="cyan").props('dense')

                            if reviewer_agents:
                                for agent in reviewer_agents:
                                    create_agent_card(agent)
                            else:
                                ui.label("No review agents registered").classes('text-slate-500 text-sm italic')

                        with ui.expansion(value=True).classes('w-full bg-slate-700 mb-2').props('dense header-class="bg-slate-700"') as pay_exp:
                            with pay_exp.add_slot('header'):
                                with ui.row().classes('items-center gap-2 w-full'):
                                    ui.icon("payment", color="purple").classes('text-lg')
                                    ui.label("Payment Agents (AP2)").classes('font-bold text-purple-400')
                                    ui.badge(f"{len(payment_agents)}", color="purple").props('dense')

                            if payment_agents:
                                for agent in payment_agents:
                                    create_agent_card(agent)
                            else:
                                ui.label("No payment agents registered").classes('text-slate-500 text-sm italic')

                        online = len([a for a in agents_data if a.get("status") == "ACTIVE"])
                        offline = len([a for a in agents_data if a.get("status") != "ACTIVE"])
                        with ui.row().classes('w-full justify-center gap-4 mt-2 pt-2 border-t border-slate-700'):
                            ui.label(f"Online: {online}").classes('text-xs text-green-400')
                            ui.label(f"Offline: {offline}").classes('text-xs text-red-400')

                    agents_container()

                    async def auto_refresh_agents():
                        nonlocal agents_data
                        while True:
                            await asyncio.sleep(5)
                            try:
                                agents_data = await fetch_registered_agents()
                                agents_container.refresh()
                            except Exception as e:
                                print(f"Auto-refresh error: {e}")

                    ui.timer(0.1, lambda: asyncio.create_task(auto_refresh_agents()), once=True)

            # Middle column - Task submission
            with ui.column().classes('w-96'):
                with ui.card().classes('w-full bg-slate-800 border border-slate-700'):
                    ui.label("Submit Code Review").classes('text-xl font-bold text-white mb-4')

                    # Sample code selector
                    sample_select = ui.select(
                        label="Load Sample Code",
                        options=list(SAMPLE_CODE.keys()),
                        value="Custom Code",
                    ).classes('w-full')

                    code_input = ui.textarea(
                        label="Paste your code for review",
                        placeholder="def my_function():\n    # paste code here...",
                    ).classes('w-full font-mono').props('rows=12')

                    def load_sample(e):
                        selected = sample_select.value
                        if selected and selected in SAMPLE_CODE and SAMPLE_CODE[selected]:
                            code_input.value = SAMPLE_CODE[selected]

                    sample_select.on_value_change(load_sample)

                    with ui.row().classes('w-full gap-4'):
                        files_input = ui.number(label="Files", value=3, min=1, max=50).classes('w-24')
                        strategy_select = ui.select(
                            label="Bid Strategy",
                            options=["balanced", "lowest_price", "best_quality"],
                            value="balanced"
                        ).classes('flex-1')

                    error_label = ui.label().classes('text-red-400 hidden')

                    async def run_workflow():
                        if not code_input.value or not code_input.value.strip():
                            error_label.text = "Please paste code to review"
                            error_label.classes(remove='hidden')
                            return

                        error_label.classes(add='hidden')
                        state.is_running = True
                        submit_btn.disable()

                        task = TaskResult(
                            task_id=f"review-{int(time.time())}",
                            description=code_input.value,
                            code_files=int(files_input.value),
                            bid_strategy=strategy_select.value,
                            status="pending",
                            current_step=0,
                        )
                        state.tasks.insert(0, task)
                        state.current_task = task

                        state.logs = []
                        log_container.clear()
                        tasks_container.refresh()

                        add_log("=== AEX Code Review Workflow Started ===", log_container)
                        add_log(f"Review ID: {task.task_id}", log_container)
                        add_log(f"Code: {task.description[:60]}...", log_container)
                        add_log("", log_container)

                        # STEP 1: Collect bids
                        task.status = "bidding"
                        task.current_step = 1
                        tasks_container.refresh()
                        add_log("[STEP 1/7] COLLECTING BIDS from Code Review Agents...", log_container)
                        await asyncio.sleep(0.1)

                        bids = await fetch_real_bids(task.code_files)
                        task.bids = bids
                        for b in bids:
                            add_log(f"  - {b['provider_name']}: ${b['price']:.2f} | {b['tier']}", log_container)
                        tasks_container.refresh()
                        add_log("", log_container)

                        # STEP 2: Evaluate bids
                        task.status = "evaluating"
                        task.current_step = 2
                        tasks_container.refresh()
                        add_log(f"[STEP 2/7] EVALUATING BIDS using '{task.bid_strategy}' strategy...", log_container)
                        await asyncio.sleep(0.1)

                        evaluated = evaluate_bids(bids, task.bid_strategy)
                        task.bids = evaluated
                        for i, b in enumerate(evaluated):
                            marker = " << WINNER" if i == 0 else ""
                            add_log(f"  {i+1}. {b['provider_name']}: score={b['score']:.3f}{marker}", log_container)
                        tasks_container.refresh()
                        add_log("", log_container)

                        # STEP 3: Award contract
                        if evaluated:
                            winner = evaluated[0]
                            task.status = "awarded"
                            task.current_step = 3
                            task.winner_name = winner["provider_name"]
                            task.winner_tier = winner["tier"]
                            task.winner_price = winner["price"]
                            task.winner_score = winner["score"]
                            task.contract_id = f"contract-{int(time.time())}"
                            tasks_container.refresh()

                            add_log(f"[STEP 3/7] CONTRACT AWARDED", log_container)
                            add_log(f"  Winner: {task.winner_name}", log_container)
                            add_log(f"  Price: ${task.winner_price:.2f}", log_container)
                            add_log(f"  Contract ID: {task.contract_id}", log_container)
                            add_log("", log_container)

                            # STEP 4: Execute code review
                            task.status = "executing"
                            task.current_step = 4
                            tasks_container.refresh()
                            add_log(f"[STEP 4/7] EXECUTING CODE REVIEW via A2A Protocol...", log_container)

                            url = PROVIDER_URL_MAP.get(winner.get("provider_id"), CODE_REVIEWER_A_URL)
                            add_log(f"  Calling {task.winner_name} at {url}...", log_container)
                            await asyncio.sleep(0.1)

                            review_prompt = f"Review the following code for issues, security vulnerabilities, and improvements:\n\n{task.description}"
                            response, elapsed_ms = await call_agent_a2a(url, review_prompt)
                            task.agent_response = response
                            task.execution_time_ms = elapsed_ms
                            tasks_container.refresh()

                            add_log(f"  Review received in {elapsed_ms}ms", log_container)
                            add_log(f"  Response length: {len(response)} chars", log_container)
                            add_log("", log_container)

                            # STEP 5: AP2 Payment Selection
                            task.status = "paying"
                            task.current_step = 5
                            tasks_container.refresh()
                            add_log(f"[STEP 5/7] AP2 PAYMENT - Selecting Payment Provider...", log_container)

                            category = "code_review"
                            code_lower = task.description.lower()
                            if any(kw in code_lower for kw in ["security", "injection", "xss", "vulnerability"]):
                                category = "security_audit"
                            elif any(kw in code_lower for kw in ["architecture", "design", "pattern"]):
                                category = "architecture_review"

                            payment_bids = await fetch_payment_bids(task.winner_price, category)
                            for pb in payment_bids:
                                net = pb.get("net_fee_percent", 0)
                                net_str = f"{net:.1f}% fee" if net >= 0 else f"{abs(net):.1f}% CASHBACK"
                                add_log(f"  - {pb['provider_name']}: {pb.get('base_fee_percent', 0):.1f}% base, {pb.get('reward_percent', 0):.1f}% reward = {net_str}", log_container)

                            best = min(payment_bids, key=lambda x: x.get("net_fee_percent", 99))
                            task.ap2_payment_provider = best["provider_name"]
                            task.ap2_cart_mandate_id = f"cart-{int(time.time())}"
                            task.ap2_base_fee = best.get("base_fee_percent", 2.0)
                            task.ap2_reward = best.get("reward_percent", 1.0)
                            task.ap2_net_fee = round(task.winner_price * best.get("net_fee_percent", 1.0) / 100, 2)
                            tasks_container.refresh()

                            add_log(f"  Selected: {task.ap2_payment_provider}", log_container)
                            if best.get("net_fee_percent", 0) < 0:
                                add_log(f"  YOU EARN {abs(best.get('net_fee_percent', 0)):.1f}% CASHBACK!", log_container)
                            add_log("", log_container)

                            # STEP 6: Process Payment
                            task.current_step = 6
                            tasks_container.refresh()
                            add_log(f"[STEP 6/7] AP2 PAYMENT - Processing...", log_container)
                            add_log(f"  Amount: ${task.winner_price:.2f}", log_container)
                            add_log(f"  Base fee ({task.ap2_base_fee}%): ${task.winner_price * task.ap2_base_fee / 100:.2f}", log_container)
                            add_log(f"  Reward ({task.ap2_reward}%): -${task.winner_price * task.ap2_reward / 100:.2f}", log_container)

                            task.ap2_payment_receipt_id = f"receipt-{int(time.time())}"
                            task.ap2_payment_method = "card"
                            tasks_container.refresh()

                            add_log(f"  Receipt ID: {task.ap2_payment_receipt_id}", log_container)
                            add_log(f"  Status: COMPLETED", log_container)
                            add_log("", log_container)

                            # STEP 7: Settlement
                            task.status = "settling"
                            task.current_step = 7
                            tasks_container.refresh()
                            add_log(f"[STEP 7/7] SETTLEMENT", log_container)

                            task.platform_fee = round(task.winner_price * 0.15, 2)
                            task.provider_payout = round(task.winner_price - task.platform_fee, 2)
                            task.timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")

                            add_log(f"  Platform Fee (15%): ${task.platform_fee:.2f}", log_container)
                            add_log(f"  Provider Payout: ${task.provider_payout:.2f}", log_container)
                            add_log("", log_container)

                            task.status = "completed"
                            state.stats["total_reviews"] += 1
                            state.stats["platform_revenue"] += task.platform_fee
                            tasks_container.refresh()

                            add_log(f"=== CODE REVIEW COMPLETE ===", log_container)
                            add_log(f"Total reviews: {state.stats['total_reviews']}", log_container)
                            add_log(f"Platform revenue: ${state.stats['platform_revenue']:.2f}", log_container)

                        state.is_running = False
                        submit_btn.enable()

                    submit_btn = ui.button(
                        "Run Code Review",
                        on_click=run_workflow,
                        color="green"
                    ).classes('w-full mt-4').props('icon=play_arrow')

                # Live log
                with ui.card().classes('w-full bg-slate-900 border border-slate-700 mt-4'):
                    ui.label("Live Output").classes('font-bold text-cyan-400 mb-2')
                    log_container = ui.column().classes('w-full max-h-64 overflow-y-auto')

            # Right column - Task results
            with ui.column().classes('flex-1'):
                with ui.row().classes('w-full justify-between items-center mb-4'):
                    ui.label(f"Review Results ({len(state.tasks)})").classes('text-xl font-bold text-white')
                    if state.tasks:
                        ui.button(
                            "Clear History",
                            on_click=lambda: (state.tasks.clear(), tasks_container.refresh()),
                            color="gray"
                        ).props('flat size=sm')

                @ui.refreshable
                def tasks_container():
                    if not state.tasks:
                        with ui.card().classes('w-full bg-slate-800 border border-slate-700 p-8 text-center'):
                            ui.icon("code", color="slate").classes('text-4xl mb-2')
                            ui.label("No code reviews yet").classes('text-slate-400')
                            ui.label("Paste code and click 'Run Code Review' to start").classes('text-slate-500 text-sm')
                    else:
                        for task in state.tasks:
                            create_task_card(task)

                tasks_container()


if __name__ in {"__main__", "__mp_main__"}:
    ui.run(
        title="AEX Code Review - Agent Exchange",
        host="0.0.0.0",
        port=8502,
        dark=True,
        storage_secret="aex-code-review-secret",
    )
