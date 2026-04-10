// Package toolsgateway implements the ToolGateway that sits between agents and MCP tools.
//
// The ToolGateway enforces strict security and observability:
// - Agents MUST NOT call tools directly; all calls flow through this gateway.
// - Inputs are validated against tool schemas.
// - ACL (Access Control List) enforcement prevents unauthorized tool usage.
// - All calls are logged for auditing and debugging.
package toolsgateway

import (
	"fmt"
	"log/slog"
	"time"

	"ai-orchestrator/internal/mcp"
)

// ToolGateway sits between agents and MCP tools, enforcing security and observability.
type ToolGateway struct {
	registry *mcp.ToolRegistry
	logger   *slog.Logger
	acl      map[string][]string // agentName -> allowed tools
}

// NewToolGateway creates a new ToolGateway with the given MCP registry and logger.
func NewToolGateway(registry *mcp.ToolRegistry, logger *slog.Logger) *ToolGateway {
	return &ToolGateway{
		registry: registry,
		logger:   logger,
		acl:      make(map[string][]string),
	}
}

// SetACL configures which tools an agent is allowed to call.
func (gw *ToolGateway) SetACL(agentName string, allowedTools []string) {
	gw.acl[agentName] = allowedTools
	gw.logger.Info("ACL configured",
		"agent", agentName,
		"allowed_tools", allowedTools,
	)
}

// Call executes a tool on behalf of an agent, with validation and ACL enforcement.
//
// This is the ONLY entry point for tool execution. Agents must use this method
// rather than calling tools directly.
func (gw *ToolGateway) Call(agentName, toolName string, args map[string]any) (map[string]any, error) {
	start := time.Now()

	// ACL enforcement
	if !gw.isToolAllowed(agentName, toolName) {
		err := fmt.Errorf("ACL denied: agent '%s' is not allowed to call tool '%s'", agentName, toolName)
		gw.logger.Warn("Tool call blocked by ACL",
			"agent", agentName,
			"tool", toolName,
		)
		return nil, err
	}

	// Log the call
	gw.logger.Info("Tool call initiated",
		"agent", agentName,
		"tool", toolName,
		"args", args,
	)

	// Execute the tool via registry
	result, err := gw.registry.ExecuteTool(toolName, args)
	duration := time.Since(start)

	if err != nil {
		gw.logger.Error("Tool call failed",
			"agent", agentName,
			"tool", toolName,
			"error", err,
			"duration_ms", duration.Milliseconds(),
		)
		return nil, err
	}

	// Log success
	gw.logger.Info("Tool call completed",
		"agent", agentName,
		"tool", toolName,
		"duration_ms", duration.Milliseconds(),
		"result", result,
	)

	return result, nil
}

// ListAvailableTools returns the tools available to a specific agent based on ACL.
func (gw *ToolGateway) ListAvailableTools(agentName string) []string {
	allowed := gw.acl[agentName]
	if len(allowed) == 0 {
		// If no ACL set, return all tools (permissive default for development)
		return gw.registry.ListTools()
	}
	return allowed
}

// isToolAllowed checks if an agent has permission to use a tool.
func (gw *ToolGateway) isToolAllowed(agentName, toolName string) bool {
	allowed, exists := gw.acl[agentName]
	if !exists {
		// No ACL configured: allow by default (permissive for development)
		// In production, default-deny is recommended.
		return true
	}

	for _, tool := range allowed {
		if tool == toolName {
			return true
		}
	}
	return false
}
