// Package statestore implements persistent task state storage.
//
// V4 replaces in-memory state tracking with a pluggable storage backend.
// Two implementations are provided:
// - MemoryStore: For development and testing
// - PostgresStore: For production (requires pgx driver)
//
// Design decisions:
// - Interface-based for easy backend swapping
// - Thread-safe implementations
// - State transitions are atomic
package statestore

import (
	"time"
)

// State represents the lifecycle state of a distributed task.
type State string

const (
	StatePending  State = "pending"
	StateRunning  State = "running"
	StateDone     State = "done"
	StateFailed   State = "failed"
	StateRequeued State = "requeued"
)

// TaskState represents the full state of a task.
type TaskState struct {
	TaskID      string    `json:"task_id"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	State       State     `json:"state"`
	WorkerID    string    `json:"worker_id"`
	Attempts    int       `json:"attempts"`
	LastError   string    `json:"last_error,omitempty"`
	Result      string    `json:"result,omitempty"` // JSON-serialized result
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Store defines the interface for task state persistence.
type Store interface {
	// SaveTaskState creates or updates a task's state.
	SaveTaskState(state TaskState) error
	// GetTaskState retrieves a task's current state.
	GetTaskState(taskID string) (TaskState, error)
	// DeleteTaskState removes a task's state.
	DeleteTaskState(taskID string) error
	// ListTaskStates returns all task states (for admin/debug).
	ListTaskStates() ([]TaskState, error)
}
