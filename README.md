# AI Orchestrator V4

A **production-ready**, distributed multi-agent AI orchestration platform built in Go. V4 evolves from single-node distributed execution to a reliable, fault-tolerant system with **no task loss**, **safe retries**, **idempotent execution**, and **persistent state**.

## Architecture Evolution

| Version | Key Capability |
|---------|---------------|
| V1 | Linear task execution + agents + tool gateway |
| V2 | Adaptive control loop (Plan→Execute→Evaluate→Replan) + DAG |
| V3 | Distributed execution with worker nodes + RPC + queue |
| **V4** | **Reliable + Persistent + Fault-Tolerant (Production-Ready)** |

## V4 Architecture

```
                ┌─────────────────────────┐
                │      Orchestrator        │
                │  ┌────────────────────┐  │
                │  │     Controller      │  │
                │  │  (Loop Protection)  │  │
                │  └─────────┬──────────┘  │
                │            │              │
                │  ┌─────────▼──────────┐  │
                │  │ DistributedExecutor │  │
                │  │ ┌────┐ ┌────────┐  │  │
                │  │ │Queue│ │ Idemp  │  │  │
                │  │ │A/N │ │ Store  │  │  │
                │  │ └────┘ └────────┘  │  │
                │  │ ┌────┐ ┌────────┐  │  │
                │  │ │ DLQ │ │ States │  │  │
                │  │ └────┘ └────────┘  │  │
                │  └─────────┬──────────┘  │
                └────────────┼─────────────┘
                             │
                   ┌─────────┼─────────┐
                   │         │         │
           ┌───────▼──┐ ┌───▼───┐ ┌───▼──────┐
           │ Worker-1  │ │Worker2│ │ Worker-N  │
           │ (Healthy) │ │(HB OK)│ │(Dead?)    │
           │ Active:2  │ │Act:1  │ │ Act:0     │
           └───────────┘ └───────┘ └──────────┘
```

## V4 Reliability Flow

```
Task Submit
    │
    ▼
Check Idempotency Store ──hit──▶ Return Cached Result
    │ miss
    ▼
Enqueue (with backpressure) ──full──▶ Block or Reject
    │
    ▼
Dequeue → Move to In-Flight
    │
    ▼
Select Worker (Least-Loaded + Healthy)
    │
    ▼
RPC Execute (with retry + backoff)
    │
    ├─ success → Ack → Save State "done" → Cache Idempotency
    │
    └─ failure → Nack
            ├─ retryable → Requeue → Retry
            └─ exhausted → Dead Letter Queue → State "failed"
```

## Project Structure

```
cmd/
  orchestrator/          # CLI entry point
  worker/                # Standalone worker process
internal/
  orchestrator/          # Core orchestrator (V4: full reliability wiring)
  controller/            # Adaptive control loop (V4: loop protection)
  executor/              # Distributed task executor (V4: DLQ + idempotency)
  queue/                 # Reliable queue with Ack/Nack + backpressure (V4)
  dlq/                   # Dead Letter Queue (V4: NEW)
  idempotency/           # Idempotency store (V4: NEW)
  retry/                 # Global retry policy (V4: NEW)
  statestore/            # Task state persistence (V4: NEW)
  registry/              # Worker registry + heartbeat + least-loaded (V4)
  rpc/                   # RPC layer with retry + error classification (V4)
  worker/                # Hardened worker (V4: panic recovery + timeout)
  evaluator/             # Heuristic-based task evaluation
  planner/               # Dynamic planning with DAG + Replan
  execution/             # DAG-aware execution engine
  agents/                # DevAgent, QAAgent, OpsAgent
  tools/                 # Tool Gateway (ACL enforcement)
  mcp/                   # MCP tool registry (mock)
  context/               # Short-term context manager
  events/                # Pub/sub event bus
  types/                 # Core typed models (IdempotencyKey added)
proto/
  worker.proto           # gRPC service definition
```

## V4 New Components

| Component | Package | Purpose |
|-----------|---------|---------|
| **Reliable Queue** | `internal/queue` | Ack/Nack semantics, in-flight tracking, backpressure control |
| **Dead Letter Queue** | `internal/dlq` | Captures exhausted tasks for post-mortem |
| **Idempotency Store** | `internal/idempotency` | Safe retries — same task executes once |
| **Retry Policy** | `internal/retry` | Global exponential backoff with error classification |
| **State Store** | `internal/statestore` | Persistent task states (memory + PostgreSQL) |
| **Hardened Worker** | `internal/worker` | Panic recovery, per-task timeout, graceful shutdown |

## V4 Enhancements to Existing Components

| Component | V4 Enhancement |
|-----------|---------------|
| **Registry** | Heartbeat tracking, health checking, least-loaded balancing |
| **RPC Client** | Built-in retry with exponential backoff, fatal vs transient error classification |
| **Controller** | Infinite loop protection (max iterations guard) |
| **Executor** | DLQ integration, idempotency checks, state persistence |
| **Types** | `IdempotencyKey` on Task, V4 config fields |
| **Orchestrator** | Wires all V4 reliability components together |

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| Ack/Nack queue semantics | No task loss — every task is accounted for |
| Idempotency before execution | Safe retries without duplicate side effects |
| DLQ for exhausted tasks | Failed tasks captured, never silently dropped |
| Least-loaded worker selection | Fair distribution based on actual load, not just round-robin |
| Heartbeat-based health tracking | Dead workers detected and excluded automatically |
| Panic recovery in workers | A buggy task never crashes the worker process |
| Build-tagged PostgreSQL code | No external deps required for stdlib-only builds |
| Backpressure: block or reject | Configurable based on system requirements |

## Features

### V4 Production Reliability
- **No Task Loss**: Ack/Nack queue with in-flight tracking
- **Safe Retries**: Idempotency guarantees — same task executes once
- **Dead Letter Queue**: Failed tasks captured for investigation
- **Persistent State**: Task lifecycle tracked in memory or PostgreSQL
- **Fault Tolerance**: Worker death detection via heartbeats
- **Panic Recovery**: Workers survive task panics gracefully
- **Backpressure Control**: Configurable block or reject when queue full
- **Infinite Loop Protection**: Max iterations guard on control loop
- **Error Classification**: Transient errors retried, fatal errors fail fast

### Running the Demo

```bash
# Local mode
go run ./cmd/orchestrator/

# Distributed mode (with simulated worker failure + retry)
go run ./cmd/orchestrator/ --distributed
```

The distributed demo demonstrates:
- **Transient worker failure** (worker-2 fails first attempt)
- **Automatic retry** (task re-dispatched to worker-1)
- **Least-loaded balancing** (tasks spread across healthy workers)
- **State tracking** (all tasks tracked as "done")

### Standalone Worker

```bash
# Terminal 1: Start worker
go run ./cmd/worker --id=worker-1 --addr=localhost:50051

# Terminal 2: Start orchestrator
go run ./cmd/orchestrator/ --distributed
```

### Running Tests

```bash
go test ./... -v
```

## Configuration

Production defaults in `types.DefaultExecutionConfig()`:

```go
ExecutionConfig{
    DefaultTimeout:     30 * time.Second,
    MaxRetries:         3,
    RetryBackoffBase:   1 * time.Second,
    MaxParallelTasks:   4,
    MaxReplans:         3,
    QueueCapacity:      100,        // V4
    RPCCallRetries:     3,          // V4
    RPCBackoff:         500ms,      // V4
    TaskTimeout:        60s,        // V4
    HeartbeatTimeout:   30s,        // V4
}
```

## PostgreSQL Setup

For persistent state, build with the `postgres` tag:

```bash
go build -tags postgres ./...
```

Required table:
```sql
CREATE TABLE task_states (
    task_id         TEXT PRIMARY KEY,
    idempotency_key TEXT,
    state           TEXT NOT NULL,
    worker_id       TEXT,
    attempts        INT DEFAULT 0,
    last_error      TEXT,
    result          TEXT,
    created_at      TIMESTAMP NOT NULL,
    updated_at      TIMESTAMP NOT NULL
);
```

## Production Deployment Checklist

1. [x] Reliable queue (Ack/Nack)
2. [x] Idempotent execution
3. [x] Dead letter queue
4. [x] Task state persistence
5. [x] Worker health tracking
6. [x] Load balancing (least-loaded)
7. [x] Retry with exponential backoff
8. [x] Panic recovery
9. [x] Backpressure control
10. [x] Infinite loop protection
11. [ ] Replace mock LLM with real API
12. [ ] Replace mock MCP tools with real network calls
13. [ ] Deploy workers as separate processes
14. [ ] Use Redis/Kafka for shared queue
15. [ ] Add Prometheus metrics
16. [ ] Add distributed tracing (OpenTelemetry)
