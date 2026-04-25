// CLI tool for AI Orchestrator.
//
// Usage:
//
//	orchestrator-cli run "goal"          # Submit a task
//	orchestrator-cli get <task-id>    # Get task status
//	orchestrator-cli list              # List all tasks
//	orchestrator-cli cancel <id>    # Cancel a task
//	orchestrator-cli queue          # Queue status
//	orchestrator-cli dlq            # Dead Letter Queue
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

var (
	addr    = flag.String("addr", "http://localhost:8080", "Server address")
	apiKey  = flag.String("api-key", "", "API key")
	helpFlag = flag.Bool("h", false, "Show help")
	httpClient = &http.Client{Timeout: 30 * time.Second}
)

func main() {
	flag.Parse()

	if *helpFlag || len(os.Args) < 2 {
		printHelp()
		os.Exit(0)
	}

	cmd := os.Args[1]
	_ = cmd // used in switch

	switch cmd {
	case "health":
		runHealth()
	case "run":
		if len(os.Args) < 3 {
			fmt.Println("Usage: orchestrator-cli run <goal>")
			os.Exit(1)
		}
		runTask(os.Args[2])
	case "get":
		if len(os.Args) < 3 {
			fmt.Println("Usage: orchestrator-cli get <task-id>")
			os.Exit(1)
		}
		getTask(os.Args[2])
	case "list":
		listTasks()
	case "cancel":
		if len(os.Args) < 3 {
			fmt.Println("Usage: orchestrator-cli cancel <task-id>")
			os.Exit(1)
		}
		cancelTask(os.Args[2])
	case "queue":
		queueStatus()
	case "dlq":
		dlqStatus()
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println("AI Orchestrator CLI")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Printf("  %s [options] <command> [args]\n", os.Args[0])
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  health              Check server health")
	fmt.Println("  run <goal>          Submit new task")
	fmt.Println("  list               List all tasks")
	fmt.Println("  get <id>           Get task status")
	fmt.Println("  cancel <id>        Cancel task")
	fmt.Println("  queue              Queue statistics")
	fmt.Println("  dlq                Dead Letter Queue")
	fmt.Println("")
	fmt.Println("Options:")
	fmt.Println("  -addr string       Server address (default \"http://localhost:8080\")")
	fmt.Println("  -api-key string    API key for auth")
	fmt.Println("  -h                Show help")
}

func request(method, path string, body []byte) ([]byte, error) {
	url := *addr + path
	var req *http.Request
	if body != nil {
		req, _ = http.NewRequest(method, url, bytes.NewReader(body))
	} else {
		req, _ = http.NewRequest(method, url, nil)
	}
	if *apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+*apiKey)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Workers struct {
		Total   int `json:"total"`
		Healthy int `json:"healthy"`
	} `json:"workers"`
}

func runHealth() {
	data, err := request("GET", "/health", nil)
	if err != nil {
		log.Fatal(err)
	}
	var resp HealthResponse
	json.Unmarshal(data, &resp)
	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Printf("Version: %s\n", resp.Version)
	fmt.Printf("Workers: %d/%d healthy\n", resp.Workers.Healthy, resp.Workers.Total)
}

type TaskRequest struct {
	Goal           string `json:"goal"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

func runTask(goal string) {
	body, _ := json.Marshal(TaskRequest{
		Goal:           goal,
		IdempotencyKey: fmt.Sprintf("cli-%d", time.Now().UnixNano()),
	})
	data, err := request("POST", "/v1/tasks", body)
	if err != nil {
		log.Fatal(err)
	}
	var resp map[string]any
	json.Unmarshal(data, &resp)
	fmt.Printf("Status: %s\n", resp["status"])
	if results, ok := resp["results"].([]any); ok {
		fmt.Printf("Results: %d\n", len(results))
		for i, r := range results {
			if m, ok := r.(map[string]any); ok {
				status := "SUCCESS"
				if !m["success"].(bool) {
					status = "FAILED"
				}
				fmt.Printf("  [%d] %s: %s\n", i+1, m["task_id"], status)
			}
		}
	}
}

func getTask(taskID string) {
	data, err := request("GET", "/v1/tasks/"+taskID, nil)
	if err != nil {
		log.Fatal(err)
	}
	var task map[string]any
	json.Unmarshal(data, &task)
	fmt.Printf("Task ID: %s\n", task["task_id"])
	fmt.Printf("State: %s\n", task["state"])
	fmt.Printf("Attempts: %d\n", task["attempts"])
	if e, ok := task["last_error"].(string); ok && e != "" {
		fmt.Printf("Last Error: %s\n", e)
	}
	if w, ok := task["worker_id"].(string); ok && w != "" {
		fmt.Printf("Worker: %s\n", w)
	}
}

func listTasks() {
	data, err := request("GET", "/v1/tasks", nil)
	if err != nil {
		log.Fatal(err)
	}
	var resp map[string]any
	json.Unmarshal(data, &resp)
	tasks := resp["tasks"].([]any)
	fmt.Printf("Total tasks: %d\n", len(tasks))
	for _, t := range tasks {
		m := t.(map[string]any)
		fmt.Printf("  %s [%s] attempts=%d\n", m["task_id"], m["state"], m["attempts"])
	}
}

func cancelTask(taskID string) {
	_, err := request("POST", "/v1/tasks/"+taskID+"/cancel", nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Cancelled: %s\n", taskID)
}

func queueStatus() {
	data, err := request("GET", "/v1/queue", nil)
	if err != nil {
		log.Fatal(err)
	}
	var resp map[string]any
	json.Unmarshal(data, &resp)
	fmt.Printf("Pending: %d\n", resp["pending"])
	fmt.Printf("In Flight: %d\n", resp["in_flight"])
	fmt.Printf("DLQ: %d\n", resp["dlq_count"])
}

func dlqStatus() {
	data, err := request("GET", "/v1/dlq", nil)
	if err != nil {
		log.Fatal(err)
	}
	var resp map[string]any
	json.Unmarshal(data, &resp)
	fmt.Printf("DLQ Count: %d\n", resp["count"])
	if entries, ok := resp["entries"].([]any); ok {
		for _, e := range entries {
			m := e.(map[string]any)
			fmt.Printf("  %s [attempt %d]: %s\n", m["task_id"], m["attempt"], m["reason"])
		}
	}
}