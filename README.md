# AI Orchestrator V3

A production-grade, **distributed** multi-agent AI orchestration platform built in Go. V3 evolves from single-node execution to a horizontally scalable distributed platform with worker nodes, task queues, RPC communication, and fault tolerance.

## Architecture Evolution

| Version | Architecture |
|---------|-------------|
| V1 | Linear task execution with agents + tool gateway |
| V2 | Adaptive control loop (Plan→Execute→Evaluate→Replan) + DAG scheduling |
| **V3** | **Distributed execution with worker nodes + task queue + RPC** |

## V3 Distributed Architecture

```
                ┌──────────────────┐
                │   Orchestrator    │
                │  (Controller +    │
                │   Distributed     │
                │    Executor)      │
                └────────┬─────────┘
                         │
                   Task Queue
                (MemoryQueue)
                         │
              Worker Registry
              (Round-Robin LB)
                         │
          ┌──────────────┼──────────────┐
          │              │              │
   ┌──────▼──────┐ ┌────▼─────┐ ┌──────▼──────┐
   │  Worker-1   │ │ Worker-2 │ │  Worker-N   │
   │  (Agents)   │ │ (Agents) │ │  (Agents)   │
   │  (Tools)    │ │ (Tools)  │ │  (Tools)    │
   └─────────────┘ └──────────┘ └─────────────┘
```

## Project Structure

```
cmd/
  orchestrator/          # CLI entry point (local or distributed mode)
  worker/                # Standalone worker process
internal/
  orchestrator/          # Core orchestrator (V3: supports distributed mode)
  controller/            # Adaptive control loop (V3: TaskExecutor interface)
  executor/              # V3: Distributed task executor (NEW)
  queue/                 # V3: Thread-safe task queue (NEW)
  registry/              # V3: Worker registry + load balancing (NEW)
  rpc/                   # V3: RPC service layer (NEW)
  worker/                # V3: Worker node implementation (NEW)
  state/                 # V3: Task state tracking (NEW)
  evaluator/             # V2: Heuristic-based task evaluation
  planner/               # V2: Dynamic planning with DAG + Replan
  execution/             # V2: DAG-aware execution engine
  agents/                # DevAgent, QAAgent, OpsAgent
  tools/                 # Tool Gateway (ACL enforcement)
  mcp/                   # MCP tool registry (mock)
  context/               # Short-term context manager
  events/                # Pub/sub event bus
  types/                 # Core typed models
proto/
  worker.proto           # gRPC service definition
```

## V3 New Components

| Component | Package | Purpose |
|-----------|---------|---------|
| **DistributedExecutor** | `internal/executor` | Enqueues tasks, selects workers, dispatches via RPC, tracks state |
| **MemoryQueue** | `internal/queue` | Bounded, blocking, thread-safe task queue (channel-based) |
| **MemoryRegistry** | `internal/registry` | Worker registration + round-robin load balancing |
| **RPC Client/Server** | `internal/rpc` | Task execution protocol (JSON-serialized, swappable for real gRPC) |
| **Worker** | `internal/worker` | Distributed execution node with registered agents |
| **TaskTracker** | `internal/state` | In-memory task lifecycle tracking |

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| `TaskExecutor` interface in Controller | Allows swapping local ↔ distributed without changing control loop |
| Bounded queue with blocking dequeue | Natural backpressure; no busy-waiting |
| Round-robin worker selection | Simple, fair distribution; swappable for least-loaded |
| RPC via JSON serialization | Transport-agnostic; real gRPC can be dropped in later |
| Direct call transport for demo | Self-contained demo; real network layer is a drop-in replacement |
| Worker stateless, orchestrator stateful | Workers can be added/removed without state migration |
| Task state tracking separate from execution | Enables observability without coupling |

## Features

### V1 (Retained)
- Typed Task/Plan/Result model
- MCP tool integration (mock: file.read, file.write, shell.exec, test.run, deploy.service)
- Tool Gateway with ACL enforcement
- Event bus (pub/sub)
- Context management with summarization
- Structured logging via `log/slog`

### V2 (Retained)
- Adaptive Control Loop: Plan → Execute → Evaluate → Replan
- DAG-Based Execution: Dependency-aware scheduling
- Intelligent Evaluation: Confidence scores, retryability detection
- Dynamic Replanning: Plan adjusts on failure
- Execution Tracing: Full observability
- Circular Dependency Detection

### V3 (New)
- **Distributed Execution**: Tasks dispatched to remote worker nodes
- **Task Queue**: Bounded, blocking, thread-safe channel-based queue
- **Worker Registry**: Registration + round-robin load balancing
- **RPC Service Layer**: Task execution protocol (JSON-serialized)
- **Worker Nodes**: Independent processes with registered agents
- **Task State Tracking**: Lifecycle tracking (pending → running → done/failed)
- **Fault Tolerance**: Retry on worker failure, timeout detection, requeue
- **Distributed Observability**: Worker ID in traces, per-task latency, state counts

## Running the Demo

### Local Mode (default)
All agents run in-process — no workers needed:

```bash
go run ./cmd/orchestrator/
```

### Distributed Mode (in-process demo)
Workers are simulated in-process for the demo:

```bash
go run ./cmd/orchestrator/ --distributed
```

Output shows:
- Tasks dispatched to `worker-1` and `worker-2` via round-robin
- RPC call logs with task serialization
- Worker IDs in execution trace
- Distributed state summary (task states, registered workers)

### Standalone Worker Process

```bash
# Terminal 1: Start worker
go run ./cmd/worker --id=worker-1 --addr=localhost:50051

# Terminal 2: Start orchestrator (would connect via gRPC in production)
go run ./cmd/orchestrator/ --distributed
```

## Running Tests

```bash
go test ./... -v
```

Tests cover:
- DAG execution ordering
- Evaluator heuristics (success, retryable, exhausted retries)
- Replan logic (task removal, recovery insertion)
- Circular dependency detection
- Full control loop execution

## Production Deployment

To deploy as a real distributed system:

1. **Replace demo RPC transport** with `google.golang.org/grpc`
2. **Compile protobuf**: `protoc --go_out=. --go-grpc_out=. proto/worker.proto`
3. **Deploy workers** as separate processes/containers on different nodes
4. **Use shared message queue** (Redis, RabbitMQ, Kafka) instead of `MemoryQueue`
5. **Add service discovery** (Consul, etcd) instead of in-memory registry
6. **Add health checks** via `rpc.HealthCheck` endpoint
7. **Monitor** with Prometheus metrics + Grafana dashboards

## Constraints

- No frameworks — stdlib only (gRPC import ready but not required for demo)
- No real LLM calls (mock implementation)
- No real MCP networking (mock tools)
- Clean architecture with decoupled modules
- V3 extends V2 — no components removed or rewritten from scratch
