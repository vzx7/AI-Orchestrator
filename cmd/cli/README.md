# CLI for AI Orchestrator

Command-line interface for AI Orchestrator HTTP API.

## Quick Start

### 1. Build CLI

```bash
# From project root
go build -o orchestrator-cli ./cmd/cli

# Or install globally
go install ./cmd/cli
```

### 2. Configure

```bash
# Set environment (optional)
export ORCHESTRATOR_ADDR=http://localhost:8080
export ORCHESTRATOR_API_KEY=your-key
```

### 3. Run Server

```bash
# Start orchestrator
go run ./cmd/server/main.go -distributed

# Or with Docker
docker run -d -p 8080:8080 ghcr.io/vzx7/orchestrator:latest
```

### 4. Use CLI

```bash
# Health check
./orchestrator-cli health

# Submit task
./orchestrator-cli run "Analyze codebase and find todos"

# List tasks
./orchestrator-cli list

# Get task status
./orchestrator-cli get <task-id>

# Cancel task
./orchestrator-cli cancel <task-id>

# Queue status
./orchestrator-cli queue

# Dead Letter Queue
./orchestrator-cli dlq
```

## Commands

| Command | Description |
|---------|-------------|
| `health` | Check server health |
| `run <goal>` | Submit new task |
| `list` | List all tasks |
| `get <id>` | Get task status |
| `cancel <id>` | Cancel task |
| `queue` | Queue statistics |
| `dlq` | Dead Letter Queue |

## Options

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `http://localhost:8080` | Server address |
| `-api-key` | `` | API key for auth |

## Examples

### Submit task with custom goal

```bash
./orchestrator-cli run "Fix failing tests and deploy"
```

### Check queue status

```bash
./orchestrator-cli queue
# Output:
# Pending: 2
# In Flight: 1
# DLQ: 0
```

### Get task details

```bash
./orchestrator-cli get abc123
# Output:
# Task ID: abc123
# State: completed
# Attempts: 1
# Worker: worker-1
```

### View failed tasks

```bash
./orchestrator-cli dlq
# Output:
# DLQ Count: 2
#   task-456 [attempt 3]: connection timeout
#   task-789 [attempt 2]: worker unavailable
```

## Docker

```bash
# Run with Docker
docker run -d -p 8080:8080 ghcr.io/vzx7/orchestrator:latest

# Use CLI
docker exec -it <container> orchestrator-cli run "hello world"
```

## API Key

```bash
# Generate key (server side)
./orchestrator-cli -api-key=secret123 health
```