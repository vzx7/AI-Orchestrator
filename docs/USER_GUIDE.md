# AI Orchestrator V5 — User Guide

> **Note:** This document describes the current state of the implementation. AI Orchestrator V5 is a **working prototype** with some V5 features (Circuit Breaker, Visibility Reaper) present in the codebase but not yet wired into the main execution path. See [Implementation Status](#implementation-status) for details.

---

## Table of Contents

1. [Overview](#1-overview)
2. [Quick Start](#2-quick-start)
3. [Local Development](#3-local-development)
4. [Distributed Mode (Demo)](#4-distributed-mode-demo)
5. [Running Workers](#5-running-workers)
6. [Code Integration](#6-code-integration)
7. [Configuration](#7-configuration)
8. [API Reference](#8-api-reference)
9. [Implementation Status](#9-implementation-status)
10. [Troubleshooting](#10-troubleshooting)

---

## 1. Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                     LOCAL DEVELOPMENT                            │
│                                                                  │
│  Terminal 1:              Terminal 2:              Terminal 3:  │
│  ┌─────────────┐         ┌─────────────┐         ┌─────────────┐ │
│  │ Orchestrator│  gRPC   │   Worker 1  │         │   Worker 2  │ │
│  │  (main.go)  │◄───────▶│ (cmd/worker)│         │ (cmd/worker)│ │
│  └─────────────┘  demo   └─────────────┘         └─────────────┘ │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Current State

| Feature | Status | Notes |
|---------|--------|-------|
| Local mode | ✅ Working | All-in-one process |
| Distributed mode | ⚠️ Demo | In-process RPC simulation |
| HTTP REST API | ❌ Not implemented | No HTTP server |
| Real gRPC | ❌ Not implemented | Uses direct Go calls |
| Circuit Breaker | ⚠️ Code exists | Not wired into execution |
| Visibility Reaper | ⚠️ Code exists | Not started in main.go |
| PostgreSQL support | ⚠️ Build tag required | `go build -tags postgres` |

---

## 2. Quick Start

### Prerequisites

```bash
# Install Go 1.26.1+
go version

# Clone repository
git clone <repo>
cd AI-Orchestrator

# Download dependencies
go mod download
```

### Run Local Mode

```bash
# Everything runs in one process
go run ./cmd/orchestrator/

# Expected output:
# INFO: ===========================================
# INFO:    AI Orchestrator V5 — Local Mode
# INFO: ===========================================
# INFO: User goal: Fix failing test and deploy service
# INFO: === Execution Results ===
# INFO: Result 1: task_id=xxx, status=SUCCESS
# INFO: === V5 Demo Complete ===
```

### Run Distributed Demo

```bash
# Simulates multiple workers in-process
go run ./cmd/orchestrator/ --distributed
```

---

## 3. Local Development

### Project Structure

```
AI-Orchestrator/
├── cmd/
│   ├── orchestrator/          # Main entry point
│   │   └── main.go
│   └── worker/                # Worker node entry point
│       └── main.go
├── internal/
│   ├── orchestrator/          # Main coordinator
│   ├── controller/            # Control loop
│   ├── planner/              # Plan generation
│   ├── executor/             # Task execution
│   ├── execution/            # Execution engine (DAG)
│   ├── evaluator/            # Result evaluation
│   ├── agents/               # Agent implementations
│   ├── tools/                # Tool gateway (ACL)
│   ├── mcp/                  # MCP tool registry
│   ├── queue/                # Memory queue
│   ├── registry/             # Worker registry
│   ├── rpc/                  # RPC layer (demo)
│   ├── resilience/           # Circuit breaker (V5)
│   ├── maintenance/          # Reaper (V5)
│   ├── idempotency/          # Idempotency store
│   ├── dlq/                  # Dead letter queue
│   ├── statestore/            # State persistence
│   ├── context/              # Context manager
│   ├── events/               # Event bus
│   └── types/                # Type definitions
├── proto/
│   └── worker.proto          # gRPC definitions (not compiled)
└── docs/
    └── *.md
```

### Key Components

| Component | File | Purpose |
|-----------|------|---------|
| **Orchestrator** | `internal/orchestrator/orchestrator.go` | Main coordinator |
| **Controller** | `internal/controller/controller.go` | Plan→Execute→Evaluate loop |
| **Execution Engine** | `internal/execution/engine.go` | DAG-based parallel execution |
| **Queue** | `internal/queue/queue.go` | Ack/Nack queue |
| **Registry** | `internal/registry/registry.go` | Worker health tracking |

### Development Commands

```bash
# Run tests
go test ./... -v

# Run specific test
go test ./cmd/orchestrator -run TestDAGExecution -v

# Format code
gofmt -w .
goimports -w .

# Build
go build ./...
go build -tags postgres ./...
```

---

## 4. Distributed Mode (Demo)

### How It Works

The distributed mode simulates multiple workers in a single process:

```bash
go run ./cmd/orchestrator/ --distributed
```

**Demo features:**
- Worker-1: Reliable, always succeeds
- Worker-2: Simulates transient failure on first attempt, then succeeds
- Automatic retry mechanism
- Least-loaded load balancing simulation
- Task state tracking

### Output Example

```
INFO: Distributed workers initialized
INFO: Worker-1 executing task, task_id=xxx, agent=dev
INFO: Worker-2 simulated transient failure, task_id=xxx
INFO: Retrying RPC call, worker_id=worker-2, attempt=1
INFO: Worker-2 executing task (retry succeeded), task_id=xxx
```

---

## 5. Running Workers

### Start Worker 1

**Terminal 1:**
```bash
go run ./cmd/worker --id=worker-1 --addr=localhost:50051
```

### Start Worker 2

**Terminal 2:**
```bash
go run ./cmd/worker --id=worker-2 --addr=localhost:50052
```

### Worker Options

```bash
go run ./cmd/worker --help
```

Output:
```
Usage of worker:
  --id string      Worker ID (default: worker-1)
  --addr string    Listen address (default: localhost:50051)
```

### Worker Agents

Each worker runs three agents:

| Agent | Tools | Purpose |
|-------|-------|---------|
| **DevAgent** | file.read, file.write, shell.exec | Code changes |
| **QAAgent** | test.run, shell.exec, file.read | Testing |
| **OpsAgent** | deploy.service, shell.exec | Deployment |

---

## 6. Code Integration

### Submit Task from Go

```go
package main

import (
    "context"
    "log/slog"
    "os"
    "time"

    "ai_orchestrator/internal/orchestrator"
    "ai_orchestrator/internal/types"
)

func main() {
    logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))

    config := types.DefaultExecutionConfig()
    config.DefaultTimeout = 30 * time.Second
    config.MaxRetries = 3

    o := orchestrator.NewOrchestrator(logger, config)

    ctx := context.Background()
    results, err := o.Execute(ctx, "Fix failing test and deploy service")
    if err != nil {
        logger.Error("Execution failed", "error", err)
        return
    }

    for _, result := range results {
        logger.Info("Result",
            "task_id", result.TaskID,
            "success", result.Success,
            "duration_ms", result.Duration.Milliseconds(),
        )
    }
}
```

### Use Execution Engine Directly

```go
import (
    "ai_orchestrator/internal/execution"
    "ai_orchestrator/internal/agents"
    "ai_orchestrator/internal/types"
)

engine := execution.NewEngine(config, logger, eventBus)
engine.RegisterAgent(agents.NewDevAgent(toolGateway, logger))
engine.RegisterAgent(agents.NewQAAgent(toolGateway, logger))

plan := types.Plan{Goal: "Fix test", Nodes: [...]}
results, err := engine.ExecutePlanDAG(ctx, plan)
```

### Custom Agent

```go
package agents

import (
    "context"
    "ai_orchestrator/internal/types"
)

type CustomAgent struct{}

func (a *CustomAgent) Name() string {
    return "custom"
}

func (a *CustomAgent) Execute(ctx context.Context, task types.Task) (types.Result, error) {
    return types.Result{
        TaskID:  task.ID,
        Success: true,
        Output:  map[string]any{"message": "Done"},
    }, nil
}
```

Register in orchestrator:
```go
engine.RegisterAgent(&CustomAgent{})
```

---

## 7. Configuration

### ExecutionConfig

```go
config := types.DefaultExecutionConfig()

// Timeouts
config.DefaultTimeout = 30 * time.Second  // Default task timeout
config.TaskTimeout = 60 * time.Second       // Per-task timeout on workers
config.HeartbeatTimeout = 30 * time.Second // Worker health check

// Retry settings
config.MaxRetries = 3              // Max retry attempts
config.RetryBackoffBase = 1 * time.Second  // Initial backoff
config.RPCCallRetries = 3         // RPC call retries
config.RPCBackoff = 500 * time.Millisecond // RPC backoff

// Concurrency
config.MaxParallelTasks = 4        // Max concurrent tasks
config.MaxReplans = 3             // Max replan iterations

// Queue
config.QueueCapacity = 100        // Task queue size
```

### Environment Variables

```bash
# Log level: debug, info, warn, error
LOG_LEVEL=debug go run ./cmd/orchestrator/

# PostgreSQL (with postgres build tag)
POSTGRES_URL=postgres://user:pass@localhost:5432/orchestrator
```

---

## 8. API Reference

### Orchestrator Methods

```go
type Orchestrator struct {
    // Execute runs a goal and returns results
    Execute(ctx context.Context, goal string) ([]Result, error)

    // EnableDistributedMode switches to distributed executor
    EnableDistributedMode()

    // RegisterWorker adds a worker to the registry
    RegisterWorker(id, address string)

    // GetExecutionTrace returns the last execution trace
    GetExecutionTrace() ExecutionTrace

    // GetWorkerRegistry returns the worker registry
    GetWorkerRegistry() *registry.MemoryRegistry

    // GetDeadLetterQueue returns the DLQ
    GetDeadLetterQueue() *dlq.DeadLetterQueue
}
```

### Task Status

```go
const (
    TaskStatusPending   TaskStatus = "pending"
    TaskStatusRunning   TaskStatus = "running"
    TaskStatusCompleted TaskStatus = "completed"
    TaskStatusFailed    TaskStatus = "failed"
    TaskStatusCancelled TaskStatus = "cancelled"
    TaskStatusRetrying  TaskStatus = "retrying"
)
```

### Result Structure

```go
type Result struct {
    TaskID    string         // Task ID
    Success   bool           // Success/failure
    Output    any            // Result output
    Error     string         // Error message
    Metadata  map[string]any// Additional data
    Duration  time.Duration  // Execution time
    Timestamp time.Time      // When completed
}
```

---

## 9. Implementation Status

### ✅ Implemented

| Feature | Package | Status |
|---------|---------|--------|
| Agent interface | `internal/agents/` | ✅ Working |
| DevAgent, QAAgent, OpsAgent | `internal/agents/` | ✅ Working (mock) |
| Tool Gateway with ACL | `internal/tools/` | ✅ Working |
| MCP Tool Registry | `internal/mcp/` | ✅ Working (mock) |
| Planner with DAG | `internal/planner/` | ✅ Working |
| Controller (Plan→Execute→Evaluate) | `internal/controller/` | ✅ Working |
| Execution Engine (DAG, parallel) | `internal/execution/` | ✅ Working |
| Evaluator | `internal/evaluator/` | ✅ Working |
| Memory Queue (Ack/Nack) | `internal/queue/` | ✅ Working |
| Idempotency Store | `internal/idempotency/` | ✅ Working |
| Dead Letter Queue | `internal/dlq/` | ✅ Working |
| Worker Registry | `internal/registry/` | ✅ Working |
| Event Bus | `internal/events/` | ✅ Working |
| Context Manager | `internal/context/` | ✅ Working |
| Task State Store | `internal/statestore/` | ✅ Working |
| Memory-based Worker Registry | `internal/registry/` | ✅ Working |
| RPC Layer (demo) | `internal/rpc/` | ✅ Working (in-process) |

### ⚠️ Partial Implementation

| Feature | Package | Status |
|---------|---------|--------|
| Circuit Breaker | `internal/resilience/` | ⚠️ Code exists, not wired in |
| Visibility Reaper | `internal/maintenance/` | ⚠️ Code exists, not started |
| Retry with Jitter | `internal/retry/` | ⚠️ Code exists, uses basic retry |
| PostgreSQL State Store | `internal/statestore/` | ⚠️ Build tag required |
| Latency-aware LB | `internal/registry/` | ⚠️ Code exists, limited effect |

### ❌ Not Implemented

| Feature | Notes |
|---------|-------|
| HTTP REST API | No HTTP server in codebase |
| Real gRPC | Proto file not compiled |
| Prometheus metrics | Not added |
| OpenTelemetry tracing | Not added |
| Redis/Kafka queue | Not added |
| Real LLM integration | Mock planner only |
| Real MCP network calls | Mock tools only |

### V5 Features in Code

Despite being labeled "V5", some features need integration work:

```go
// Circuit breaker - exists but not used
cb := resilience.NewCircuitBreaker(resilience.DefaultConfig())
// Not called anywhere in execution path

// Visibility reaper - exists but not started
reaper := maintenance.NewVisibilityReaper(logger, q, config)
reaper.Start(ctx)  // Not called in cmd/orchestrator/main.go
```

---

## 10. Troubleshooting

### Build Errors

```bash
# Missing dependencies
go mod download

# Wrong Go version
go version  # Need 1.26.1+
```

### Test Failures

```bash
# Run with verbose output
go test ./... -v

# Run specific test
go test ./cmd/orchestrator -run TestDAGExecution -v
```

### Worker Not Connecting

The current implementation uses **in-process demo RPC**. Workers don't actually connect to the orchestrator in real networking. This is a demo limitation.

```bash
# For real distributed mode, use --distributed flag
go run ./cmd/orchestrator/ --distributed
```

### PostgreSQL Issues

```bash
# Build with postgres support
go build -tags postgres ./...

# Check PostgreSQL is running
docker ps | grep postgres

# Verify connection
docker exec -it postgres psql -U admin -d orchestrator -c "SELECT 1"
```

### Verbose Logging

```bash
# Debug mode
LOG_LEVEL=debug go run ./cmd/orchestrator/ --distributed
```

---

## Next Steps

- [Deployment Guide](DEPLOYMENT.md) — deployment instructions
- [Service Explained](SERVICE_EXPLAINED.md) — system internals
- [README.md](../README.md) — project overview

---

*Document prepared for AI Orchestrator V5 users*