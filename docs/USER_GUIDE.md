# AI Orchestrator V5 — User Guide

> This guide explains how to use AI Orchestrator V5 with the HTTP API.

---

## Table of Contents

1. [Quick Start](#1-quick-start)
2. [HTTP API](#2-http-api)
3. [CLI Alternatives](#3-cli-alternatives)
4. [Code Integration](#4-code-integration)
5. [Configuration](#5-configuration)

---

## 1. Quick Start

### Start HTTP Server

```bash
# Run with distributed workers (recommended)
go run ./cmd/server/main.go -distributed

# Run in local mode (no HTTP)
go run ./cmd/orchestrator/
```

### Test Connection

```bash
# Health check
curl http://localhost:8080/health
```

Expected response:
```json
{
  "status": "healthy",
  "version": "5.0.0",
  "workers": {
    "total": 2,
    "healthy": 2
  }
}
```

---

## 2. HTTP API

### Base URL

```
http://localhost:8080
```

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| POST | `/v1/tasks` | Create new task |
| GET | `/v1/tasks` | List all tasks |
| GET | `/v1/tasks/{id}` | Get task by ID |
| GET | `/v1/queue` | Queue status |
| GET | `/v1/dlq` | Dead Letter Queue |

### Create Task

```bash
curl -X POST http://localhost:8080/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "goal": "Fix failing test and deploy service"
  }'
```

Response:
```json
{
  "results": [
    {
      "task_id": "task-generic-xxx",
      "success": true,
      "output": {
        "agent": "dev",
        "executed_by": "worker-1"
      }
    }
  ],
  "status": "completed"
}
```

### With Options

```bash
curl -X POST http://localhost:8080/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "goal": "Fix failing test",
    "idempotency_key": "my-unique-key",
    "timeout": 300,
    "metadata": {
      "project": "backend"
    }
  }'
```

### List Tasks

```bash
curl http://localhost:8080/v1/tasks
```

### Get Task Status

```bash
curl http://localhost:8080/v1/tasks/task-xxx
```

### Queue Status

```bash
curl http://localhost:8080/v1/queue
```

Response:
```json
{
  "pending": 0,
  "in_flight": 0,
  "dlq_count": 0
}
```

### Dead Letter Queue

```bash
curl http://localhost:8080/v1/dlq
```

---

## 3. CLI Alternatives

### Local Mode (no network)

```bash
go run ./cmd/orchestrator/
```

Output goes directly to console.

### Distributed Demo

```bash
go run ./cmd/orchestrator/ --distributed
```

---

## 4. Code Integration

### Go

```go
import "ai_orchestrator/internal/orchestrator"
import "ai_orchestrator/internal/types"

func main() {
    logger := slog.New(...)
    config := types.DefaultExecutionConfig()
    
    o := orchestrator.NewOrchestrator(logger, config)
    results, err := o.Execute(ctx, "Fix failing test")
}
```

### Example Scripts

```python
import requests

# Create task
response = requests.post(
    "http://localhost:8080/v1/tasks",
    json={"goal": "Fix test"}
)
print(response.json())
```

---

## 5. Configuration

### Environment Variables

```bash
# Server port (default: 8080)
go run ./cmd/server/main.go -addr=:9090

# Distributed mode
go run ./cmd/server/main.go -distributed
```

### Options

```bash
go run ./cmd/server/main.go --help
```

Output:
```
  -addr string
        HTTP server address (default ":8080")
  -api-key string
        API key for authentication
  -distributed
        Enable distributed mode
```

---

## Next Steps

- [Deployment Guide](DEPLOYMENT.md) — production deployment
- [Service Explained](SERVICE_EXPLAINED.md) — system internals

---

*Document prepared for AI Orchestrator V5 users*