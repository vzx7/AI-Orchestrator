// Package executor implements the distributed task executor with V5 reliability.
//
// V5 adds:
// - Idempotency checks before execution
// - Dead Letter Queue integration
// - Queue Ack/Nack semantics
// - Backpressure-aware enqueueing
// - Task state persistence
package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"ai_orchestrator/internal/dlq"
	"ai_orchestrator/internal/idempotency"
	"ai_orchestrator/internal/queue"
	"ai_orchestrator/internal/registry"
	"ai_orchestrator/internal/rpc"
	"ai_orchestrator/internal/statestore"
	"ai_orchestrator/internal/types"
)

var ErrTaskNotFound = errors.New("task not found")

type DistributedExecutor struct {
	queue      *queue.MemoryQueue
	registry   *registry.MemoryRegistry
	client     *rpc.Client
	tracker    *statestore.MemoryStore
	idempStore *idempotency.MemoryStore
	deadLetter *dlq.DeadLetterQueue
	logger     *slog.Logger
	config     types.ExecutionConfig
	workerWg   sync.WaitGroup
	stopCh     chan struct{}
	stopped    bool
	stopMu     sync.RWMutex
}

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
		stopCh:     make(chan struct{}),
	}
}

func (de *DistributedExecutor) ExecuteTask(ctx context.Context, task types.Task) (types.Result, error) {
	if task.IdempotencyKey != "" && de.idempStore.Exists(task.IdempotencyKey) {
		result, ok := de.idempStore.Get(task.IdempotencyKey)
		if ok {
			de.logger.Info("Idempotent cache hit, returning cached result",
				"task_id", task.ID,
				"idempotency_key", task.IdempotencyKey,
			)
			return result, nil
		}
		return types.Result{}, fmt.Errorf("%w: no cached result for key %s", ErrTaskNotFound, task.IdempotencyKey)
	}

	state := statestore.TaskState{
		TaskID:         task.ID,
		IdempotencyKey: task.IdempotencyKey,
		State:          statestore.StatePending,
		Attempts:       0,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	de.tracker.SaveTaskState(state)

	if task.MaxRetries == 0 {
		task.MaxRetries = de.config.MaxRetries
	}
	if task.Timeout == 0 {
		task.Timeout = de.config.DefaultTimeout
	}

	return de.executeWithQueueAndRetry(ctx, task)
}

func (de *DistributedExecutor) executeWithQueueAndRetry(ctx context.Context, task types.Task) (types.Result, error) {
	msg := queue.TaskMessage{
		TaskID:         task.ID,
		Agent:          task.AssignedAgent,
		Payload:        task,
		IdempotencyKey: task.IdempotencyKey,
	}

	if err := de.queue.Enqueue(ctx, msg); err != nil {
		de.logger.Error("Failed to enqueue task", "task_id", task.ID, "error", err)
		return types.Result{}, fmt.Errorf("enqueue failed: %w", err)
	}

	for attempt := 0; attempt <= task.MaxRetries; attempt++ {
		if attempt > 0 {
			task.RetryCount = attempt
			backoff := de.config.RetryBackoffBase * time.Duration(1<<uint(attempt-1))
			de.logger.Info("Retrying task on distributed executor",
				"task_id", task.ID,
				"attempt", attempt,
				"backoff", backoff,
			)

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
				de.queue.Nack(task.ID, false)
				return types.Result{
					TaskID:  task.ID,
					Success: false,
					Error:   fmt.Sprintf("cancelled during retry backoff: %v", ctx.Err()),
				}, ctx.Err()
			case <-time.After(backoff):
			}

			if err := de.queue.Enqueue(ctx, msg); err != nil {
				de.logger.Error("Failed to re-enqueue task", "task_id", task.ID, "error", err)
				return types.Result{}, fmt.Errorf("re-enqueue failed: %w", err)
			}
		}

		result, err := de.executeFromQueue(ctx, task)
		if err == nil && result.Success {
			if task.IdempotencyKey != "" {
				de.idempStore.Save(task.IdempotencyKey, result)
			}

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

			de.queue.Ack(task.ID)
			return result, nil
		}

		de.logger.Warn("Distributed task attempt failed",
			"task_id", task.ID,
			"attempt", attempt+1,
			"error", err,
		)

		state := statestore.TaskState{
			TaskID:         task.ID,
			IdempotencyKey: task.IdempotencyKey,
			State:          statestore.StateRunning,
			Attempts:       attempt + 1,
			LastError:      fmt.Sprintf("%v", err),
			UpdatedAt:      time.Now(),
		}
		de.tracker.SaveTaskState(state)

		if attempt >= task.MaxRetries {
			de.queue.Nack(task.ID, false)
			break
		}

		de.queue.Nack(task.ID, true)
	}

	de.logger.Error("Task exhausted all retries, sending to DLQ",
		"task_id", task.ID,
		"attempts", task.MaxRetries+1,
	)

	state := statestore.TaskState{
		TaskID:         task.ID,
		IdempotencyKey: task.IdempotencyKey,
		State:          statestore.StateFailed,
		Attempts:       task.MaxRetries + 1,
		LastError:      fmt.Sprintf("failed after %d retries", task.MaxRetries),
		UpdatedAt:      time.Now(),
	}
	de.tracker.SaveTaskState(state)

	de.deadLetter.Push(queue.TaskMessage{
		TaskID:  task.ID,
		Agent:   task.AssignedAgent,
		Payload: task,
		Attempt: task.MaxRetries + 1,
	}, fmt.Sprintf("exhausted %d retries", task.MaxRetries))

	return types.Result{
		TaskID:  task.ID,
		Success: false,
		Error:   fmt.Sprintf("failed after %d retries", task.MaxRetries),
	}, fmt.Errorf("exhausted %d retries", task.MaxRetries)
}

func (de *DistributedExecutor) executeFromQueue(ctx context.Context, task types.Task) (types.Result, error) {
	deadline := time.Now().Add(task.Timeout)
	dequeueCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	_, err := de.queue.Dequeue(dequeueCtx)
	if err != nil {
		return types.Result{}, fmt.Errorf("dequeue failed: %w", err)
	}

	return de.executeOnce(ctx, task)
}

func (de *DistributedExecutor) executeOnce(ctx context.Context, task types.Task) (types.Result, error) {
	worker, err := de.registry.Next()
	if err != nil {
		return types.Result{
			TaskID:  task.ID,
			Success: false,
			Error:   fmt.Sprintf("no workers available: %v", err),
		}, err
	}

	de.registry.UpdateActiveTasks(worker.ID, worker.ActiveTasks+1)

	taskCtx, cancel := context.WithTimeout(ctx, task.Timeout)
	defer cancel()

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

	resp, err := de.client.ExecuteTask(taskCtx, worker.ID, task)

	de.registry.UpdateActiveTasks(worker.ID, worker.ActiveTasks-1)

	if err != nil {
		de.logger.Error("RPC call failed",
			"task_id", task.ID,
			"worker_id", worker.ID,
			"error", err,
		)
		return types.Result{
			TaskID:  task.ID,
			Success: false,
			Error:   fmt.Sprintf("rpc error: %v", err),
		}, err
	}

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

func (de *DistributedExecutor) GetTracker() *statestore.MemoryStore {
	return de.tracker
}

func (de *DistributedExecutor) GetQueue() *queue.MemoryQueue {
	return de.queue
}

func (de *DistributedExecutor) GetDeadLetter() *dlq.DeadLetterQueue {
	return de.deadLetter
}

func (de *DistributedExecutor) GetIdempotencyStore() *idempotency.MemoryStore {
	return de.idempStore
}

func (de *DistributedExecutor) Stop() {
	de.stopMu.Lock()
	if de.stopped {
		de.stopMu.Unlock()
		return
	}
	de.stopped = true
	close(de.stopCh)
	de.stopMu.Unlock()

	de.logger.Info("DistributedExecutor stopping, draining queue...")
	de.queue.Close()
	de.workerWg.Wait()
	de.logger.Info("DistributedExecutor stopped")
}
