package orchestrator_test

import (
	"context"
	"testing"
	"time"

	"ai_orchestrator/sdk/go/orchestrator"
)

func TestClient_Options(t *testing.T) {
	client := orchestrator.NewClient(
		orchestrator.WithURL("http://localhost:9999"),
		orchestrator.WithAPIKey("test-key"),
		orchestrator.WithTimeout(5*time.Second),
	)

	if client == nil {
		t.Fatal("expected client to be non-nil")
	}
}

func TestClient_Health(t *testing.T) {
	client := orchestrator.NewClient(orchestrator.WithURL("http://localhost:8080"))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	health, err := client.Health(ctx)
	if err != nil {
		t.Logf("Health check failed (server may not be running): %v", err)
		return
	}

	if health.Status != "healthy" {
		t.Errorf("expected status healthy, got %s", health.Status)
	}

	if health.Version == "" {
		t.Error("expected version to be set")
	}
}

func TestClient_SubmitTask(t *testing.T) {
	client := orchestrator.NewClient(orchestrator.WithURL("http://localhost:8080"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.SubmitTask(ctx, &orchestrator.TaskRequest{
		Goal:           "test goal",
		IdempotencyKey: "test-key-" + time.Now().Format(time.RFC3339Nano),
	})

	if err != nil {
		t.Logf("SubmitTask failed (server may not be running): %v", err)
		return
	}

	if len(resp.Results) == 0 {
		t.Error("expected at least one result")
	}
}

func TestClient_ListTasks(t *testing.T) {
	client := orchestrator.NewClient(orchestrator.WithURL("http://localhost:8080"))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.ListTasks(ctx)
	if err != nil {
		t.Logf("ListTasks failed (server may not be running): %v", err)
		return
	}

	t.Logf("Found %d tasks", resp.Count)
}

func TestClient_GetQueueStatus(t *testing.T) {
	client := orchestrator.NewClient(orchestrator.WithURL("http://localhost:8080"))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	status, err := client.GetQueueStatus(ctx)
	if err != nil {
		t.Logf("GetQueueStatus failed (server may not be running): %v", err)
		return
	}

	t.Logf("Queue: pending=%d, in_flight=%d, dlq=%d", status.Pending, status.InFlight, status.DLQCount)
}