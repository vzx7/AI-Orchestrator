// Package executor implements the distributed task executor.
//
// The DistributedExecutor:
// - Enqueues tasks to the queue
// - Selects workers via the registry
// - Dispatches tasks via RPC client
// - Tracks task state
// - Handles retries and timeouts
//
// Design decisions:
// - Decoupled from controller via TaskExecutor interface
// - State tracking enables observability and recovery
// - Retries are handled here, not in the controller
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"ai-orchestrator/internal/queue"
	"ai-orchestrator/internal/registry"
	"ai-orchestrator/internal/rpc"
	"ai-orchestrator/internal/state"
	"ai-orchestrator/internal/types"
)

// DistributedExecutor manages distributed task execution across workers.
type DistributedExecutor struct {
	queue     *queue.MemoryQueue
	registry  *registry.MemoryRegistry
	client    *rpc.Client
	tracker   *state.TaskTracker
	logger    *slog.Logger
	config    types.ExecutionConfig
}

// NewDistributedExecutor creates a new distributed executor.
func NewDistributedExecutor(
	q *queue.MemoryQueue,
	reg *registry.MemoryRegistry,
	client *rpc.Client,
	tracker *state.TaskTracker,
	logger *slog.Logger,
	config types.ExecutionConfig,
) *DistributedExecutor {
	return &DistributedExecutor{
		queue:    q,
		registry: reg,
		client:   client,
		tracker:  tracker,
		logger:   logger,
		config:   config,
	}
}

// ExecuteTask submits a task for distributed execution and waits for the result.
func (de *DistributedExecutor) ExecuteTask(ctx context.Context, task types.Task) (types.Result, error) {
	de.tracker.Register(task.ID)

	// Apply defaults
	if task.MaxRetries == 0 {
		task.MaxRetries = de.config.MaxRetries
	}
	if task.Timeout == 0 {
		task.Timeout = de.config.DefaultTimeout
	}

	return de.executeWithRetry(ctx, task)
}

// executeWithRetry handles task execution with retries across workers.
func (de *DistributedExecutor) executeWithRetry(ctx context.Context, task types.Task) (types.Result, error) {
	var lastResult types.Result
	var lastErr error

	for attempt := 0; attempt <= task.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := de.config.RetryBackoffBase * time.Duration(1<<uint(attempt-1))
			de.logger.Info("Retrying task on distributed executor",
				"task_id", task.ID,
				"attempt", attempt,
				"backoff", backoff,
			)

			de.tracker.MarkRequeued(task.ID)

			select {
			case <-ctx.Done():
				return types.Result{
					TaskID:  task.ID,
					Success: false,
					Error:   fmt.Sprintf("cancelled during retry backoff: %v", ctx.Err()),
				}, ctx.Err()
			case <-time.After(backoff):
			}
		}

		result, err := de.executeOnce(ctx, task)
		lastResult = result
		lastErr = err

		if err == nil && result.Success {
			de.tracker.UpdateDone(task.ID)
			return result, nil
		}

		de.logger.Warn("Distributed task attempt failed",
			"task_id", task.ID,
			"attempt", attempt+1,
			"error", lastErr,
		)
	}

	de.tracker.UpdateFailed(task.ID, fmt.Sprintf("failed after %d retries: %v", task.MaxRetries, lastErr))
	lastResult.Success = false
	lastResult.Error = fmt.Sprintf("failed after %d retries: %v", task.MaxRetries, lastErr)

	return lastResult, lastErr
}

// executeOnce executes a task once on a selected worker.
func (de *DistributedExecutor) executeOnce(ctx context.Context, task types.Task) (types.Result, error) {
	// Select a worker via registry
	worker, err := de.registry.Next()
	if err != nil {
		return types.Result{
			TaskID:  task.ID,
			Success: false,
			Error:   fmt.Sprintf("no workers available: %v", err),
		}, err
	}

	// Create timeout context
	taskCtx, cancel := context.WithTimeout(ctx, task.Timeout)
	defer cancel()

	// Update state
	de.tracker.UpdateRunning(task.ID, worker.ID)

	de.logger.Info("Dispatching task to worker",
		"task_id", task.ID,
		"worker_id", worker.ID,
		"worker_addr", worker.Address,
	)

	// Execute via RPC client
	resp, err := de.client.ExecuteTask(taskCtx, worker.ID, task)
	if err != nil {
		de.tracker.UpdateFailed(task.ID, err.Error())
		return types.Result{
			TaskID:  task.ID,
			Success: false,
			Error:   fmt.Sprintf("rpc error: %v", err),
		}, err
	}

	// Parse result
	var result types.Result
	if resp.Success && len(resp.Result) > 0 {
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			de.logger.Error("Failed to deserialize result",
				"task_id", task.ID,
				"error", err,
			)
		}
	}

	result.TaskID = task.ID
	result.Timestamp = time.Now()

	// Attach worker info to metadata
	if result.Metadata == nil {
		result.Metadata = make(map[string]any)
	}
	result.Metadata["worker_id"] = worker.ID
	result.Metadata["worker_addr"] = worker.Address

	if !resp.Success {
		return result, fmt.Errorf("worker execution failed: %s", resp.Error)
	}

	return result, nil
}

// GetTracker returns the task state tracker for observability.
func (de *DistributedExecutor) GetTracker() *state.TaskTracker {
	return de.tracker
}

// GetQueue returns the task queue.
func (de *DistributedExecutor) GetQueue() *queue.MemoryQueue {
	return de.queue
}
