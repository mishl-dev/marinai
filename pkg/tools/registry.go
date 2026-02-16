package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

func (r *Registry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[tool.Name()]; exists {
		return fmt.Errorf("tool %s already registered", tool.Name())
	}
	r.tools[tool.Name()] = tool
	return nil
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

func (r *Registry) Execute(ctx context.Context, name string, params map[string]any) (Result, error) {
	tool, ok := r.Get(name)
	if !ok {
		return Result{}, fmt.Errorf("tool %s not found", name)
	}
	return tool.Execute(ctx, params, nil)
}

func (r *Registry) GetToolDefinitions() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	definitions := make([]ToolDefinition, 0, len(r.tools))
	for _, tool := range r.tools {
		definitions = append(definitions, ToolDefinition{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  tool.Parameters(),
		})
	}
	return definitions
}

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  ParameterSchema `json:"parameters"`
}

func (r *Registry) ListTools() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

func ParseToolCall(data []byte) (name string, params map[string]any, err error) {
	var call struct {
		Name       string          `json:"name"`
		Parameters json.RawMessage `json:"parameters"`
	}
	if err := json.Unmarshal(data, &call); err != nil {
		return "", nil, fmt.Errorf("parse tool call: %w", err)
	}

	params = make(map[string]any)
	if len(call.Parameters) > 0 {
		if err := json.Unmarshal(call.Parameters, &params); err != nil {
			return "", nil, fmt.Errorf("parse parameters: %w", err)
		}
	}

	return call.Name, params, nil
}
