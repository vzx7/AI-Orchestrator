// Package server provides HTTP API for the AI Orchestrator.
//
// Endpoints:
//   GET  /health           - Health check
//   GET  /v1/tasks         - List tasks
//   POST /v1/tasks        - Create task
//   GET  /v1/tasks/:id    - Get task status
//   POST /v1/tasks/:id/cancel - Cancel task
//   GET  /v1/queue        - Queue status
//   GET  /v1/dlq          - Dead Letter Queue
package server

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ai_orchestrator/internal/orchestrator"
	"ai_orchestrator/internal/rpc"
	"ai_orchestrator/internal/types"
)

func Run() {
	addr := flag.String("addr", ":8080", "HTTP server address")
	apiKey := flag.String("api-key", "", "API key for authentication")
	distributed := flag.Bool("distributed", false, "Enable distributed mode")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	config := types.DefaultExecutionConfig()
	config.DefaultTimeout = 30 * time.Second
	config.MaxRetries = 3

	o := orchestrator.NewOrchestrator(logger, config)

	if *distributed {
		setupDistributedDemo(o, logger)
		o.EnableDistributedMode()
	}

	handler := NewHandler(o, logger, *apiKey)
	mux := http.NewServeMux()
	handler.Register(mux)

	server := &http.Server{
		Addr:         *addr,
		Handler:      middlewareChain(mux, apiKeyAuth(*apiKey, logger)),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout: 60 * time.Second,
	}

	logger.Info("HTTP server starting", "addr", *addr)

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("Shutting down HTTP server...")
		server.Close()
	}()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		logger.Error("HTTP server error", "error", err)
	}

	logger.Info("HTTP server stopped")
}

func middlewareChain(h http.Handler, auth func(http.Handler) http.Handler) http.Handler {
	return auth(h)
}

func apiKeyAuth(apiKey string, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if apiKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get("Authorization")
			if key == "" {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}

			if key != fmt.Sprintf("Bearer %s", apiKey) {
				http.Error(w, "invalid api key", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func setupDistributedDemo(o *orchestrator.Orchestrator, logger *slog.Logger) {
	logger.Info("Setting up distributed demo with workers...")

	srv1 := rpc.NewServer("worker-1", logger, func(ctx context.Context, task types.Task) (types.Result, error) {
		logger.Info("Worker-1 executing task", "task_id", task.ID, "agent", task.AssignedAgent)
		return types.Result{
			TaskID:  task.ID,
			Success: true,
			Output: map[string]any{"executed_by": "worker-1", "agent": task.AssignedAgent},
		}, nil
	})

	srv2 := rpc.NewServer("worker-2", logger, func(ctx context.Context, task types.Task) (types.Result, error) {
		logger.Info("Worker-2 executing task", "task_id", task.ID, "agent", task.AssignedAgent)
		return types.Result{
			TaskID:  task.ID,
			Success: true,
			Output: map[string]any{"executed_by": "worker-2", "agent": task.AssignedAgent},
		}, nil
	})

	o.RegisterWorker("worker-1", "localhost:50051")
	o.RegisterWorker("worker-2", "localhost:50052")
	o.RegisterWorkerServer("worker-1", srv1)
	o.RegisterWorkerServer("worker-2", srv2)

	o.EnableDistributedMode()
	logger.Info("Distributed demo with workers ready")
}

// Handler handles HTTP requests.
type Handler struct {
	orch  *orchestrator.Orchestrator
	logger *slog.Logger
	apiKey string
}

// NewHandler creates a new HTTP handler.
func NewHandler(o *orchestrator.Orchestrator, logger *slog.Logger, apiKey string) *Handler {
	return &Handler{
		orch:  o,
		logger: logger,
		apiKey: apiKey,
	}
}

// Register registers HTTP routes.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/v1/tasks", h.tasks)
	mux.HandleFunc("/v1/tasks/", h.taskByID)
	mux.HandleFunc("/v1/queue", h.queueStatus)
	mux.HandleFunc("/v1/dlq", h.dlqStatus)
}

// health handles GET /health
func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workers := h.orch.GetWorkerRegistry().List()
	healthyCount := 0
	for _, w := range workers {
		if w.Healthy {
			healthyCount++
		}
	}

	jsonResponse(w, map[string]any{
		"status":  "healthy",
		"version": "5.0.0",
		"workers": map[string]int{
			"total":   len(workers),
			"healthy": healthyCount,
		},
	})
}

// tasks handles GET/POST /v1/tasks
func (h *Handler) tasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		states, _ := h.orch.GetTaskTracker().ListTaskStates()
		var taskList []map[string]any
		for _, s := range states {
			taskList = append(taskList, map[string]any{
				"task_id":    s.TaskID,
				"state":     s.State,
				"attempts":  s.Attempts,
				"created":   s.CreatedAt,
				"updated":   s.UpdatedAt,
			})
		}
		jsonResponse(w, map[string]any{"tasks": taskList, "count": len(taskList)})

	case http.MethodPost:
		var req struct {
			Goal           string         `json:"goal"`
			IdempotencyKey string         `json:"idempotency_key,omitempty"`
			Timeout       time.Duration  `json:"timeout,omitempty"`
			Metadata      map[string]any `json:"metadata,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
			return
		}

		if req.Goal == "" {
			http.Error(w, "goal is required", http.StatusBadRequest)
			return
		}

		results, err := h.orch.Execute(r.Context(), req.Goal)
		if err != nil {
			http.Error(w, fmt.Sprintf("execution failed: %v", err), http.StatusInternalServerError)
			return
		}

		var taskResults []map[string]any
		for _, res := range results {
			taskResults = append(taskResults, map[string]any{
				"task_id":  res.TaskID,
				"success": res.Success,
				"output":  res.Output,
			})
		}

		jsonResponse(w, map[string]any{
			"status":  "completed",
			"results": taskResults,
		})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// taskByID handles /v1/tasks/{id}
func (h *Handler) taskByID(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Path[len("/v1/tasks/"):]
	if taskID == "" {
		http.Error(w, "task id required", http.StatusBadRequest)
		return
	}

	states, _ := h.orch.GetTaskTracker().ListTaskStates()
	for _, s := range states {
		if s.TaskID == taskID {
			jsonResponse(w, map[string]any{
				"task_id":      s.TaskID,
				"state":       s.State,
				"attempts":    s.Attempts,
				"last_error":   s.LastError,
				"worker_id":  s.WorkerID,
				"created":    s.CreatedAt,
				"updated":    s.UpdatedAt,
			})
			return
		}
	}

	http.Error(w, "task not found", http.StatusNotFound)
}

// queueStatus handles GET /v1/queue
func (h *Handler) queueStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := h.orch.GetTaskQueue()
	if q == nil {
		jsonResponse(w, map[string]any{"error": "queue not available in local mode"})
		return
	}

	dlq := h.orch.GetDeadLetterQueue()

	jsonResponse(w, map[string]any{
		"pending":    q.Size(),
		"in_flight": q.InFlight(),
		"dlq_count": dlq.Count(),
	})
}

// dlqStatus handles GET /v1/dlq
func (h *Handler) dlqStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	dlq := h.orch.GetDeadLetterQueue()
	entries := dlq.Peek()

	var entryList []map[string]any
	for _, e := range entries {
		entryList = append(entryList, map[string]any{
			"task_id":    e.Message.TaskID,
			"attempt":   e.Message.Attempt,
			"reason":    e.FailReason,
			"failed_at": e.FailedAt,
		})
	}

	jsonResponse(w, map[string]any{
		"count":    dlq.Count(),
		"entries": entryList,
	})
}

func jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode response: %v\n", err)
	}
}