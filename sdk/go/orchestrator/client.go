// Package orchestrator provides a Go client for AI Orchestrator HTTP API.
//
// Installation:
//
//	go get ai_orchestrator/sdk/go/orchestrator
//
// Usage:
//
//	package main
//
//	import (
//		"context"
//		"fmt"
//		"log"
//
//	    "ai_orchestrator/sdk/go/orchestrator"
//	)
//
//	func main() {
//		client := orchestrator.NewClient(
//			orchestrator.WithURL("http://localhost:8080"),
//		)
//
//		ctx := context.Background()
//		health, err := client.Health(ctx)
//		if err != nil {
//			log.Fatal(err)
//		}
//		fmt.Printf("Status: %s\n", health.Status)
//	}
package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultTimeout = 30 * time.Second
	defaultURL    = "http://localhost:8080"
)

// Client for AI Orchestrator API.
type Client struct {
	httpClient *http.Client
	baseURL   string
	apiKey    string
}

// Option configures the Client.
type Option func(*Client)

// WithURL sets the base URL.
func WithURL(url string) Option {
	return func(c *Client) {
		if url != "" {
			c.baseURL = url
		}
	}
}

// WithAPIKey sets the API key for authentication.
func WithAPIKey(apiKey string) Option {
	return func(c *Client) {
		c.apiKey = apiKey
	}
}

// WithTimeout sets the request timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			c.httpClient.Timeout = timeout
		}
	}
}

// NewClient creates a new orchestrator client.
func NewClient(opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		baseURL: defaultURL,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// ============================================
// Request/Response Types
// ============================================

// TaskRequest represents a task creation request.
type TaskRequest struct {
	Goal           string         `json:"goal"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	Timeout        time.Duration  `json:"timeout,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// TaskResult represents a single task result.
type TaskResult struct {
	TaskID  string         `json:"task_id"`
	Success bool           `json:"success"`
	Output  map[string]any `json:"output,omitempty"`
	Error   string        `json:"error,omitempty"`
}

// TaskResponse represents the API response.
type TaskResponse struct {
	Status  string        `json:"status"`
	Results []TaskResult  `json:"results"`
}

// TaskInfo represents task metadata.
type TaskInfo struct {
	TaskID     string         `json:"task_id"`
	State      string         `json:"state"`
	Attempts   int           `json:"attempts"`
	LastError  string        `json:"last_error,omitempty"`
	WorkerID   string        `json:"worker_id,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
}

// TaskListResponse represents a list of tasks.
type TaskListResponse struct {
	Tasks []TaskInfo `json:"tasks"`
	Count int        `json:"count"`
}

// QueueStatus represents queue status.
type QueueStatus struct {
	Pending   int `json:"pending"`
	InFlight int `json:"in_flight"`
	DLQCount int `json:"dlq_count"`
}

// DLQEntry represents a Dead Letter Queue entry.
type DLQEntry struct {
	TaskID   string    `json:"task_id"`
	Attempt  int       `json:"attempt"`
	Reason   string    `json:"reason"`
	FailedAt time.Time `json:"failed_at"`
}

// DLQResponse represents DLQ API response.
type DLQResponse struct {
	Count   int        `json:"count"`
	Entries []DLQEntry `json:"entries"`
}

// HealthResponse represents health check response.
type HealthResponse struct {
	Status  string      `json:"status"`
	Version string      `json:"version"`
	Workers HealthWorkers `json:"workers"`
}

// HealthWorkers represents worker health info.
type HealthWorkers struct {
	Total   int `json:"total"`
	Healthy int `json:"healthy"`
}

// ============================================
// API Methods
// ============================================

// Health checks if the orchestrator is healthy.
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	var health HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &health, nil
}

// SubmitTask submits a new task for execution.
func (c *Client) SubmitTask(ctx context.Context, req *TaskRequest) (*TaskResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/tasks", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	c.addAuth(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("submit failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var taskResp TaskResponse
	if err := json.Unmarshal(respBody, &taskResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &taskResp, nil
}

// GetTask retrieves a task by ID.
func (c *Client) GetTask(ctx context.Context, taskID string) (*TaskInfo, error) {
	url := fmt.Sprintf("%s/v1/tasks/%s", c.baseURL, taskID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get task failed with status %d", resp.StatusCode)
	}

	var task TaskInfo
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &task, nil
}

// ListTasks retrieves all tasks.
func (c *Client) ListTasks(ctx context.Context) (*TaskListResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/tasks", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list tasks failed with status %d", resp.StatusCode)
	}

	var list TaskListResponse
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &list, nil
}

// GetQueueStatus retrieves queue status.
func (c *Client) GetQueueStatus(ctx context.Context) (*QueueStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/queue", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("queue status failed with status %d", resp.StatusCode)
	}

	var status QueueStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &status, nil
}

// GetDLQ retrieves Dead Letter Queue entries.
func (c *Client) GetDLQ(ctx context.Context) (*DLQResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/dlq", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dlq request failed with status %d", resp.StatusCode)
	}

	var dlq DLQResponse
	if err := json.NewDecoder(resp.Body).Decode(&dlq); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &dlq, nil
}

// CancelTask cancels a running task.
func (c *Client) CancelTask(ctx context.Context, taskID string) error {
	url := fmt.Sprintf("%s/v1/tasks/%s/cancel", c.baseURL, taskID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("cancel failed with status %d", resp.StatusCode)
	}

	return nil
}

// ============================================
// Helpers
// ============================================

func (c *Client) addAuth(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}