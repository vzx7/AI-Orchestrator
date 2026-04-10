package agents

import (
	"fmt"
	"log/slog"
	"time"

	"ai-orchestrator/internal/types"
)

// BaseAgent provides common functionality for all agent implementations.
type BaseAgent struct {
	name         string
	capabilities []string
	gateway      ToolGateway
	log          *slog.Logger
}

// ToolGateway defines the minimal interface needed for tool access.
// This avoids tight coupling to the full ToolGateway implementation.
type ToolGateway interface {
	Call(agentName, toolName string, args map[string]any) (map[string]any, error)
}

func (a *BaseAgent) Name() string {
	return a.name
}

func (a *BaseAgent) logger() *slog.Logger {
	return a.log.With("agent", a.name)
}

// DevAgent is responsible for development tasks like fixing code and running builds.
type DevAgent struct {
	BaseAgent
}

// NewDevAgent creates a new DevAgent with the given dependencies.
func NewDevAgent(gateway ToolGateway, logger *slog.Logger) *DevAgent {
	return &DevAgent{
		BaseAgent: BaseAgent{
			name: "dev",
			capabilities: []string{
				"fix_code",
				"run_build",
				"fix_tests",
			},
			gateway: gateway,
			log:     logger,
		},
	}
}

// Execute processes a development-related task.
func (a *DevAgent) Execute(task types.Task) (types.Result, error) {
	start := time.Now()
	a.logger().Info("Executing dev task", "task_id", task.ID, "goal", task.Goal)

	var result types.Result
	result.TaskID = task.ID
	result.Timestamp = time.Now()

	switch {
	case task.Goal == "fix_test":
		result = a.fixTest(task)
	case task.Goal == "run_build":
		result = a.runBuild(task)
	case task.Goal == "fix_code":
		result = a.fixCode(task)
	default:
		result.Success = false
		result.Error = fmt.Sprintf("dev agent: unsupported task goal: %s", task.Goal)
	}

	result.Duration = time.Since(start)
	a.logger().Info("Dev task completed",
		"task_id", task.ID,
		"success", result.Success,
		"duration_ms", result.Duration.Milliseconds(),
	)

	return result, nil
}

func (a *DevAgent) fixTest(task types.Task) types.Result {
	// Simulate fixing a failing test
	a.logger().Info("Analyzing test failure", "task_id", task.ID)

	_, err := a.gateway.Call(a.name, "file.read", map[string]any{
		"path": "test/output.log",
	})
	if err != nil {
		return types.Result{
			TaskID:  task.ID,
			Success: false,
			Error:   fmt.Sprintf("failed to read test output: %v", err),
		}
	}

	a.logger().Info("Applying test fix", "task_id", task.ID)

	_, err = a.gateway.Call(a.name, "shell.exec", map[string]any{
		"command": "echo 'Fixed test - mock operation'",
	})
	if err != nil {
		return types.Result{
			TaskID:  task.ID,
			Success: false,
			Error:   fmt.Sprintf("failed to apply fix: %v", err),
		}
	}

	return types.Result{
		TaskID:  task.ID,
		Success: true,
		Output: map[string]any{
			"action":        "test_fixed",
			"files_changed": []string{"test/example_test.go"},
		},
	}
}

func (a *DevAgent) runBuild(task types.Task) types.Result {
	a.logger().Info("Running build", "task_id", task.ID)

	result, err := a.gateway.Call(a.name, "shell.exec", map[string]any{
		"command": "go build ./...",
	})
	if err != nil {
		return types.Result{
			TaskID:  task.ID,
			Success: false,
			Error:   fmt.Sprintf("build failed: %v", err),
		}
	}

	return types.Result{
		TaskID:  task.ID,
		Success: true,
		Output:  result,
	}
}

func (a *DevAgent) fixCode(task types.Task) types.Result {
	a.logger().Info("Fixing code", "task_id", task.ID)

	_, err := a.gateway.Call(a.name, "shell.exec", map[string]any{
		"command": fmt.Sprintf("echo 'Fixing code for: %s'", task.Goal),
	})
	if err != nil {
		return types.Result{
			TaskID:  task.ID,
			Success: false,
			Error:   fmt.Sprintf("code fix failed: %v", err),
		}
	}

	return types.Result{
		TaskID:  task.ID,
		Success: true,
		Output: map[string]any{
			"action": "code_fixed",
		},
	}
}
