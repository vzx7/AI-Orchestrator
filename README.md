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

### 1. Clone and build

```bash
git clone <repo>
cd AI-Orchestrator
go mod download
go build ./...
```

### 2. Run server

```bash
# With distributed workers (recommended)
go run ./cmd/server/main.go -distributed

# Or local mode (all-in-one)
go run ./cmd/server/main.go
```

### 3. Use CLI

```bash
# Build CLI
go build -o orchestrator-cli ./cmd/cli

# Health check
./orchestrator-cli health

# Submit task
./orchestrator-cli run "Analyze codebase and find todos"

# List tasks
./orchestrator-cli list
```

### 4. Or use HTTP API

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
```

## Project Structure

```
AI-Orchestrator/
├── cmd/
│   ├── server/main.go       # HTTP API server (recommended)
│   ├── orchestrator/main.go # Demo orchestrator (local mode)
│   ├── cli/main.go         # CLI tool
│   └── worker/main.go       # Standalone worker
├── sdk/
│   ├── go/orchestrator/    # Go SDK
│   └── python/             # Python SDK (async)
├── internal/              # Core components
├── deploy/
│   ├── docker-compose.yml  # Docker Compose
│   ├── k8s/              # Kubernetes manifests
│   └── Dockerfile        # Multi-stage build
└── docs/                  # Documentation
```

## SDK Clients

### Go SDK

```bash
go get ai_orchestrator/sdk/go/orchestrator
```

```go
import "ai_orchestrator/sdk/go/orchestrator"

client := orchestrator.NewClient(
    orchestrator.WithURL("http://localhost:8080"),
)
health, _ := client.Health(ctx)
```

### Python SDK

```bash
pip install -e ai_orchestrator/sdk/python
```

```python
import asyncio
from ai_orchestrator import OrchestratorClient

async def main():
    async with OrchestratorClient(url="http://localhost:8080") as client:
        health = await client.health()
```

## Documentation

| Language | Guide |
|----------|-------|
| 🇬🇧 English | [Deployment Guide](docs/DEPLOYMENT.md) |
| 🇷🇺 Русский | [Руководство по развёртыванию](docs/DEPLOYMENT_RU.md) |
| 🇬🇧 English | [User Guide](docs/USER_GUIDE.md) |
| 🇷🇺 Русский | [Руководство пользователя](docs/USER_GUIDE_RU.md) |
| CLI | [CLI README](cmd/cli/README.md) |

## Architecture Evolution

| Version | Key Capability |
|---------|---------------|
| V1 | Linear task execution + agents + tool gateway |
| V2 | Adaptive control loop (Plan→Execute→Evaluate→Replan) + DAG |
| V3 | Distributed execution with worker nodes + RPC + queue |
| V4 | Reliable + Persistent + Fault-Tolerant |
| **V5** | **Circuit Breaker + Visibility Timeout + HTTP API** |

## Running Tests

```bash
go test ./... -v
```

## Implementation Status

### ✅ Working Components

| Component | Status | Notes |
|-----------|--------|-------|
| HTTP REST API | ✅ | Full API on :8080 |
| CLI | ✅ | Standalone, built-in HTTP |
| Go SDK | ✅ | Part of repo |
| Python SDK | ✅ | Async aiohttp |
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
- [x] Add CLI tool
- [x] Add Go SDK
- [x] Add Python SDK
- [ ] Replace mock planner with real LLM API
- [ ] Replace mock MCP tools with real network calls
- [ ] Add Prometheus metrics
- [ ] Add distributed tracing (OpenTelemetry)
- [ ] Use Redis/Kafka for shared queue (multi-instance)

See [User Guide](docs/USER_GUIDE.md) for current usage instructions.