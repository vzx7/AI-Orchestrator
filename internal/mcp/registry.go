// Package mcp provides a mock implementation of MCP (Model Context Protocol) tools.
//
// In production, this module would handle real network communication with
// external MCP servers. For now, it simulates tool behavior to enable
// testing and development of the orchestration layer.
package mcp

import (
	"fmt"
	"time"
)

// Tool represents a mock MCP tool.
type Tool struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Parameters  []string `json:"parameters"`
	Handler     func(args map[string]any) (map[string]any, error)
}

// ToolRegistry manages available MCP tools.
type ToolRegistry struct {
	tools map[string]*Tool
}

// NewToolRegistry creates a new tool registry with mock MCP tools.
func NewToolRegistry() *ToolRegistry {
	r := &ToolRegistry{
		tools: make(map[string]*Tool),
	}
	r.registerMockTools()
	return r
}

func (r *ToolRegistry) registerMockTools() {
	// file.read mock
	r.tools["file.read"] = &Tool{
		Name:        "file.read",
		Description: "Reads content from a file",
		Parameters:  []string{"path"},
		Handler: func(args map[string]any) (map[string]any, error) {
			path, ok := args["path"].(string)
			if !ok {
				return nil, fmt.Errorf("file.read: missing or invalid 'path' parameter")
			}
			// Simulated file read
			return map[string]any{
				"content":    fmt.Sprintf("# Simulated content of %s\nThis is mock file data.", path),
				"path":       path,
				"size_bytes": 128,
			}, nil
		},
	}

	// file.write mock
	r.tools["file.write"] = &Tool{
		Name:        "file.write",
		Description: "Writes content to a file",
		Parameters:  []string{"path", "content"},
		Handler: func(args map[string]any) (map[string]any, error) {
			path, _ := args["path"].(string)
			content, _ := args["content"].(string)
			// Simulated file write
			return map[string]any{
				"path":        path,
				"bytes_written": len(content),
				"success":     true,
			}, nil
		},
	}

	// shell.exec mock
	r.tools["shell.exec"] = &Tool{
		Name:        "shell.exec",
		Description: "Executes a shell command",
		Parameters:  []string{"command"},
		Handler: func(args map[string]any) (map[string]any, error) {
			command, ok := args["command"].(string)
			if !ok {
				return nil, fmt.Errorf("shell.exec: missing or invalid 'command' parameter")
			}
			// Simulated command execution
			return map[string]any{
				"command":    command,
				"stdout":     fmt.Sprintf("Simulated output of: %s", command),
				"stderr":     "",
				"exit_code":  0,
				"duration_ms": 150,
			}, nil
		},
	}

	// test.run mock
	r.tools["test.run"] = &Tool{
		Name:        "test.run",
		Description: "Runs tests and returns results",
		Parameters:  []string{"package", "verbose"},
		Handler: func(args map[string]any) (map[string]any, error) {
			pkg, _ := args["package"].(string)
			if pkg == "" {
				pkg = "./..."
			}
			// Simulated test run
			return map[string]any{
				"package":    pkg,
				"passed":     42,
				"failed":     0,
				"skipped":    2,
				"duration_ms": 1250,
				"success":    true,
			}, nil
		},
	}

	// deploy.service mock
	r.tools["deploy.service"] = &Tool{
		Name:        "deploy.service",
		Description: "Deploys a service",
		Parameters:  []string{"service", "environment", "version"},
		Handler: func(args map[string]any) (map[string]any, error) {
			service, _ := args["service"].(string)
			env, _ := args["environment"].(string)
			if service == "" {
				service = "unknown"
			}
			if env == "" {
				env = "development"
			}
			// Simulated deployment
			return map[string]any{
				"service":      service,
				"environment":  env,
				"version":      "1.0.0-mock",
				"status":       "deployed",
				"timestamp":    time.Now().UTC().Format(time.RFC3339),
				"health_check": "passing",
			}, nil
		},
	}
}

// GetTool retrieves a tool by name from the registry.
func (r *ToolRegistry) GetTool(name string) (*Tool, error) {
	tool, exists := r.tools[name]
	if !exists {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	return tool, nil
}

// ExecuteTool executes a tool with the given arguments.
func (r *ToolRegistry) ExecuteTool(name string, args map[string]any) (map[string]any, error) {
	tool, err := r.GetTool(name)
	if err != nil {
		return nil, err
	}

	// Validate parameters
	if len(tool.Parameters) > 0 {
		for _, param := range tool.Parameters {
			if _, ok := args[param]; !ok {
				return nil, fmt.Errorf("tool %s: missing required parameter '%s'", name, param)
			}
		}
	}

	return tool.Handler(args)
}

// ListTools returns all registered tool names.
func (r *ToolRegistry) ListTools() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}
