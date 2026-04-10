// Package registry implements worker registration, health tracking, and load balancing.
//
// V4 adds:
// - Worker heartbeat tracking
// - Automatic removal of dead workers
// - Least-loaded load balancing (replaces round-robin)
package registry

import (
	"fmt"
	"sync"
	"time"
)

// WorkerInfo represents a registered worker node with health tracking.
type WorkerInfo struct {
	ID            string    `json:"id"`
	Address       string    `json:"address"`
	Capacity      int       `json:"capacity"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	ActiveTasks   int       `json:"active_tasks"`
	Healthy       bool      `json:"healthy"`
}

// WorkerRegistry defines the interface for worker management.
type WorkerRegistry interface {
	// Register adds a worker to the registry.
	Register(worker WorkerInfo)
	// Deregister removes a worker from the registry.
	Deregister(workerID string)
	// Heartbeat updates a worker's last heartbeat time.
	Heartbeat(workerID string) error
	// List returns all registered workers.
	List() []WorkerInfo
	// Next selects the next worker based on load balancing strategy.
	Next() (WorkerInfo, error)
	// UpdateActiveTasks sets the active task count for a worker.
	UpdateActiveTasks(workerID string, count int)
}

// MemoryRegistry implements WorkerRegistry with least-loaded balancing.
type MemoryRegistry struct {
	mu        sync.RWMutex
	workers   map[string]*WorkerInfo
	order     []string // maintains insertion order
	heartbeat time.Duration // heartbeat timeout threshold
}

// NewMemoryRegistry creates a worker registry with health checking.
func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{
		workers:   make(map[string]*WorkerInfo),
		order:     make([]string, 0),
		heartbeat: 30 * time.Second, // default heartbeat timeout
	}
}

// SetHeartbeatTimeout sets the threshold for considering a worker dead.
func (r *MemoryRegistry) SetHeartbeatTimeout(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.heartbeat = d
}

// Register adds a worker to the registry.
func (r *MemoryRegistry) Register(worker WorkerInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	worker.LastHeartbeat = time.Now()
	worker.Healthy = true

	_, exists := r.workers[worker.ID]
	ptr := &worker
	r.workers[worker.ID] = ptr

	if !exists {
		r.order = append(r.order, worker.ID)
	}
}

// Deregister removes a worker from the registry.
func (r *MemoryRegistry) Deregister(workerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.workers, workerID)

	for i, id := range r.order {
		if id == workerID {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
}

// Heartbeat updates a worker's last heartbeat time.
func (r *MemoryRegistry) Heartbeat(workerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	w, exists := r.workers[workerID]
	if !exists {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	w.LastHeartbeat = time.Now()
	w.Healthy = true
	return nil
}

// UpdateActiveTasks sets the active task count for a worker.
func (r *MemoryRegistry) UpdateActiveTasks(workerID string, count int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if w, exists := r.workers[workerID]; exists {
		w.ActiveTasks = count
	}
}

// List returns all registered workers.
func (r *MemoryRegistry) List() []WorkerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]WorkerInfo, 0, len(r.workers))
	for _, id := range r.order {
		w := r.workers[id]
		result = append(result, *w)
	}
	return result
}

// Next selects the least-loaded healthy worker.
//
// Strategy:
// 1. Filter to healthy workers only
// 2. Remove workers with heartbeat timeout
// 3. Select worker with fewest active tasks (least-loaded)
func (r *MemoryRegistry) Next() (WorkerInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clean up dead workers
	r.cleanupDeadWorkers()

	// Find least-loaded healthy worker
	var best *WorkerInfo
	for _, id := range r.order {
		w := r.workers[id]
		if !w.Healthy {
			continue
		}

		// Check heartbeat timeout
		if time.Since(w.LastHeartbeat) > r.heartbeat {
			w.Healthy = false
			continue
		}

		if best == nil || w.ActiveTasks < best.ActiveTasks {
			best = w
		}
	}

	if best == nil {
		return WorkerInfo{}, ErrNoWorkers
	}

	// Increment active tasks count
	best.ActiveTasks++

	return *best, nil
}

// cleanupDeadWorkers marks workers that haven't heartbeat as unhealthy.
func (r *MemoryRegistry) cleanupDeadWorkers() {
	for _, id := range r.order {
		w := r.workers[id]
		if time.Since(w.LastHeartbeat) > r.heartbeat {
			w.Healthy = false
		}
	}
}

// HealthyWorkers returns only healthy workers.
func (r *MemoryRegistry) HealthyWorkers() []WorkerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]WorkerInfo, 0)
	for _, id := range r.order {
		w := r.workers[id]
		if w.Healthy && time.Since(w.LastHeartbeat) <= r.heartbeat {
			result = append(result, *w)
		}
	}
	return result
}

// ErrNoWorkers is returned when no healthy workers are available.
var ErrNoWorkers = &NoWorkersError{}

// NoWorkersError indicates that no workers are available.
type NoWorkersError struct{}

func (e *NoWorkersError) Error() string {
	return "no healthy workers registered"
}
