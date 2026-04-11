// Package contextmanager implements short-term context management for the orchestrator.
//
// V5 adds:
// - MemoryItem with importance scoring
// - RetrieveRelevant for context-aware retrieval
// - Time-based and importance-based scoring
//
// The ContextManager stores step results, provides context retrieval,
// and offers basic summarization capabilities. It acts as the memory
// layer for the orchestration system, allowing agents and tasks to
// share information across execution steps.
package contextmanager

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

// MemoryItem represents a stored context item with scoring.
type MemoryItem struct {
	Key        string    `json:"key"`
	Value      any       `json:"value"`
	Score      float64   `json:"score"`
	Timestamp  time.Time `json:"timestamp"`
	Importance float64   `json:"importance"` // Manual importance 0-1
}

// ContextEntry represents a single piece of stored context.
type ContextEntry struct {
	TaskID    string    `json:"task_id"`
	Key       string    `json:"key"`
	Value     any       `json:"value"`
	Timestamp time.Time `json:"timestamp"`
}

// ContextManager manages short-term execution context.
//
// Design decisions:
// - In-memory store for low-latency access during a single orchestration run.
// - For persistence across restarts, inject a storage backend interface.
// - Thread-safe with RWMutex for concurrent read-heavy workloads.
// V5: Added scoring for context relevance.
type ContextManager struct {
	mu      sync.RWMutex
	entries []ContextEntry
	memory  []MemoryItem
}

const (
	scoreRecencyWeight    = 0.6
	scoreImportanceWeight = 0.4
	recencyDecayHours     = 24.0
)

// NewContextManager creates a new context manager.
func NewContextManager() *ContextManager {
	return &ContextManager{
		entries: make([]ContextEntry, 0),
		memory:  make([]MemoryItem, 0),
	}
}

// Append stores a context entry associated with a task.
func (cm *ContextManager) Append(taskID, key string, value any) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.entries = append(cm.entries, ContextEntry{
		TaskID:    taskID,
		Key:       key,
		Value:     value,
		Timestamp: time.Now(),
	})
}

// AppendResult stores a task result as context.
func (cm *ContextManager) AppendResult(taskID string, success bool, data map[string]any, errMsg string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.entries = append(cm.entries, ContextEntry{
		TaskID:    taskID,
		Key:       "result",
		Value:     map[string]any{"success": success, "data": data, "error": errMsg},
		Timestamp: time.Now(),
	})
}

// Get retrieves context entries by key.
// Returns all matching entries in chronological order.
func (cm *ContextManager) Get(key string) []ContextEntry {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var results []ContextEntry
	for _, entry := range cm.entries {
		if entry.Key == key {
			results = append(results, entry)
		}
	}
	return results
}

// GetByTaskID retrieves all context entries for a specific task.
func (cm *ContextManager) GetByTaskID(taskID string) []ContextEntry {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var results []ContextEntry
	for _, entry := range cm.entries {
		if entry.TaskID == taskID {
			results = append(results, entry)
		}
	}
	return results
}

// GetLatest returns the most recent value for a given key.
func (cm *ContextManager) GetLatest(key string) (ContextEntry, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for i := len(cm.entries) - 1; i >= 0; i-- {
		if cm.entries[i].Key == key {
			return cm.entries[i], nil
		}
	}
	return ContextEntry{}, fmt.Errorf("no context found for key: %s", key)
}

// GetAll returns all stored context entries.
func (cm *ContextManager) GetAll() []ContextEntry {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	results := make([]ContextEntry, len(cm.entries))
	copy(results, cm.entries)
	return results
}

// Summarize provides a basic stub for context summarization.
// In production, this would invoke an LLM to generate a concise summary.
func (cm *ContextManager) Summarize(maxLength int) string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if len(cm.entries) == 0 {
		return "No context available."
	}

	var parts []string
	count := 0
	for _, entry := range cm.entries {
		if count >= maxLength {
			break
		}
		parts = append(parts, fmt.Sprintf("[%s] %s: %v", entry.TaskID, entry.Key, entry.Value))
		count++
	}

	return strings.Join(parts, "\n")
}

// Clear removes all stored context.
func (cm *ContextManager) Clear() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.entries = nil
}

// Count returns the number of context entries.
func (cm *ContextManager) Count() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.entries)
}

// AddMemory stores a memory item with automatic scoring.
func (cm *ContextManager) AddMemory(key string, value any, importance float64) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	item := MemoryItem{
		Key:        key,
		Value:      value,
		Importance: importance,
		Timestamp:  time.Now(),
	}

	item.Score = cm.calculateScore(item)

	cm.memory = append(cm.memory, item)
}

// calculateScore computes relevance score based on recency and importance.
func (cm *ContextManager) calculateScore(item MemoryItem) float64 {
	hoursOld := time.Since(item.Timestamp).Hours()
	recencyScore := math.Max(0, 1.0-hoursOld/recencyDecayHours)
	importanceScore := item.Importance

	return recencyScore*scoreRecencyWeight + importanceScore*scoreImportanceWeight
}

// RetrieveRelevant returns top-N memory items by score.
func (cm *ContextManager) RetrieveRelevant(limit int) []MemoryItem {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if len(cm.memory) == 0 {
		return nil
	}

	// Sort by score descending
	sorted := make([]MemoryItem, len(cm.memory))
	copy(sorted, cm.memory)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Score > sorted[i].Score {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	if limit > len(sorted) {
		limit = len(sorted)
	}
	return sorted[:limit]
}

// GetMemory returns all memory items.
func (cm *ContextManager) GetMemory() []MemoryItem {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make([]MemoryItem, len(cm.memory))
	copy(result, cm.memory)
	return result
}
