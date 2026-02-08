"""Code Reviewer C (Premium) - Expert-level architectural review using Claude."""

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

# Premium tier prompts - exhaustive expert analysis
CODE_REVIEW_PROMPT = """You are a principal software engineer at a top tech company providing EXHAUSTIVE code analysis.

Deliver EXPERT-LEVEL review including:

1. **Executive Summary** - Strategic overview for engineering leadership
2. **Architecture Assessment**
   - System design evaluation
   - Component coupling analysis
   - Dependency graph concerns
   - Scalability implications
3. **Code Quality Deep Dive**
   - SOLID principles adherence
   - DRY/KISS/YAGNI compliance
   - Cyclomatic complexity hotspots
   - Test coverage and quality
4. **Design Pattern Analysis**
   - Patterns currently used
   - Anti-patterns detected
   - Recommended pattern improvements
   - Pattern migration strategy
5. **Performance Analysis**
   - Time complexity of critical paths
   - Space complexity concerns
   - Database query optimization
   - Caching opportunities
6. **Maintainability Score**
   - Technical debt assessment
   - Documentation quality
   - API design evaluation
   - Breaking change risks
7. **Refactoring Roadmap**
   - Prioritized refactoring items
   - Effort estimates
   - Risk/reward analysis
   - Migration strategies
8. **Strategic Recommendations**
   - Short-term improvements
   - Long-term architecture evolution
   - Team skill development areas

This is staff-engineer-level analysis. Be exhaustive. Miss nothing."""

ARCHITECTURE_REVIEW_PROMPT = """You are a chief architect providing EXHAUSTIVE architectural analysis.

Deliver EXPERT-LEVEL architecture assessment:

1. **Executive Summary** - Board-level architecture status
2. **System Architecture Evaluation**
   - Current architecture style (monolith/microservices/serverless)
   - Component boundaries and cohesion
   - Service communication patterns
   - Data flow analysis
3. **Design Pattern Assessment**
   - Patterns in use and their effectiveness
   - Anti-patterns and technical debt
   - Pattern recommendations with examples
   - Migration path for pattern changes
4. **Scalability Analysis**
   - Horizontal vs vertical scaling readiness
   - Bottleneck identification
   - Load distribution concerns
   - Database scaling strategy
5. **Reliability & Resilience**
   - Single points of failure
   - Fault tolerance mechanisms
   - Circuit breaker patterns
   - Disaster recovery readiness
6. **Performance Architecture**
   - Critical path analysis
   - Caching strategy evaluation
   - Async processing opportunities
   - Resource utilization optimization
7. **Evolution Roadmap**
   - Current state assessment
   - Target architecture vision
   - Migration phases with milestones
   - Investment requirements
   - Risk mitigation strategies
8. **Technology Stack Review**
   - Current stack evaluation
   - Upgrade recommendations
   - Emerging technology opportunities
   - Vendor lock-in assessment

This is for architecture review board. Be exhaustive."""


@dataclass
class CodeReviewerC(BaseAgent):
    """Premium Code Reviewer using Claude for exhaustive, expert-level analysis."""

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
        logger.info(f"Initialized Claude LLM (Premium): {self.config.llm.model}")

    def _build_graph(self):
        """Build the LangGraph workflow."""
        self._graph = StateGraph(AgentState)

    def _detect_skill(self, content: str) -> str:
        """Detect which skill to use based on content."""
        content_lower = content.lower()

        architecture_keywords = [
            "architecture", "design pattern", "refactor", "scalability",
            "microservice", "monolith", "coupling", "cohesion",
            "system design", "component", "module", "dependency"
        ]
        if any(kw in content_lower for kw in architecture_keywords):
            return "architecture_review"

        return "code_review"

    async def process(self, state: AgentState) -> AgentState:
        """Process the code review request through Claude (premium mode)."""
        messages = state["messages"]
        if not messages:
            state["result"] = "No message provided."
            return state

        user_content = messages[-1].get("content", "")
        skill = self._detect_skill(user_content)

        prompts = {
            "code_review": CODE_REVIEW_PROMPT,
            "architecture_review": ARCHITECTURE_REVIEW_PROMPT,
        }
        system_prompt = prompts.get(skill, CODE_REVIEW_PROMPT)

        if self.llm is None:
            state["result"] = self._mock_response(skill, user_content)
            state["artifacts"] = [{
                "name": f"{skill}_premium_report.txt",
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
                "name": f"{skill}_premium_report.txt",
                "parts": [{"type": "text", "text": result}],
            }]

        except Exception as e:
            logger.exception(f"Error calling Claude: {e}")
            state["result"] = f"Error processing request: {str(e)}"

        return state

    def _mock_response(self, skill: str, content: str) -> str:
        """Generate mock response for testing (premium tier - exhaustive)."""
        if skill == "code_review":
            return """## Premium Code Analysis Report
### Prepared by ArchitectAI Labs

---

## 1. Executive Summary

This codebase exhibits a **moderate architectural maturity level** with significant opportunities for improvement. The primary concerns are tight coupling between modules, inconsistent application of design patterns, and accumulating technical debt in the data layer. **Recommendation: Prioritize refactoring before adding new features.**

---

## 2. Architecture Assessment

### 2.1 Component Coupling Analysis

| Component Pair | Coupling Type | Severity | Impact |
|---------------|---------------|----------|--------|
| UserService <-> Database | Tight (direct SQL) | **High** | Hard to test, migrate |
| Controller <-> BusinessLogic | Moderate | **Medium** | Mixed concerns |
| API Layer <-> Validation | Loose | **Low** | Well-structured |
| Config <-> All Modules | Global state | **High** | Testing nightmare |

### 2.2 Dependency Graph Concerns
- **Circular dependency**: `auth` -> `user` -> `permissions` -> `auth`
- **God object**: `AppContext` class has 23 methods, 15 dependencies
- **Missing abstraction**: Direct database calls in 4 controller files

### 2.3 Scalability Implications
- Current architecture supports ~1000 concurrent users
- Database layer is the primary bottleneck (no connection pooling)
- Stateful session management prevents horizontal scaling

---

## 3. Code Quality Deep Dive

### 3.1 SOLID Principles Adherence

| Principle | Score | Issues Found |
|-----------|-------|-------------|
| **S**ingle Responsibility | 4/10 | UserService handles auth + profile + notifications |
| **O**pen/Closed | 6/10 | Some extension points, but many switch statements |
| **L**iskov Substitution | 8/10 | Good interface compliance |
| **I**nterface Segregation | 5/10 | Fat interfaces in data layer |
| **D**ependency Inversion | 3/10 | Concrete dependencies everywhere |

### 3.2 Complexity Hotspots

| Function | Cyclomatic Complexity | Risk | Recommendation |
|----------|----------------------|------|----------------|
| `process_order()` | 24 | **Critical** | Split into 4-5 functions |
| `validate_input()` | 18 | **High** | Use strategy pattern |
| `generate_report()` | 15 | **High** | Extract report builders |
| `handle_request()` | 12 | **Medium** | Simplify branching |

---

## 4. Design Pattern Analysis

### 4.1 Patterns Currently Used
- **Repository Pattern**: Partially implemented (3 of 8 models)
- **MVC**: Present but controller layer is bloated
- **Singleton**: Overused (config, logger, cache, db - all singletons)

### 4.2 Anti-Patterns Detected

| Anti-Pattern | Location | Severity | Fix |
|-------------|----------|----------|-----|
| God Class | `AppContext` | **Critical** | Decompose into focused services |
| Spaghetti Code | `process_order()` | **High** | Apply chain of responsibility |
| Magic Numbers | Throughout | **Medium** | Extract to named constants |
| Copy-Paste Code | `validators/` | **Medium** | Create base validator class |
| Premature Optimization | `cache_layer.py` | **Low** | Remove unused cache logic |

### 4.3 Recommended Pattern Improvements
1. **Strategy Pattern** for validation logic (eliminate switch statements)
2. **Factory Pattern** for service instantiation (remove manual wiring)
3. **Observer Pattern** for event handling (decouple notification system)
4. **Repository Pattern** completion for all data models

---

## 5. Performance Analysis

### 5.1 Critical Path Analysis

| Operation | Current Latency | Bottleneck | Optimized Target |
|-----------|----------------|------------|-----------------|
| User login | 450ms | Password hashing + DB | 120ms |
| List items | 800ms | N+1 query problem | 150ms |
| Generate report | 3.2s | Sequential processing | 800ms |
| File upload | 2.1s | Synchronous processing | 200ms (async) |

### 5.2 Database Query Issues
- **N+1 Queries**: 6 endpoints fetch related data in loops
- **Missing Indexes**: `orders.user_id`, `products.category_id` not indexed
- **Full Table Scans**: Search endpoint scans entire products table
- **No Connection Pooling**: New connection per request (~50ms overhead)

### 5.3 Caching Opportunities
- User session data: **Save ~200ms/request** with Redis
- Product catalog: **Save ~500ms** with 5-minute TTL cache
- Report generation: **Save ~2s** with pre-computation

---

## 6. Maintainability Score

### Overall: 42/100 (Needs Improvement)

| Dimension | Score | Details |
|-----------|-------|---------|
| Code readability | 55/100 | Inconsistent style, missing docs |
| Test quality | 35/100 | Low coverage, no integration tests |
| Documentation | 30/100 | Outdated README, no API docs |
| API design | 50/100 | Inconsistent naming, missing versioning |
| Technical debt | 40/100 | Significant accumulated debt |

### Technical Debt Inventory

| Item | Effort | Business Impact | Priority |
|------|--------|----------------|----------|
| Database abstraction | 3 weeks | Enables migration | **P1** |
| Test suite expansion | 2 weeks | Reduces bug rate 40% | **P1** |
| Service decomposition | 4 weeks | Enables scaling | **P2** |
| API versioning | 1 week | Client stability | **P2** |
| Documentation overhaul | 1 week | Onboarding speed 2x | **P3** |

---

## 7. Refactoring Roadmap

### Phase 1: Foundation (Weeks 1-2) - Risk: Low
1. Add dependency injection container
2. Extract configuration to typed config classes
3. Standardize error handling with custom exceptions
4. Add comprehensive logging

### Phase 2: Data Layer (Weeks 3-4) - Risk: Medium
5. Complete Repository Pattern for all models
6. Add connection pooling
7. Fix N+1 queries with eager loading
8. Add database migrations framework

### Phase 3: Service Layer (Weeks 5-8) - Risk: Medium
9. Decompose `AppContext` into focused services
10. Apply Strategy Pattern to validators
11. Implement event-driven notifications
12. Add caching layer with Redis

### Phase 4: API & Testing (Weeks 9-10) - Risk: Low
13. Add API versioning
14. Write integration test suite
15. Add OpenAPI documentation
16. Implement health check endpoints

---

## 8. Strategic Recommendations

### Short-Term (This Quarter)
- Fix critical performance bottlenecks (N+1 queries, connection pooling)
- Add dependency injection to enable proper testing
- Establish coding standards and automated linting

### Long-Term (Next 2 Quarters)
- Migrate to hexagonal architecture for better testability
- Evaluate microservices extraction for scaling-critical components
- Implement CI/CD pipeline with automated quality gates

### Team Development
- Design patterns workshop (focus on SOLID)
- Code review culture improvement
- Architecture decision records (ADR) practice

---

*Premium analysis - $30 | ~10 min*
*Prepared by ArchitectAI Labs - Confidential*"""
        else:
            return """## Premium Architecture Review Report
### Prepared by ArchitectAI Labs

---

## 1. Executive Summary

**Architecture Maturity: Level 2 of 5 (Developing)**

The current system architecture is a tightly-coupled monolith with emerging microservice aspirations. Key risks include single points of failure in the data layer, lack of service boundaries, and insufficient resilience patterns. A phased migration strategy is recommended.

---

## 2. System Architecture Evaluation

### 2.1 Current Architecture Style
- **Primary**: Monolithic with layered architecture
- **Emerging**: Some service extraction attempted (auth, notifications)
- **Data**: Single relational database, no event sourcing
- **Communication**: Synchronous HTTP only, no message queues

### 2.2 Component Boundaries

| Component | Cohesion | Coupling | Boundary Quality |
|-----------|----------|----------|-----------------|
| Auth Service | High | Medium | **Good** - well-defined API |
| User Module | Medium | High | **Poor** - leaky abstractions |
| Order Processing | Low | High | **Critical** - god module |
| Notification | High | Low | **Good** - loosely coupled |
| Reporting | Low | High | **Poor** - queries everything |

### 2.3 Data Flow Analysis
```
Client -> API Gateway -> Monolith -> Single Database
                           |-> Auth Service (extracted)
                           |-> Notification Service (extracted)
                           |-> [Everything else still coupled]
```

**Issues identified:**
- No API gateway pattern (direct monolith access)
- Database as integration point (shared tables between modules)
- Synchronous chain: one slow service blocks everything

---

## 3. Design Pattern Assessment

### 3.1 Pattern Effectiveness

| Pattern | Implementation | Effectiveness | Recommendation |
|---------|---------------|---------------|----------------|
| MVC | Full | 60% | Migrate to hexagonal |
| Repository | Partial (3/8) | 40% | Complete implementation |
| Singleton | Overused | 30% | Replace with DI container |
| Observer | None | N/A | Add for event handling |
| CQRS | None | N/A | Add for read-heavy paths |

### 3.2 Recommended Architecture Patterns

**1. Hexagonal Architecture (Ports & Adapters)**
- Decouple business logic from infrastructure
- Enable testing without database/external services
- Clear dependency direction (inward only)

**2. Event-Driven Architecture**
- Decouple service communication
- Enable eventual consistency
- Support audit trail and replay

**3. Strangler Fig Pattern (for migration)**
- Incrementally extract services from monolith
- Route traffic between old and new implementations
- Zero-downtime migration

---

## 4. Scalability Analysis

### 4.1 Current Capacity Limits

| Resource | Current Max | Scaling Type | Bottleneck |
|----------|-------------|-------------|------------|
| Concurrent Users | ~1,000 | Vertical only | DB connections |
| Requests/sec | ~200 | N/A | CPU-bound processing |
| Data Volume | ~50GB | Single node | No sharding |
| File Storage | ~10GB | Local disk | No CDN/S3 |

### 4.2 Scaling Readiness Assessment

| Criteria | Ready? | Blocker |
|----------|--------|---------|
| Horizontal scaling | No | Stateful sessions, local file storage |
| Database scaling | No | No read replicas, no sharding strategy |
| Auto-scaling | No | No containerization, no metrics |
| Geographic distribution | No | Single region, no CDN |

### 4.3 Recommended Scaling Strategy
1. **Immediate**: Add connection pooling + read replicas
2. **Short-term**: Containerize with Kubernetes, externalize state
3. **Medium-term**: Extract hot-path services, add message queue
4. **Long-term**: Multi-region deployment with data partitioning

---

## 5. Reliability & Resilience

### 5.1 Single Points of Failure

| SPOF | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Primary database | **Total outage** | Medium | Add failover replica |
| Application server | **Total outage** | Medium | Add load balancer + 2nd instance |
| Auth service | **Login blocked** | Low | Add circuit breaker + cache |
| File storage | **Data loss** | Low | Migrate to S3 with versioning |

### 5.2 Missing Resilience Patterns
- No circuit breakers on external calls
- No retry logic with exponential backoff
- No bulkhead isolation between components
- No graceful degradation strategy
- No health check endpoints

---

## 6. Performance Architecture

### 6.1 Critical Path Optimization

| Path | Current P95 | Target P95 | Strategy |
|------|------------|------------|----------|
| User authentication | 450ms | 100ms | Cache + async token |
| Product listing | 800ms | 150ms | CQRS + materialized view |
| Order placement | 1.2s | 300ms | Async processing + queue |
| Report generation | 5s | 500ms | Pre-computation + cache |

### 6.2 Caching Architecture Recommendation
```
Client -> CDN (static assets)
       -> API Gateway (response cache, 30s TTL)
       -> Application (Redis session + entity cache)
       -> Database (query cache + read replica)
```

---

## 7. Evolution Roadmap

### Phase 1: Stabilize (Months 1-2)
- Add health checks and monitoring (Prometheus + Grafana)
- Implement circuit breakers (Resilience4j / Polly)
- Add database connection pooling and read replica
- Containerize application (Docker)
- **Investment: $40K | Risk: Low**

### Phase 2: Decouple (Months 3-4)
- Introduce message queue (RabbitMQ / SQS)
- Extract order processing to async pipeline
- Implement CQRS for read-heavy endpoints
- Add API gateway (Kong / AWS API Gateway)
- **Investment: $80K | Risk: Medium**

### Phase 3: Scale (Months 5-8)
- Deploy to Kubernetes with auto-scaling
- Extract 2-3 bounded contexts to microservices
- Implement event sourcing for audit trail
- Add CDN and edge caching
- **Investment: $120K | Risk: Medium-High**

### Phase 4: Optimize (Months 9-12)
- Multi-region deployment
- Advanced observability (distributed tracing)
- Performance tuning and load testing
- Chaos engineering practices
- **Investment: $60K | Risk: Low**

### Total Investment: $300K over 12 months
### Expected ROI: 4x through reduced incidents, faster delivery, scaling capability

---

## 8. Technology Stack Review

### 8.1 Current Stack Assessment

| Technology | Version | Status | Recommendation |
|------------|---------|--------|----------------|
| Python | 3.9 | Aging | Upgrade to 3.12 |
| Flask | 2.0 | Adequate | Consider FastAPI for new services |
| PostgreSQL | 13 | Adequate | Upgrade to 16, add replicas |
| Redis | None | Missing | Add for caching + sessions |
| Docker | None | Missing | Add for containerization |
| CI/CD | Basic | Incomplete | Add quality gates |

### 8.2 Vendor Lock-in Assessment
- **Low risk**: Open-source stack, portable
- **Watch**: If moving to cloud-managed services, prefer abstractions
- **Recommendation**: Use infrastructure-as-code (Terraform) from day 1

---

*Premium architecture analysis - $30 | ~10 min*
*Prepared by ArchitectAI Labs - Confidential*"""
