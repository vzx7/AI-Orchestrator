# AI Orchestrator V5

A distributed multi-agent AI orchestration platform built in Go (1.26.1). V5 adds **circuit breakers**, **visibility timeouts**, **jittered retries**, and **latency-aware load balancing**.

> ⚠️ **Status:** Working prototype with HTTP API. Core components work. Some features still need real integrations (LLM, MCP tools).

## Key Features

- **DAG-based Execution**: Tasks with dependencies executed in parallel when possible
- **Adaptive Control Loop**: Plan→Execute→Evaluate→Replan
- **Reliable Queue**: Ack/Nack semantics ensure no task loss
- **Idempotent Execution**: Safe retries with idempotency keys
- **Dead Letter Queue**: Failed tasks captured for inspection
- **Worker Health Tracking**: Heartbeat-based worker monitoring
- **Retry with Jitter**: ±30% randomization prevents thundering herd
- **Circuit Breaker**: Automatic failure isolation per worker
- **Visibility Reaper**: Auto-recovery of stuck tasks
- **State Validation**: Enforces valid state transitions
- **Latency-Aware LB**: Considers worker latency in selection
- **Context Relevance**: Retrieves most relevant context items
- **HTTP REST API**: Full HTTP API for remote access

## Quick Start

```bash
# Clone and build
git clone <repo>
cd AI-Orchestrator
go mod download

# Run HTTP server (recommended for remote access)
go run ./cmd/server/main.go -distributed

# Run local mode (all-in-one, no network)
go run ./cmd/orchestrator/

# Run distributed demo (simulated workers in-process)
go run ./cmd/orchestrator/ --distributed
```

## HTTP API Usage

```bash
# Health check
curl http://localhost:8080/health

# Create task
curl -X POST http://localhost:8080/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"goal": "Fix failing test and deploy service"}'

# Get task list
curl http://localhost:8080/v1/tasks

# Get queue status
curl http://localhost:8080/v1/queue

# Get Dead Letter Queue
curl http://localhost:8080/v1/dlq
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
| **V5** | **Circuit Breaker + Visibility Timeout + HTTP API + gRPC** |

## Running Tests

```bash
go test ./... -v
```

## Implementation Status

### ✅ Working Components

| Component | Status | Notes |
|-----------|--------|-------|
| HTTP REST API | ✅ | Full API on :8080 |
| Circuit Breaker | ✅ | Wired into RPC |
| Visibility Reaper | ✅ | Started in distributed mode |
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
| gRPC Proto | ✅ | Compiled to internal/rpc/proto |

### ⚠️ Partial Implementation

| Component | Status | Notes |
|-----------|--------|-------|
| Real gRPC transport | ⚠️ | Proto compiled, using demo transport |
| PostgreSQL Store | ⚠️ | Requires `-tags postgres` build |
| Real LLM integration | ⚠️ | Mock planner only |
| Real MCP network calls | ⚠️ | Mock tools only |

### ❌ Not Implemented

| Feature | Notes |
|---------|-------|
| Redis/Kafka queue | Not added |
| Prometheus metrics | Not added |
| OpenTelemetry tracing | Not added |

## Production TODO

- [x] Wire Circuit Breaker into RPC/execution path
- [x] Start Visibility Reaper in main.go
- [x] Add HTTP REST API server
- [x] Compile gRPC proto
- [ ] Replace mock planner with real LLM API
- [ ] Replace mock MCP tools with real network calls
- [ ] Add Prometheus metrics
- [ ] Add distributed tracing (OpenTelemetry)
- [ ] Use Redis/Kafka for shared queue (multi-instance)

See [User Guide](docs/USER_GUIDE.md) for current usage instructions.