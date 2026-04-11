# AI Orchestrator V4

A **production-ready**, distributed multi-agent AI orchestration platform built in Go (1.26.1). V4 evolves from single-node distributed execution to a reliable, fault-tolerant system with **no task loss**, **safe retries**, **idempotent execution**, and **persistent state**.

## Key Features

- **No Task Loss**: Ack/Nack queue with in-flight tracking
- **Safe Retries**: Idempotency guarantees — same task executes once
- **Dead Letter Queue**: Failed tasks captured for investigation
- **Persistent State**: Task lifecycle tracked in memory or PostgreSQL
- **Fault Tolerance**: Worker death detection via heartbeats
- **Panic Recovery**: Workers survive task panics gracefully
- **Backpressure Control**: Configurable block or reject when queue full
- **Infinite Loop Protection**: Max iterations guard on control loop
- **Error Classification**: Transient errors retried, fatal errors fail fast

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
| **V4** | **Reliable + Persistent + Fault-Tolerant (Production-Ready)** |

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
- [ ] Replace mock LLM with real API
- [ ] Replace mock MCP tools with real network calls
- [ ] Deploy workers as separate processes
- [ ] Use Redis/Kafka for shared queue
- [ ] Add Prometheus metrics
- [ ] Add distributed tracing (OpenTelemetry)

See [Deployment Guide](docs/DEPLOYMENT.md) for detailed instructions.
