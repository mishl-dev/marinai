package tools

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

type mockTool struct {
	name        string
	description string
	schema      ParameterSchema
	executeFunc func(ctx context.Context, params map[string]any, toolCtx *ToolContext) (Result, error)
}

func (t *mockTool) Name() string {
	return t.name
}

func (t *mockTool) Description() string {
	return t.description
}

func (t *mockTool) Parameters() ParameterSchema {
	return t.schema
}

func (t *mockTool) Execute(ctx context.Context, params map[string]any, toolCtx *ToolContext) (Result, error) {
	if t.executeFunc != nil {
		return t.executeFunc(ctx, params, toolCtx)
	}
	return Result{Success: true, Data: params}, nil
}

func TestControlFlowManager_ProcessToolCalls_SuccessfulValidationAndExecution(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&mockTool{
		name:        "test_tool",
		description: "A test tool",
		schema: ParameterSchema{
			Type: "object",
			Properties: map[string]PropertySchema{
				"name": {Type: "string"},
			},
			Required: []string{"name"},
		},
		executeFunc: func(ctx context.Context, params map[string]any, toolCtx *ToolContext) (Result, error) {
			return Result{Success: true, Data: params["name"]}, nil
		},
	})

	manager := NewControlFlowManagerWithDefaults()

	toolCalls := []ToolCall{
		{ID: "call_1", Name: "test_tool", Arguments: map[string]any{"name": "test"}},
	}

	toolCtx := &ToolContext{StartedAt: time.Now()}
	results := manager.ProcessToolCalls(context.Background(), toolCalls, registry, toolCtx)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if !results[0].Success {
		t.Errorf("expected success, got failure: %v", results[0].Error)
	}

	if results[0].Name != "test_tool" {
		t.Errorf("expected name 'test_tool', got '%s'", results[0].Name)
	}

	if results[0].Result.Data != "test" {
		t.Errorf("expected result data 'test', got '%v'", results[0].Result.Data)
	}

	if results[0].NeedsRepair {
		t.Error("expected no repair needed")
	}

	if len(results[0].ValidationErrors) > 0 {
		t.Errorf("unexpected validation errors: %v", results[0].ValidationErrors)
	}
}

func TestControlFlowManager_ProcessToolCalls_ValidationFailureTypeMismatch(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&mockTool{
		name:        "count_tool",
		description: "A tool that requires an integer",
		schema: ParameterSchema{
			Type: "object",
			Properties: map[string]PropertySchema{
				"count": {Type: "integer"},
			},
			Required: []string{"count"},
		},
	})

	manager := NewControlFlowManagerWithDefaults()
	manager.ResetAllRepairAttempts()

	toolCalls := []ToolCall{
		{ID: "call_1", Name: "count_tool", Arguments: map[string]any{"count": "not_a_number"}},
	}

	toolCtx := &ToolContext{StartedAt: time.Now()}
	results := manager.ProcessToolCalls(context.Background(), toolCalls, registry, toolCtx)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Success {
		t.Error("expected failure due to validation error")
	}

	if !results[0].NeedsRepair {
		t.Error("expected repair needed")
	}

	if len(results[0].ValidationErrors) == 0 {
		t.Error("expected validation errors")
	}

	foundTypeError := false
	for _, err := range results[0].ValidationErrors {
		if err.Code == "coercion_failed" || err.Code == "type_mismatch" {
			foundTypeError = true
			break
		}
	}

	if !foundTypeError {
		t.Errorf("expected type/coercion error, got: %v", results[0].ValidationErrors)
	}
}

func TestControlFlowManager_ProcessToolCalls_MissingRequiredField(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&mockTool{
		name:        "required_field_tool",
		description: "A tool with required fields",
		schema: ParameterSchema{
			Type: "object",
			Properties: map[string]PropertySchema{
				"required_field": {Type: "string"},
				"optional_field": {Type: "string"},
			},
			Required: []string{"required_field"},
		},
	})

	manager := NewControlFlowManagerWithDefaults()
	manager.ResetAllRepairAttempts()

	toolCalls := []ToolCall{
		{ID: "call_1", Name: "required_field_tool", Arguments: map[string]any{"optional_field": "value"}},
	}

	toolCtx := &ToolContext{StartedAt: time.Now()}
	results := manager.ProcessToolCalls(context.Background(), toolCalls, registry, toolCtx)

	if results[0].Success {
		t.Error("expected failure due to missing required field")
	}

	if !results[0].NeedsRepair {
		t.Error("expected repair needed")
	}

	if len(results[0].ValidationErrors) == 0 {
		t.Error("expected validation errors")
	}

	if results[0].ValidationErrors[0].Code != "required" {
		t.Errorf("expected 'required' error code, got '%s'", results[0].ValidationErrors[0].Code)
	}
}

func TestControlFlowManager_BuildRepairMessage(t *testing.T) {
	manager := NewControlFlowManagerWithDefaults()

	call := ToolCall{
		ID:   "call_1",
		Name: "test_tool",
		Arguments: map[string]any{
			"count": "invalid",
		},
	}

	validationErrors := ValidationErrors{
		{Field: "count", Message: "expected integer, got string", Code: "type_mismatch", Value: "invalid"},
		{Field: "name", Message: "required field is missing", Code: "required"},
	}

	message := manager.BuildRepairMessage(call, validationErrors)

	if message == "" {
		t.Error("expected non-empty repair message")
	}

	if !contains(message, "test_tool") {
		t.Error("expected message to contain tool name")
	}

	if !contains(message, "count") {
		t.Error("expected message to contain field 'count'")
	}

	if !contains(message, "name") {
		t.Error("expected message to contain field 'name'")
	}
}

func TestControlFlowManager_BuildDetailedRepairMessage(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&mockTool{
		name:        "detailed_tool",
		description: "A tool for detailed repair testing",
		schema: ParameterSchema{
			Type: "object",
			Properties: map[string]PropertySchema{
				"status": {
					Type:        "string",
					Description: "The status value",
					Enum:        []string{"active", "inactive"},
				},
				"count": {
					Type:        "integer",
					Description: "A count value",
				},
			},
			Required: []string{"status", "count"},
		},
	})

	manager := NewControlFlowManagerWithDefaults()

	call := ToolCall{
		ID:   "call_1",
		Name: "detailed_tool",
		Arguments: map[string]any{
			"status": "unknown",
		},
	}

	validationErrors := ValidationErrors{
		{Field: "status", Message: "value must be one of: active, inactive", Code: "enum_violation", Value: "unknown"},
		{Field: "count", Message: "required field is missing", Code: "required"},
	}

	message := manager.BuildDetailedRepairMessage(call, validationErrors, registry)

	if message == "" {
		t.Error("expected non-empty detailed repair message")
	}

	if !contains(message, "active") || !contains(message, "inactive") {
		t.Error("expected message to contain enum values")
	}

	if !contains(message, "integer") {
		t.Error("expected message to contain type information")
	}
}

func TestControlFlowManager_ParallelExecution(t *testing.T) {
	registry := NewRegistry()

	executionOrder := make([]string, 0)
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		toolName := fmt.Sprintf("parallel_tool_%d", i)
		registry.Register(&mockTool{
			name:        toolName,
			description: "A parallel test tool",
			schema: ParameterSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"value": {Type: "string"},
				},
			},
			executeFunc: func(ctx context.Context, params map[string]any, toolCtx *ToolContext) (Result, error) {
				time.Sleep(10 * time.Millisecond)
				mu.Lock()
				executionOrder = append(executionOrder, params["value"].(string))
				mu.Unlock()
				return Result{Success: true, Data: params["value"]}, nil
			},
		})
	}

	manager := NewControlFlowManager(ControlFlowConfig{
		MaxConcurrency:          3,
		ToolTimeout:             5 * time.Second,
		EnableParallelExecution: true,
		EnableValidation:        true,
	})

	toolCalls := make([]ToolCall, 5)
	for i := 0; i < 5; i++ {
		toolCalls[i] = ToolCall{
			ID:        fmt.Sprintf("call_%d", i),
			Name:      fmt.Sprintf("parallel_tool_%d", i),
			Arguments: map[string]any{"value": fmt.Sprintf("result_%d", i)},
		}
	}

	toolCtx := &ToolContext{StartedAt: time.Now()}
	results := manager.ProcessToolCalls(context.Background(), toolCalls, registry, toolCtx)

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	for i, result := range results {
		if !result.Success {
			t.Errorf("result %d: expected success, got failure: %v", i, result.Error)
		}
	}
}

func TestControlFlowManager_MaxRepairAttempts(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&mockTool{
		name:        "failing_tool",
		description: "A tool that always fails validation",
		schema: ParameterSchema{
			Type: "object",
			Properties: map[string]PropertySchema{
				"value": {Type: "integer"},
			},
			Required: []string{"value"},
		},
	})

	manager := NewControlFlowManager(ControlFlowConfig{
		MaxRepairAttempts:       2,
		MaxToolIterations:       5,
		MaxConcurrency:          10,
		ToolTimeout:             5 * time.Second,
		EnableValidation:        true,
		EnableParallelExecution: true,
	})
	manager.ResetAllRepairAttempts()

	callID := "call_1"

	for i := 0; i < 3; i++ {
		manager.incrementRepairAttempts(callID)
	}

	toolCalls := []ToolCall{
		{ID: callID, Name: "failing_tool", Arguments: map[string]any{"value": 123}},
	}

	toolCtx := &ToolContext{StartedAt: time.Now()}
	results := manager.ProcessToolCalls(context.Background(), toolCalls, registry, toolCtx)

	if results[0].Error == nil {
		t.Error("expected error due to max repair attempts exceeded")
	}

	if !contains(results[0].Error.Error(), "max repair attempts") {
		t.Errorf("expected max repair attempts error, got: %v", results[0].Error)
	}
}

func TestControlFlowManager_ToolNotFound(t *testing.T) {
	registry := NewRegistry()

	manager := NewControlFlowManagerWithDefaults()

	toolCalls := []ToolCall{
		{ID: "call_1", Name: "nonexistent_tool", Arguments: map[string]any{}},
	}

	toolCtx := &ToolContext{StartedAt: time.Now()}
	results := manager.ProcessToolCalls(context.Background(), toolCalls, registry, toolCtx)

	if results[0].Success {
		t.Error("expected failure for nonexistent tool")
	}

	if !results[0].NeedsRepair {
		t.Error("expected repair needed for nonexistent tool")
	}

	if len(results[0].ValidationErrors) == 0 {
		t.Error("expected validation errors for nonexistent tool")
	}

	if results[0].ValidationErrors[0].Code != "tool_not_found" {
		t.Errorf("expected 'tool_not_found' error code, got '%s'", results[0].ValidationErrors[0].Code)
	}
}

func TestControlFlowManager_ExecutionError(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&mockTool{
		name:        "error_tool",
		description: "A tool that returns errors",
		schema: ParameterSchema{
			Type:       "object",
			Properties: map[string]PropertySchema{},
		},
		executeFunc: func(ctx context.Context, params map[string]any, toolCtx *ToolContext) (Result, error) {
			return Result{}, errors.New("execution failed")
		},
	})

	manager := NewControlFlowManagerWithDefaults()

	toolCalls := []ToolCall{
		{ID: "call_1", Name: "error_tool", Arguments: map[string]any{}},
	}

	toolCtx := &ToolContext{StartedAt: time.Now()}
	results := manager.ProcessToolCalls(context.Background(), toolCalls, registry, toolCtx)

	if results[0].Success {
		t.Error("expected failure due to execution error")
	}

	if results[0].Error == nil {
		t.Error("expected execution error")
	}

	if !contains(results[0].Error.Error(), "execution failed") {
		t.Errorf("expected 'execution failed' error, got: %v", results[0].Error)
	}
}

func TestControlFlowManager_ValidationDisabled(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&mockTool{
		name:        "no_validation_tool",
		description: "A tool with schema but validation disabled",
		schema: ParameterSchema{
			Type: "object",
			Properties: map[string]PropertySchema{
				"count": {Type: "integer"},
			},
			Required: []string{"count"},
		},
	})

	manager := NewControlFlowManager(ControlFlowConfig{
		MaxConcurrency:          10,
		ToolTimeout:             5 * time.Second,
		EnableValidation:        false,
		EnableParallelExecution: true,
	})

	toolCalls := []ToolCall{
		{ID: "call_1", Name: "no_validation_tool", Arguments: map[string]any{}},
	}

	toolCtx := &ToolContext{StartedAt: time.Now()}
	results := manager.ProcessToolCalls(context.Background(), toolCalls, registry, toolCtx)

	if len(results[0].ValidationErrors) > 0 {
		t.Errorf("expected no validation errors when validation disabled, got: %v", results[0].ValidationErrors)
	}

	if results[0].NeedsRepair {
		t.Error("expected no repair needed when validation disabled")
	}
}

func TestControlFlowManager_ValidateToolCall(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&mockTool{
		name:        "validate_tool",
		description: "A tool for testing ValidateToolCall",
		schema: ParameterSchema{
			Type: "object",
			Properties: map[string]PropertySchema{
				"name":  {Type: "string"},
				"count": {Type: "integer"},
			},
			Required: []string{"name"},
		},
	})

	manager := NewControlFlowManagerWithDefaults()

	call := ToolCall{
		ID:   "call_1",
		Name: "validate_tool",
		Arguments: map[string]any{
			"name":  "test",
			"count": 42.0,
		},
	}

	validatedCall, errs := manager.ValidateToolCall(call, registry)

	if len(errs) > 0 {
		t.Errorf("unexpected validation errors: %v", errs)
	}

	if validatedCall.Arguments["name"] != "test" {
		t.Errorf("expected name 'test', got '%v'", validatedCall.Arguments["name"])
	}

	if validatedCall.Arguments["count"] != int64(42) {
		t.Errorf("expected count to be coerced to int64(42), got %v (%T)", validatedCall.Arguments["count"], validatedCall.Arguments["count"])
	}
}

func TestControlFlowManager_CanRetry(t *testing.T) {
	manager := NewControlFlowManager(ControlFlowConfig{
		MaxRepairAttempts:       3,
		MaxConcurrency:          10,
		ToolTimeout:             5 * time.Second,
		EnableValidation:        true,
		EnableParallelExecution: true,
	})
	manager.ResetAllRepairAttempts()

	callID := "test_call"

	if !manager.CanRetry(callID) {
		t.Error("expected CanRetry to return true for new call")
	}

	manager.incrementRepairAttempts(callID)
	if !manager.CanRetry(callID) {
		t.Error("expected CanRetry to return true after 1 attempt (limit is 3)")
	}

	manager.incrementRepairAttempts(callID)
	manager.incrementRepairAttempts(callID)

	if manager.CanRetry(callID) {
		t.Error("expected CanRetry to return false after 3 attempts (limit is 3)")
	}
}

func TestControlFlowManager_ResetRepairAttempts(t *testing.T) {
	manager := NewControlFlowManagerWithDefaults()
	manager.ResetAllRepairAttempts()

	callID := "reset_test_call"

	manager.incrementRepairAttempts(callID)
	manager.incrementRepairAttempts(callID)

	if manager.getRepairAttempts(callID) != 2 {
		t.Errorf("expected 2 attempts, got %d", manager.getRepairAttempts(callID))
	}

	manager.ResetRepairAttempts(callID)

	if manager.getRepairAttempts(callID) != 0 {
		t.Errorf("expected 0 attempts after reset, got %d", manager.getRepairAttempts(callID))
	}
}

func TestProcessedToolResult_ToLLMMessage(t *testing.T) {
	tests := []struct {
		name     string
		result   ProcessedToolResult
		contains []string
	}{
		{
			name: "successful result",
			result: ProcessedToolResult{
				ID:      "call_1",
				Name:    "test_tool",
				Success: true,
				Result:  Result{Success: true, Data: "output data"},
			},
			contains: []string{"test_tool", "call_1", "Success"},
		},
		{
			name: "validation error result",
			result: ProcessedToolResult{
				ID:          "call_2",
				Name:        "failing_tool",
				Success:     false,
				NeedsRepair: true,
				ValidationErrors: ValidationErrors{
					{Field: "name", Message: "required field is missing", Code: "required"},
				},
			},
			contains: []string{"failing_tool", "call_2", "Validation Failed", "Repair Required", "name"},
		},
		{
			name: "execution error result",
			result: ProcessedToolResult{
				ID:      "call_3",
				Name:    "error_tool",
				Success: false,
				Error:   errors.New("something went wrong"),
			},
			contains: []string{"error_tool", "call_3", "Execution Failed", "something went wrong"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			message := tc.result.ToLLMMessage()

			for _, substr := range tc.contains {
				if !contains(message, substr) {
					t.Errorf("expected message to contain '%s', got: %s", substr, message)
				}
			}
		})
	}
}

func TestControlFlowManager_EmptyToolCalls(t *testing.T) {
	registry := NewRegistry()
	manager := NewControlFlowManagerWithDefaults()

	toolCtx := &ToolContext{StartedAt: time.Now()}
	results := manager.ProcessToolCalls(context.Background(), []ToolCall{}, registry, toolCtx)

	if len(results) != 0 {
		t.Errorf("expected 0 results for empty tool calls, got %d", len(results))
	}
}

func TestControlFlowManager_CreateRepairRequest(t *testing.T) {
	manager := NewControlFlowManagerWithDefaults()

	t.Run("result needing repair", func(t *testing.T) {
		result := ProcessedToolResult{
			ID:          "call_1",
			Name:        "test_tool",
			NeedsRepair: true,
			ValidationErrors: ValidationErrors{
				{Field: "name", Message: "required field is missing", Code: "required"},
			},
		}

		req := manager.CreateRepairRequest(result)
		if req == nil {
			t.Fatal("expected repair request, got nil")
		}

		if req.CallID != "call_1" {
			t.Errorf("expected call ID 'call_1', got '%s'", req.CallID)
		}

		if req.ToolName != "test_tool" {
			t.Errorf("expected tool name 'test_tool', got '%s'", req.ToolName)
		}

		if len(req.Errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(req.Errors))
		}
	})

	t.Run("result not needing repair", func(t *testing.T) {
		result := ProcessedToolResult{
			ID:          "call_2",
			Name:        "test_tool",
			NeedsRepair: false,
			Success:     true,
		}

		req := manager.CreateRepairRequest(result)
		if req != nil {
			t.Error("expected nil repair request for successful result")
		}
	})
}

func TestDefaultControlFlowConfig(t *testing.T) {
	config := DefaultControlFlowConfig()

	if config.MaxRepairAttempts != 2 {
		t.Errorf("expected MaxRepairAttempts 2, got %d", config.MaxRepairAttempts)
	}

	if config.MaxToolIterations != 5 {
		t.Errorf("expected MaxToolIterations 5, got %d", config.MaxToolIterations)
	}

	if config.MaxConcurrency != 10 {
		t.Errorf("expected MaxConcurrency 10, got %d", config.MaxConcurrency)
	}

	if config.ToolTimeout != 30*time.Second {
		t.Errorf("expected ToolTimeout 30s, got %v", config.ToolTimeout)
	}

	if !config.EnableValidation {
		t.Error("expected EnableValidation to be true")
	}

	if !config.EnableParallelExecution {
		t.Error("expected EnableParallelExecution to be true")
	}
}
