// Package agents defines the Agent interface and concrete implementations.
//
// Agents are specialized workers that execute tasks assigned by the
// orchestration engine. Each agent has specific capabilities and
// interacts with external tools ONLY through the ToolGateway.
package agents

import (
	"ai_orchestrator/internal/types"
)

// Agent defines the contract for all specialized agents.
//
// Agents are stateless workers that receive a Task and produce a Result.
// They must not call tools directly — all tool access goes through
// the ToolGateway to enforce ACL and observability.
type Agent interface {
	// Name returns the agent's unique identifier.
	Name() string

	// Execute processes a task and returns the result.
	Execute(task types.Task) (types.Result, error)
}
