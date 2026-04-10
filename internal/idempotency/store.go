// Package idempotency implements idempotent task execution guarantees.
//
// Idempotency ensures that executing the same task multiple times
// produces the same result as executing it once. This is critical
// for safe retries in distributed systems.
//
// Flow:
// 1. Before executing task → check if IdempotencyKey exists
// 2. If exists → return cached result (skip execution)
// 3. After success → store result with key
package idempotency

import (
	"sync"
	"time"

	"ai-orchestrator/internal/types"
)

// Store defines the interface for idempotency tracking.
type Store interface {
	// Exists checks if a result is already cached for the given key.
	Exists(key string) bool
	// Save stores a result for the given idempotency key.
	Save(key string, result types.Result)
	// Get retrieves a cached result, if any.
	Get(key string) (types.Result, bool)
}

// MemoryStore implements Store using in-memory storage.
//
// For production, replace with Redis or PostgreSQL-backed store
// to survive restarts.
type MemoryStore struct {
	mu      sync.RWMutex
	cache   map[string]cachedResult
	maxSize int
}

type cachedResult struct {
	result    types.Result
	createdAt time.Time
}

// NewMemoryStore creates an in-memory idempotency store.
func NewMemoryStore(maxSize int) *MemoryStore {
	return &MemoryStore{
		cache:   make(map[string]cachedResult),
		maxSize: maxSize,
	}
}

// Exists checks if a result is already cached.
func (s *MemoryStore) Exists(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.cache[key]
	return ok
}

// Save stores a result with TTL-like behavior.
func (s *MemoryStore) Save(key string, result types.Result) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Evict oldest if at capacity
	if len(s.cache) >= s.maxSize {
		oldestKey := ""
		oldestTime := time.Now()
		for k, v := range s.cache {
			if v.createdAt.Before(oldestTime) {
				oldestTime = v.createdAt
				oldestKey = k
			}
		}
		if oldestKey != "" {
			delete(s.cache, oldestKey)
		}
	}

	s.cache[key] = cachedResult{
		result:    result,
		createdAt: time.Now(),
	}
}

// Get retrieves a cached result.
func (s *MemoryStore) Get(key string) (types.Result, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.cache[key]
	return entry.result, ok
}

// Count returns the number of cached entries.
func (s *MemoryStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.cache)
}

// Clear removes all entries.
func (s *MemoryStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = nil
	s.cache = make(map[string]cachedResult)
}
