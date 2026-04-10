// Package planner implements the dynamic planning module with DAG support.
//
// V2 changes:
// - Plans now use TaskNode with dependency information (DAG structure)
// - Added Replan method for adaptive plan adjustment
// - GeneratePlan produces DAG-structured plans instead of linear task lists
//
// The Planner uses an LLM (mock for now) to decompose high-level user goals
// into structured, executable tasks with proper dependency ordering.
package planner

import (
	"fmt"
	"log/slog"
	"time"

	"ai_orchestrator/internal/events"
	"ai_orchestrator/internal/types"
)

// Planner generates and adjusts structured execution plans.
type Planner struct {
	logger   *slog.Logger
	eventBus *events.EventBus
}

// NewPlanner creates a new Planner instance.
func NewPlanner(logger *slog.Logger, eventBus *events.EventBus) *Planner {
	return &Planner{
		logger:   logger,
		eventBus: eventBus,
	}
}

// GeneratePlan takes a user goal and returns a DAG-structured execution plan.
//
// V2: Returns Plan with TaskNode array instead of flat Task array.
// Tasks are ordered with dependency information to enable parallel execution
// of independent nodes.
func (p *Planner) GeneratePlan(goal string) (types.Plan, error) {
	p.logger.Info("Generating DAG plan", "goal", goal)

	nodes := p.decomposeGoalToDAG(goal)
	if len(nodes) == 0 {
		return types.Plan{}, fmt.Errorf("unable to generate tasks for goal: %s", goal)
	}

	plan := types.Plan{
		ID:        fmt.Sprintf("plan-%d", time.Now().UnixNano()),
		Goal:      goal,
		Nodes:     nodes,
		Dynamic:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	p.eventBus.Publish(events.Event{
		Type:   events.EventPlanCreated,
		Source: "planner",
		Payload: map[string]any{
			"plan_id":    plan.ID,
			"node_count": len(plan.Nodes),
			"goal":       goal,
		},
	})

	p.logger.Info("DAG plan generated",
		"plan_id", plan.ID,
		"node_count", len(plan.Nodes),
	)

	return plan, nil
}

// Replan adjusts an existing plan based on a failed task and its evaluation.
//
// Strategy:
// 1. Remove the failed task if it's not retryable
// 2. Insert recovery tasks before dependent nodes
// 3. Update dependencies to maintain DAG integrity
//
// This enables adaptive orchestration: the plan evolves based on feedback.
func (p *Planner) Replan(
	currentPlan types.Plan,
	failedTask types.Task,
	eval types.Evaluation,
) types.Plan {
	if !currentPlan.Dynamic {
		p.logger.Warn("Attempted to replan a non-dynamic plan", "plan_id", currentPlan.ID)
		return currentPlan
	}

	p.logger.Info("Replanning",
		"plan_id", currentPlan.ID,
		"failed_task", failedTask.ID,
		"evaluation_reason", eval.Reason,
	)

	newNodes := make([]types.TaskNode, 0, len(currentPlan.Nodes)+2)

	// Build new plan with adjustments
	for _, node := range currentPlan.Nodes {
		// Skip the failed task
		if node.Task.ID == failedTask.ID {
			p.logger.Info("Removing failed task from plan", "task_id", failedTask.ID)
			continue
		}

		// Update dependencies that pointed to the failed task
		updatedDeps := make([]string, 0, len(node.DependsOn))
		for _, dep := range node.DependsOn {
			if dep != failedTask.ID {
				updatedDeps = append(updatedDeps, dep)
			}
		}
		node.DependsOn = updatedDeps
		newNodes = append(newNodes, node)
	}

	// Insert recovery/alternative tasks
	recoveryTask := types.Task{
		ID:            fmt.Sprintf("task-recovery-%d", time.Now().UnixNano()),
		Goal:          "analyze_failure",
		AssignedAgent: "ops",
		Status:        types.TaskStatusPending,
		MaxRetries:    1,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Recovery task has no dependencies (can run immediately)
	newNodes = append(newNodes, types.TaskNode{
		Task:      recoveryTask,
		DependsOn: []string{},
	})

	// Find tasks that depended on the failed task and make them depend on recovery instead
	for i := range newNodes {
		// These tasks originally depended on the failed task; redirect to recovery
		// (already removed failed task from deps above)
		// If they had NO other deps, make them depend on recovery
		if len(newNodes[i].DependsOn) == 0 && newNodes[i].Task.ID != recoveryTask.ID {
			// Only redirect QA-type tasks (they need the dev work to be done first)
			if newNodes[i].Task.AssignedAgent == "qa" {
				newNodes[i].DependsOn = append(newNodes[i].DependsOn, recoveryTask.ID)
			}
		}
	}

	updatedPlan := types.Plan{
		ID:        fmt.Sprintf("plan-replan-%d", time.Now().UnixNano()),
		Goal:      currentPlan.Goal,
		Nodes:     newNodes,
		Dynamic:   true,
		CreatedAt: currentPlan.CreatedAt,
		UpdatedAt: time.Now(),
	}

	p.logger.Info("Replan completed",
		"new_plan_id", updatedPlan.ID,
		"node_count", len(updatedPlan.Nodes),
	)

	return updatedPlan
}

// decomposeGoalToDAG maps a high-level goal to a DAG of TaskNodes.
//
// Dependency structure for "Fix failing test and deploy service":
//
//	[DevAgent: fix_test] → [QAAgent: run_tests] → [OpsAgent: deploy]
//
// Independent tasks (no deps) can execute in parallel.
func (p *Planner) decomposeGoalToDAG(goal string) []types.TaskNode {
	var nodes []types.TaskNode
	now := time.Now()

	// Identify goal keywords
	var hasFixTest, hasDeploy bool

	if containsKeyword(goal, "fix", "test") {
		hasFixTest = true
	}
	if containsKeyword(goal, "deploy", "release", "service") {
		hasDeploy = true
	}

	var lastTaskID string

	// Task 1: DevAgent fixes test (no dependencies)
	if hasFixTest {
		taskID := fmt.Sprintf("task-dev-%d", now.UnixNano())
		nodes = append(nodes, types.TaskNode{
			Task: types.Task{
				ID:            taskID,
				Goal:          "fix_test",
				AssignedAgent: "dev",
				Status:        types.TaskStatusPending,
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			DependsOn: []string{},
		})
		lastTaskID = taskID
	}

	// Task 2: QAAgent runs tests (depends on dev fix)
	if hasFixTest {
		qaTaskID := fmt.Sprintf("task-qa-%d", now.UnixNano()+1)
		nodes = append(nodes, types.TaskNode{
			Task: types.Task{
				ID:            qaTaskID,
				Goal:          "run_tests",
				AssignedAgent: "qa",
				Status:        types.TaskStatusPending,
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			DependsOn: []string{lastTaskID},
		})
		lastTaskID = qaTaskID
	}

	// Task 3: OpsAgent deploys (depends on QA passing)
	if hasDeploy {
		nodes = append(nodes, types.TaskNode{
			Task: types.Task{
				ID:            fmt.Sprintf("task-deploy-%d", now.UnixNano()+2),
				Goal:          "deploy",
				AssignedAgent: "ops",
				Status:        types.TaskStatusPending,
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			DependsOn: []string{lastTaskID},
		})
	}

	// Fallback: generic task if no specific pattern matched
	if len(nodes) == 0 {
		nodes = append(nodes, types.TaskNode{
			Task: types.Task{
				ID:            fmt.Sprintf("task-generic-%d", now.UnixNano()),
				Goal:          "analyze",
				AssignedAgent: "dev",
				Status:        types.TaskStatusPending,
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			DependsOn: []string{},
		})
	}

	return nodes
}

// containsKeyword checks if the goal contains any of the given keywords.
func containsKeyword(goal string, keywords ...string) bool {
	for _, keyword := range keywords {
		if len(goal) >= len(keyword) {
			for i := 0; i <= len(goal)-len(keyword); i++ {
				if goal[i:i+len(keyword)] == keyword {
					return true
				}
			}
		}
	}
	return false
}

// UpdatePlan allows dynamic modification of an existing plan.
func (p *Planner) UpdatePlan(plan *types.Plan, newNodes []types.TaskNode) {
	if !plan.Dynamic {
		p.logger.Warn("Attempted to update a non-dynamic plan", "plan_id", plan.ID)
		return
	}

	plan.Nodes = append(plan.Nodes, newNodes...)
	plan.UpdatedAt = time.Now()
	p.logger.Info("Plan updated dynamically",
		"plan_id", plan.ID,
		"new_node_count", len(plan.Nodes),
	)
}
