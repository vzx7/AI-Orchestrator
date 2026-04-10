// Package registry implements worker registration and load-balanced selection.
//
// The WorkerRegistry tracks available worker nodes and provides
// strategies for distributing tasks across them.
package registry

import (
	"sync"
)

// WorkerInfo represents a registered worker node.
type WorkerInfo struct {
	ID       string `json:"id"`
	Address  string `json:"address"` // gRPC address (e.g., "localhost:50051")
	Capacity int    `json:"capacity"`
}

// WorkerRegistry defines the interface for worker management.
type WorkerRegistry interface {
	// Register adds a worker to the registry.
	Register(worker WorkerInfo)
	// Deregister removes a worker from the registry.
	Deregister(workerID string)
	// List returns all registered workers.
	List() []WorkerInfo
	// Next selects the next worker based on load balancing strategy.
	Next() (WorkerInfo, error)
}

// MemoryRegistry implements WorkerRegistry with in-memory storage.
type MemoryRegistry struct {
	mu      sync.RWMutex
	workers map[string]WorkerInfo
	order   []string        // maintains insertion order for round-robin
	index   int             // current round-robin index
}

// NewMemoryRegistry creates a new worker registry.
func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{
		workers: make(map[string]WorkerInfo),
		order:   make([]string, 0),
	}
}

// Register adds a worker to the registry.
func (r *MemoryRegistry) Register(worker WorkerInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, exists := r.workers[worker.ID]
	r.workers[worker.ID] = worker

	if !exists {
		r.order = append(r.order, worker.ID)
	}
}

// Deregister removes a worker from the registry.
func (r *MemoryRegistry) Deregister(workerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.workers, workerID)

	// Remove from order slice
	for i, id := range r.order {
		if id == workerID {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}

	// Reset index if out of bounds
	if r.index >= len(r.order) {
		r.index = 0
	}
}

// List returns all registered workers.
func (r *MemoryRegistry) List() []WorkerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]WorkerInfo, 0, len(r.workers))
	for _, id := range r.order {
		result = append(result, r.workers[id])
	}
	return result
}

// Next selects the next worker using round-robin strategy.
func (r *MemoryRegistry) Next() (WorkerInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.order) == 0 {
		return WorkerInfo{}, ErrNoWorkers
	}

	worker := r.workers[r.order[r.index]]
	r.index = (r.index + 1) % len(r.order)
	return worker, nil
}

// ErrNoWorkers is returned when no workers are registered.
var ErrNoWorkers = &NoWorkersError{}

// NoWorkersError indicates that no workers are available.
type NoWorkersError struct{}

func (e *NoWorkersError) Error() string {
	return "no workers registered"
}
