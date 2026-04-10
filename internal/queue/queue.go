// Package queue implements a thread-safe, blocking task queue for distributed execution.
//
// Design decisions:
// - Channel-based for natural blocking behavior (no busy-waiting)
// - Bounded capacity to prevent memory exhaustion under load
// - Graceful shutdown via context cancellation
package queue

import (
	"context"
	"fmt"
	"sync"

	"ai-orchestrator/internal/types"
)

// TaskMessage wraps a task with execution metadata for queue transport.
type TaskMessage struct {
	TaskID  string     `json:"task_id"`
	Agent   string     `json:"agent"`
	Payload types.Task `json:"payload"`
	Attempt int        `json:"attempt"`
}

// TaskQueue defines the interface for distributed task queuing.
type TaskQueue interface {
	// Enqueue adds a task message to the queue.
	Enqueue(msg TaskMessage) error
	// Dequeue removes and returns a task message from the queue.
	// Blocks until a task is available or context is cancelled.
	Dequeue(ctx context.Context) (TaskMessage, error)
	// Size returns the current number of pending tasks.
	Size() int
	// Close signals the queue to stop accepting new tasks.
	Close()
}

// MemoryQueue implements TaskQueue using a buffered channel.
type MemoryQueue struct {
	ch       chan TaskMessage
	closed   bool
	mu       sync.RWMutex
	capacity int
}

// NewMemoryQueue creates a new bounded, blocking memory queue.
func NewMemoryQueue(capacity int) *MemoryQueue {
	return &MemoryQueue{
		ch:       make(chan TaskMessage, capacity),
		capacity: capacity,
	}
}

// Enqueue adds a task message to the queue.
func (q *MemoryQueue) Enqueue(msg TaskMessage) error {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return fmt.Errorf("queue is closed")
	}
	q.mu.RUnlock()

	select {
	case q.ch <- msg:
		return nil
	default:
		return fmt.Errorf("queue is full (capacity: %d)", q.capacity)
	}
}

// Dequeue blocks until a task is available or context is cancelled.
func (q *MemoryQueue) Dequeue(ctx context.Context) (TaskMessage, error) {
	select {
	case msg, ok := <-q.ch:
		if !ok {
			return TaskMessage{}, fmt.Errorf("queue is closed")
		}
		return msg, nil
	case <-ctx.Done():
		return TaskMessage{}, ctx.Err()
	}
}

// Size returns the current number of pending tasks.
func (q *MemoryQueue) Size() int {
	return len(q.ch)
}

// Close signals the queue to stop accepting new tasks and unblocks waiting consumers.
func (q *MemoryQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		q.closed = true
		close(q.ch)
	}
}
