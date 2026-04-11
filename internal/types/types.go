// Package types defines the core data structures for the AI Orchestrator V2.
//
// V2 adds:
// - TaskNode for DAG-based execution
// - Enhanced Result with Metadata
// - Evaluation for feedback loop
// - ExecutionTrace for observability
//
// V5 adds:
// - Safety limits for workflows
// - Enhanced execution config
package types

import (
	"fmt"
	"time"
)

// Safety limits for workflow execution.
const (
	MaxRetriesPerTask   = 10
	MaxExecutionTime    = 10 * time.Minute
	MaxTasksPerWorkflow = 100
	MaxReplans          = 5
)

// TaskStatus represents the current state of a task in the execution lifecycle.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
	TaskStatusRetrying  TaskStatus = "retrying"
	TaskStatusBlocked   TaskStatus = "blocked" // New: waiting for dependencies
)

// Task represents a single unit of work to be executed by an agent.
type Task struct {
	ID             string         `json:"id"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"` // V5: idempotent execution
	Goal           string         `json:"goal"`
	Context        map[string]any `json:"context,omitempty"`
	Constraints    []string       `json:"constraints,omitempty"`
	AssignedAgent  string         `json:"assigned_agent"`
	Status         TaskStatus     `json:"status"`
	RetryCount     int            `json:"retry_count"`
	MaxRetries     int            `json:"max_retries"`
	Timeout        time.Duration  `json:"timeout"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// TaskNode wraps a Task with DAG dependency information.
//
// DependsOn contains Task IDs that must complete before this node can execute.
// An empty DependsOn slice means the task has no dependencies and can run immediately.
type TaskNode struct {
	Task      Task     `json:"task"`
	DependsOn []string `json:"depends_on"` // Task IDs this node depends on
}

// Plan represents a structured execution plan with DAG support.
//
// Nodes form a directed acyclic graph where edges represent dependencies.
// The Dynamic flag indicates whether the plan can be modified at runtime.
type Plan struct {
	ID        string     `json:"id"`
	Goal      string     `json:"goal"`
	Nodes     []TaskNode `json:"nodes"`
	Dynamic   bool       `json:"dynamic"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Result represents the outcome of a task execution.
//
// V2 enhancement: added Metadata for rich observability data.
type Result struct {
	TaskID    string         `json:"task_id"`
	Success   bool           `json:"success"`
	Output    any            `json:"output,omitempty"`
	Error     string         `json:"error,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Duration  time.Duration  `json:"duration"`
	Timestamp time.Time      `json:"timestamp"`
}

// Evaluation represents the result of evaluating a task execution.
//
// This is the feedback mechanism that drives the control loop.
type Evaluation struct {
	Success    bool    `json:"success"`
	Confidence float64 `json:"confidence"` // 0.0 to 1.0
	Reason     string  `json:"reason"`
	Retryable  bool    `json:"retryable"`
}

// ExecutionConfig holds configuration for the execution engine.
type ExecutionConfig struct {
	DefaultTimeout   time.Duration `json:"default_timeout"`
	MaxRetries       int           `json:"max_retries"`
	RetryBackoffBase time.Duration `json:"retry_backoff_base"`
	MaxParallelTasks int           `json:"max_parallel_tasks"`
	MaxReplans       int           `json:"max_replans"`
	// V5 fields
	QueueCapacity    int           `json:"queue_capacity"`
	RPCCallRetries   int           `json:"rpc_call_retries"`
	RPCBackoff       time.Duration `json:"rpc_backoff"`
	TaskTimeout      time.Duration `json:"task_timeout"` // Per-task timeout on workers
	HeartbeatTimeout time.Duration `json:"heartbeat_timeout"`
}

// DefaultExecutionConfig returns production-ready defaults.
func DefaultExecutionConfig() ExecutionConfig {
	return ExecutionConfig{
		DefaultTimeout:   30 * time.Second,
		MaxRetries:       3,
		RetryBackoffBase: 1 * time.Second,
		MaxParallelTasks: 4,
		MaxReplans:       3,
		// V5 defaults
		QueueCapacity:    100,
		RPCCallRetries:   3,
		RPCBackoff:       500 * time.Millisecond,
		TaskTimeout:      60 * time.Second,
		HeartbeatTimeout: 30 * time.Second,
	}
}

// AgentInfo provides metadata about a registered agent.
type AgentInfo struct {
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
	Version      string   `json:"version"`
}

// StepTrace records execution details for a single task step.
type StepTrace struct {
	TaskID     string     `json:"task_id"`
	Agent      string     `json:"agent"`
	WorkerID   string     `json:"worker_id"`
	StartTime  time.Time  `json:"start_time"`
	EndTime    time.Time  `json:"end_time"`
	Success    bool       `json:"success"`
	Evaluation Evaluation `json:"evaluation"`
	Retries    int        `json:"retries"`
}

// ExecutionTrace collects all step traces for a single orchestration run.
type ExecutionTrace struct {
	Goal          string        `json:"goal"`
	PlanID        string        `json:"plan_id"`
	Steps         []StepTrace   `json:"steps"`
	ReplanCount   int           `json:"replan_count"`
	TotalDuration time.Duration `json:"total_duration"`
}

// AddStep appends a step trace to the execution trace.
func (et *ExecutionTrace) AddStep(step StepTrace) {
	et.Steps = append(et.Steps, step)
}

// Summary returns a human-readable summary of the execution trace.
func (et *ExecutionTrace) Summary() string {
	totalSteps := len(et.Steps)
	successSteps := 0
	for _, s := range et.Steps {
		if s.Success {
			successSteps++
		}
	}
	return fmt.Sprintf("Plan: %s | Steps: %d/%d succeeded | Replans: %d | Duration: %s",
		et.PlanID, successSteps, totalSteps, et.ReplanCount, et.TotalDuration)
}
