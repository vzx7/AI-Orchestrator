package statestore

import (
	"fmt"
	"sync"
	"time"
)

// MemoryStore implements Store using in-memory storage.
type MemoryStore struct {
	mu     sync.RWMutex
	states map[string]TaskState
}

// NewMemoryStore creates an in-memory state store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		states: make(map[string]TaskState),
	}
}

// SaveTaskState creates or updates a task's state.
func (s *MemoryStore) SaveTaskState(ts TaskState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ts.UpdatedAt = time.Now()
	if ts.CreatedAt.IsZero() {
		ts.CreatedAt = ts.UpdatedAt
	}

	s.states[ts.TaskID] = ts
	return nil
}

// GetTaskState retrieves a task's current state.
func (s *MemoryStore) GetTaskState(taskID string) (TaskState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, exists := s.states[taskID]
	if !exists {
		return TaskState{}, fmt.Errorf("task state not found: %s", taskID)
	}
	return state, nil
}

// DeleteTaskState removes a task's state.
func (s *MemoryStore) DeleteTaskState(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.states[taskID]; !exists {
		return fmt.Errorf("task state not found: %s", taskID)
	}

	delete(s.states, taskID)
	return nil
}

// ListTaskStates returns all task states.
func (s *MemoryStore) ListTaskStates() ([]TaskState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	states := make([]TaskState, 0, len(s.states))
	for _, state := range s.states {
		states = append(states, state)
	}
	return states, nil
}

// Count returns the number of tracked task states.
func (s *MemoryStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.states)
}
