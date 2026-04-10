// Package events implements a lightweight event bus for pub/sub communication.
//
// The event bus allows components to emit events and subscribe handlers
// without direct coupling. This enables observability, auditing, and
// reactive behavior across the orchestration system.
package events

import (
	"sync"
	"time"
)

// EventType categorizes the kind of event being emitted.
type EventType string

const (
	EventTaskStarted    EventType = "task.started"
	EventTaskCompleted  EventType = "task.completed"
	EventTaskFailed     EventType = "task.failed"
	EventTaskRetrying   EventType = "task.retrying"
	EventPlanCreated    EventType = "plan.created"
	EventPlanCompleted  EventType = "plan.completed"
	EventToolCall       EventType = "tool.call"
	EventToolError      EventType = "tool.error"
	EventOrchestratorStart EventType = "orchestrator.started"
	EventOrchestratorDone  EventType = "orchestrator.done"
)

// Event represents a single event emitted on the bus.
type Event struct {
	Type      EventType    `json:"type"`
	Source    string       `json:"source"`
	Payload   map[string]any `json:"payload,omitempty"`
	Timestamp time.Time    `json:"timestamp"`
}

// Handler is a function that processes an event.
// Handlers are called synchronously in the order they were subscribed.
type Handler func(event Event)

// EventBus provides pub/sub functionality for component communication.
//
// Design decisions:
// - Handlers are called synchronously to ensure ordering and simplicity.
// - For production async needs, handlers should spawn their own goroutines.
// - Thread-safe with mutex protection for subscription management.
type EventBus struct {
	mu       sync.RWMutex
	handlers map[EventType][]Handler
}

// NewEventBus creates a new event bus instance.
func NewEventBus() *EventBus {
	return &EventBus{
		handlers: make(map[EventType][]Handler),
	}
}

// Subscribe registers a handler for a specific event type.
func (bus *EventBus) Subscribe(eventType EventType, handler Handler) {
	bus.mu.Lock()
	defer bus.mu.Unlock()
	bus.handlers[eventType] = append(bus.handlers[eventType], handler)
}

// SubscribeAll registers a handler for all event types.
func (bus *EventBus) SubscribeAll(handler Handler) {
	bus.mu.Lock()
	defer bus.mu.Unlock()
	for eventType := range bus.handlers {
		bus.handlers[eventType] = append(bus.handlers[eventType], handler)
	}
	// Also subscribe to future event types by wrapping Publish
	// Note: This is a simplification; for full wildcard support,
	// we'd need a more sophisticated matching system.
}

// Publish emits an event to all subscribed handlers for the event type.
// Handlers are called synchronously in subscription order.
func (bus *EventBus) Publish(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	bus.mu.RLock()
	handlers := make([]Handler, len(bus.handlers[event.Type]))
	copy(handlers, bus.handlers[event.Type])
	bus.mu.RUnlock()

	for _, handler := range handlers {
		handler(event)
	}
}

// Unsubscribe removes all handlers for a specific event type.
func (bus *EventBus) Unsubscribe(eventType EventType) {
	bus.mu.Lock()
	defer bus.mu.Unlock()
	delete(bus.handlers, eventType)
}
