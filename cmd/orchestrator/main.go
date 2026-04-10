// Package main is the entry point for the AI Orchestrator CLI (V3).
//
// V3 Demo: Distributed orchestration with worker nodes.
//
// This demo runs in two modes:
// 1. Local mode (default) — all agents run in-process
// 2. Distributed mode — workers registered and tasks dispatched remotely
//
// For a full distributed demo, run the worker process separately:
//
//	# Terminal 1: Start workers
//	go run ./cmd/worker --id=worker-1 --addr=localhost:50051
//	go run ./cmd/worker --id=worker-2 --addr=localhost:50052
//
//	# Terminal 2: Start orchestrator
//	go run ./cmd/orchestrator --distributed
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"ai-orchestrator/internal/orchestrator"
	"ai-orchestrator/internal/rpc"
	"ai-orchestrator/internal/types"
)

func main() {
	distributed := flag.Bool("distributed", false, "Enable distributed execution mode")
	flag.Parse()

	// Configure structured logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	printBanner(logger, *distributed)

	// Create orchestrator with V3 distributed support
	config := types.DefaultExecutionConfig()
	config.DefaultTimeout = 10 * time.Second
	config.MaxRetries = 2
	config.RetryBackoffBase = 500 * time.Millisecond
	config.MaxReplans = 3

	o := orchestrator.NewOrchestrator(logger, config)

	if *distributed {
		// In a real distributed setup, workers would be separate processes.
		// For this demo, we create workers in-process and connect via RPC.
		setupDistributedWorkers(o, logger)
		o.EnableDistributedMode()
	}

	// Demo scenario
	goal := "Fix failing test and deploy service"
	logger.Info("User goal", "goal", goal)
	fmt.Println()

	ctx := context.Background()

	// Execute the goal
	results, err := o.Execute(ctx, goal)

	// Print results summary
	printResults(logger, results)

	// Print execution trace
	printTrace(logger, o.GetExecutionTrace())

	// Print distributed state if in distributed mode
	if o.IsDistributed() {
		printDistributedState(logger, o)
	}

	// Final status
	if err != nil {
		logger.Error("Orchestration finished with errors", "error", err)
		os.Exit(1)
	}

	logger.Info("=== V3 Demo Complete ===")
}

// setupDistributedWorkers creates in-process workers for the demo.
// In production, workers would be separate processes with real gRPC.
func setupDistributedWorkers(o *orchestrator.Orchestrator, logger *slog.Logger) {
	// In a real distributed setup, workers would be separate processes.
	// For this demo, we create workers in-process and connect via RPC.

	// For demo purposes, we create RPC servers that use the orchestrator's agents
	// This simulates remote execution while keeping the demo self-contained
	srv1 := rpc.NewServer("worker-1", logger, func(ctx context.Context, task types.Task) (types.Result, error) {
		// This would normally delegate to local agents on the worker
		// For demo, we just log and return a mock result
		logger.Info("Worker-1 executing task", "task_id", task.ID, "agent", task.AssignedAgent)
		return types.Result{
			TaskID:  task.ID,
			Success: true,
			Output:  map[string]any{"executed_by": "worker-1", "agent": task.AssignedAgent},
		}, nil
	})

	srv2 := rpc.NewServer("worker-2", logger, func(ctx context.Context, task types.Task) (types.Result, error) {
		logger.Info("Worker-2 executing task", "task_id", task.ID, "agent", task.AssignedAgent)
		return types.Result{
			TaskID:  task.ID,
			Success: true,
			Output:  map[string]any{"executed_by": "worker-2", "agent": task.AssignedAgent},
		}, nil
	})

	// Register workers
	o.RegisterWorker("worker-1", "localhost:50051")
	o.RegisterWorker("worker-2", "localhost:50052")

	// Connect RPC servers (demo mode: direct call transport)
	o.RegisterWorkerServer("worker-1", srv1)
	o.RegisterWorkerServer("worker-2", srv2)

	logger.Info("Distributed workers initialized",
		"worker_count", 2,
		"mode", "in-process demo",
	)
}

func printBanner(logger *slog.Logger, distributed bool) {
	mode := "Local Mode"
	if distributed {
		mode = "Distributed Mode"
	}
	logger.Info("===========================================")
	logger.Info(fmt.Sprintf("   AI Orchestrator V3 — %s", mode))
	logger.Info("   Worker Nodes + Task Queue + RPC")
	logger.Info("===========================================")
}

func printResults(logger *slog.Logger, results []types.Result) {
	fmt.Println()
	logger.Info("=== Execution Results ===")
	for i, result := range results {
		status := "SUCCESS"
		if !result.Success {
			status = "FAILED"
		}
		workerID := ""
		if result.Metadata != nil {
			if wid, ok := result.Metadata["worker_id"].(string); ok {
				workerID = wid
			}
		}
		logger.Info(fmt.Sprintf("Result %d", i+1),
			"task_id", result.TaskID,
			"status", status,
			"worker_id", workerID,
			"duration_ms", result.Duration.Milliseconds(),
			"output", result.Output,
		)
		if result.Error != "" {
			logger.Error(fmt.Sprintf("Error %d", i+1),
				"task_id", result.TaskID,
				"error", result.Error,
			)
		}
	}
}

func printTrace(logger *slog.Logger, trace types.ExecutionTrace) {
	fmt.Println()
	logger.Info("=== Execution Trace ===")
	logger.Info(trace.Summary())

	for i, step := range trace.Steps {
		evalStatus := "✓"
		if !step.Success {
			evalStatus = "✗"
		}
		logger.Info(fmt.Sprintf("Step %d", i+1),
			"task_id", step.TaskID,
			"agent", step.Agent,
			"worker_id", step.WorkerID,
			"status", evalStatus,
			"confidence", step.Evaluation.Confidence,
			"eval_reason", step.Evaluation.Reason,
			"retries", step.Retries,
			"duration_ms", step.EndTime.Sub(step.StartTime).Milliseconds(),
		)
	}

	if trace.ReplanCount > 0 {
		logger.Info("Replans occurred", "count", trace.ReplanCount)
	}
}

func printDistributedState(logger *slog.Logger, o *orchestrator.Orchestrator) {
	fmt.Println()
	logger.Info("=== Distributed State ===")

	// Worker registry
	workers := o.GetWorkerRegistry().List()
	logger.Info("Registered workers", "count", len(workers))
	for _, w := range workers {
		logger.Info("  Worker",
			"id", w.ID,
			"address", w.Address,
			"capacity", w.Capacity,
		)
	}

	// Task states
	tracker := o.GetTaskTracker()
	counts := tracker.CountByStatus()
	logger.Info("Task states", "counts", counts)
}
