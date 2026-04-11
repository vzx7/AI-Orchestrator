# AGENTS.md - AI Orchestrator Development Guide

This file provides guidance for agentic coding agents working in this repository.

## Project Overview

AI Orchestrator V4 is a production-ready, distributed multi-agent AI orchestration platform built in Go (1.26.1). It implements fault-tolerant distributed task execution with gRPC communication.

## Build Commands

### Standard Build
```bash
go build ./...                    # Build all packages
go run ./cmd/orchestrator/        # Run in local mode
go run ./cmd/orchestrator/ --distributed  # Run in distributed mode
go run ./cmd/worker --id=worker-1 --addr=localhost:50051  # Run worker
```

### PostgreSQL Build (optional)
```bash
go build -tags postgres ./...      # Build with PostgreSQL state persistence
```

### Dependencies
```bash
go mod download                   # Download dependencies
go mod tidy                       # Clean up go.mod/go.sum
```

## Test Commands

### Run All Tests
```bash
go test ./... -v                  # Run all tests verbose
go test ./...                     # Run all tests
```

### Run Single Test
```bash
go test ./cmd/orchestrator -run TestDAGExecution -v
go test ./... -run "^TestEvaluator$" -v
go test ./internal/queue -run "TestEnqueue" -v
```

### Test Coverage
```bash
go test ./... -cover              # Show coverage summary
go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out
```

## Code Style

### Formatting
- Use `gofmt` for automatic formatting (standard Go style)
- Use `goimports` for import organization (recommended)
- Run before committing: `gofmt -w .` or `goimports -w .`

### Import Organization
```go
import (
    "context"
    "errors"
    "fmt"
    "log/slog"
    "time"

    "ai_orchestrator/internal/types"
)
```
Order: stdlib → external packages → internal packages. Use `goimports` to auto-organize.

### Naming Conventions

| Element | Convention | Example |
|---------|-----------|---------|
| Packages | lowercase, short | `queue`, `retry`, `dlq` |
| Types | PascalCase | `TaskQueue`, `ExecutionConfig` |
| Interfaces | PascalCase, often with "er" suffix | `Store`, `TaskQueue` |
| Functions | PascalCase exported, camelCase unexported | `NewMemoryQueue`, `dequeue` |
| Variables | camelCase | `taskID`, `maxRetries` |
| Constants | PascalCase exported, camelCase unexported | `TaskStatusPending`, `backoffBase` |
| Acronyms | Same case as word start | `ID`, `URL` not `Id`, `Url` |

### File Naming
- One type per file when reasonable (`queue.go`, `store.go`)
- Test files: `*_test.go` suffix
- Internal packages: lowercase names

## Error Handling

### Patterns
```go
// Return errors for expected failures
if err != nil {
    return fmt.Errorf("operation failed: %w", err)
}

// Wrap with context
return nil, fmt.Errorf("enqueue timeout: %w", ctx.Err())

// Check for specific errors
if errors.Is(err, context.Canceled) {
    return nil
}

// Sentinel errors for known conditions
var ErrQueueClosed = errors.New("queue is closed")
```

### Don't
- Don't ignore errors with `_`
- Don't silently log and continue without context
- Don't use error strings for control flow

### Panic Recovery
Workers must recover from panics to prevent crashing:
```go
func (w *Worker) ExecuteTask(ctx context.Context, task types.Task) (result types.Result, err error) {
    defer func() {
        if r := recover(); r != nil {
            w.logger.Error("Panic recovered", "panic", r, "stack", string(debug.Stack()))
            err = fmt.Errorf("panic during task execution: %v", r)
        }
    }()
    // ...
}
```

## Type Definitions

### Struct Tags
Use JSON tags for serialization:
```go
type Task struct {
    ID             string         `json:"id"`
    IdempotencyKey string         `json:"idempotency_key,omitempty"`
    Status         TaskStatus     `json:"status"`
}
```

### Interfaces
Define interfaces where behavior needs abstraction:
```go
type TaskQueue interface {
    Enqueue(ctx context.Context, msg TaskMessage) error
    Dequeue(ctx context.Context) (TaskMessage, error)
    Ack(taskID string) error
    Nack(taskID string, retry bool) error
    Size() int
}
```

## Concurrency

### Mutex Patterns
```go
type MemoryStore struct {
    mu    sync.RWMutex  // Use RWMutex for read-heavy workloads
    cache map[string]Result
}

// Read operations
func (s *MemoryStore) Exists(key string) bool {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.cache[key] != ""
}

// Write operations
func (s *MemoryStore) Save(key string, result Result) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.cache[key] = result
}
```

### Channel Patterns
```go
// Use buffered channels for backpressure
pending := make(chan TaskMessage, capacity)

// Select for non-blocking operations
select {
case msg := <-ch:
    return msg, nil
case <-ctx.Done():
    return nil, ctx.Err()
}
```

## Documentation

### Package Documentation
Every package should have a doc comment:
```go
// Package queue implements reliable task queues with Ack/Nack semantics.
//
// V4 adds:
// - Reliable queue with Ack/Nack/Requeue
// - In-flight task tracking
package queue
```

### Function Documentation
Export functions should have doc comments:
```go
// DefaultExecutionConfig returns production-ready defaults.
func DefaultExecutionConfig() ExecutionConfig
```

## Protocol Buffers

Proto files are in `proto/worker.proto`. After editing:
```bash
protoc --go_out=. --go-grpc_out=. proto/worker.proto
```

## Project Structure

```
cmd/
  orchestrator/          # CLI entry point
  worker/                # Standalone worker process
internal/
  orchestrator/          # Core orchestrator
  controller/            # Adaptive control loop
  executor/              # Distributed task executor
  queue/                 # Reliable queue (Ack/Nack)
  dlq/                   # Dead Letter Queue
  idempotency/           # Idempotency store
  retry/                 # Retry policy
  statestore/            # Persistent state store
  registry/              # Worker registry
  rpc/                   # RPC layer
  worker/                # Hardened worker
  agents/                # Agent implementations
  tools/                 # Tool Gateway
  context/               # Context management
  events/                # Event bus
  types/                 # Core type definitions
  planner/               # Dynamic planning
  execution/             # DAG-aware execution
proto/
  worker.proto           # gRPC service definition
```

## Key Design Patterns

- **Ack/Nack Queue**: Tasks are acknowledged or negatively-acknowledged; no task loss
- **Idempotency**: Safe retries via idempotency keys prevent duplicate execution
- **Dead Letter Queue**: Failed tasks after max retries go to DLQ for inspection
- **Least-Loaded Balancing**: Workers selected by current active task count
- **Panic Recovery**: Workers survive task panics gracefully
