// Package execution implements the deterministic execution engine with DAG support.
//
// V2 changes:
// - ExecutePlanDAG runs tasks respecting dependency ordering
// - Parallel execution of independent nodes
// - Circular dependency detection
// - Track completed tasks for dependency resolution
//
// The engine handles:
// - Sequential and parallel task execution
// - Retries with exponential backoff
// - Timeouts per task
// - Cancellation via context
package execution

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"ai-orchestrator/internal/agents"
	"ai-orchestrator/internal/events"
	"ai-orchestrator/internal/types"
)

// Engine manages the deterministic execution of tasks.
type Engine struct {
	config   types.ExecutionConfig
	logger   *slog.Logger
	eventBus *events.EventBus
	agents   map[string]agents.Agent
}

// NewEngine creates a new execution engine.
func NewEngine(config types.ExecutionConfig, logger *slog.Logger, eventBus *events.EventBus) *Engine {
	return &Engine{
		config:   config,
		logger:   logger,
		eventBus: eventBus,
		agents:   make(map[string]agents.Agent),
	}
}

// RegisterAgent adds an agent to the engine's registry.
func (e *Engine) RegisterAgent(agent agents.Agent) {
	e.agents[agent.Name()] = agent
	e.logger.Info("Agent registered", "agent", agent.Name())
}

// ExecutePlan runs all tasks sequentially (V1 compatibility).
// Deprecated: Use ExecutePlanDAG for DAG-based execution.
func (e *Engine) ExecutePlan(ctx context.Context, plan types.Plan) ([]types.Result, error) {
	e.logger.Info("Executing plan (sequential)", "plan_id", plan.ID, "task_count", len(plan.Nodes))

	results := make([]types.Result, 0, len(plan.Nodes))

	for _, node := range plan.Nodes {
		select {
		case <-ctx.Done():
			e.logger.Warn("Plan execution cancelled", "plan_id", plan.ID, "reason", ctx.Err())
			return results, ctx.Err()
		default:
		}

		result, err := e.ExecuteTask(ctx, node.Task)
		results = append(results, result)

		if err != nil {
			e.logger.Error("Task failed", "task_id", node.Task.ID, "error", err)
		}
	}

	e.logger.Info("Plan execution completed", "plan_id", plan.ID, "results", len(results))
	return results, nil
}

// ExecutePlanDAG executes a DAG-structured plan respecting dependencies.
//
// Algorithm:
// 1. Find all nodes with satisfied dependencies (no deps or all deps completed)
// 2. Execute them (potentially in parallel)
// 3. Mark as completed and repeat until all nodes are done
// 4. Detect circular dependencies if progress stalls
func (e *Engine) ExecutePlanDAG(ctx context.Context, plan types.Plan) ([]types.Result, error) {
	e.logger.Info("Executing DAG plan",
		"plan_id", plan.ID,
		"node_count", len(plan.Nodes),
	)

	e.eventBus.Publish(events.Event{
		Type:   events.EventPlanCreated,
		Source: "execution_engine",
		Payload: map[string]any{
			"plan_id":    plan.ID,
			"node_count": len(plan.Nodes),
			"type":       "dag",
		},
	})

	// Track state
	completed := make(map[string]bool)        // taskID -> success
	results := make(map[string]types.Result) // taskID -> Result
	resultsMu := sync.Mutex{}

	// Check for circular dependencies
	if err := e.detectCircularDeps(plan.Nodes); err != nil {
		return nil, fmt.Errorf("circular dependency detected: %w", err)
	}

	// Execute loop: keep going until all nodes are done
	totalNodes := len(plan.Nodes)
	for len(completed) < totalNodes {
		// Find ready nodes (all dependencies satisfied)
		readyNodes := e.findReadyNodes(plan.Nodes, completed)

		if len(readyNodes) == 0 {
			// No progress made — something is wrong
			if len(completed) < totalNodes {
				return nil, fmt.Errorf("deadlock: %d/%d nodes completed, no ready nodes",
					len(completed), totalNodes)
			}
			break
		}

		e.logger.Info("DAG scheduling",
			"ready_count", len(readyNodes),
			"completed_count", len(completed),
			"remaining_count", totalNodes-len(completed),
		)

		// Execute ready nodes in parallel
		var wg sync.WaitGroup
		for _, node := range readyNodes {
			wg.Add(1)

			go func(n types.TaskNode) {
				defer wg.Done()

				select {
				case <-ctx.Done():
					e.logger.Warn("Task cancelled", "task_id", n.Task.ID)
					resultsMu.Lock()
					completed[n.Task.ID] = false
					results[n.Task.ID] = types.Result{
						TaskID:  n.Task.ID,
						Success: false,
						Error:   "cancelled",
					}
					resultsMu.Unlock()
					return
				default:
				}

				result, err := e.ExecuteTask(ctx, n.Task)
				resultsMu.Lock()
				completed[n.Task.ID] = result.Success
				results[n.Task.ID] = result
				resultsMu.Unlock()

				if err != nil {
					e.logger.Error("DAG task failed",
						"task_id", n.Task.ID,
						"error", err,
					)
				}
			}(node)
		}

		wg.Wait()
	}

	// Collect results in plan order
	orderedResults := make([]types.Result, 0, totalNodes)
	for _, node := range plan.Nodes {
		if result, ok := results[node.Task.ID]; ok {
			orderedResults = append(orderedResults, result)
		}
	}

	e.eventBus.Publish(events.Event{
		Type:   events.EventPlanCompleted,
		Source: "execution_engine",
		Payload: map[string]any{
			"plan_id":       plan.ID,
			"results_count": len(orderedResults),
		},
	})

	e.logger.Info("DAG plan execution completed",
		"plan_id", plan.ID,
		"results", len(orderedResults),
	)

	return orderedResults, nil
}

// findReadyNodes returns nodes whose dependencies are all satisfied.
func (e *Engine) findReadyNodes(nodes []types.TaskNode, completed map[string]bool) []types.TaskNode {
	var ready []types.TaskNode

	for _, node := range nodes {
		// Skip already completed
		if _, done := completed[node.Task.ID]; done {
			continue
		}

		// Check all dependencies are satisfied
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

// detectCircularDeps performs a basic cycle detection using DFS.
func (e *Engine) detectCircularDeps(nodes []types.TaskNode) error {
	// Build adjacency map
	adj := make(map[string][]string)
	nodeIDs := make(map[string]bool)
	for _, node := range nodes {
		nodeIDs[node.Task.ID] = true
		adj[node.Task.ID] = node.DependsOn
	}

	// DFS-based cycle detection
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var dfs func(id string) error
	dfs = func(id string) error {
		visited[id] = true
		recStack[id] = true

		for _, dep := range adj[id] {
			if !visited[dep] {
				if err := dfs(dep); err != nil {
					return err
				}
			} else if recStack[dep] {
				return fmt.Errorf("cycle detected involving task %s", dep)
			}
		}

		recStack[id] = false
		return nil
	}

	for id := range nodeIDs {
		if !visited[id] {
			if err := dfs(id); err != nil {
				return err
			}
		}
	}

	return nil
}

// ExecuteTask runs a single task with retries, timeouts, and cancellation.
func (e *Engine) ExecuteTask(ctx context.Context, task types.Task) (types.Result, error) {
	agent, exists := e.agents[task.AssignedAgent]
	if !exists {
		return types.Result{
			TaskID:  task.ID,
			Success: false,
			Error:   fmt.Sprintf("agent not found: %s", task.AssignedAgent),
		}, fmt.Errorf("agent not found: %s", task.AssignedAgent)
	}

	// Apply defaults
	if task.MaxRetries == 0 {
		task.MaxRetries = e.config.MaxRetries
	}
	if task.Timeout == 0 {
		task.Timeout = e.config.DefaultTimeout
	}

	return e.executeWithRetry(ctx, task, agent)
}

// executeWithRetry handles task execution with exponential backoff retries.
func (e *Engine) executeWithRetry(ctx context.Context, task types.Task, agent agents.Agent) (types.Result, error) {
	var lastResult types.Result
	var lastErr error

	for attempt := 0; attempt <= task.MaxRetries; attempt++ {
		if attempt > 0 {
			task.Status = types.TaskStatusRetrying
			task.RetryCount = attempt

			backoff := e.config.RetryBackoffBase * time.Duration(1<<uint(attempt-1))
			e.logger.Info("Retrying task",
				"task_id", task.ID,
				"attempt", attempt,
				"max_retries", task.MaxRetries,
				"backoff", backoff,
			)

			e.eventBus.Publish(events.Event{
				Type:   events.EventTaskRetrying,
				Source: "execution_engine",
				Payload: map[string]any{
					"task_id": task.ID,
					"attempt": attempt,
				},
			})

			select {
			case <-ctx.Done():
				return types.Result{
					TaskID:  task.ID,
					Success: false,
					Error:   fmt.Sprintf("cancelled during retry backoff: %v", ctx.Err()),
				}, ctx.Err()
			case <-time.After(backoff):
			}
		}

		result, err := e.executeWithTimeout(ctx, task, agent)
		lastResult = result
		lastErr = err

		if err == nil {
			result.Success = true
			task.Status = types.TaskStatusCompleted
			return result, nil
		}

		e.logger.Warn("Task attempt failed",
			"task_id", task.ID,
			"attempt", attempt+1,
			"error", err,
		)
	}

	task.Status = types.TaskStatusFailed
	lastResult.Success = false
	lastResult.Error = fmt.Sprintf("failed after %d retries: %v", task.MaxRetries, lastErr)

	return lastResult, lastErr
}

// executeWithTimeout runs a single task attempt with a timeout.
func (e *Engine) executeWithTimeout(ctx context.Context, task types.Task, agent agents.Agent) (types.Result, error) {
	task.Status = types.TaskStatusRunning

	e.eventBus.Publish(events.Event{
		Type:   events.EventTaskStarted,
		Source: "execution_engine",
		Payload: map[string]any{
			"task_id": task.ID,
			"agent":   agent.Name(),
		},
	})

	// Create a timeout context
	taskCtx, cancel := context.WithTimeout(ctx, task.Timeout)
	defer cancel()

	// Channel to receive the result
	resultCh := make(chan types.Result, 1)
	errCh := make(chan error, 1)

	go func() {
		result, err := agent.Execute(task)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	select {
	case <-taskCtx.Done():
		task.Status = types.TaskStatusCancelled
		e.eventBus.Publish(events.Event{
			Type:   events.EventTaskFailed,
			Source: "execution_engine",
			Payload: map[string]any{
				"task_id": task.ID,
				"reason":  "timeout",
			},
		})
		return types.Result{
			TaskID:  task.ID,
			Success: false,
			Error:   fmt.Sprintf("task timed out: %v", taskCtx.Err()),
		}, taskCtx.Err()

	case err := <-errCh:
		e.eventBus.Publish(events.Event{
			Type:   events.EventTaskFailed,
			Source: "execution_engine",
			Payload: map[string]any{
				"task_id": task.ID,
				"error":   err.Error(),
			},
		})
		return types.Result{
			TaskID:  task.ID,
			Success: false,
			Error:   err.Error(),
		}, err

	case result := <-resultCh:
		e.eventBus.Publish(events.Event{
			Type:   events.EventTaskCompleted,
			Source: "execution_engine",
			Payload: map[string]any{
				"task_id": task.ID,
				"success": result.Success,
			},
		})
		return result, nil
	}
}
