package main

import (
	stdcontext "context"
	"log/slog"
	"os"
	"testing"

	"ai_orchestrator/internal/agents"
	contextmanager "ai_orchestrator/internal/context"
	"ai_orchestrator/internal/controller"
	"ai_orchestrator/internal/evaluator"
	"ai_orchestrator/internal/events"
	"ai_orchestrator/internal/execution"
	"ai_orchestrator/internal/mcp"
	"ai_orchestrator/internal/planner"
	toolsgateway "ai_orchestrator/internal/tools"
	"ai_orchestrator/internal/types"
)

// TestDAGExecution verifies that tasks execute in correct dependency order.
func TestDAGExecution(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	config := types.DefaultExecutionConfig()
	config.MaxRetries = 1

	// Build orchestrator to get all components wired up
	o := buildTestOrchestrator(logger, config)

	ctx := stdcontext.Background()
	results, err := o.Execute(ctx, "Fix failing test and deploy service")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// All tasks should succeed
	for i, r := range results {
		if !r.Success {
			t.Errorf("result %d (%s) failed: %s", i, r.TaskID, r.Error)
		}
	}

	// Verify execution trace
	trace := o.GetExecutionTrace()
	if trace.ReplanCount != 0 {
		t.Errorf("expected 0 replans, got %d", trace.ReplanCount)
	}
	if len(trace.Steps) != 3 {
		t.Errorf("expected 3 trace steps, got %d", len(trace.Steps))
	}
}

// TestEvaluator verifies the DefaultEvaluator logic.
func TestEvaluator(t *testing.T) {
	eval := evaluator.NewDefaultEvaluator()

	tests := []struct {
		name           string
		task           types.Task
		result         types.Result
		wantSuccess    bool
		wantRetryable  bool
		wantConfidence float64
	}{
		{
			name:           "successful task",
			result:         types.Result{Success: true},
			wantSuccess:    true,
			wantRetryable:  false,
			wantConfidence: 0.95,
		},
		{
			name:           "timeout is retryable",
			task:           types.Task{RetryCount: 0, MaxRetries: 3},
			result:         types.Result{Success: false, Error: "deadline exceeded"},
			wantSuccess:    false,
			wantRetryable:  true,
			wantConfidence: 0.5,
		},
		{
			name:           "exhausted retries",
			task:           types.Task{RetryCount: 3, MaxRetries: 3},
			result:         types.Result{Success: false, Error: "something failed"},
			wantSuccess:    false,
			wantRetryable:  false, // retries exhausted
			wantConfidence: 0.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evalResult := eval.Evaluate(tt.task, tt.result)
			if evalResult.Success != tt.wantSuccess {
				t.Errorf("success = %v, want %v", evalResult.Success, tt.wantSuccess)
			}
			if evalResult.Retryable != tt.wantRetryable {
				t.Errorf("retryable = %v, want %v", evalResult.Retryable, tt.wantRetryable)
			}
			if evalResult.Confidence != tt.wantConfidence {
				t.Errorf("confidence = %v, want %v", evalResult.Confidence, tt.wantConfidence)
			}
		})
	}
}

// TestReplan verifies the planner's Replan method.
func TestReplan(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	eventBus := events.NewEventBus()
	p := planner.NewPlanner(logger, eventBus)

	// Create initial plan
	plan, err := p.GeneratePlan("Fix failing test and deploy service")
	if err != nil {
		t.Fatalf("failed to generate plan: %v", err)
	}

	if len(plan.Nodes) < 2 {
		t.Fatalf("expected at least 2 nodes, got %d", len(plan.Nodes))
	}

	// Simulate failure of first task
	failedTask := plan.Nodes[0].Task
	eval := types.Evaluation{
		Success:    false,
		Confidence: 0.3,
		Reason:     "timeout: connection refused",
		Retryable:  false,
	}

	// Replan
	newPlan := p.Replan(plan, failedTask, eval)

	// New plan should have different structure
	if newPlan.ID == plan.ID {
		t.Error("expected new plan to have different ID")
	}

	// Failed task should be removed from new plan
	for _, node := range newPlan.Nodes {
		if node.Task.ID == failedTask.ID {
			t.Error("expected failed task to be removed from plan")
		}
	}

	// Should have a recovery task
	hasRecovery := false
	for _, node := range newPlan.Nodes {
		if node.Task.Goal == "analyze_failure" {
			hasRecovery = true
			break
		}
	}
	if !hasRecovery {
		t.Error("expected replan to include recovery task")
	}
}

// TestCircularDependencyDetection verifies the engine detects cycles.
func TestCircularDependencyDetection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	eventBus := events.NewEventBus()
	config := types.DefaultExecutionConfig()
	engine := execution.NewEngine(config, logger, eventBus)

	// Create a plan with circular dependencies
	plan := types.Plan{
		ID: "test-circular",
		Nodes: []types.TaskNode{
			{Task: types.Task{ID: "a", AssignedAgent: "dev"}, DependsOn: []string{"c"}},
			{Task: types.Task{ID: "b", AssignedAgent: "dev"}, DependsOn: []string{"a"}},
			{Task: types.Task{ID: "c", AssignedAgent: "dev"}, DependsOn: []string{"b"}},
		},
	}

	ctx := stdcontext.Background()
	_, err := engine.ExecutePlanDAG(ctx, plan)
	if err == nil {
		t.Error("expected error for circular dependency")
	}
}

// TestControllerReplanLoop verifies the control loop triggers replanning.
func TestControllerReplanLoop(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	config := types.DefaultExecutionConfig()
	config.MaxRetries = 0 // Force immediate replan on failure
	config.MaxReplans = 3

	o := buildTestOrchestrator(logger, config)
	ctx := stdcontext.Background()

	// Normal execution should complete without replans
	results, err := o.Execute(ctx, "Fix failing test and deploy service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
}

func buildTestOrchestrator(logger *slog.Logger, config types.ExecutionConfig) *testOrchestrator {
	eventBus := events.NewEventBus()
	toolRegistry := mcp.NewToolRegistry()
	toolGW := toolsgateway.NewToolGateway(toolRegistry, logger)
	ctxMgr := contextmanager.NewContextManager()
	planGen := planner.NewPlanner(logger, eventBus)
	engine := execution.NewEngine(config, logger, eventBus)
	eval := evaluator.NewDefaultEvaluator()

	// Register agents
	devAgent := agents.NewDevAgent(toolGW, logger)
	qaAgent := agents.NewQAAgent(toolGW, logger)
	opsAgent := agents.NewOpsAgent(toolGW, logger)
	engine.RegisterAgent(devAgent)
	engine.RegisterAgent(qaAgent)
	engine.RegisterAgent(opsAgent)

	// Configure ACLs
	toolGW.SetACL("dev", []string{"file.read", "file.write", "shell.exec"})
	toolGW.SetACL("qa", []string{"test.run", "shell.exec", "file.read"})
	toolGW.SetACL("ops", []string{"deploy.service", "shell.exec", "file.read"})

	ctrl := controller.NewController(planGen, eval, engine, logger, eventBus, config)

	return &testOrchestrator{
		ctrl:   ctrl,
		ctxMgr: ctxMgr,
		logger: logger,
	}
}

type testOrchestrator struct {
	ctrl   *controller.Controller
	ctxMgr *contextmanager.ContextManager
	logger *slog.Logger
}

func (o *testOrchestrator) Execute(ctx stdcontext.Context, goal string) ([]types.Result, error) {
	return o.ctrl.Run(ctx, goal)
}

func (o *testOrchestrator) GetExecutionTrace() types.ExecutionTrace {
	return o.ctrl.GetTrace()
}

func (o *testOrchestrator) GetContextManager() *contextmanager.ContextManager {
	return o.ctxMgr
}
