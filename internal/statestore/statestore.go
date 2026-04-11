// Package statestore implements persistent task state storage.
//
// V5 extends V4 state store with transition validation.
// Two implementations are provided:
// - MemoryStore: For development and testing
// - PostgresStore: For production (requires pgx driver)
//
// V5 adds:
// - StateTransition validation
// - Valid state transitions enforcement
//
// Design decisions:
// - Interface-based for easy backend swapping
// - Thread-safe implementations
// - State transitions are atomic
package statestore

import (
	"errors"
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

// ValidTransitions defines which state transitions are allowed.
var ValidTransitions = map[State][]State{
	StatePending:  {StateRunning, StateFailed},
	StateRunning:  {StateDone, StateFailed, StateRequeued},
	StateDone:     {}, // Terminal state
	StateFailed:   {}, // Terminal state (can be manually retried to pending)
	StateRequeued: {StateRunning, StateFailed},
}

// IsValidTransition checks if a state transition is allowed.
func IsValidTransition(from, to State) bool {
	allowed, exists := ValidTransitions[from]
	if !exists {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// StateTransition represents a state change for audit purposes.
type StateTransition struct {
	TaskID    string    `json:"task_id"`
	From      State     `json:"from"`
	To        State     `json:"to"`
	Timestamp time.Time `json:"timestamp"`
	Reason    string    `json:"reason,omitempty"`
}

// ErrInvalidTransition indicates an invalid state transition.
var ErrInvalidTransition = errors.New("invalid state transition")

// TaskState represents the full state of a task.
type TaskState struct {
	TaskID         string    `json:"task_id"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	State          State     `json:"state"`
	WorkerID       string    `json:"worker_id"`
	Attempts       int       `json:"attempts"`
	LastError      string    `json:"last_error,omitempty"`
	Result         string    `json:"result,omitempty"` // JSON-serialized result
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
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
