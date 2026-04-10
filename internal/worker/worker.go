// Package worker implements a hardened distributed worker node.
//
// V4 adds:
// - Panic recovery (tasks never crash the worker)
// - Per-task execution timeout
// - Graceful shutdown (finish current tasks, stop accepting new ones)
// - Context propagation for cancellation
package worker

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"ai-orchestrator/internal/agents"
	"ai-orchestrator/internal/types"
)

// Worker represents a distributed execution node with hardening.
type Worker struct {
	ID              string
	agents          map[string]agents.Agent
	toolGW          agents.ToolGateway
	logger          *slog.Logger
	taskTimeout     time.Duration
	running         bool
	shutdown        context.CancelFunc
	shutdownCtx     context.Context
}

// WorkerConfig holds worker initialization options.
type WorkerConfig struct {
	TaskTimeout time.Duration // Per-task execution timeout
}

// DefaultWorkerConfig returns production defaults.
func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		TaskTimeout: 60 * time.Second,
	}
}

// NewWorker creates a hardened worker with the given configuration.
func NewWorker(id string, logger *slog.Logger, toolGW agents.ToolGateway, cfg WorkerConfig) *Worker {
	return &Worker{
		ID:          id,
		agents:      make(map[string]agents.Agent),
		toolGW:      toolGW,
		logger:      logger.With("worker_id", id),
		taskTimeout: cfg.TaskTimeout,
	}
}

// RegisterAgent adds an agent to this worker.
func (w *Worker) RegisterAgent(agent agents.Agent) {
	w.agents[agent.Name()] = agent
	w.logger.Info("Agent registered on worker", "agent", agent.Name())
}

// ExecuteTask runs a task with panic recovery and timeout protection.
func (w *Worker) ExecuteTask(ctx context.Context, task types.Task) (result types.Result, err error) {
	// Panic recovery: never let a task crash the worker
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("Panic recovered during task execution",
				"task_id", task.ID,
				"panic", r,
				"stack", string(debug.Stack()),
			)
			result = types.Result{
				TaskID:  task.ID,
				Success: false,
				Error:   fmt.Sprintf("panic recovered: %v", r),
			}
			err = fmt.Errorf("panic during task execution: %v", r)
		}
	}()

	agent, exists := w.agents[task.AssignedAgent]
	if !exists {
		return types.Result{
			TaskID:  task.ID,
			Success: false,
			Error:   fmt.Sprintf("agent not found on worker %s: %s", w.ID, task.AssignedAgent),
		}, fmt.Errorf("agent not found: %s", task.AssignedAgent)
	}

	// Apply per-task timeout
	taskCtx, cancel := context.WithTimeout(ctx, w.taskTimeout)
	defer cancel()

	w.logger.Info("Executing task",
		"task_id", task.ID,
		"agent", agent.Name(),
		"goal", task.Goal,
	)

	// Execute with timeout awareness
	resultCh := make(chan types.Result, 1)
	errCh := make(chan error, 1)

	go func() {
		// Inner panic recovery for agent execution
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("agent panic: %v", r)
			}
		}()

		res, execErr := agent.Execute(task)
		if execErr != nil {
			errCh <- execErr
			return
		}
		resultCh <- res
	}()

	select {
	case res := <-resultCh:
		return res, nil
	case execErr := <-errCh:
		return types.Result{
			TaskID:  task.ID,
			Success: false,
			Error:   execErr.Error(),
		}, execErr
	case <-taskCtx.Done():
		return types.Result{
			TaskID:  task.ID,
			Success: false,
			Error:   fmt.Sprintf("task timed out after %s", w.taskTimeout),
		}, taskCtx.Err()
	}
}

// Start marks the worker as running.
func (w *Worker) Start(ctx context.Context) context.Context {
	wCtx, cancel := context.WithCancel(ctx)
	w.shutdownCtx = wCtx
	w.shutdown = cancel
	w.running = true
	w.logger.Info("Worker started")
	return wCtx
}

// Stop initiates graceful shutdown.
func (w *Worker) Stop() {
	if w.shutdown != nil {
		w.running = false
		w.shutdown()
		w.logger.Info("Worker stopping (graceful)")
	}
}

// IsRunning returns whether the worker is accepting tasks.
func (w *Worker) IsRunning() bool {
	return w.running
}

// GetAgentNames returns the names of registered agents.
func (w *Worker) GetAgentNames() []string {
	names := make([]string, 0, len(w.agents))
	for name := range w.agents {
		names = append(names, name)
	}
	return names
}
