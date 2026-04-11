# AI Orchestrator V4 - Deployment Guide

A comprehensive guide to deploying AI Orchestrator V4 in development and production environments.

## Table of Contents

1. [System Overview](#system-overview)
2. [Prerequisites](#prerequisites)
3. [Quick Start](#quick-start)
4. [Local Development](#local-development)
5. [Distributed Mode](#distributed-mode)
6. [MCP Server Integration](#mcp-server-integration)
7. [Configuration](#configuration)
8. [PostgreSQL Setup](#postgresql-setup)
9. [Production Deployment](#production-deployment)
10. [Troubleshooting](#troubleshooting)

---

## System Overview

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                        Orchestrator                          │
│  ┌─────────────┐  ┌──────────────┐  ┌──────────────────┐   │
│  │ Controller  │  │   Planner    │  │  DistributedExec  │   │
│  │ (Loop Prot) │  │  (DAG Gen)   │  │                  │   │
│  └─────────────┘  └──────────────┘  └──────────────────┘   │
│         │                │                   │              │
│  ┌──────▼────────────────▼──────────────────▼────────┐    │
│  │  ┌────────┐ ┌─────────┐ ┌──────┐ ┌──────────────┐ │    │
│  │  │ Queue  │ │Idempotency│ │ DLQ │ │State Store │ │    │
│  │  │(A/N)  │ │ Store   │ │     │ │(PostgreSQL) │ │    │
│  │  └────────┘ └─────────┘ └──────┘ └──────────────┘ │    │
│  └────────────────────────────────────────────────────┘    │
└────────────────────────────┬────────────────────────────────┘
                             │ gRPC
         ┌───────────────────┼───────────────────┐
         ▼                   ▼                   ▼
    ┌─────────┐         ┌─────────┐         ┌─────────┐
    │Worker-1 │         │Worker-2 │         │Worker-N │
    │(Agents) │         │(Agents) │         │(Agents) │
    └────┬────┘         └────┬────┘         └────┬────┘
         │                   │                   │
         ▼                   ▼                   ▼
    ┌─────────────────────────────────────────────────┐
    │              MCP Tool Gateway                      │
    │  (file I/O, shell, deploy, tests, etc.)         │
    └─────────────────────────────────────────────────┘
```

### Key Components

| Component | Package | Purpose |
|-----------|--------|---------|
| **Reliable Queue** | `internal/queue` | Ack/Nack semantics, in-flight tracking |
| **Dead Letter Queue** | `internal/dlq` | Captures exhausted tasks |
| **Idempotency Store** | `internal/idempotency` | Safe retries — same task = once |
| **Retry Policy** | `internal/retry` | Exponential backoff |
| **State Store** | `internal/statestore` | Persistent task states |
| **Hardened Worker** | `internal/worker` | Panic recovery, timeout |
| **Worker Registry** | `internal/registry` | Health tracking, least-loaded |

---

## Prerequisites

- **Go 1.26.1** or later
- **Docker** (for production)
- **PostgreSQL 16** (optional, for persistent state)
- **Git**

### Install Go

```bash
# Ubuntu/Debian
sudo apt update
sudo apt install golang-go

# Arch Linux / Manjaro
sudo pacman -S go

# macOS
brew install go

# Verify
go version
```

### Install Docker

```bash
# Ubuntu/Debian
sudo apt install docker.io docker-compose
sudo systemctl start docker
sudo usermod -aG docker $USER

# Arch Linux / Manjaro
sudo pacman -S docker docker-compose
sudo systemctl start docker
sudo usermod -aG docker $USER

# macOS
brew install --cask docker
```

---

## Quick Start

Get a running system in 5 minutes:

```bash
# 1. Clone the repository
git clone <repo>
cd AI-Orchestrator

# 2. Download dependencies
go mod download

# 3. Run in local mode
go run ./cmd/orchestrator/

# Expected output:
# INFO: AI Orchestrator V4 — Local Mode
# INFO: User goal: Fix failing test and deploy service
# INFO: === Execution Results ===
# INFO: Result 1: task_id=xxx, status=SUCCESS
# INFO: Result 2: task_id=xxx, status=SUCCESS
# INFO: V4 Demo Complete
```

---

## Local Development

### Running in Local Mode

All components run in a single process (no network):

```bash
go run ./cmd/orchestrator/
```

### Running in Distributed Mode (Demo)

Simulates multiple workers in one process:

```bash
go run ./cmd/orchestrator/ --distributed
```

This demo shows:
- **Transient failure** — Worker-2 fails first attempt
- **Automatic retry** — Task re-dispatched to Worker-1
- **Least-loaded balancing** — Tasks spread across workers
- **State tracking** — All tasks tracked as "done"

### Running Tests

```bash
# All tests
go test ./... -v

# Single test
go test ./cmd/orchestrator -run TestDAGExecution -v

# Specific test by name
go test ./... -run "^TestEvaluator$" -v

# With coverage
go test ./... -cover
```

### Code Formatting

```bash
# Format all code
gofmt -w .

# Organize imports
goimports -w .
```

---

## Distributed Mode

### Architecture

In distributed mode, the system consists of:

1. **Orchestrator** — Central coordinator (1 instance)
2. **Workers** — Task executors (1-N instances)
3. **Communication** — gRPC between components

### Starting Standalone Workers

Worker 1:
```bash
go run ./cmd/worker --id=worker-1 --addr=localhost:50051
```

Worker 2 (separate terminal):
```bash
go run ./cmd/worker --id=worker-2 --addr=localhost:50052
```

### Connecting Orchestrator to Workers

```bash
go run ./cmd/orchestrator/ --distributed
```

### Least-Loaded Load Balancing

Workers are selected based on:
1. **Health status** — Only healthy workers (heartbeat OK)
2. **Active tasks** — Worker with fewest active tasks selected
3. **Capacity** — Maximum concurrent tasks per worker

### Health Monitoring

Workers send heartbeats every 10 seconds. If no heartbeat for 30 seconds, worker is marked unhealthy.

---

## MCP Server Integration

### Current State: Mock Implementation

The current MCP implementation (`internal/mcp/registry.go`) is a **mock**. It simulates tool behavior without real network calls.

### Available Mock Tools

| Tool | Purpose |
|------|---------|
| `file.read` | Read file content |
| `file.write` | Write file content |
| `shell.exec` | Execute shell command |
| `test.run` | Run test suite |
| `deploy.service` | Deploy a service |

### Interface for Real MCP Server

To integrate a real MCP server, implement this interface:

```go
// internal/mcp/client.go
type MCPClient interface {
    Connect(ctx context.Context, serverAddr string) error
    CallTool(ctx context.Context, name string, args map[string]any) (map[string]any, error)
    ListTools(ctx context.Context) ([]Tool, error)
    Close() error
}
```

### HTTP/SSE Client Example

```go
// internal/mcp/http_client.go
type HTTPClient struct {
    baseURL    string
    authToken  string
    httpClient *http.Client
}

func (c *HTTPClient) CallTool(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
    req := ToolRequest{Name: name, Arguments: args}
    
    httpReq, _ := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/tools/call", marshal(req))
    httpReq.Header.Set("Authorization", "Bearer "+c.authToken)
    httpReq.Header.Set("Content-Type", "application/json")
    
    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("MCP call failed: %w", err)
    }
    defer resp.Body.Close()
    
    var result ToolResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("decode response failed: %w", err)
    }
    
    return result.Data, nil
}
```

### Tool Gateway ACL Configuration

```go
// Configure which tools each agent can use
toolGW.SetACL("dev", []string{
    "file.read", "file.write", "shell.exec", "git.*",
})
toolGW.SetACL("qa", []string{
    "test.run", "shell.exec", "file.read",
})
toolGW.SetACL("ops", []string{
    "deploy.service", "deploy.rollback", "shell.exec",
})
```

---

## Configuration

### ExecutionConfig Parameters

```go
type ExecutionConfig struct {
    DefaultTimeout     time.Duration // Default task timeout
    MaxRetries         int           // Max retry attempts per task
    RetryBackoffBase   time.Duration // Initial backoff delay
    MaxParallelTasks    int           // Max concurrent tasks
    MaxReplans         int           // Max replan iterations
    
    // V4-specific
    QueueCapacity       int           // Task queue size
    RPCCallRetries     int           // RPC call retries
    RPCBackoff         time.Duration // RPC backoff delay
    TaskTimeout        time.Duration // Per-task timeout on workers
    HeartbeatTimeout   time.Duration // Worker health check interval
}
```

### Production Defaults

```go
config := types.DefaultExecutionConfig()
// DefaultTimeout:     30s
// MaxRetries:        3
// RetryBackoffBase:  1s
// MaxParallelTasks:  4
// MaxReplans:        3
// QueueCapacity:     100
// RPCCallRetries:    3
// RPCBackoff:        500ms
// TaskTimeout:       60s
// HeartbeatTimeout:  30s
```

### Custom Configuration

```go
config := types.DefaultExecutionConfig()
config.DefaultTimeout = 10 * time.Second
config.MaxRetries = 5
config.QueueCapacity = 200
config.TaskTimeout = 120 * time.Second
```

---

## PostgreSQL Setup

### When to Use PostgreSQL

- **Multi-instance orchestration** — Shared state across orchestrator instances
- **Long-running tasks** — State survives restarts
- **Audit trail** — Historical task data
- **High availability** — Task recovery after failures

### Schema

```sql
CREATE TABLE task_states (
    task_id          TEXT PRIMARY KEY,
    idempotency_key  TEXT,
    state            TEXT NOT NULL,
    worker_id        TEXT,
    attempts         INT DEFAULT 0,
    last_error       TEXT,
    result           TEXT,
    created_at       TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_task_states_idempotency ON task_states(idempotency_key);
CREATE INDEX idx_task_states_state ON task_states(state);
```

### Docker PostgreSQL

```bash
docker run -d \
  --name orchestrator-postgres \
  -e POSTGRES_DB=orchestrator \
  -e POSTGRES_USER=admin \
  -e POSTGRES_PASSWORD=secret \
  -p 5432:5432 \
  postgres:16-alpine
```

### Building with PostgreSQL Support

```bash
go build -tags postgres ./...
```

---

## Production Deployment

### Docker Compose

```yaml
version: '3.8'

services:
  # ============================================
  # Orchestrator - Central coordinator
  # ============================================
  orchestrator:
    build: .
    ports:
      - "8080:8080"
    environment:
      - WORKERS=worker-1:50051,worker-2:50052
      - POSTGRES_URL=postgres://admin:secret@postgres:5432/orchestrator
      - LOG_LEVEL=info
    depends_on:
      - postgres
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3

  # ============================================
  # Worker 1 - Task executor
  # ============================================
  worker-1:
    build: .
    command: worker --id=worker-1 --addr=:50051
    ports:
      - "50051:50051"
    environment:
      - LOG_LEVEL=info
    restart: unless-stopped

  # ============================================
  # Worker 2 - Task executor
  # ============================================
  worker-2:
    build: .
    command: worker --id=worker-2 --addr=:50052
    ports:
      - "50052:50052"
    environment:
      - LOG_LEVEL=info
    restart: unless-stopped

  # ============================================
  # PostgreSQL - Persistent state
  # ============================================
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: orchestrator
      POSTGRES_USER: admin
      POSTGRES_PASSWORD: secret
    volumes:
      - postgres_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"
    restart: unless-stopped
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U admin -d orchestrator"]
      interval: 10s
      timeout: 5s
      retries: 5

volumes:
  postgres_data:
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `WORKERS` | Comma-separated worker addresses | - |
| `POSTGRES_URL` | PostgreSQL connection string | - |
| `LOG_LEVEL` | Logging level (debug/info/warn/error) | info |
| `QUEUE_CAPACITY` | Task queue size | 100 |
| `MAX_RETRIES` | Max retry attempts | 3 |

### Dockerfile

```dockerfile
FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -tags postgres -o orchestrator ./cmd/orchestrator
RUN CGO_ENABLED=0 GOOS=linux go build -tags postgres -o worker ./cmd/worker

FROM alpine:latest
RUN apk --no-cache add ca-certificates curl

WORKDIR /app
COPY --from=builder /app/orchestrator /app/
COPY --from=builder /app/worker /app/

ENTRYPOINT ["/app/orchestrator"]
CMD ["--distributed"]
```

### Health Checks

```bash
# Check orchestrator health
curl http://localhost:8080/health

# Check worker status
curl http://localhost:50051/health
```

---

## Troubleshooting

### Common Issues

#### 1. "No healthy workers registered"

**Problem:** Orchestrator can't find available workers.

**Solution:**
```bash
# Check workers are running
docker ps | grep worker

# Check worker logs
docker logs worker-1

# Verify worker registration
curl http://localhost:50051/health
```

#### 2. "Queue is full (backpressure: reject)"

**Problem:** Task queue at capacity.

**Solution:**
```bash
# Increase queue capacity
config.QueueCapacity = 500  # or via env: QUEUE_CAPACITY=500

# Or switch to block policy (waits for space)
```

#### 3. "Panic recovered during task execution"

**Problem:** Task code panicked but worker survived.

**Solution:**
```bash
# Check task logs for panic stack trace
docker logs worker-1

# Review DLQ for failed tasks
# Access via orchestrator API or logs
```

#### 4. PostgreSQL connection failed

**Problem:** Can't connect to database.

**Solution:**
```bash
# Verify PostgreSQL is running
docker ps | grep postgres

# Check connection string format
POSTGRES_URL=postgres://user:pass@host:5432/dbname

# Test connection
docker exec -it postgres psql -U admin -d orchestrator -c "SELECT 1"
```

### Log Levels

Set via environment or code:

```bash
# Via environment
LOG_LEVEL=debug go run ./cmd/orchestrator/

# Via code
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))
```

### Debug Mode

Enable verbose logging:

```bash
LOG_LEVEL=debug go run ./cmd/orchestrator/ --distributed
```

---

## Next Steps

1. **Replace mock MCP tools** with real network calls
2. **Configure PostgreSQL** for persistent state
3. **Add Prometheus metrics** for monitoring
4. **Set up distributed tracing** with OpenTelemetry
5. **Configure Redis/Kafka** for shared task queue (multi-orchestrator setup)

---

## Support

For issues and questions:
- Check logs (set `LOG_LEVEL=debug`)
- Review [Production Deployment Checklist](../README.md#production-deployment-checklist)
- Inspect Dead Letter Queue for failed tasks
