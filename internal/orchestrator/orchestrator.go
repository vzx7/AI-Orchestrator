// Package orchestrator implements the core Orchestrator that coordinates
// all components of the AI orchestration system.
//
// V4: Production-ready with reliability, persistence, and fault tolerance.
// - Reliable queue with Ack/Nack semantics
// - Idempotent execution guarantees
// - Dead letter queue for failed tasks
// - Task state persistence
// - Worker health tracking with heartbeats
// - Least-loaded load balancing
// - Infinite loop protection
package orchestrator

import (
	"context"
	"log/slog"

	"ai_orchestrator/internal/agents"
	contextmanager "ai_orchestrator/internal/context"
	"ai_orchestrator/internal/controller"
	"ai_orchestrator/internal/dlq"
	"ai_orchestrator/internal/evaluator"
	"ai_orchestrator/internal/events"
	"ai_orchestrator/internal/execution"
	"ai_orchestrator/internal/executor"
	"ai_orchestrator/internal/idempotency"
	"ai_orchestrator/internal/mcp"
	"ai_orchestrator/internal/planner"
	"ai_orchestrator/internal/queue"
	"ai_orchestrator/internal/registry"
	"ai_orchestrator/internal/rpc"
	"ai_orchestrator/internal/statestore"
	toolsgateway "ai_orchestrator/internal/tools"
	"ai_orchestrator/internal/types"
)

// Orchestrator coordinates planning, execution, and tool access.
// V4: Supports local and distributed modes with full reliability.
type Orchestrator struct {
	logger   *slog.Logger
	eventBus *events.EventBus
	planGen  *planner.Planner
	engine   *execution.Engine
	ctrl     *controller.Controller
	ctxMgr   *contextmanager.ContextManager
	toolGW   *toolsgateway.ToolGateway
	eval     evaluator.Evaluator
	config   types.ExecutionConfig

	// V4 distributed components
	distExecutor *executor.DistributedExecutor
	queue        *queue.MemoryQueue
	workerReg    *registry.MemoryRegistry
	rpcClient    *rpc.Client
	taskTracker  *statestore.MemoryStore
	idempStore   *idempotency.MemoryStore
	deadLetter   *dlq.DeadLetterQueue
	distributed  bool
}

// NewOrchestrator creates a production-ready Orchestrator (V4).
func NewOrchestrator(logger *slog.Logger, config types.ExecutionConfig) *Orchestrator {
	eventBus := events.NewEventBus()
	toolRegistry := mcp.NewToolRegistry()
	toolGW := toolsgateway.NewToolGateway(toolRegistry, logger)
	ctxMgr := contextmanager.NewContextManager()
	planGen := planner.NewPlanner(logger, eventBus)
	engine := execution.NewEngine(config, logger, eventBus)
	eval := evaluator.NewDefaultEvaluator()

	o := &Orchestrator{
		logger:   logger,
		eventBus: eventBus,
		planGen:  planGen,
		engine:   engine,
		ctxMgr:   ctxMgr,
		toolGW:   toolGW,
		eval:     eval,
		config:   config,
	}

	// Register default agents
	devAgent := agents.NewDevAgent(toolGW, logger)
	qaAgent := agents.NewQAAgent(toolGW, logger)
	opsAgent := agents.NewOpsAgent(toolGW, logger)

	engine.RegisterAgent(devAgent)
	engine.RegisterAgent(qaAgent)
	engine.RegisterAgent(opsAgent)

	// Configure ACLs
	toolGW.SetACL("dev", []string{"file.read", "file.write", "shell.exec"})
	toolGW.SetACL("qa", []string{"test.run", "shell.exec", "file.read"})
	toolGW.SetACL("ops", []string{"deploy.service", "shell.exec", "file.read"})

	// Create Controller
	o.ctrl = controller.NewController(planGen, eval, engine, logger, eventBus, config)

	// Initialize V4 components
	o.queue = queue.NewMemoryQueue(config.QueueCapacity, queue.BackpressureBlock)
	o.workerReg = registry.NewMemoryRegistry()
	o.rpcClient = rpc.NewClientDefault(logger)
	o.taskTracker = statestore.NewMemoryStore()
	o.idempStore = idempotency.NewMemoryStore(10000)    // 10k cached results
	o.deadLetter = dlq.NewDeadLetterQueue(logger, 1000) // 1k DLQ entries

	o.distExecutor = executor.NewDistributedExecutor(
		o.queue,
		o.workerReg,
		o.rpcClient,
		o.taskTracker,
		o.idempStore,
		o.deadLetter,
		logger,
		config,
	)

	// Wire DLQ to queue's nack handler
	o.queue.SetNackHandler(func(msg queue.TaskMessage) {
		o.deadLetter.Push(msg, "nack without retry")
	})

	// Subscribe to events
	o.subscribeToEvents()

	return o
}

// EnableDistributedMode switches to distributed execution.
func (o *Orchestrator) EnableDistributedMode() {
	o.ctrl.SetExecutor(o.distExecutor)
	o.distributed = true
	o.logger.Info("Orchestrator switched to distributed mode")
}

// RegisterWorker adds a worker to the distributed orchestrator.
func (o *Orchestrator) RegisterWorker(id, address string) {
	o.workerReg.Register(registry.WorkerInfo{
		ID:       id,
		Address:  address,
		Capacity: 10,
	})
	o.logger.Info("Worker registered", "worker_id", id, "address", address)
}

// RegisterWorkerServer connects a worker server for direct execution (demo mode).
func (o *Orchestrator) RegisterWorkerServer(workerID string, server *rpc.Server) {
	o.rpcClient.RegisterServer(workerID, server, server.GetWorkerID())
	o.logger.Info("Worker server connected", "worker_id", workerID)
}

// IsDistributed returns whether the orchestrator is in distributed mode.
func (o *Orchestrator) IsDistributed() bool {
	return o.distributed
}

// Execute processes a user goal end-to-end.
func (o *Orchestrator) Execute(ctx context.Context, goal string) ([]types.Result, error) {
	mode := "local"
	if o.distributed {
		mode = "distributed"
	}
	o.logger.Info("Orchestrator started", "goal", goal, "mode", mode)

	results, err := o.ctrl.Run(ctx, goal)

	// Store results in context
	for _, result := range results {
		outputMap, _ := result.Output.(map[string]any)
		o.ctxMgr.AppendResult(result.TaskID, result.Success, outputMap, result.Error)
	}

	// Log execution trace
	trace := o.ctrl.GetTrace()
	o.logger.Info("Execution trace",
		"trace_summary", trace.Summary(),
	)

	summary := o.ctxMgr.Summarize(20)
	o.logger.Info("Context summary", "summary", summary)

	o.logger.Info("Orchestrator completed",
		"goal", goal,
		"results", len(results),
		"replans", trace.ReplanCount,
	)

	return results, err
}

// subscribeToEvents registers logging handlers for all event types.
func (o *Orchestrator) subscribeToEvents() {
	o.eventBus.Subscribe(events.EventTaskStarted, func(e events.Event) {
		o.logger.Info("[EVENT] Task started",
			"task_id", e.Payload["task_id"],
			"agent", e.Payload["agent"],
		)
	})

	o.eventBus.Subscribe(events.EventTaskCompleted, func(e events.Event) {
		o.logger.Info("[EVENT] Task completed",
			"task_id", e.Payload["task_id"],
			"success", e.Payload["success"],
		)
	})

	o.eventBus.Subscribe(events.EventTaskFailed, func(e events.Event) {
		o.logger.Error("[EVENT] Task failed",
			"task_id", e.Payload["task_id"],
			"error", e.Payload["error"],
		)
	})

	o.eventBus.Subscribe(events.EventTaskRetrying, func(e events.Event) {
		o.logger.Warn("[EVENT] Task retrying",
			"task_id", e.Payload["task_id"],
			"attempt", e.Payload["attempt"],
		)
	})

	o.eventBus.Subscribe(events.EventToolCall, func(e events.Event) {
		o.logger.Info("[EVENT] Tool call",
			"tool", e.Payload["tool"],
			"agent", e.Payload["agent"],
		)
	})
}

// GetContextManager returns the context manager for inspection.
func (o *Orchestrator) GetContextManager() *contextmanager.ContextManager {
	return o.ctxMgr
}

// GetEventBus returns the event bus for external subscriptions.
func (o *Orchestrator) GetEventBus() *events.EventBus {
	return o.eventBus
}

// GetExecutionTrace returns the execution trace from the last run.
func (o *Orchestrator) GetExecutionTrace() types.ExecutionTrace {
	return o.ctrl.GetTrace()
}

// GetTaskTracker returns the task state tracker.
func (o *Orchestrator) GetTaskTracker() *statestore.MemoryStore {
	return o.taskTracker
}

// GetWorkerRegistry returns the worker registry.
func (o *Orchestrator) GetWorkerRegistry() *registry.MemoryRegistry {
	return o.workerReg
}

// GetDeadLetterQueue returns the dead letter queue.
func (o *Orchestrator) GetDeadLetterQueue() *dlq.DeadLetterQueue {
	return o.deadLetter
}

// GetIdempotencyStore returns the idempotency store.
func (o *Orchestrator) GetIdempotencyStore() *idempotency.MemoryStore {
	return o.idempStore
}

// Stop initiates graceful shutdown of all components.
func (o *Orchestrator) Stop() {
	o.logger.Info("Orchestrator stopping...")
	if o.distExecutor != nil {
		o.distExecutor.Stop()
	}
	if o.ctrl != nil {
		o.ctrl.Stop()
	}
	if o.engine != nil {
		o.engine.Stop()
	}
	o.logger.Info("Orchestrator stopped")
}
