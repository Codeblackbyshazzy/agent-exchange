"""Code Reviewer A (Budget) - Fast, affordable code review using Claude."""

import logging
import os
from dataclasses import dataclass, field
from typing import Any, Optional

from langchain_anthropic import ChatAnthropic
from langchain_core.messages import HumanMessage, SystemMessage
from langgraph.graph import StateGraph, END

import sys
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from common.base_agent import BaseAgent, AgentState
from common.config import AgentConfig

logger = logging.getLogger(__name__)

# Budget tier prompts - concise and fast
CODE_REVIEW_PROMPT = """You are a quick code reviewer providing CONCISE feedback.
Keep responses SHORT and focused on the most critical issues only.

Provide:
1. 3-5 key issues (one line each)
2. Top 3 action items
3. Overall code quality rating (Poor/Fair/Good/Excellent)

Be direct. No lengthy explanations. Speed is priority."""

LINTING_PROMPT = """You are a code linter providing QUICK style and formatting feedback.
Keep responses SHORT and actionable.

Provide:
1. Style violations (bullet points)
2. Naming convention issues
3. Immediate fixes needed

Be concise. No detailed explanations."""


@dataclass
class CodeReviewerA(BaseAgent):
    """Budget Code Reviewer using Claude for fast, affordable analysis."""

    llm: Optional[ChatAnthropic] = field(default=None, init=False)

    def _setup_llm(self):
        """Initialize Claude LLM."""
        api_key = os.environ.get("ANTHROPIC_API_KEY")
        if not api_key:
            logger.warning("ANTHROPIC_API_KEY not set, using mock responses")
            self.llm = None
            return

        self.llm = ChatAnthropic(
            model=self.config.llm.model,
            temperature=self.config.llm.temperature,
            max_tokens=self.config.llm.max_tokens,
            anthropic_api_key=api_key,
        )
        logger.info(f"Initialized Claude LLM (Budget): {self.config.llm.model}")

    def _build_graph(self):
        """Build the LangGraph workflow."""
        self._graph = StateGraph(AgentState)

    def _detect_skill(self, content: str) -> str:
        """Detect which skill to use based on content."""
        content_lower = content.lower()
        lint_keywords = ["lint", "style", "format", "naming", "convention", "pep8", "eslint"]
        if any(kw in content_lower for kw in lint_keywords):
            return "linting"
        return "code_review"

    async def process(self, state: AgentState) -> AgentState:
        """Process the code review request through Claude (fast mode)."""
        messages = state["messages"]
        if not messages:
            state["result"] = "No message provided."
            return state

        user_content = messages[-1].get("content", "")
        skill = self._detect_skill(user_content)

        system_prompt = (
            CODE_REVIEW_PROMPT if skill == "code_review"
            else LINTING_PROMPT
        )

        if self.llm is None:
            state["result"] = self._mock_response(skill, user_content)
            state["artifacts"] = [{
                "name": f"{skill}_quick_analysis.txt",
                "parts": [{"type": "text", "text": state["result"]}],
            }]
            return state

        try:
            response = await self.llm.ainvoke([
                SystemMessage(content=system_prompt),
                HumanMessage(content=user_content),
            ])

            result = response.content
            state["result"] = result
            state["artifacts"] = [{
                "name": f"{skill}_quick_analysis.txt",
                "parts": [{"type": "text", "text": result}],
            }]

        except Exception as e:
            logger.exception(f"Error calling Claude: {e}")
            state["result"] = f"Error processing request: {str(e)}"

        return state

    def _mock_response(self, skill: str, content: str) -> str:
        """Generate mock response for testing (budget tier - concise)."""
        if skill == "code_review":
            return """## Quick Code Review

**Code Quality: FAIR**

### Key Issues:
- Missing error handling in main function
- Variable naming inconsistent (camelCase vs snake_case)
- No input validation on user parameters
- Hardcoded configuration values
- Missing type hints on public functions

### Action Items:
1. Add try/except blocks around I/O operations
2. Standardize naming convention to snake_case
3. Extract config values to environment variables

*Budget review - $5 | ~2 min*"""
        else:
            return """## Quick Lint Report

### Style Violations:
- Line 12: line too long (120 > 88 chars)
- Line 25: missing blank line after function
- Line 38: trailing whitespace

### Naming Issues:
- `getData` should be `get_data` (PEP 8)
- `processItem` should be `process_item`

### Fixes Needed:
1. Run black formatter
2. Fix variable names
3. Add missing docstrings

*Budget lint - $3 | ~1 min*"""
