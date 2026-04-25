# AI Orchestrator V5

A distributed multi-agent AI orchestration platform built in Go (1.26.1). V5 adds **circuit breakers**, **visibility timeouts**, **jittered retries**, and **latency-aware load balancing**.

> ⚠️ **Status:** This is a **working prototype**. Core components (planner, executor, agents, queue, DLQ) work. Some V5 features (Circuit Breaker, Visibility Reaper) are implemented but not yet wired into the execution path.

## Key Features

- **DAG-based Execution**: Tasks with dependencies executed in parallel when possible
- **Adaptive Control Loop**: Plan→Execute→Evaluate→Replan
- **Reliable Queue**: Ack/Nack semantics ensure no task loss
- **Idempotent Execution**: Safe retries with idempotency keys
- **Dead Letter Queue**: Failed tasks captured for inspection
- **Worker Health Tracking**: Heartbeat-based worker monitoring
- **Retry with Jitter**: ±30% randomization prevents thundering herd
- **State Validation**: Enforces valid state transitions
- **Latency-Aware LB**: Considers worker latency in selection
- **Context Relevance**: Retrieves most relevant context items

## Quick Start

```bash
# Clone and build
git clone <repo>
cd AI-Orchestrator
go mod download

# Run in local mode (all-in-one)
go run ./cmd/orchestrator/

# Run in distributed mode (simulated workers)
go run ./cmd/orchestrator/ --distributed
```

## Documentation

| Language | Guide |
|----------|-------|
| 🇬🇧 English | [Deployment Guide](docs/DEPLOYMENT.md) |
| 🇷🇺 Русский | [Руководство по развёртыванию](docs/DEPLOYMENT_RU.md) |
| 🇬🇧 English | [User Guide](docs/USER_GUIDE.md) |
| 🇷🇺 Русский | [Руководство пользователя](docs/USER_GUIDE_RU.md) |
| 🇬🇧/🇷🇺 | [How It Works](docs/SERVICE_EXPLAINED.md) |

## Architecture Evolution

| Version | Key Capability |
|---------|---------------|
| V1 | Linear task execution + agents + tool gateway |
| V2 | Adaptive control loop (Plan→Execute→Evaluate→Replan) + DAG |
| V3 | Distributed execution with worker nodes + RPC + queue |
| V4 | Reliable + Persistent + Fault-Tolerant |
| **V5** | **Circuit Breaker + Visibility Timeout + Jitter + Relevance Scoring** |

## Running Tests

```bash
go test ./... -v
```

## Implementation Status

### ✅ Working Components

| Component | Status | Notes |
|-----------|--------|-------|
| Agent interface | ✅ | DevAgent, QAAgent, OpsAgent (mock) |
| Tool Gateway | ✅ | ACL enforcement working |
| MCP Registry | ✅ | Mock tools implemented |
| Planner | ✅ | DAG generation working |
| Controller | ✅ | Control loop working |
| Execution Engine | ✅ | DAG-based parallel execution |
| Evaluator | ✅ | Result evaluation working |
| Memory Queue | ✅ | Ack/Nack semantics |
| Idempotency Store | ✅ | LRU cache |
| Dead Letter Queue | ✅ | Working |
| Worker Registry | ✅ | In-memory, working |
| Event Bus | ✅ | Working |
| Context Manager | ✅ | Working |
| State Store | ✅ | In-memory |
| RPC Layer | ✅ | Demo mode (in-process) |

### ⚠️ Partial Implementation

| Component | Status | Notes |
|-----------|--------|-------|
| Circuit Breaker | ⚠️ | Code exists, not wired into execution |
| Visibility Reaper | ⚠️ | Code exists, not started in main.go |
| Retry with Jitter | ⚠️ | Jitter code exists, basic retry used |
| PostgreSQL Store | ⚠️ | Requires `-tags postgres` build |
| Latency-aware LB | ⚠️ | Code exists, limited effect |

### ❌ Not Implemented

| Feature | Notes |
|---------|-------|
| HTTP REST API | No HTTP server in codebase |
| Real gRPC | Proto file not compiled |
| Real LLM integration | Mock planner only |
| Real MCP network calls | Mock tools only |
| Redis/Kafka queue | Not added |
| Prometheus metrics | Not added |
| OpenTelemetry tracing | Not added |

## Production TODO

- [ ] Wire Circuit Breaker into RPC/execution path
- [ ] Start Visibility Reaper in main.go
- [ ] Add HTTP REST API server
- [ ] Compile and use real gRPC
- [ ] Replace mock planner with real LLM API
- [ ] Replace mock MCP tools with real network calls
- [ ] Add Prometheus metrics
- [ ] Add distributed tracing (OpenTelemetry)
- [ ] Use Redis/Kafka for shared queue (multi-instance)

See [User Guide](docs/USER_GUIDE.md) for current usage instructions.