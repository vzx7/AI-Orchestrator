// Package worker implements a distributed worker node that executes tasks.
//
// Workers:
// - Pull tasks from the queue (or receive via RPC)
// - Execute tasks via registered agents
// - Return results to the orchestrator
//
// Design decisions:
// - Workers are stateless — all state is in the orchestrator
// - Agents are injected for flexibility
// - Context propagation for graceful shutdown
package worker

import (
	"context"
	"fmt"
	"log/slog"

	"ai-orchestrator/internal/agents"
	"ai-orchestrator/internal/types"
)

// Worker represents a distributed execution node.
type Worker struct {
	ID       string
	agents   map[string]agents.Agent
	toolGW   agents.ToolGateway
	logger   *slog.Logger
}

// NewWorker creates a new worker with the given agents.
func NewWorker(id string, logger *slog.Logger, toolGW agents.ToolGateway) *Worker {
	return &Worker{
		ID:     id,
		agents: make(map[string]agents.Agent),
		toolGW: toolGW,
		logger: logger.With("worker_id", id),
	}
}

// RegisterAgent adds an agent to this worker.
func (w *Worker) RegisterAgent(agent agents.Agent) {
	w.agents[agent.Name()] = agent
	w.logger.Info("Agent registered on worker", "agent", agent.Name())
}

// ExecuteTask runs a task using the appropriate agent.
func (w *Worker) ExecuteTask(ctx context.Context, task types.Task) (types.Result, error) {
	agent, exists := w.agents[task.AssignedAgent]
	if !exists {
		return types.Result{
			TaskID: task.ID,
			Success: false,
			Error:   fmt.Sprintf("agent not found on worker %s: %s", w.ID, task.AssignedAgent),
		}, fmt.Errorf("agent not found: %s", task.AssignedAgent)
	}

	w.logger.Info("Executing task",
		"task_id", task.ID,
		"agent", agent.Name(),
		"goal", task.Goal,
	)

	return agent.Execute(task)
}

// GetAgentNames returns the names of registered agents.
func (w *Worker) GetAgentNames() []string {
	names := make([]string, 0, len(w.agents))
	for name := range w.agents {
		names = append(names, name)
	}
	return names
}
