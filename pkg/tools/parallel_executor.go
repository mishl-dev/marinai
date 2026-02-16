package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type ToolResult struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Result  Result      `json:"result"`
	Error   error       `json:"error,omitempty"`
	Panic   interface{} `json:"panic,omitempty"`
	Success bool        `json:"success"`
}

type ParallelExecutorConfig struct {
	MaxConcurrency int           `json:"max_concurrency"`
	Timeout        time.Duration `json:"timeout"`
}

type ParallelExecutor struct {
	config ParallelExecutorConfig
	sem    chan struct{}
}

func NewParallelExecutor(config ParallelExecutorConfig) *ParallelExecutor {
	if config.MaxConcurrency <= 0 {
		config.MaxConcurrency = 10
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	return &ParallelExecutor{
		config: config,
		sem:    make(chan struct{}, config.MaxConcurrency),
	}
}

func NewParallelExecutorWithDefaults() *ParallelExecutor {
	return NewParallelExecutor(ParallelExecutorConfig{
		MaxConcurrency: 10,
		Timeout:        30 * time.Second,
	})
}

func (e *ParallelExecutor) ParallelExecute(ctx context.Context, calls []ToolCall, registry *Registry, toolCtx *ToolContext) []ToolResult {
	if len(calls) == 0 {
		return []ToolResult{}
	}

	results := make([]ToolResult, len(calls))
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, c ToolCall) {
			defer wg.Done()
			results[idx] = e.executeCall(ctx, c, registry, toolCtx)
		}(i, call)
	}

	wg.Wait()
	return results
}

func (e *ParallelExecutor) executeCall(ctx context.Context, call ToolCall, registry *Registry, toolCtx *ToolContext) ToolResult {
	result := ToolResult{
		ID:   call.ID,
		Name: call.Name,
	}

	select {
	case e.sem <- struct{}{}:
		defer func() { <-e.sem }()
	case <-ctx.Done():
		result.Error = ctx.Err()
		result.Success = false
		return result
	}

	defer func() {
		if r := recover(); r != nil {
			result.Panic = r
			result.Error = fmt.Errorf("panic recovered: %v", r)
			result.Success = false
		}
	}()

	callCtx, cancel := context.WithTimeout(ctx, e.config.Timeout)
	defer cancel()

	tool, ok := registry.Get(call.Name)
	if !ok {
		result.Error = fmt.Errorf("tool %s not found", call.Name)
		result.Success = false
		return result
	}

	toolResult, err := tool.Execute(callCtx, call.Arguments, toolCtx)
	if err != nil {
		result.Error = err
		result.Success = false
		return result
	}

	result.Result = toolResult
	result.Success = toolResult.Success
	return result
}

func (e *ParallelExecutor) ParallelExecuteWithTimeout(ctx context.Context, calls []ToolCall, registry *Registry, toolCtx *ToolContext, timeout time.Duration) []ToolResult {
	originalTimeout := e.config.Timeout
	e.config.Timeout = timeout
	defer func() { e.config.Timeout = originalTimeout }()

	return e.ParallelExecute(ctx, calls, registry, toolCtx)
}

func (e *ParallelExecutor) SetTimeout(timeout time.Duration) {
	e.config.Timeout = timeout
}

func (e *ParallelExecutor) SetMaxConcurrency(max int) {
	if max <= 0 {
		return
	}
	e.sem = make(chan struct{}, max)
	e.config.MaxConcurrency = max
}

func (e *ParallelExecutor) GetConfig() ParallelExecutorConfig {
	return e.config
}

func ParseToolCallFromJSON(id, name, argumentsJSON string) (ToolCall, error) {
	call := ToolCall{
		ID:   id,
		Name: name,
	}

	if argumentsJSON != "" {
		if err := json.Unmarshal([]byte(argumentsJSON), &call.Arguments); err != nil {
			return ToolCall{}, fmt.Errorf("parse arguments: %w", err)
		}
	}

	if call.Arguments == nil {
		call.Arguments = make(map[string]any)
	}

	return call, nil
}
