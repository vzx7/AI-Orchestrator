# AI Orchestrator V5

A **production-ready**, distributed multi-agent AI orchestration platform built in Go (1.26.1). V5 evolves V4 with **circuit breakers**, **visibility timeouts**, **jittered retries**, and **latency-aware load balancing** for enhanced reliability under failure and high load.

## Key Features

- **No Stuck Tasks**: Visibility timeout reaper auto-recovers crashed worker tasks
- **Circuit Breaker**: Prevents cascading failures to workers
- **Retry with Jitter**: ±30% randomization prevents thundering herd
- **State Validation**: Enforces valid state transitions only
- **Latency-Aware Load Balancing**: Considers worker latency in selection
- **Context Relevance Scoring**: Retrieves most relevant context items
- **Safety Limits**: Max retries, execution time, tasks per workflow
- **Graceful Shutdown**: Background maintenance loops with context support
- All V4 features: Ack/Nack queue, idempotency, DLQ, persistent state

## Quick Start

```bash
# Clone and build
git clone <repo>
cd AI-Orchestrator
go mod download

# Run in local mode (all-in-one)
go run ./cmd/orchestrator/

# Run in distributed mode (with simulated workers)
go run ./cmd/orchestrator/ --distributed
```

## Documentation

| Language | Guide |
|----------|-------|
| 🇬🇧 English | [Deployment Guide](docs/DEPLOYMENT.md) |
| 🇷🇺 Русский | [Руководство по развёртыванию](docs/DEPLOYMENT_RU.md) |

## Architecture Evolution

| Version | Key Capability |
|---------|---------------|
| V1 | Linear task execution + agents + tool gateway |
| V2 | Adaptive control loop (Plan→Execute→Evaluate→Replan) + DAG |
| V3 | Distributed execution with worker nodes + RPC + queue |
| V4 | Reliable + Persistent + Fault-Tolerant (Production-Ready) |
| **V5** | **Circuit Breaker + Visibility Timeout + Jitter + Relevance Scoring** |

## Running Tests

```bash
go test ./... -v
```

## Production Deployment Checklist

- [x] Reliable queue (Ack/Nack)
- [x] Idempotent execution
- [x] Dead letter queue
- [x] Task state persistence
- [x] Worker health tracking
- [x] Load balancing (least-loaded)
- [x] Retry with exponential backoff
- [x] Panic recovery
- [x] Backpressure control
- [x] Infinite loop protection
- [x] Circuit breaker (V5)
- [x] Visibility timeout reaper (V5)
- [x] Retry with jitter (V5)
- [x] State transition validation (V5)
- [x] Latency-aware load balancing (V5)
- [x] Context relevance scoring (V5)
- [ ] Replace mock LLM with real API
- [ ] Replace mock MCP tools with real network calls
- [ ] Deploy workers as separate processes
- [ ] Use Redis/Kafka for shared queue
- [ ] Add Prometheus metrics
- [ ] Add distributed tracing (OpenTelemetry)

See [Deployment Guide](docs/DEPLOYMENT.md) for detailed instructions.
