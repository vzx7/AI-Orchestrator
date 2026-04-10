// Package rpc defines the gRPC service layer for distributed worker communication.
//
// This package provides:
// - WorkerService interface (as if generated from protobuf)
// - Server implementation for workers
// - Client implementation for orchestrator
//
// Design decisions:
// - Service interface abstracts the transport for testability
// - Messages map directly to internal types via JSON
// - Context propagation for cancellation and timeouts
// - Direct function call transport for demo (swappable for real gRPC)
package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"ai-orchestrator/internal/types"
)

// TaskRequest represents a task execution request sent to a worker.
type TaskRequest struct {
	TaskID  string          `json:"task_id"`
	Agent   string          `json:"agent"`
	Payload json.RawMessage `json:"payload"` // serialized Task
}

// TaskResponse represents a task execution result from a worker.
type TaskResponse struct {
	Success  bool            `json:"success"`
	Result   json.RawMessage `json:"result,omitempty"`
	Error    string          `json:"error,omitempty"`
	WorkerID string          `json:"worker_id"`
}

// WorkerService defines the interface for remote task execution.
// This mirrors what would be generated from a .proto file.
type WorkerService interface {
	// ExecuteTask runs a task on the remote worker.
	ExecuteTask(ctx context.Context, req *TaskRequest) (*TaskResponse, error)
}

// TaskExecutor is the function signature for task execution on workers.
type TaskExecutor func(ctx context.Context, task types.Task) (types.Result, error)

// Server implements the gRPC server for a worker node.
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

// ExecuteTask handles a task execution request.
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

	// Execute the task
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

	// Serialize result
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

// Client implements the gRPC client for the orchestrator.
// In production, this would use google.golang.org/grpc.
// For the demo, it uses direct function call transport.
type Client struct {
	logger    *slog.Logger
	servers   map[string]*Server // workerID -> server (direct call for demo)
	addresses map[string]string  // workerID -> address (for future real gRPC)
}

// NewClient creates a new worker client.
func NewClient(logger *slog.Logger) *Client {
	return &Client{
		logger:    logger,
		servers:   make(map[string]*Server),
		addresses: make(map[string]string),
	}
}

// RegisterServer adds a worker server for direct call transport (demo mode).
func (c *Client) RegisterServer(workerID string, server *Server, address string) {
	c.servers[workerID] = server
	c.addresses[workerID] = address
}

// ExecuteTask calls a worker to execute a task.
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

	c.logger.Info("Calling worker",
		"worker_id", workerID,
		"task_id", task.ID,
		"address", c.addresses[workerID],
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
