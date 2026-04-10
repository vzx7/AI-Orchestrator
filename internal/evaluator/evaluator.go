// Package evaluator implements the feedback mechanism for the orchestration control loop.
//
// The Evaluator assesses task execution results and produces Evaluations that
// drive adaptive decision-making: whether to continue, retry, or replan.
//
// Design decisions:
// - Interface-based to allow swapping in ML-based or LLM-based evaluators later.
// - DefaultEvaluator uses simple heuristics (no LLM) for deterministic behavior.
// - Confidence scores enable nuanced decision-making beyond binary success/failure.
package evaluator

import (
	"strings"

	"ai-orchestrator/internal/types"
)

// Evaluator assesses task execution results and produces evaluations.
type Evaluator interface {
	// Evaluate analyzes a task result and returns an evaluation.
	Evaluate(task types.Task, result types.Result) types.Evaluation
}

// DefaultEvaluator implements heuristic-based evaluation without LLM calls.
//
// Evaluation logic:
// 1. If Result.Success is true → high confidence success
// 2. If Result.Success is false:
//   - Check if error suggests retryable condition (timeout, transient)
//   - Check retry count vs max retries
//   - Assign lower confidence for repeated failures
type DefaultEvaluator struct{}

// NewDefaultEvaluator creates a new heuristic-based evaluator.
func NewDefaultEvaluator() *DefaultEvaluator {
	return &DefaultEvaluator{}
}

// Evaluate analyzes a task result using heuristics.
func (e *DefaultEvaluator) Evaluate(task types.Task, result types.Result) types.Evaluation {
	if result.Success {
		return types.Evaluation{
			Success:    true,
			Confidence: 0.95,
			Reason:     "Task completed successfully",
			Retryable:  false,
		}
	}

	// Task failed — determine if retryable
	reason := result.Error
	retryable := e.isRetryable(reason)
	confidence := e.calculateFailureConfidence(task, result)

	return types.Evaluation{
		Success:    false,
		Confidence: confidence,
		Reason:     reason,
		Retryable:  retryable && task.RetryCount < task.MaxRetries,
	}
}

// isRetryable determines if an error suggests a retryable condition.
func (e *DefaultEvaluator) isRetryable(errMsg string) bool {
	lower := strings.ToLower(errMsg)

	// Transient errors are good candidates for retry
	transientKeywords := []string{
		"timeout",
		"deadline exceeded",
		"connection refused",
		"rate limit",
		"too many requests",
		"temporary",
		"unavailable",
	}

	for _, keyword := range transientKeywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}

	// Default: most failures might be retryable (optimistic)
	return true
}

// calculateFailureConfidence assigns a confidence score for failed tasks.
// Lower scores indicate higher likelihood that replanning (not retrying) is needed.
func (e *DefaultEvaluator) calculateFailureConfidence(task types.Task, result types.Result) float64 {
	baseConfidence := 0.5

	// Reduce confidence with each retry
	retryPenalty := float64(task.RetryCount) * 0.15
	confidence := baseConfidence - retryPenalty

	// If we've exhausted retries, very low confidence
	if task.RetryCount >= task.MaxRetries {
		confidence = 0.1
	}

	// Clamp to [0, 1]
	if confidence < 0.0 {
		confidence = 0.0
	}
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}
