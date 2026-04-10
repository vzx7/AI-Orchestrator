// Package main is the entry point for the AI Orchestrator CLI (V2).
//
// V2 Demo: Adaptive orchestration with DAG execution and feedback loop.
//
// Input: "Fix failing test and deploy service"
//
// Expected DAG structure:
//
//	[DevAgent: fix_test] → [QAAgent: run_tests] → [DevAgent: deploy]
//
// Execution demonstrates:
// - Dependency-aware DAG scheduling
// - Evaluation after each task
// - Adaptive replanning on failure
// - Full execution trace output
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"ai-orchestrator/internal/orchestrator"
	"ai-orchestrator/internal/types"
)

func main() {
	// Configure structured logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	printBanner(logger)

	// Create orchestrator with V2 adaptive mode
	config := types.DefaultExecutionConfig()
	config.DefaultTimeout = 10 * time.Second
	config.MaxRetries = 2
	config.RetryBackoffBase = 500 * time.Millisecond
	config.MaxReplans = 3

	o := orchestrator.NewOrchestrator(logger, config)

	// Demo scenario
	goal := "Fix failing test and deploy service"
	logger.Info("User goal", "goal", goal)
	fmt.Println()

	ctx := context.Background()

	// Execute the goal via V2 adaptive control loop
	results, err := o.Execute(ctx, goal)

	// Print results summary
	printResults(logger, results)

	// Print execution trace
	printTrace(logger, o.GetExecutionTrace())

	// Print context summary
	printContext(logger, o.GetContextManager())

	// Final status
	if err != nil {
		logger.Error("Orchestration finished with errors", "error", err)
		os.Exit(1)
	}

	logger.Info("=== V2 Demo Complete ===")
}

func printBanner(logger *slog.Logger) {
	logger.Info("===========================================")
	logger.Info("   AI Orchestrator V2 — Adaptive Mode")
	logger.Info("   DAG Execution + Feedback Loop")
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
		logger.Info(fmt.Sprintf("Result %d", i+1),
			"task_id", result.TaskID,
			"status", status,
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

func printContext(logger *slog.Logger, ctxMgr interface{ Summarize(int) string }) {
	fmt.Println()
	logger.Info("=== Context Summary ===")
	logger.Info(ctxMgr.Summarize(10))
}
