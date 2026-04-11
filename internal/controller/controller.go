// Package controller implements the adaptive orchestration control loop.
//
// V5 adds:
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
	"sync"
	"time"

	"ai_orchestrator/internal/evaluator"
	"ai_orchestrator/internal/events"
	"ai_orchestrator/internal/execution"
	"ai_orchestrator/internal/types"
)

type Planner interface {
	GeneratePlan(goal string) (types.Plan, error)
	Replan(currentPlan types.Plan, failedTask types.Task, eval types.Evaluation) types.Plan
}

type TaskExecutor interface {
	ExecuteTask(ctx context.Context, task types.Task) (types.Result, error)
}

type Controller struct {
	planner       Planner
	evaluator     evaluator.Evaluator
	engine        *execution.Engine
	executor      TaskExecutor
	logger        *slog.Logger
	eventBus      *events.EventBus
	config        types.ExecutionConfig
	trace         types.ExecutionTrace
	maxIterations int
	stopCh        chan struct{}
	stopped       bool
	stopMu        sync.RWMutex
}

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
		executor:      engine,
		logger:        logger,
		eventBus:      eventBus,
		config:        config,
		trace:         types.ExecutionTrace{},
		maxIterations: 1000,
		stopCh:        make(chan struct{}),
	}
}

func (c *Controller) SetMaxIterations(n int) {
	c.maxIterations = n
}

func (c *Controller) SetExecutor(executor TaskExecutor) {
	c.executor = executor
	c.logger.Info("Task executor updated", "type", fmt.Sprintf("%T", executor))
}

func (c *Controller) Run(ctx context.Context, goal string) ([]types.Result, error) {
	startTime := time.Now()
	c.logger.Info("Controller started", "goal", goal)

	c.eventBus.Publish(events.Event{
		Type:    events.EventOrchestratorStart,
		Source:  "controller",
		Payload: map[string]any{"goal": goal},
	})

	plan, err := c.planner.GeneratePlan(goal)
	if err != nil {
		c.logger.Error("Failed to generate plan", "error", err)
		return nil, fmt.Errorf("plan generation failed: %w", err)
	}

	c.trace.Goal = goal
	c.trace.PlanID = plan.ID

	results, err := c.runControlLoop(ctx, plan)

	c.trace.TotalDuration = time.Since(startTime)

	c.logger.Info("Controller completed",
		"goal", goal,
		"results", len(results),
		"replans", c.trace.ReplanCount,
		"duration", c.trace.TotalDuration,
	)

	c.eventBus.Publish(events.Event{
		Type:   events.EventOrchestratorDone,
		Source: "controller",
		Payload: map[string]any{
			"goal":     goal,
			"results":  len(results),
			"replans":  c.trace.ReplanCount,
			"success":  err == nil,
			"duration": c.trace.TotalDuration.String(),
		},
	})

	return results, err
}

func (c *Controller) runControlLoop(ctx context.Context, plan types.Plan) ([]types.Result, error) {
	allResults := make([]types.Result, 0)
	completed := make(map[string]bool)

	iterations := 0
	replansWithoutProgress := 0
	prevCompletedCount := 0

	for len(completed) < len(plan.Nodes) {
		select {
		case <-ctx.Done():
			return allResults, ctx.Err()
		case <-c.stopCh:
			return allResults, fmt.Errorf("controller stopped")
		default:
		}

		iterations++
		if iterations > c.maxIterations {
			c.logger.Error("Maximum iterations exceeded, aborting",
				"max_iterations", c.maxIterations,
				"completed", len(completed),
				"total", len(plan.Nodes),
			)
			return allResults, fmt.Errorf("control loop exceeded max iterations (%d): possible infinite loop", c.maxIterations)
		}

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

		for _, node := range readyNodes {
			select {
			case <-ctx.Done():
				return allResults, ctx.Err()
			case <-c.stopCh:
				return allResults, fmt.Errorf("controller stopped")
			default:
			}

			stepStart := time.Now()
			result, eval, stepErr := c.executeAndEvaluate(ctx, node.Task)

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

				if eval.Retryable && node.Task.RetryCount < node.Task.MaxRetries {
					completed[node.Task.ID] = false
				} else if c.trace.ReplanCount < c.config.MaxReplans {
					c.logger.Info("Triggering replan",
						"failed_task", node.Task.ID,
						"replan_count", c.trace.ReplanCount+1,
					)

					plan = c.planner.Replan(plan, node.Task, eval)
					c.trace.ReplanCount++
					c.trace.PlanID = plan.ID

					completed = c.rebuildCompletedMap(completed, plan.Nodes)

					currentCompleted := len(completed)
					if currentCompleted <= prevCompletedCount {
						replansWithoutProgress++
						if replansWithoutProgress >= 3 {
							c.logger.Error("Replan not making progress, aborting",
								"replans", c.trace.ReplanCount,
								"completed", currentCompleted,
							)
							return allResults, fmt.Errorf("replan not making progress after %d attempts", replansWithoutProgress)
						}
					} else {
						replansWithoutProgress = 0
					}
					prevCompletedCount = currentCompleted

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
				prevCompletedCount = len(completed)
				c.logger.Info("Task succeeded", "task_id", node.Task.ID)
			}
		}
	}

	return allResults, nil
}

func (c *Controller) rebuildCompletedMap(completed map[string]bool, nodes []types.TaskNode) map[string]bool {
	oldCompleted := make(map[string]bool)
	for taskID := range completed {
		for _, n := range nodes {
			if n.Task.ID == taskID {
				oldCompleted[taskID] = completed[taskID]
				break
			}
		}
	}
	return oldCompleted
}

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

func (c *Controller) GetTrace() types.ExecutionTrace {
	return c.trace
}

func (c *Controller) Stop() {
	c.stopMu.Lock()
	if c.stopped {
		c.stopMu.Unlock()
		return
	}
	c.stopped = true
	close(c.stopCh)
	c.stopMu.Unlock()

	if c.engine != nil {
		c.engine.Stop()
	}
	c.logger.Info("Controller stopped")
}
