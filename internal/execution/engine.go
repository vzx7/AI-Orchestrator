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
	"runtime/debug"
	"sync"
	"time"

	"ai_orchestrator/internal/agents"
	"ai_orchestrator/internal/events"
	"ai_orchestrator/internal/types"
)

type Engine struct {
	config    types.ExecutionConfig
	logger    *slog.Logger
	eventBus  *events.EventBus
	agents    map[string]agents.Agent
	semaphore chan struct{}
	stopCh    chan struct{}
	stopped   bool
	stopMu    sync.RWMutex
}

func NewEngine(config types.ExecutionConfig, logger *slog.Logger, eventBus *events.EventBus) *Engine {
	return &Engine{
		config:    config,
		logger:    logger,
		eventBus:  eventBus,
		agents:    make(map[string]agents.Agent),
		semaphore: make(chan struct{}, config.MaxParallelTasks),
		stopCh:    make(chan struct{}),
	}
}

func (e *Engine) RegisterAgent(agent agents.Agent) {
	e.agents[agent.Name()] = agent
	e.logger.Info("Agent registered", "agent", agent.Name())
}

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

	if err := e.detectCircularDeps(plan.Nodes); err != nil {
		return nil, fmt.Errorf("circular dependency detected: %w", err)
	}

	completed := make(map[string]bool)
	results := make(map[string]types.Result)
	resultsMu := sync.Mutex{}
	taskWg := sync.WaitGroup{}
	totalNodes := len(plan.Nodes)

	for len(completed) < totalNodes {
		select {
		case <-ctx.Done():
			e.logger.Warn("DAG execution cancelled", "plan_id", plan.ID)
			taskWg.Wait()
			return e.collectResults(plan.Nodes, results), ctx.Err()
		case <-e.stopCh:
			e.logger.Warn("DAG execution stopped", "plan_id", plan.ID)
			taskWg.Wait()
			return e.collectResults(plan.Nodes, results), fmt.Errorf("engine stopped")
		default:
		}

		readyNodes := e.findReadyNodes(plan.Nodes, completed)

		if len(readyNodes) == 0 {
			if len(completed) < totalNodes {
				taskWg.Wait()
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

		for _, node := range readyNodes {
			e.acquireSemaphore(ctx)

			taskWg.Add(1)
			go func(n types.TaskNode) {
				defer func() {
					e.releaseSemaphore()
					taskWg.Done()

					if r := recover(); r != nil {
						e.logger.Error("Panic recovered in DAG task goroutine",
							"task_id", n.Task.ID,
							"panic", r,
							"stack", string(debug.Stack()),
						)
						resultsMu.Lock()
						results[n.Task.ID] = types.Result{
							TaskID:  n.Task.ID,
							Success: false,
							Error:   fmt.Sprintf("panic recovered: %v", r),
						}
						completed[n.Task.ID] = false
						resultsMu.Unlock()
					}
				}()

				select {
				case <-ctx.Done():
					resultsMu.Lock()
					completed[n.Task.ID] = false
					results[n.Task.ID] = types.Result{
						TaskID:  n.Task.ID,
						Success: false,
						Error:   "cancelled",
					}
					resultsMu.Unlock()
					return
				case <-e.stopCh:
					resultsMu.Lock()
					completed[n.Task.ID] = false
					results[n.Task.ID] = types.Result{
						TaskID:  n.Task.ID,
						Success: false,
						Error:   "engine stopped",
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
	}

	taskWg.Wait()

	e.eventBus.Publish(events.Event{
		Type:   events.EventPlanCompleted,
		Source: "execution_engine",
		Payload: map[string]any{
			"plan_id":       plan.ID,
			"results_count": len(results),
		},
	})

	e.logger.Info("DAG plan execution completed",
		"plan_id", plan.ID,
		"results", len(results),
	)

	return e.collectResults(plan.Nodes, results), nil
}

func (e *Engine) acquireSemaphore(ctx context.Context) {
	select {
	case e.semaphore <- struct{}{}:
	case <-ctx.Done():
	case <-e.stopCh:
	}
}

func (e *Engine) releaseSemaphore() {
	select {
	case <-e.semaphore:
	default:
	}
}

func (e *Engine) collectResults(nodes []types.TaskNode, results map[string]types.Result) []types.Result {
	orderedResults := make([]types.Result, 0, len(nodes))
	for _, node := range nodes {
		if result, ok := results[node.Task.ID]; ok {
			orderedResults = append(orderedResults, result)
		}
	}
	return orderedResults
}

func (e *Engine) findReadyNodes(nodes []types.TaskNode, completed map[string]bool) []types.TaskNode {
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

func (e *Engine) detectCircularDeps(nodes []types.TaskNode) error {
	adj := make(map[string][]string)
	nodeIDs := make(map[string]bool)
	for _, node := range nodes {
		nodeIDs[node.Task.ID] = true
		adj[node.Task.ID] = node.DependsOn
	}

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

func (e *Engine) ExecuteTask(ctx context.Context, task types.Task) (types.Result, error) {
	agent, exists := e.agents[task.AssignedAgent]
	if !exists {
		return types.Result{
			TaskID:  task.ID,
			Success: false,
			Error:   fmt.Sprintf("agent not found: %s", task.AssignedAgent),
		}, fmt.Errorf("agent not found: %s", task.AssignedAgent)
	}

	if task.MaxRetries == 0 {
		task.MaxRetries = e.config.MaxRetries
	}
	if task.Timeout == 0 {
		task.Timeout = e.config.DefaultTimeout
	}

	return e.executeWithRetry(ctx, task, agent)
}

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

	taskCtx, cancel := context.WithTimeout(ctx, task.Timeout)
	defer cancel()

	resultCh := make(chan types.Result, 1)
	errCh := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				e.logger.Error("Panic in agent execution",
					"task_id", task.ID,
					"agent", agent.Name(),
					"panic", r,
					"stack", string(debug.Stack()),
				)
				errCh <- fmt.Errorf("panic during agent execution: %v", r)
			}
		}()

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

func (e *Engine) Stop() {
	e.stopMu.Lock()
	if e.stopped {
		e.stopMu.Unlock()
		return
	}
	e.stopped = true
	close(e.stopCh)
	e.stopMu.Unlock()

	e.logger.Info("Engine stopped")
}
