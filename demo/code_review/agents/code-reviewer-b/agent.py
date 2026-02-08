"""Code Reviewer B (Standard) - Security-focused code review using Claude."""

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

# Standard tier prompts - balanced detail with security focus
CODE_REVIEW_PROMPT = """You are an experienced code reviewer with a strong security background.

Provide a COMPREHENSIVE review including:

1. **Executive Summary** (2-3 sentences)
2. **Code Quality Assessment**
   - Readability and maintainability
   - Error handling coverage
   - Test coverage gaps
3. **Security Analysis**
   - Input validation issues
   - Authentication/authorization flaws
   - Data exposure risks
   - Injection vulnerabilities
4. **Bug Detection**
   - Logic errors
   - Edge cases not handled
   - Race conditions
   - Memory/resource leaks
5. **Recommendations**
   - Prioritized fixes (Critical/High/Medium/Low)
   - Suggested improvements
   - Security hardening steps

Be thorough but organized. Use tables where helpful."""

SECURITY_AUDIT_PROMPT = """You are a security specialist performing a detailed code security audit.

Provide a COMPREHENSIVE security assessment:

1. **Vulnerability Summary**
   - Critical vulnerabilities found
   - OWASP Top 10 mapping
   - CVSS severity scores
2. **Injection Analysis**
   - SQL injection vectors
   - XSS vulnerabilities
   - Command injection risks
   - Path traversal issues
3. **Authentication & Authorization**
   - Auth bypass possibilities
   - Session management flaws
   - Privilege escalation risks
4. **Data Security**
   - Sensitive data exposure
   - Encryption weaknesses
   - Logging of secrets
5. **Remediation Plan**
   - Prioritized fix list
   - Code examples for fixes
   - Timeline recommendations

Be specific about which lines and functions are affected."""


@dataclass
class CodeReviewerB(BaseAgent):
    """Standard Code Reviewer using Claude for security-focused analysis."""

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
            api_key=api_key,
        )
        logger.info(f"Initialized Claude LLM (Standard): {self.config.llm.model}")

    def _build_graph(self):
        """Build the LangGraph workflow."""
        self._graph = StateGraph(AgentState)

    def _detect_skill(self, content: str) -> str:
        """Detect which skill to use based on content."""
        content_lower = content.lower()

        security_keywords = [
            "security", "vulnerability", "exploit", "injection",
            "xss", "csrf", "auth", "owasp", "audit", "penetration",
            "cve", "attack", "threat"
        ]
        if any(kw in content_lower for kw in security_keywords):
            return "security_audit"

        return "code_review"

    async def process(self, state: AgentState) -> AgentState:
        """Process the code review request through Claude (standard mode)."""
        messages = state["messages"]
        if not messages:
            state["result"] = "No message provided."
            return state

        user_content = messages[-1].get("content", "")
        skill = self._detect_skill(user_content)

        prompts = {
            "code_review": CODE_REVIEW_PROMPT,
            "security_audit": SECURITY_AUDIT_PROMPT,
        }
        system_prompt = prompts.get(skill, CODE_REVIEW_PROMPT)

        if self.llm is None:
            state["result"] = self._mock_response(skill, user_content)
            state["artifacts"] = [{
                "name": f"{skill}_standard_report.txt",
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
                "name": f"{skill}_standard_report.txt",
                "parts": [{"type": "text", "text": result}],
            }]

        except Exception as e:
            logger.exception(f"Error calling Claude: {e}")
            state["result"] = f"Error processing request: {str(e)}"

        return state

    def _mock_response(self, skill: str, content: str) -> str:
        """Generate mock response for testing (standard tier - security focused)."""
        if skill == "code_review":
            return """## Code Review Report (Security Focus)

### Executive Summary
This codebase has several security concerns that need immediate attention. While the general code quality is acceptable, input validation and error handling gaps create exploitable attack vectors.

### Code Quality Assessment

| Aspect | Rating | Notes |
|--------|--------|-------|
| **Readability** | Good | Clear naming, decent structure |
| **Maintainability** | Fair | Some functions too long (>50 lines) |
| **Error Handling** | Poor | Missing try/catch in 3 critical paths |
| **Test Coverage** | Fair | ~60% coverage, no security tests |

### Security Analysis

| Vulnerability | Severity | Location | OWASP Category |
|--------------|----------|----------|----------------|
| SQL Injection | **Critical** | `db.query()` line 45 | A03:2021 |
| XSS (Reflected) | **High** | `render()` line 78 | A03:2021 |
| Hardcoded Secret | **High** | `config.py` line 12 | A02:2021 |
| Missing Auth Check | **Medium** | `api/admin.py` line 30 | A01:2021 |
| Verbose Error Messages | **Low** | `handler.py` line 92 | A09:2021 |

### Bug Detection

- **Logic Error**: Off-by-one in pagination (`page * size` should be `(page-1) * size`)
- **Edge Case**: Null user input crashes `process_data()` at line 67
- **Race Condition**: Concurrent writes to shared cache without locking
- **Resource Leak**: Database connection not closed in error path (line 52)

### Recommendations

**Critical (Fix Immediately):**
1. Parameterize all SQL queries - use prepared statements
2. Sanitize user input before rendering HTML output
3. Move secrets to environment variables

**High (Fix This Sprint):**
4. Add authentication middleware to admin endpoints
5. Close DB connections in finally blocks

**Medium (Fix Next Sprint):**
6. Add input validation on all public API endpoints
7. Implement rate limiting on authentication endpoints

*Standard review - $16 | ~5 min*"""
        else:
            return """## Security Audit Report

### Vulnerability Summary

**Overall Security Posture: HIGH RISK**

| Severity | Count | Status |
|----------|-------|--------|
| Critical | 2 | Requires immediate fix |
| High | 3 | Fix within 48 hours |
| Medium | 4 | Fix within 1 week |
| Low | 2 | Fix in next release |

### Injection Analysis

#### SQL Injection (CRITICAL)
- **Location**: `models/user.py:45` - `db.query(f"SELECT * FROM users WHERE id={user_id}")`
- **Impact**: Full database access, data exfiltration
- **Fix**: Use parameterized queries: `db.query("SELECT * FROM users WHERE id=?", [user_id])`

#### XSS - Cross-Site Scripting (HIGH)
- **Location**: `views/profile.py:78` - `return f"<h1>Welcome {username}</h1>"`
- **Impact**: Session hijacking, credential theft
- **Fix**: HTML-encode all user input: `html.escape(username)`

#### Command Injection (HIGH)
- **Location**: `utils/file_handler.py:23` - `os.system(f"convert {filename}")`
- **Impact**: Arbitrary command execution on server
- **Fix**: Use `subprocess.run()` with shell=False and argument list

### Authentication & Authorization

| Issue | Risk | Details |
|-------|------|---------|
| No CSRF tokens | **High** | State-changing operations unprotected |
| Session fixation | **Medium** | Session ID not rotated after login |
| Weak password policy | **Medium** | No minimum length or complexity |
| Missing rate limiting | **Medium** | Brute force attacks possible |

### Data Security

- **Hardcoded API Key**: `config.py:12` - AWS key in source code
- **Sensitive Logging**: `auth.py:34` - Password logged in plaintext
- **No Encryption**: User PII stored in plaintext in database
- **Insecure Cookie**: Session cookie missing Secure and HttpOnly flags

### Remediation Plan

| Priority | Action | Effort | Deadline |
|----------|--------|--------|----------|
| 1 | Fix SQL injection vectors | 2 hours | Immediate |
| 2 | Implement input sanitization | 4 hours | Day 1 |
| 3 | Remove hardcoded secrets | 1 hour | Day 1 |
| 4 | Add CSRF protection | 3 hours | Day 2 |
| 5 | Implement rate limiting | 2 hours | Day 3 |
| 6 | Add encryption for PII | 4 hours | Week 1 |
| 7 | Security test suite | 8 hours | Week 2 |

*Standard security audit - $16 | ~5 min*"""
