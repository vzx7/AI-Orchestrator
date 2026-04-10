# AI Orchestrator V2

A production-grade, adaptive multi-agent AI orchestration platform built in Go. V2 evolves from linear execution to a **feedback-driven control loop** with DAG-based scheduling, intelligent evaluation, and dynamic replanning.

## Architecture V2

```
User Goal
    │
    ▼
┌─────────────────┐
│   Orchestrator   │  ← Entry point, wires all components
└────────┬────────┘
         │ delegates to
    ┌────┴────┐
    │Controller│  ← V2: Plan → Execute → Evaluate → Replan
    └────┬─────┘
         │
    ┌────┼────────────┐
    ▼    ▼            ▼
┌──────┐ ┌────────┐ ┌──────────┐
│Planner│ │Engine  │ │Evaluator │
└───┬──┘ └───┬────┘ └──────────┘
    │        │
    │   ┌────┴─────────┐
    │   ▼              ▼
    │ ┌──────┐  ┌────────────┐
    │ │Agents│→│ ToolGateway  │→ MCP Tools (mock)
    │ └──────┘  └────────────┘
    │             │
    ▼             ▼
┌───────┐  ┌──────────────┐
│EventBus│  │ContextManager│
└───────┘  └──────────────┘
```

## V2 Control Loop

```
Plan → Execute Task → Evaluate Result
→ if success → next task (DAG-aware)
→ if failed → retry (exponential backoff)
→ if retries exhausted → Replan (dynamic plan adjustment)
→ repeat until all tasks complete or max replans exceeded
```

## Project Structure

```
cmd/orchestrator/           # CLI entry point + tests
/internal/
  orchestrator/             # Core orchestrator (V2: delegates to Controller)
  controller/               # V2: Adaptive control loop (NEW)
  evaluator/                # V2: Heuristic-based task evaluation (NEW)
  planner/                  # Dynamic planning with DAG + Replan support
  execution/                # DAG-aware execution engine
  agents/                   # DevAgent, QAAgent, OpsAgent (NEW)
  tools/                    # Tool Gateway (ACL enforcement)
  mcp/                      # MCP tool registry (mock)
  context/                  # Short-term context manager
  events/                   # Pub/sub event bus
  types/                    # Core typed models (TaskNode, Evaluation, ExecutionTrace)
```

## V2 New Components

| Component | Purpose |
|-----------|---------|
| **Controller** | Runs the adaptive `Plan→Execute→Evaluate→Replan` loop |
| **Evaluator** | Assesses task results with confidence scores and retryability |
| **DAG Execution** | Tasks run only when dependencies are satisfied; parallel for independent nodes |
| **Replanning** | Planner dynamically adjusts plans on failure (removes failed tasks, inserts recovery) |
| **OpsAgent** | Handles deployment and infrastructure operations |
| **ExecutionTrace** | Collects per-step observability data (timing, evaluation, retries) |

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| Controller owns Planner, Evaluator, Engine | Composition over inheritance; clear ownership |
| DAG via `TaskNode.DependsOn` | Enables parallel execution of independent tasks |
| Cycle detection via DFS | Prevents infinite loops from malformed plans |
| Heuristic Evaluator (no LLM) | Deterministic, fast, swappable for ML-based later |
| `Planner.Replan` removes failed tasks + inserts recovery | Plan evolves based on feedback, not static |
| Agents never call tools directly | All tool access flows through `ToolGateway` with ACL |
| Interfaces everywhere | Swappable LLM backends, storage, evaluators |
| No global state | Everything injected via constructor — testable |

## Features

### V1 (Retained)
- Typed Task/Plan/Result model
- MCP tool integration (mock: file.read, file.write, shell.exec, test.run, deploy.service)
- Tool Gateway with ACL enforcement
- Event bus (pub/sub)
- Context management with summarization
- Structured logging via `log/slog`

### V2 (New)
- **Adaptive Control Loop**: Plan → Execute → Evaluate → Replan
- **DAG-Based Execution**: Dependency-aware scheduling with parallel support
- **Intelligent Evaluation**: Confidence scores, retryability detection
- **Dynamic Replanning**: Plan adjusts on failure with recovery tasks
- **Execution Tracing**: Full observability with per-step evaluation data
- **Circular Dependency Detection**: DFS-based cycle detection
- **OpsAgent**: Deployment and infrastructure operations

## Running the Demo

```bash
go run ./cmd/orchestrator/
```

Input: `"Fix failing test and deploy service"`

DAG structure:
```
[DevAgent: fix_test] → [QAAgent: run_tests] → [OpsAgent: deploy]
```

### Running Tests

```bash
go test ./... -v
```

Tests cover:
- DAG execution ordering
- Evaluator heuristics (success, retryable, exhausted retries)
- Replan logic (task removal, recovery insertion)
- Circular dependency detection
- Full control loop execution

## Production Improvements

1. **Replace mock LLM** with real API calls (OpenAI, Claude, local models)
2. **ML-based Evaluator** using historical execution data
3. **Persistent storage** for context and execution history (PostgreSQL, Redis)
4. **Distributed execution** via gRPC between orchestrator instances
5. **Real MCP networking** with proper protocol implementation
6. **Metrics export** (Prometheus) for monitoring and alerting
7. **Human-in-the-loop** approval gates for critical operations
8. **Secret management** integration (Vault, AWS Secrets Manager)
9. **Rate limiting** and circuit breakers for external tool calls
10. **Advanced DAG features**: conditional branches, sub-graphs, dynamic node injection

## Constraints

- No frameworks — stdlib only (`log/slog`, `context`, `sync`, `time`)
- No real LLM calls (mock implementation)
- No real MCP networking (mock tools)
- Clean architecture with decoupled modules
- V2 extends V1 — no components removed or rewritten from scratch
