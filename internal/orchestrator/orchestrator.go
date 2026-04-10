// Package orchestrator implements the core Orchestrator that coordinates
// all components of the AI orchestration system.
//
// V3: The Orchestrator now supports distributed execution via worker nodes.
// It can run in local mode (V2 compatible) or distributed mode (V3).
//
// The Orchestrator is the entry point for user requests. It:
// 1. Receives a user goal
// 2. Delegates to Controller for adaptive execution
// 3. Manages context and events throughout the lifecycle
// 4. Provides execution trace for observability
//
// Design decisions:
// - The Orchestrator owns all dependencies and injects them.
// - No global state — everything is passed via the Orchestrator struct.
// - Context lifecycle is managed at the orchestrator level.
// - Controller handles the adaptive loop; Orchestrator handles wiring.
// - Distributed mode is opt-in via EnableDistributedMode().
package orchestrator

import (
	"context"
	"log/slog"

	"ai-orchestrator/internal/agents"
	contextmanager "ai-orchestrator/internal/context"
	"ai-orchestrator/internal/controller"
	"ai-orchestrator/internal/events"
	"ai-orchestrator/internal/evaluator"
	"ai-orchestrator/internal/execution"
	"ai-orchestrator/internal/executor"
	"ai-orchestrator/internal/mcp"
	"ai-orchestrator/internal/planner"
	"ai-orchestrator/internal/queue"
	"ai-orchestrator/internal/registry"
	"ai-orchestrator/internal/rpc"
	"ai-orchestrator/internal/state"
	toolsgateway "ai-orchestrator/internal/tools"
	"ai-orchestrator/internal/types"
)

// Orchestrator coordinates planning, execution, and tool access.
// V3: Supports both local and distributed execution modes.
type Orchestrator struct {
	logger     *slog.Logger
	eventBus   *events.EventBus
	planGen    *planner.Planner
	engine     *execution.Engine
	ctrl       *controller.Controller
	ctxMgr     *contextmanager.ContextManager
	toolGW     *toolsgateway.ToolGateway
	eval       evaluator.Evaluator
	config     types.ExecutionConfig

	// V3 distributed components
	distExecutor *executor.DistributedExecutor
	queue        *queue.MemoryQueue
	workerReg    *registry.MemoryRegistry
	rpcClient    *rpc.Client
	taskTracker  *state.TaskTracker
	distributed  bool
}

// NewOrchestrator creates a fully configured Orchestrator (V3).
func NewOrchestrator(logger *slog.Logger, config types.ExecutionConfig) *Orchestrator {
	// Initialize components
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

	// Configure ACLs for agents
	toolGW.SetACL("dev", []string{
		"file.read",
		"file.write",
		"shell.exec",
	})
	toolGW.SetACL("qa", []string{
		"test.run",
		"shell.exec",
		"file.read",
	})
	toolGW.SetACL("ops", []string{
		"deploy.service",
		"shell.exec",
		"file.read",
	})

	// Create Controller with all dependencies
	o.ctrl = controller.NewController(planGen, eval, engine, logger, eventBus, config)

	// Initialize V3 distributed components
	o.queue = queue.NewMemoryQueue(100)
	o.workerReg = registry.NewMemoryRegistry()
	o.rpcClient = rpc.NewClient(logger)
	o.taskTracker = state.NewTaskTracker()

	o.distExecutor = executor.NewDistributedExecutor(
		o.queue,
		o.workerReg,
		o.rpcClient,
		o.taskTracker,
		logger,
		config,
	)

	// Subscribe to events for logging
	o.subscribeToEvents()

	return o
}

// EnableDistributedMode switches the orchestrator to distributed execution.
// Workers must be registered via RegisterWorker before calling Execute.
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

	// Delegate to Controller for adaptive Plan→Execute→Evaluate→Replan loop
	results, err := o.ctrl.Run(ctx, goal)

	// Store results in context
	for _, result := range results {
		// Convert Output to map for context storage
		outputMap, _ := result.Output.(map[string]any)
		o.ctxMgr.AppendResult(result.TaskID, result.Success, outputMap, result.Error)
	}

	// Log execution trace
	trace := o.ctrl.GetTrace()
	o.logger.Info("Execution trace",
		"trace_summary", trace.Summary(),
	)

	// Log context summary
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

// GetTaskTracker returns the distributed task state tracker.
func (o *Orchestrator) GetTaskTracker() *state.TaskTracker {
	return o.taskTracker
}

// GetWorkerRegistry returns the worker registry.
func (o *Orchestrator) GetWorkerRegistry() *registry.MemoryRegistry {
	return o.workerReg
}
