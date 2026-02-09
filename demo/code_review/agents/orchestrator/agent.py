"""Orchestrator Agent - Task decomposition and multi-agent coordination for code review."""

import asyncio
import json
import logging
import os
from dataclasses import dataclass, field
from typing import Any, Optional
import aiohttp

from langchain_anthropic import ChatAnthropic
from langchain_core.messages import HumanMessage, SystemMessage
from langgraph.graph import StateGraph, END

import sys
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from common.base_agent import BaseAgent, AgentState
from common.config import AgentConfig
from common.aex_client import AEXClient

logger = logging.getLogger(__name__)

ORCHESTRATOR_PROMPT = """You are an intelligent orchestrator that decomposes complex code review requests into subtasks.

Given a user request, identify the required subtasks and the skills needed for each.
Output a JSON object with the following structure:

{
    "understanding": "Brief summary of what the user wants",
    "subtasks": [
        {
            "id": "task_1",
            "description": "What needs to be done",
            "skill_tags": ["skill_tag_1", "skill_tag_2"],
            "input": "Specific input for this subtask",
            "depends_on": []
        }
    ],
    "execution_order": "parallel" or "sequential"
}

Available skill tags:
- code_review: General code review (readability, correctness, best practices)
- linting: Code style, formatting, and lint rule enforcement
- security_audit: Security vulnerability scanning and threat analysis
- architecture_review: Architecture, design patterns, and refactoring recommendations
- bug_detection: Bug detection, error handling, and edge case analysis
- performance_review: Performance optimization and bottleneck analysis

For comprehensive code review requests, consider decomposing into:
1. Basic code_review for quick overview and readability
2. security_audit for vulnerability scanning
3. architecture_review for design pattern and structural feedback
4. performance_review for optimization opportunities

Be specific about what each subtask should accomplish.
Return ONLY the JSON object, no additional text."""


@dataclass
class SubTask:
    """A subtask identified by the orchestrator."""
    id: str
    description: str
    skill_tags: list[str]
    input: str
    depends_on: list[str]
    result: Optional[str] = None
    status: str = "pending"
    provider_id: Optional[str] = None
    agent_url: Optional[str] = None


@dataclass
class OrchestratorAgent(BaseAgent):
    """Orchestrator that coordinates multiple code review agents via AEX + A2A."""

    llm: Optional[ChatAnthropic] = field(default=None, init=False)
    http_session: Optional[aiohttp.ClientSession] = field(default=None, init=False)

    def _setup_llm(self):
        """Initialize Claude LLM for task decomposition."""
        api_key = os.environ.get("ANTHROPIC_API_KEY")
        if not api_key:
            logger.warning("ANTHROPIC_API_KEY not set, using mock decomposition")
            self.llm = None
            return

        self.llm = ChatAnthropic(
            model=self.config.llm.model,
            temperature=self.config.llm.temperature,
            max_tokens=self.config.llm.max_tokens,
            api_key=api_key,
        )
        logger.info(f"Initialized Claude LLM: {self.config.llm.model}")

    def _build_graph(self):
        """Build the orchestration workflow."""
        self._graph = StateGraph(AgentState)

    async def process(self, state: AgentState) -> AgentState:
        """Process request through orchestration pipeline."""
        messages = state["messages"]
        if not messages:
            state["result"] = "No message provided."
            return state

        user_content = messages[-1].get("content", "")

        # Step 1: Decompose task
        subtasks = await self._decompose_task(user_content)
        if not subtasks:
            state["result"] = "Could not decompose the request into subtasks."
            return state

        logger.info(f"Decomposed into {len(subtasks)} subtasks")

        # Step 2: Discover providers via AEX for each subtask
        await self._discover_providers(subtasks)

        # Step 3: Execute subtasks via A2A
        results = await self._execute_subtasks(subtasks)

        # Step 4: Aggregate results
        state["result"] = self._aggregate_results(user_content, subtasks)
        state["artifacts"] = [
            {
                "name": "orchestration_report.json",
                "parts": [{"type": "text", "text": json.dumps({
                    "subtasks": [
                        {
                            "id": st.id,
                            "description": st.description,
                            "status": st.status,
                            "provider": st.provider_id,
                        }
                        for st in subtasks
                    ]
                }, indent=2)}],
            }
        ]

        return state

    async def _decompose_task(self, user_request: str) -> list[SubTask]:
        """Use LLM to decompose request into subtasks."""
        if self.llm is None:
            return self._mock_decompose(user_request)

        try:
            response = await self.llm.ainvoke([
                SystemMessage(content=ORCHESTRATOR_PROMPT),
                HumanMessage(content=user_request),
            ])

            # Parse JSON response
            content = response.content.strip()
            # Handle markdown code blocks
            if content.startswith("```"):
                content = content.split("```")[1]
                if content.startswith("json"):
                    content = content[4:]
                content = content.strip()

            data = json.loads(content)

            subtasks = []
            for st in data.get("subtasks", []):
                subtasks.append(SubTask(
                    id=st["id"],
                    description=st["description"],
                    skill_tags=st.get("skill_tags", []),
                    input=st.get("input", ""),
                    depends_on=st.get("depends_on", []),
                ))

            return subtasks

        except Exception as e:
            logger.exception(f"Error decomposing task: {e}")
            return self._mock_decompose(user_request)

    def _mock_decompose(self, user_request: str) -> list[SubTask]:
        """Mock decomposition for testing."""
        request_lower = user_request.lower()

        subtasks = []
        task_id = 1

        # Security audit (Standard agent - CodeGuard)
        if "security" in request_lower or "vulnerability" in request_lower or "injection" in request_lower:
            subtasks.append(SubTask(
                id=f"task_{task_id}",
                description="Scan code for security vulnerabilities and threats",
                skill_tags=["security_audit", "code_review"],
                input=user_request,
                depends_on=[],
            ))
            task_id += 1

        # Architecture review (Premium agent - ArchitectAI)
        if "architecture" in request_lower or "design" in request_lower or "pattern" in request_lower or "refactor" in request_lower:
            subtasks.append(SubTask(
                id=f"task_{task_id}",
                description="Review architecture, design patterns, and structural quality",
                skill_tags=["architecture_review", "code_review"],
                input=user_request,
                depends_on=[],
            ))
            task_id += 1

        # Linting (Budget agent - QuickReview)
        if "lint" in request_lower or "style" in request_lower or "format" in request_lower:
            subtasks.append(SubTask(
                id=f"task_{task_id}",
                description="Check code style, formatting, and lint compliance",
                skill_tags=["linting", "code_review"],
                input=user_request,
                depends_on=[],
            ))
            task_id += 1

        # Bug detection (Budget agent - QuickReview)
        if "bug" in request_lower or "error" in request_lower or "fix" in request_lower:
            subtasks.append(SubTask(
                id=f"task_{task_id}",
                description="Detect bugs, error handling issues, and edge cases",
                skill_tags=["bug_detection", "code_review"],
                input=user_request,
                depends_on=[],
            ))
            task_id += 1

        # Performance review (Premium agent - ArchitectAI)
        if "performance" in request_lower or "optimize" in request_lower or "slow" in request_lower:
            subtasks.append(SubTask(
                id=f"task_{task_id}",
                description="Analyze performance bottlenecks and optimization opportunities",
                skill_tags=["performance_review", "code_review"],
                input=user_request,
                depends_on=[],
            ))
            task_id += 1

        # Default: comprehensive review using all three agents
        if not subtasks:
            subtasks = [
                SubTask(
                    id="task_1",
                    description="Quick code review for readability and correctness",
                    skill_tags=["code_review"],
                    input=user_request,
                    depends_on=[],
                ),
                SubTask(
                    id="task_2",
                    description="Security vulnerability scan",
                    skill_tags=["security_audit", "code_review"],
                    input=user_request,
                    depends_on=[],
                ),
                SubTask(
                    id="task_3",
                    description="Architecture and design pattern review",
                    skill_tags=["architecture_review", "code_review"],
                    input=user_request,
                    depends_on=[],
                ),
            ]

        return subtasks

    async def _discover_providers(self, subtasks: list[SubTask]):
        """Discover providers via AEX for each subtask."""
        # Demo agent URLs (Docker network hostnames)
        # Each agent has different specialties to demonstrate multi-agent coordination
        demo_agents = {
            # Budget agent - quick basic reviews and linting
            "code_review": ("http://code-reviewer-a:8100", "Budget QuickReview ($5+$2/file)"),
            "linting": ("http://code-reviewer-a:8100", "Budget QuickReview ($5+$2/file)"),
            "bug_detection": ("http://code-reviewer-a:8100", "Budget QuickReview ($5+$2/file)"),
            # Standard agent - security scanning
            "security_audit": ("http://code-reviewer-b:8101", "Standard CodeGuard ($15+$0.50/file)"),
            # Premium agent - architecture and performance
            "architecture_review": ("http://code-reviewer-c:8102", "Premium ArchitectAI ($30+$0.20/file)"),
            "performance_review": ("http://code-reviewer-c:8102", "Premium ArchitectAI ($30+$0.20/file)"),
        }

        for st in subtasks:
            # Try AEX discovery first
            if self.aex_client:
                try:
                    providers = await self.aex_client.search_providers(
                        skill_tags=st.skill_tags,
                    )
                    if providers:
                        # Select best provider based on skill requirements
                        selected = self._select_best_provider(providers, st.skill_tags)
                        st.provider_id = selected.get("provider_id")
                        st.agent_url = selected.get("endpoint")
                        agent_name = selected.get("name", st.provider_id)
                        logger.info(f"[AEX] Found {len(providers)} providers, selected {agent_name} for {st.id} (skills: {st.skill_tags})")
                        continue
                except Exception as e:
                    logger.warning(f"[AEX] Discovery failed for {st.id}: {e}")

            # Fall back to demo agents
            for tag in st.skill_tags:
                if tag in demo_agents:
                    st.agent_url, st.provider_id = demo_agents[tag]
                    logger.info(f"[A2A] Using {st.provider_id} for subtask: {st.description}")
                    break

            # Default to budget code review agent
            if not st.agent_url:
                st.agent_url = "http://code-reviewer-a:8100"
                st.provider_id = "Budget QuickReview ($5+$2/file)"
                logger.info(f"[A2A] Default to {st.provider_id} for subtask: {st.description}")

    def _select_best_provider(self, providers: list[dict], skill_tags: list[str]) -> dict:
        """Select the best provider based on skill requirements.

        Strategy:
        - For premium-only skills (architecture_review, performance_review), use Premium
        - For standard skills (security_audit), prefer Standard over Premium
        - For basic skills (code_review, linting, bug_detection), use Budget
        """
        premium_only_skills = {"architecture_review", "performance_review"}
        standard_skills = {"security_audit"}

        # Check if any skill requires premium
        needs_premium = any(tag in premium_only_skills for tag in skill_tags)
        needs_standard = any(tag in standard_skills for tag in skill_tags)

        def get_tier(p):
            """Get tier priority (lower = cheaper)."""
            name = p.get("name", "")
            if "QuickReview" in name:
                return 0
            elif "CodeGuard" in name:
                return 1
            elif "ArchitectAI" in name:
                return 2
            return 3  # Unknown

        sorted_providers = sorted(providers, key=get_tier)

        if needs_premium:
            # Must use Premium
            for p in reversed(sorted_providers):
                if "ArchitectAI" in p.get("name", ""):
                    return p
        elif needs_standard:
            # Use Standard if available, otherwise cheapest
            for p in sorted_providers:
                if "CodeGuard" in p.get("name", ""):
                    return p

        # Default: use cheapest (first in sorted list)
        return sorted_providers[0] if sorted_providers else providers[0]

    async def _execute_subtasks(self, subtasks: list[SubTask]) -> dict[str, str]:
        """Execute subtasks via A2A protocol."""
        results = {}

        async with aiohttp.ClientSession() as session:
            for st in subtasks:
                if not st.agent_url:
                    st.status = "failed"
                    st.result = "No provider available"
                    continue

                st.status = "running"
                try:
                    result = await self._call_a2a_agent(session, st)
                    st.result = result
                    st.status = "completed"
                    results[st.id] = result
                except Exception as e:
                    logger.exception(f"Error executing {st.id}: {e}")
                    st.status = "failed"
                    st.result = str(e)

        return results

    async def _call_a2a_agent(self, session: aiohttp.ClientSession, subtask: SubTask) -> str:
        """Call an agent via A2A JSON-RPC."""
        a2a_url = f"{subtask.agent_url}/a2a"
        logger.info(f"[A2A] Calling {subtask.provider_id} at {a2a_url}")

        payload = {
            "jsonrpc": "2.0",
            "method": "message/send",
            "id": subtask.id,
            "params": {
                "message": {
                    "role": "user",
                    "parts": [{"type": "text", "text": subtask.input}],
                }
            },
        }

        try:
            async with session.post(a2a_url, json=payload) as resp:
                if resp.status != 200:
                    error = await resp.text()
                    raise Exception(f"A2A call failed: {error}")

                data = await resp.json()

                if "error" in data:
                    raise Exception(data["error"].get("message", "Unknown error"))

                result = data.get("result", {})
                history = result.get("history", [])

                # Extract agent response
                for msg in reversed(history):
                    if msg.get("role") == "agent":
                        parts = msg.get("parts", [])
                        for part in parts:
                            if part.get("type") == "text":
                                return part.get("text", "")

                return "No response from agent"

        except aiohttp.ClientError as e:
            # If agent not reachable, return mock response
            logger.warning(f"Could not reach agent at {a2a_url}: {e}")
            return f"[Demo] Mock response for: {subtask.description}"

    def _aggregate_results(self, original_request: str, subtasks: list[SubTask]) -> str:
        """Aggregate results from all subtasks."""
        lines = ["# Orchestration Results\n"]
        lines.append(f"**Original Request**: {original_request}\n")
        lines.append(f"**Subtasks Executed**: {len(subtasks)}\n")

        # Agent Selection Summary
        lines.append("\n## Agent Selection Summary\n")
        lines.append("| Subtask | Skill Tags | Selected Agent | Selection Reason |")
        lines.append("|---------|------------|----------------|------------------|")

        for st in subtasks:
            tags_str = ", ".join(st.skill_tags[:2]) if st.skill_tags else "N/A"
            reason = self._get_selection_reason(st.skill_tags)
            lines.append(f"| {st.description[:40]}... | `{tags_str}` | {st.provider_id or 'Unknown'} | {reason} |")

        lines.append("\n---\n")

        for st in subtasks:
            status_icon = "PASS" if st.status == "completed" else "FAIL"
            lines.append(f"\n## [{status_icon}] {st.description}")
            lines.append(f"**Provider**: {st.provider_id or 'Unknown'}")
            lines.append(f"**Skill Tags**: {', '.join(st.skill_tags)}")
            lines.append(f"**Status**: {st.status}\n")
            if st.result:
                lines.append(st.result)
            lines.append("\n---")

        return "\n".join(lines)

    def _get_selection_reason(self, skill_tags: list[str]) -> str:
        """Get reason for agent selection based on skill tags."""
        reasons = {
            "code_review": "Basic review -> Budget tier (QuickReview)",
            "linting": "Lint/style check -> Budget tier (QuickReview)",
            "bug_detection": "Bug detection -> Budget tier (QuickReview)",
            "security_audit": "Security scan -> Standard tier (CodeGuard)",
            "architecture_review": "Architecture review -> Premium tier (ArchitectAI)",
            "performance_review": "Performance analysis -> Premium tier (ArchitectAI)",
        }
        for tag in skill_tags:
            if tag in reasons:
                return reasons[tag]
        return "Default routing"
