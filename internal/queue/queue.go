// Package queue implements reliable task queues with Ack/Nack semantics.
//
// V4 adds:
// - Reliable queue with Ack/Nack/Requeue
// - In-flight task tracking
// - Dead letter queue integration
// - Backpressure control (block or reject)
package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"ai_orchestrator/internal/types"
)

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
	pending     chan TaskMessage
	inFlight    map[string]TaskMessage
	mu          sync.RWMutex
	closed      bool
	policy      BackpressurePolicy
	nackHandler func(msg TaskMessage) // called on Nack(retry=false) for DLQ
}

// NewMemoryQueue creates a reliable memory queue.
func NewMemoryQueue(capacity int, policy BackpressurePolicy) *MemoryQueue {
	return &MemoryQueue{
		pending:  make(chan TaskMessage, capacity),
		inFlight: make(map[string]TaskMessage),
		policy:   policy,
	}
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
