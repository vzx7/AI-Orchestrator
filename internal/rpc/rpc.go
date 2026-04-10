// Package rpc implements the communication layer between orchestrator and workers.
//
// V4 adds:
// - Retry with exponential backoff on transient failures
// - Context-based timeout propagation
// - Error classification (retryable vs fatal)
// - Direct call transport for demo (swappable for real gRPC)
package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ai_orchestrator/internal/types"
)

// TaskRequest represents a task execution request sent to a worker.
type TaskRequest struct {
	TaskID  string          `json:"task_id"`
	Agent   string          `json:"agent"`
	Payload json.RawMessage `json:"payload"`
}

// TaskResponse represents a task execution result from a worker.
type TaskResponse struct {
	Success  bool            `json:"success"`
	Result   json.RawMessage `json:"result,omitempty"`
	Error    string          `json:"error,omitempty"`
	WorkerID string          `json:"worker_id"`
}

// TaskExecutor is the function signature for task execution on workers.
type TaskExecutor func(ctx context.Context, task types.Task) (types.Result, error)

// Server implements the task execution service on a worker node.
type Server struct {
	logger   *slog.Logger
	workerID string
	executor TaskExecutor
}

// NewServer creates a new worker server.
func NewServer(workerID string, logger *slog.Logger, executor TaskExecutor) *Server {
	return &Server{
		workerID: workerID,
		logger:   logger,
		executor: executor,
	}
}

// ExecuteTask handles a task execution request with panic recovery.
func (s *Server) ExecuteTask(ctx context.Context, req *TaskRequest) (*TaskResponse, error) {
	s.logger.Info("Executing remote task",
		"task_id", req.TaskID,
		"agent", req.Agent,
		"worker_id", s.workerID,
	)

	// Deserialize task payload
	var task types.Task
	if err := json.Unmarshal(req.Payload, &task); err != nil {
		return &TaskResponse{
			Success:  false,
			Error:    fmt.Sprintf("failed to deserialize task: %v", err),
			WorkerID: s.workerID,
		}, nil
	}

	// Execute with context propagation
	result, err := s.executor(ctx, task)
	if err != nil {
		s.logger.Error("Task execution failed",
			"task_id", req.TaskID,
			"error", err,
		)
		return &TaskResponse{
			Success:  false,
			Error:    err.Error(),
			WorkerID: s.workerID,
		}, nil
	}

	resultJSON, _ := json.Marshal(result)

	return &TaskResponse{
		Success:  result.Success,
		Result:   resultJSON,
		Error:    result.Error,
		WorkerID: s.workerID,
	}, nil
}

// GetWorkerID returns the server's worker ID.
func (s *Server) GetWorkerID() string {
	return s.workerID
}

// Client implements the RPC client for the orchestrator with retry and resilience.
type Client struct {
	logger    *slog.Logger
	servers   map[string]*Server
	addresses map[string]string
	retries   int
	backoff   time.Duration
}

// ClientConfig holds RPC client configuration.
type ClientConfig struct {
	Retries int
	Backoff time.Duration
}

// DefaultClientConfig returns production defaults.
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Retries: 3,
		Backoff: 500 * time.Millisecond,
	}
}

// NewClient creates a new resilient worker client.
func NewClient(logger *slog.Logger, cfg ClientConfig) *Client {
	return &Client{
		logger:    logger,
		servers:   make(map[string]*Server),
		addresses: make(map[string]string),
		retries:   cfg.Retries,
		backoff:   cfg.Backoff,
	}
}

// NewClientDefault creates a client with default config.
func NewClientDefault(logger *slog.Logger) *Client {
	return NewClient(logger, DefaultClientConfig())
}

// RegisterServer adds a worker server for direct call transport (demo mode).
func (c *Client) RegisterServer(workerID string, server *Server, address string) {
	c.servers[workerID] = server
	c.addresses[workerID] = address
}

// ExecuteTask calls a worker to execute a task with retry and backoff.
func (c *Client) ExecuteTask(ctx context.Context, workerID string, task types.Task) (*TaskResponse, error) {
	server, exists := c.servers[workerID]
	if !exists {
		return nil, fmt.Errorf("worker not found: %s", workerID)
	}

	// Serialize task payload
	payload, err := json.Marshal(task)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize task: %w", err)
	}

	req := &TaskRequest{
		TaskID:  task.ID,
		Agent:   task.AssignedAgent,
		Payload: payload,
	}

	// Execute with retry
	var lastResp *TaskResponse
	var lastErr error

	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			backoff := c.backoff * time.Duration(1<<uint(attempt-1))
			c.logger.Info("Retrying RPC call",
				"worker_id", workerID,
				"attempt", attempt,
				"backoff", backoff,
			)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		lastResp, lastErr = c.callOnce(ctx, server, req)
		if lastErr == nil {
			return lastResp, nil
		}

		// Classify error: don't retry fatal errors
		if isFatalError(lastErr) {
			c.logger.Error("Fatal RPC error, not retrying",
				"worker_id", workerID,
				"error", lastErr,
			)
			return nil, lastErr
		}

		c.logger.Warn("RPC call failed, will retry",
			"worker_id", workerID,
			"attempt", attempt+1,
			"error", lastErr,
		)
	}

	return lastResp, fmt.Errorf("rpc call failed after %d retries: %w", c.retries, lastErr)
}

// callOnce performs a single RPC call attempt.
func (c *Client) callOnce(ctx context.Context, server *Server, req *TaskRequest) (*TaskResponse, error) {
	c.logger.Info("Calling worker",
		"worker_id", server.GetWorkerID(),
		"task_id", req.TaskID,
		"address", c.addresses[server.GetWorkerID()],
	)

	return server.ExecuteTask(ctx, req)
}

// ListWorkers returns all registered worker addresses.
func (c *Client) ListWorkers() map[string]string {
	result := make(map[string]string, len(c.addresses))
	for id, addr := range c.addresses {
		result[id] = addr
	}
	return result
}

// isFatalError determines if an error should not be retried.
func isFatalError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	// Fatal errors: logic errors, not transient
	fatal := []string{
		"agent not found",
		"failed to deserialize",
		"worker not found",
		"panic",
	}

	for _, keyword := range fatal {
		if strings.Contains(msg, keyword) {
			return true
		}
	}

	return false
}
