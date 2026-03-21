package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"

	"github.com/peterwwillis/zop/internal/provider"
)

// Definition is the interface that a tool must implement.
type Definition interface {
	Name() string
	Description() string
	Parameters() interface{} // JSON schema
	Execute(ctx context.Context, args string) (string, error)
}

// Registry manages available tools.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Definition
}

// NewRegistry creates a new tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Definition),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool Definition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

// Get retrieves a tool by name.
func (r *Registry) Get(name string) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// List returns a list of all registered tools as provider.Tool.
func (r *Registry) List() []provider.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]provider.Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		list = append(list, provider.Tool{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  tool.Parameters(),
		})
	}
	return list
}

// RunCommandTool executes a shell command.
type RunCommandTool struct {
	Policy *PolicyChecker
}

func (t *RunCommandTool) Name() string {
	return "run_command"
}

func (t *RunCommandTool) Description() string {
	return "Execute a shell command. Returns the command output (stdout and stderr)."
}

func (t *RunCommandTool) Parameters() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The command to execute.",
			},
		},
		"required": []string{"command"},
	}
}

func (t *RunCommandTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", fmt.Errorf("unmarshaling arguments: %w", err)
	}

	if t.Policy != nil {
		if !t.Policy.IsAllowed(t.Name(), args) {
			return "", fmt.Errorf("command is denied by tool policy")
		}
	}

	// For safety, in a real tool we might want to ask the user, but for now we'll just execute.
	// Actually, the user asked for CLI tool calling support.
	cmd := exec.CommandContext(ctx, "sh", "-c", input.Command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("executing command: %w (output: %s)", err, string(output))
	}
	return string(output), nil
}
