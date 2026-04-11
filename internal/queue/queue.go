// Package queue implements reliable task queues with Ack/Nack semantics.
//
// V5 adds:
// - Visibility timeout for stuck task recovery
// - Background reaper for automatic task reprocessing
package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"ai_orchestrator/internal/types"
)

type VisibilityConfig struct {
	Timeout       time.Duration
	CheckInterval time.Duration
}

func DefaultVisibilityConfig() VisibilityConfig {
	return VisibilityConfig{
		Timeout:       60 * time.Second,
		CheckInterval: 10 * time.Second,
	}
}

// BackpressurePolicy defines behavior when queue is full.
type BackpressurePolicy string

const (
	// BackpressureBlock blocks until space is available or context expires.
	BackpressureBlock BackpressurePolicy = "block"
	// BackpressureReject immediately returns error when full.
	BackpressureReject BackpressurePolicy = "reject"
)

// TaskMessage wraps a task with execution metadata for queue transport.
type TaskMessage struct {
	TaskID         string     `json:"task_id"`
	Agent          string     `json:"agent"`
	Payload        types.Task `json:"payload"`
	Attempt        int        `json:"attempt"`
	IdempotencyKey string     `json:"idempotency_key,omitempty"`
}

// TaskQueue defines the interface for reliable task queuing.
type TaskQueue interface {
	// Enqueue adds a task message to the queue.
	Enqueue(ctx context.Context, msg TaskMessage) error
	// Dequeue removes and returns a task message from the queue.
	// Blocks until a task is available or context is cancelled.
	Dequeue(ctx context.Context) (TaskMessage, error)
	// Ack marks a task as successfully processed and removes it from in-flight.
	Ack(taskID string) error
	// Nack marks a task as failed. If retry=true, requeues it.
	Nack(taskID string, retry bool) error
	// Size returns the number of pending tasks.
	Size() int
	// InFlight returns the number of tasks currently being processed.
	InFlight() int
	// Close signals the queue to stop accepting new tasks.
	Close()
}

// MemoryQueue implements TaskQueue with in-memory storage and reliable semantics.
//
// Design:
// - pending: channel of tasks waiting to be processed
// - inFlight: map of tasks currently being worked on
// - On Ack: remove from inFlight
// - On Nack(retry=true): move back to pending
// - On Nack(retry=false): drop (send to DLQ externally)
type MemoryQueue struct {
	pending          chan TaskMessage
	inFlight         map[string]TaskMessage
	inFlightTimes    map[string]time.Time // V5: timestamps for visibility timeout
	mu               sync.RWMutex
	closed           bool
	policy           BackpressurePolicy
	nackHandler      func(msg TaskMessage)
	visibilityConfig VisibilityConfig
}

// inFlightEntry holds task with its start time for visibility timeout tracking.
type inFlightEntry struct {
	Message   TaskMessage
	StartTime time.Time
}

// NewMemoryQueue creates a reliable memory queue.
func NewMemoryQueue(capacity int, policy BackpressurePolicy) *MemoryQueue {
	return &MemoryQueue{
		pending:          make(chan TaskMessage, capacity),
		inFlight:         make(map[string]TaskMessage),
		inFlightTimes:    make(map[string]time.Time),
		policy:           policy,
		visibilityConfig: DefaultVisibilityConfig(),
	}
}

// SetVisibilityConfig sets the visibility timeout configuration.
func (q *MemoryQueue) SetVisibilityConfig(cfg VisibilityConfig) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.visibilityConfig = cfg
}

// SetNackHandler sets a callback for tasks that are nacked without retry.
// This is typically used to push tasks to a Dead Letter Queue.
func (q *MemoryQueue) SetNackHandler(fn func(msg TaskMessage)) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.nackHandler = fn
}

// Enqueue adds a task message to the queue with backpressure control.
func (q *MemoryQueue) Enqueue(ctx context.Context, msg TaskMessage) error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return errors.New("queue is closed")
	}
	policy := q.policy
	q.mu.Unlock()

	switch policy {
	case BackpressureReject:
		select {
		case q.pending <- msg:
			return nil
		default:
			return fmt.Errorf("queue is full (backpressure: reject)")
		}
	case BackpressureBlock:
		select {
		case q.pending <- msg:
			return nil
		case <-ctx.Done():
			return fmt.Errorf("enqueue timeout: %w", ctx.Err())
		}
	default:
		select {
		case q.pending <- msg:
			return nil
		default:
			return fmt.Errorf("queue is full (backpressure: reject)")
		}
	}
}

// Dequeue blocks until a task is available or context is cancelled.
// The task is moved to in-flight and must be Acked or Nacked.
func (q *MemoryQueue) Dequeue(ctx context.Context) (TaskMessage, error) {
	select {
	case msg, ok := <-q.pending:
		if !ok {
			return TaskMessage{}, errors.New("queue is closed")
		}
		q.mu.Lock()
		q.inFlight[msg.TaskID] = msg
		q.inFlightTimes[msg.TaskID] = time.Now()
		q.mu.Unlock()
		return msg, nil
	case <-ctx.Done():
		return TaskMessage{}, ctx.Err()
	}
}

// Ack marks a task as successfully processed.
func (q *MemoryQueue) Ack(taskID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.inFlight[taskID]; !exists {
		return fmt.Errorf("task not in-flight: %s", taskID)
	}

	delete(q.inFlight, taskID)
	delete(q.inFlightTimes, taskID)
	return nil
}

// Nack marks a task as failed. If retry=true, it's requeued.
func (q *MemoryQueue) Nack(taskID string, retry bool) error {
	q.mu.Lock()
	msg, exists := q.inFlight[taskID]
	if !exists {
		q.mu.Unlock()
		return fmt.Errorf("task not in-flight: %s", taskID)
	}

	delete(q.inFlight, taskID)
	delete(q.inFlightTimes, taskID)
	q.mu.Unlock()

	if retry {
		// Requeue: put back at the end of pending
		select {
		case q.pending <- msg:
			return nil
		default:
			// Queue full — try to push to nack handler
			if q.nackHandler != nil {
				q.nackHandler(msg)
			}
			return fmt.Errorf("requeue failed: queue full, task %s sent to DLQ", taskID)
		}
	}

	// No retry — send to DLQ if handler set
	if q.nackHandler != nil {
		q.nackHandler(msg)
	}

	return nil
}

// Size returns the number of pending tasks.
func (q *MemoryQueue) Size() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.pending)
}

// InFlight returns the number of tasks currently being processed.
func (q *MemoryQueue) InFlight() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.inFlight)
}

// Close signals the queue to stop accepting new tasks.
func (q *MemoryQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		q.closed = true
		close(q.pending)
	}
}

// IsClosed returns whether the queue is closed.
func (q *MemoryQueue) IsClosed() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.closed
}

// ReapTimedOutTasks moves tasks that exceeded visibility timeout back to pending.
// Returns the number of tasks reaped.
func (q *MemoryQueue) ReapTimedOutTasks() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	timeout := q.visibilityConfig.Timeout
	var reaped []string

	for taskID, startTime := range q.inFlightTimes {
		if time.Since(startTime) > timeout {
			reaped = append(reaped, taskID)
		}
	}

	for _, taskID := range reaped {
		msg := q.inFlight[taskID]
		delete(q.inFlight, taskID)
		delete(q.inFlightTimes, taskID)

		select {
		case q.pending <- msg:
		default:
			if q.nackHandler != nil {
				q.nackHandler(msg)
			}
		}
	}

	return len(reaped)
}

// GetInFlightTimes returns a copy of in-flight task timestamps for debugging.
func (q *MemoryQueue) GetInFlightTimes() map[string]time.Time {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make(map[string]time.Time)
	for k, v := range q.inFlightTimes {
		result[k] = v
	}
	return result
}
