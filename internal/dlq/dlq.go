// Package dlq implements a Dead Letter Queue for tasks that exceed retry limits.
//
// The DLQ captures tasks that have failed too many times, enabling:
// - Post-mortem analysis
// - Manual intervention
// - Automated recovery workflows
package dlq

import (
	"log/slog"
	"sync"
	"time"

	"ai_orchestrator/internal/queue"
)

// DeadLetterEntry wraps a failed task with failure metadata.
type DeadLetterEntry struct {
	Message    queue.TaskMessage `json:"message"`
	FailReason string            `json:"fail_reason"`
	FailedAt   time.Time         `json:"failed_at"`
}

// DeadLetterQueue stores tasks that have exhausted all retry attempts.
type DeadLetterQueue struct {
	mu      sync.RWMutex
	entries []DeadLetterEntry
	logger  *slog.Logger
	limit   int // max entries before oldest are dropped
}

// NewDeadLetterQueue creates a new DLQ with the given size limit.
func NewDeadLetterQueue(logger *slog.Logger, limit int) *DeadLetterQueue {
	return &DeadLetterQueue{
		entries: make([]DeadLetterEntry, 0),
		logger:  logger,
		limit:   limit,
	}
}

// Push adds a task to the dead letter queue.
func (d *DeadLetterQueue) Push(msg queue.TaskMessage, reason string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	entry := DeadLetterEntry{
		Message:    msg,
		FailReason: reason,
		FailedAt:   time.Now(),
	}

	d.entries = append(d.entries, entry)

	// Enforce limit by dropping oldest
	if len(d.entries) > d.limit {
		d.entries = d.entries[1:]
		d.logger.Warn("DLQ limit reached, dropped oldest entry", "limit", d.limit)
	}

	d.logger.Error("Task moved to dead letter queue",
		"task_id", msg.TaskID,
		"attempt", msg.Attempt,
		"reason", reason,
	)
}

// Peek returns all entries without removing them.
func (d *DeadLetterQueue) Peek() []DeadLetterEntry {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]DeadLetterEntry, len(d.entries))
	copy(result, d.entries)
	return result
}

// Pop removes and returns the first entry.
func (d *DeadLetterQueue) Pop() (DeadLetterEntry, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.entries) == 0 {
		return DeadLetterEntry{}, false
	}

	entry := d.entries[0]
	d.entries = d.entries[1:]
	return entry, true
}

// Count returns the number of entries in the DLQ.
func (d *DeadLetterQueue) Count() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.entries)
}

// Clear removes all entries.
func (d *DeadLetterQueue) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.entries = nil
}
