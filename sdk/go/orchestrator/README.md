# Go SDK

Go client library for AI Orchestrator HTTP API.

## Installation

```bash
go get ai_orchestrator/sdk/go/orchestrator
```

## Usage

```go
package main

import (
	"context"
	"fmt"
	"log"

	"ai_orchestrator/sdk/go/orchestrator"
)

func main() {
	client := orchestrator.NewClient(
		orchestrator.WithURL("http://localhost:8080"),
		orchestrator.WithAPIKey("your-api-key"),
	)

	ctx := context.Background()

	// Health check
	health, err := client.Health(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Status: %s, Version: %s\n", health.Status, health.Version)

	// Submit task
	resp, err := client.SubmitTask(ctx, &orchestrator.TaskRequest{
		Goal:           "Analyze the codebase",
		IdempotencyKey: "unique-key-123",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Results: %+v\n", resp.Results)

	// List tasks
	tasks, err := client.ListTasks(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Total tasks: %d\n", tasks.Count)

	// Get queue status
	status, err := client.GetQueueStatus(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Queue: pending=%d, in_flight=%d, dlq=%d\n",
		status.Pending, status.InFlight, status.DLQCount)
}
```

## API

| Method | Description |
|--------|-------------|
| `Health()` | Check server health |
| `SubmitTask(req)` | Submit a new task |
| `GetTask(id)` | Get task by ID |
| `ListTasks()` | List all tasks |
| `CancelTask(id)` | Cancel a task |
| `GetQueueStatus()` | Get queue statistics |
| `GetDLQ()` | Get Dead Letter Queue |https://github.com/vzx7/orchestrator-python.git