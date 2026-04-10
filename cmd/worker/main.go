// Package main is the entry point for a distributed Worker Node.
//
// Workers:
// - Register with the orchestrator's task queue
// - Execute tasks via local agents
// - Return results via RPC
//
// Usage:
//
//	go run ./cmd/worker --id=worker-1 --addr=localhost:50051
//	go run ./cmd/worker --id=worker-2 --addr=localhost:50052
//
// In production, workers would connect to a shared message queue
// and gRPC service registry.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"ai_orchestrator/internal/agents"
	"ai_orchestrator/internal/mcp"
	toolsgateway "ai_orchestrator/internal/tools"
	"ai_orchestrator/internal/worker"
)

func main() {
	workerID := flag.String("id", "worker-1", "Unique worker identifier")
	addr := flag.String("addr", "localhost:50051", "Worker listen address")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	logger.Info("=== Worker Node Starting ===",
		"id", *workerID,
		"addr", *addr,
	)

	// Create tool gateway for agent tool access
	toolRegistry := mcp.NewToolRegistry()
	toolGW := toolsgateway.NewToolGateway(toolRegistry, logger)

	// Create worker with config
	wCfg := worker.DefaultWorkerConfig()
	w := worker.NewWorker(*workerID, logger, toolGW, wCfg)

	// Register agents with tool access
	devAgent := agents.NewDevAgent(toolGW, logger)
	qaAgent := agents.NewQAAgent(toolGW, logger)
	opsAgent := agents.NewOpsAgent(toolGW, logger)

	w.RegisterAgent(devAgent)
	w.RegisterAgent(qaAgent)
	w.RegisterAgent(opsAgent)

	// Configure ACLs for agents on this worker
	toolGW.SetACL("dev", []string{"file.read", "file.write", "shell.exec"})
	toolGW.SetACL("qa", []string{"test.run", "shell.exec", "file.read"})
	toolGW.SetACL("ops", []string{"deploy.service", "shell.exec", "file.read"})

	logger.Info("Worker ready",
		"id", *workerID,
		"agents", w.GetAgentNames(),
	)

	// Wait for shutdown signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("Worker running. Press Ctrl+C to stop.")

	go func() {
		<-sigCh
		logger.Info("Shutdown signal received", "id", *workerID)
		cancel()
	}()

	<-ctx.Done()
	logger.Info("Worker shutting down...", "id", *workerID)

	// Graceful shutdown
	// In production, this would:
	// 1. Stop accepting new tasks
	// 2. Wait for in-flight tasks to complete
	// 3. Deregister from service discovery
	// 4. Close gRPC listener

	logger.Info("Worker stopped", "id", *workerID)
}
