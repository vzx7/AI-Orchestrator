// Package executor implements the distributed task executor with V4 reliability.
//
// V4 adds:
// - Idempotency checks before execution
// - Dead Letter Queue integration
// - Queue Ack/Nack semantics
// - Backpressure-aware enqueueing
// - Task state persistence
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"ai-orchestrator/internal/dlq"
	"ai-orchestrator/internal/idempotency"
	"ai-orchestrator/internal/queue"
	"ai-orchestrator/internal/registry"
	"ai-orchestrator/internal/rpc"
	"ai-orchestrator/internal/statestore"
	"ai-orchestrator/internal/types"
)

// DistributedExecutor manages distributed task execution across workers.
type DistributedExecutor struct {
	queue     *queue.MemoryQueue
	registry  *registry.MemoryRegistry
	client    *rpc.Client
	tracker   *statestore.MemoryStore
	idempStore *idempotency.MemoryStore
	deadLetter *dlq.DeadLetterQueue
	logger    *slog.Logger
	config    types.ExecutionConfig
}

// NewDistributedExecutor creates a new distributed executor with V4 reliability.
func NewDistributedExecutor(
	q *queue.MemoryQueue,
	reg *registry.MemoryRegistry,
	client *rpc.Client,
	tracker *statestore.MemoryStore,
	idempStore *idempotency.MemoryStore,
	deadLetter *dlq.DeadLetterQueue,
	logger *slog.Logger,
	config types.ExecutionConfig,
) *DistributedExecutor {
	return &DistributedExecutor{
		queue:      q,
		registry:   reg,
		client:     client,
		tracker:    tracker,
		idempStore: idempStore,
		deadLetter: deadLetter,
		logger:     logger,
		config:     config,
	}
}

// ExecuteTask submits a task for distributed execution with idempotency and retries.
func (de *DistributedExecutor) ExecuteTask(ctx context.Context, task types.Task) (types.Result, error) {
	// Check idempotency cache first
	if task.IdempotencyKey != "" && de.idempStore.Exists(task.IdempotencyKey) {
		result, ok := de.idempStore.Get(task.IdempotencyKey)
		if ok {
			de.logger.Info("Idempotent cache hit, returning cached result",
				"task_id", task.ID,
				"idempotency_key", task.IdempotencyKey,
			)
			return result, nil
		}
	}

	// Save initial state
	state := statestore.TaskState{
		TaskID:         task.ID,
		IdempotencyKey: task.IdempotencyKey,
		State:          statestore.StatePending,
		Attempts:       0,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	de.tracker.SaveTaskState(state)

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
			task.RetryCount = attempt
			backoff := de.config.RetryBackoffBase * time.Duration(1<<uint(attempt-1))
			de.logger.Info("Retrying task on distributed executor",
				"task_id", task.ID,
				"attempt", attempt,
				"backoff", backoff,
			)

			// Update state to requeued
			state := statestore.TaskState{
				TaskID:         task.ID,
				IdempotencyKey: task.IdempotencyKey,
				State:          statestore.StateRequeued,
				Attempts:       attempt,
				UpdatedAt:      time.Now(),
			}
			de.tracker.SaveTaskState(state)

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
			// Store idempotency result
			if task.IdempotencyKey != "" {
				de.idempStore.Save(task.IdempotencyKey, result)
			}

			// Update state to done
			resultJSON, _ := json.Marshal(result)
			state := statestore.TaskState{
				TaskID:         task.ID,
				IdempotencyKey: task.IdempotencyKey,
				State:          statestore.StateDone,
				Attempts:       attempt + 1,
				Result:         string(resultJSON),
				UpdatedAt:      time.Now(),
			}
			de.tracker.SaveTaskState(state)

			return result, nil
		}

		de.logger.Warn("Distributed task attempt failed",
			"task_id", task.ID,
			"attempt", attempt+1,
			"error", lastErr,
		)

		// Update state with error
		state := statestore.TaskState{
			TaskID:         task.ID,
			IdempotencyKey: task.IdempotencyKey,
			State:          statestore.StateRunning,
			Attempts:       attempt + 1,
			LastError:      fmt.Sprintf("%v", lastErr),
			UpdatedAt:      time.Now(),
		}
		de.tracker.SaveTaskState(state)
	}

	// All retries exhausted — send to DLQ
	de.logger.Error("Task exhausted all retries, sending to DLQ",
		"task_id", task.ID,
		"attempts", task.MaxRetries+1,
	)

	// Update state to failed
	state := statestore.TaskState{
		TaskID:         task.ID,
		IdempotencyKey: task.IdempotencyKey,
		State:          statestore.StateFailed,
		Attempts:       task.MaxRetries + 1,
		LastError:      fmt.Sprintf("failed after %d retries: %v", task.MaxRetries, lastErr),
		UpdatedAt:      time.Now(),
	}
	de.tracker.SaveTaskState(state)

	// Push to DLQ
	de.deadLetter.Push(queue.TaskMessage{
		TaskID:  task.ID,
		Agent:   task.AssignedAgent,
		Payload: task,
		Attempt: task.MaxRetries + 1,
	}, fmt.Sprintf("exhausted %d retries: %v", task.MaxRetries, lastErr))

	lastResult.Success = false
	lastResult.Error = fmt.Sprintf("failed after %d retries: %v", task.MaxRetries, lastErr)

	return lastResult, lastErr
}

// executeOnce executes a task once on a selected worker.
func (de *DistributedExecutor) executeOnce(ctx context.Context, task types.Task) (types.Result, error) {
	// Select least-loaded worker via registry
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

	// Update state to running
	state := statestore.TaskState{
		TaskID:         task.ID,
		IdempotencyKey: task.IdempotencyKey,
		State:          statestore.StateRunning,
		WorkerID:       worker.ID,
		Attempts:       task.RetryCount + 1,
		UpdatedAt:      time.Now(),
	}
	de.tracker.SaveTaskState(state)

	de.logger.Info("Dispatching task to worker",
		"task_id", task.ID,
		"worker_id", worker.ID,
		"worker_addr", worker.Address,
		"worker_active_tasks", worker.ActiveTasks,
	)

	// Execute via RPC client (has built-in retry)
	resp, err := de.client.ExecuteTask(taskCtx, worker.ID, task)
	if err != nil {
		de.logger.Error("RPC call failed",
			"task_id", task.ID,
			"worker_id", worker.ID,
			"error", err,
		)

		// Decrement active tasks on failure
		de.registry.UpdateActiveTasks(worker.ID, worker.ActiveTasks-1)

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
func (de *DistributedExecutor) GetTracker() *statestore.MemoryStore {
	return de.tracker
}

// GetQueue returns the task queue.
func (de *DistributedExecutor) GetQueue() *queue.MemoryQueue {
	return de.queue
}

// GetDeadLetter returns the dead letter queue.
func (de *DistributedExecutor) GetDeadLetter() *dlq.DeadLetterQueue {
	return de.deadLetter
}

// GetIdempotencyStore returns the idempotency store.
func (de *DistributedExecutor) GetIdempotencyStore() *idempotency.MemoryStore {
	return de.idempStore
}
