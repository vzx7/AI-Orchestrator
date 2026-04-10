package agents

import (
	"fmt"
	"log/slog"
	"time"

	"ai-orchestrator/internal/types"
)

// OpsAgent handles operations tasks like deployment and infrastructure.
type OpsAgent struct {
	BaseAgent
}

// NewOpsAgent creates a new OpsAgent with the given dependencies.
func NewOpsAgent(gateway ToolGateway, logger *slog.Logger) *OpsAgent {
	return &OpsAgent{
		BaseAgent: BaseAgent{
			name: "ops",
			capabilities: []string{
				"deploy",
				"infrastructure",
				"monitoring",
			},
			gateway: gateway,
			log:     logger,
		},
	}
}

// Execute processes an operations-related task.
func (a *OpsAgent) Execute(task types.Task) (types.Result, error) {
	start := time.Now()
	a.logger().Info("Executing ops task", "task_id", task.ID, "goal", task.Goal)

	var result types.Result
	result.TaskID = task.ID
	result.Timestamp = time.Now()

	switch {
	case task.Goal == "deploy":
		result = a.deploy(task)
	case task.Goal == "analyze_failure":
		result = a.analyzeFailure(task)
	default:
		result.Success = false
		result.Error = fmt.Sprintf("ops agent: unsupported task goal: %s", task.Goal)
	}

	result.Duration = time.Since(start)
	a.logger().Info("Ops task completed",
		"task_id", task.ID,
		"success", result.Success,
		"duration_ms", result.Duration.Milliseconds(),
	)

	return result, nil
}

func (a *OpsAgent) deploy(task types.Task) types.Result {
	a.logger().Info("Deploying service", "task_id", task.ID)

	result, err := a.gateway.Call(a.name, "deploy.service", map[string]any{
		"service":     "my-service",
		"environment": "production",
		"version":     "1.0.0",
	})
	if err != nil {
		return types.Result{
			TaskID:  task.ID,
			Success: false,
			Error:   fmt.Sprintf("deployment failed: %v", err),
		}
	}

	return types.Result{
		TaskID:  task.ID,
		Success: true,
		Output:  result,
	}
}

func (a *OpsAgent) analyzeFailure(task types.Task) types.Result {
	a.logger().Info("Analyzing failure for recovery", "task_id", task.ID)

	_, err := a.gateway.Call(a.name, "shell.exec", map[string]any{
		"command": "echo 'Analyzing failure and generating recovery plan'",
	})
	if err != nil {
		return types.Result{
			TaskID:  task.ID,
			Success: false,
			Error:   fmt.Sprintf("failure analysis failed: %v", err),
		}
	}

	return types.Result{
		TaskID:  task.ID,
		Success: true,
		Output: map[string]any{
			"action":        "failure_analyzed",
			"recovery_plan": "rollback_and_retry",
		},
	}
}
