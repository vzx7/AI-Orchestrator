// Package state implements in-memory task state tracking for distributed execution.
//
// The TaskTracker maintains the lifecycle state of each task across the
// distributed system, enabling observability and recovery.
package state

import (
	"sync"
	"time"
)

// TaskStatus represents the state of a task in the distributed system.
type TaskStatus string

const (
	StatusPending  TaskStatus = "pending"
	StatusRunning  TaskStatus = "running"
	StatusDone     TaskStatus = "done"
	StatusFailed   TaskStatus = "failed"
	StatusRequeued TaskStatus = "requeued"
)

// TaskState tracks the execution state of a distributed task.
type TaskState struct {
	TaskID     string     `json:"task_id"`
	Status     TaskStatus `json:"status"`
	WorkerID   string     `json:"worker_id"`
	Attempts   int        `json:"attempts"`
	LastError  string     `json:"last_error,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// TaskTracker manages in-memory task state for the distributed system.
type TaskTracker struct {
	mu     sync.RWMutex
	states map[string]*TaskState
}

// NewTaskTracker creates a new task state tracker.
func NewTaskTracker() *TaskTracker {
	return &TaskTracker{
		states: make(map[string]*TaskState),
	}
}

// Register creates a new task state in pending status.
func (t *TaskTracker) Register(taskID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.states[taskID] = &TaskState{
		TaskID:   taskID,
		Status:   StatusPending,
		Attempts: 0,
	}
}

// UpdateRunning marks a task as running on a specific worker.
func (t *TaskTracker) UpdateRunning(taskID, workerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if state, exists := t.states[taskID]; exists {
		state.Status = StatusRunning
		state.WorkerID = workerID
		state.Attempts++
		state.StartedAt = time.Now()
	}
}

// UpdateDone marks a task as successfully completed.
func (t *TaskTracker) UpdateDone(taskID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if state, exists := t.states[taskID]; exists {
		state.Status = StatusDone
		state.CompletedAt = time.Now()
	}
}

// UpdateFailed marks a task as failed with an error message.
func (t *TaskTracker) UpdateFailed(taskID, errMsg string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if state, exists := t.states[taskID]; exists {
		state.Status = StatusFailed
		state.LastError = errMsg
		state.CompletedAt = time.Now()
	}
}

// MarkRequeued marks a task as requeued for retry.
func (t *TaskTracker) MarkRequeued(taskID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if state, exists := t.states[taskID]; exists {
		state.Status = StatusRequeued
		state.WorkerID = ""
	}
}

// Get returns the current state of a task.
func (t *TaskTracker) Get(taskID string) (*TaskState, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	state, exists := t.states[taskID]
	if !exists {
		return nil, false
	}

	// Return a copy to avoid race conditions
	copy := *state
	return &copy, true
}

// List returns all task states.
func (t *TaskTracker) List() map[string]*TaskState {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]*TaskState, len(t.states))
	for id, state := range t.states {
		copy := *state
		result[id] = &copy
	}
	return result
}

// CountByStatus returns the count of tasks in each status.
func (t *TaskTracker) CountByStatus() map[TaskStatus]int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	counts := make(map[TaskStatus]int)
	for _, state := range t.states {
		counts[state.Status]++
	}
	return counts
}
