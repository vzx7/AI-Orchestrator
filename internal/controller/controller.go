// Package controller implements the adaptive orchestration control loop.
//
// V4 adds:
// - Retry policy integration for retry decisions
// - Idempotency enforcement (skip duplicate tasks)
// - Infinite loop protection (max iterations guard)
//
// The Controller is the brain of the system, implementing:
//
//	Plan → Execute Task → Evaluate Result
//	→ if success → next task
//	→ if failed → retry OR replan
package controller

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"ai-orchestrator/internal/events"
	"ai-orchestrator/internal/evaluator"
	"ai-orchestrator/internal/execution"
	"ai-orchestrator/internal/types"
)

// Planner abstracts the planner for dependency injection.
type Planner interface {
	GeneratePlan(goal string) (types.Plan, error)
	Replan(currentPlan types.Plan, failedTask types.Task, eval types.Evaluation) types.Plan
}

// TaskExecutor abstracts task execution for local or distributed modes.
type TaskExecutor interface {
	ExecuteTask(ctx context.Context, task types.Task) (types.Result, error)
}

// Controller orchestrates the adaptive Plan→Execute→Evaluate→Replan loop.
type Controller struct {
	planner   Planner
	evaluator evaluator.Evaluator
	engine    *execution.Engine
	executor  TaskExecutor
	logger    *slog.Logger
	eventBus  *events.EventBus
	config    types.ExecutionConfig
	trace     types.ExecutionTrace
	// V4: loop protection
	maxIterations int
}

// NewController creates a fully configured Controller.
func NewController(
	planner Planner,
	eval evaluator.Evaluator,
	engine *execution.Engine,
	logger *slog.Logger,
	eventBus *events.EventBus,
	config types.ExecutionConfig,
) *Controller {
	return &Controller{
		planner:       planner,
		evaluator:     eval,
		engine:        engine,
		executor:      engine, // default to local engine
		logger:        logger,
		eventBus:      eventBus,
		config:        config,
		trace:         types.ExecutionTrace{},
		maxIterations: 1000, // V4: prevent infinite loops
	}
}

// SetMaxIterations sets the maximum control loop iterations (V4 loop protection).
func (c *Controller) SetMaxIterations(n int) {
	c.maxIterations = n
}

// SetExecutor overrides the task executor for distributed execution.
func (c *Controller) SetExecutor(executor TaskExecutor) {
	c.executor = executor
	c.logger.Info("Task executor updated", "type", fmt.Sprintf("%T", executor))
}

// Run executes the full adaptive control loop for a user goal.
func (c *Controller) Run(ctx context.Context, goal string) ([]types.Result, error) {
	startTime := time.Now()
	c.logger.Info("Controller started", "goal", goal)

	c.eventBus.Publish(events.Event{
		Type:    events.EventOrchestratorStart,
		Source:  "controller",
		Payload: map[string]any{"goal": goal},
	})

	// Step 1: Generate initial plan
	plan, err := c.planner.GeneratePlan(goal)
	if err != nil {
		c.logger.Error("Failed to generate plan", "error", err)
		return nil, fmt.Errorf("plan generation failed: %w", err)
	}

	c.trace.Goal = goal
	c.trace.PlanID = plan.ID

	// Step 2: Run the adaptive control loop
	results, err := c.runControlLoop(ctx, plan)

	// Step 3: Finalize trace
	c.trace.TotalDuration = time.Since(startTime)

	c.logger.Info("Controller completed",
		"goal", goal,
		"results", len(results),
		"replans", c.trace.ReplanCount,
		"duration", c.trace.TotalDuration,
	)

	c.eventBus.Publish(events.Event{
		Type:    events.EventOrchestratorDone,
		Source:  "controller",
		Payload: map[string]any{
			"goal":    goal,
			"results": len(results),
			"replans": c.trace.ReplanCount,
			"success": err == nil,
			"duration": c.trace.TotalDuration.String(),
		},
	})

	return results, err
}

// runControlLoop executes the Plan→Execute→Evaluate→Replan cycle.
func (c *Controller) runControlLoop(ctx context.Context, plan types.Plan) ([]types.Result, error) {
	allResults := make([]types.Result, 0)

	// Track which nodes are done
	completed := make(map[string]bool)

	// V4: iteration counter for loop protection
	iterations := 0

	// Main loop
	for len(completed) < len(plan.Nodes) {
		iterations++
		if iterations > c.maxIterations {
			c.logger.Error("Maximum iterations exceeded, aborting",
				"max_iterations", c.maxIterations,
				"completed", len(completed),
				"total", len(plan.Nodes),
			)
			return allResults, fmt.Errorf("control loop exceeded max iterations (%d): possible infinite loop", c.maxIterations)
		}

		// Find ready nodes
		readyNodes := c.findReadyNodes(plan.Nodes, completed)
		if len(readyNodes) == 0 {
			if len(completed) < len(plan.Nodes) {
				return allResults, fmt.Errorf("deadlock: cannot progress")
			}
			break
		}

		c.logger.Info("Control loop iteration",
			"iteration", iterations,
			"ready_count", len(readyNodes),
			"completed_count", len(completed),
			"total_count", len(plan.Nodes),
		)

		// Execute each ready node
		for _, node := range readyNodes {
			select {
			case <-ctx.Done():
				return allResults, ctx.Err()
			default:
			}

			stepStart := time.Now()
			result, eval, stepErr := c.executeAndEvaluate(ctx, node.Task)

			// Extract worker ID from result metadata (distributed mode)
			workerID := ""
			if result.Metadata != nil {
				if wid, ok := result.Metadata["worker_id"].(string); ok {
					workerID = wid
				}
			}

			stepTrace := types.StepTrace{
				TaskID:     node.Task.ID,
				Agent:      node.Task.AssignedAgent,
				WorkerID:   workerID,
				StartTime:  stepStart,
				EndTime:    time.Now(),
				Success:    result.Success,
				Evaluation: eval,
				Retries:    node.Task.RetryCount,
			}
			c.trace.AddStep(stepTrace)

			allResults = append(allResults, result)

			if stepErr != nil {
				c.logger.Warn("Task failed after evaluation",
					"task_id", node.Task.ID,
					"eval_reason", eval.Reason,
					"eval_retryable", eval.Retryable,
				)

				// V4: Evaluate retry vs replan decision
				if eval.Retryable && node.Task.RetryCount < node.Task.MaxRetries {
					// Retry handled by executor (executeWithRetry already exhausted)
					completed[node.Task.ID] = false
				} else if c.trace.ReplanCount < c.config.MaxReplans {
					// Replan
					c.logger.Info("Triggering replan",
						"failed_task", node.Task.ID,
						"replan_count", c.trace.ReplanCount+1,
					)

					completed[node.Task.ID] = false

					plan = c.planner.Replan(plan, node.Task, eval)
					c.trace.ReplanCount++
					c.trace.PlanID = plan.ID

					// Reset completed map for new plan
					oldCompleted := make(map[string]bool)
					for taskID := range completed {
						for _, n := range plan.Nodes {
							if n.Task.ID == taskID {
								oldCompleted[taskID] = true
								break
							}
						}
					}
					completed = oldCompleted

					continue
				} else {
					c.logger.Error("Max replans exceeded, marking task as failed",
						"task_id", node.Task.ID,
						"max_replans", c.config.MaxReplans,
					)
					completed[node.Task.ID] = false
				}
			} else {
				completed[node.Task.ID] = true
				c.logger.Info("Task succeeded", "task_id", node.Task.ID)
			}
		}
	}

	return allResults, nil
}

// executeAndEvaluate runs a task and evaluates the result.
// V4: Idempotency is handled by the executor layer.
func (c *Controller) executeAndEvaluate(ctx context.Context, task types.Task) (types.Result, types.Evaluation, error) {
	result, err := c.executor.ExecuteTask(ctx, task)
	eval := c.evaluator.Evaluate(task, result)

	c.logger.Info("Task evaluated",
		"task_id", task.ID,
		"success", eval.Success,
		"confidence", eval.Confidence,
		"reason", eval.Reason,
		"retryable", eval.Retryable,
	)

	return result, eval, err
}

// findReadyNodes returns nodes whose dependencies are all satisfied.
func (c *Controller) findReadyNodes(nodes []types.TaskNode, completed map[string]bool) []types.TaskNode {
	var ready []types.TaskNode

	for _, node := range nodes {
		if _, done := completed[node.Task.ID]; done {
			continue
		}

		allDepsMet := true
		for _, dep := range node.DependsOn {
			if _, ok := completed[dep]; !ok {
				allDepsMet = false
				break
			}
		}

		if allDepsMet {
			ready = append(ready, node)
		}
	}

	return ready
}

// GetTrace returns the execution trace collected during the run.
func (c *Controller) GetTrace() types.ExecutionTrace {
	return c.trace
}
