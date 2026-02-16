package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type ControlFlowConfig struct {
	MaxRepairAttempts       int           `json:"max_repair_attempts"`
	MaxToolIterations       int           `json:"max_tool_iterations"`
	MaxConcurrency          int           `json:"max_concurrency"`
	ToolTimeout             time.Duration `json:"tool_timeout"`
	EnableValidation        bool          `json:"enable_validation"`
	EnableParallelExecution bool          `json:"enable_parallel_execution"`
}

func DefaultControlFlowConfig() ControlFlowConfig {
	return ControlFlowConfig{
		MaxRepairAttempts:       2,
		MaxToolIterations:       5,
		MaxConcurrency:          10,
		ToolTimeout:             30 * time.Second,
		EnableValidation:        true,
		EnableParallelExecution: true,
	}
}

type ProcessedToolResult struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Result           Result           `json:"result"`
	Error            error            `json:"error,omitempty"`
	ValidationErrors ValidationErrors `json:"validation_errors,omitempty"`
	RepairAttempts   int              `json:"repair_attempts"`
	Success          bool             `json:"success"`
	NeedsRepair      bool             `json:"needs_repair"`
	ValidatedParams  map[string]any   `json:"validated_params,omitempty"`
}

func (r ProcessedToolResult) ToLLMMessage() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Tool: %s (ID: %s)\n", r.Name, r.ID))

	if r.NeedsRepair && len(r.ValidationErrors) > 0 {
		sb.WriteString("Status: Validation Failed - Repair Required\n")
		sb.WriteString("Errors:\n")
		sb.WriteString(r.ValidationErrors.ToLLMMessage())
	} else if r.Error != nil {
		sb.WriteString(fmt.Sprintf("Status: Execution Failed\nError: %s\n", r.Error.Error()))
	} else if r.Success {
		sb.WriteString("Status: Success\n")
		if r.Result.Data != nil {
			sb.WriteString(fmt.Sprintf("Result: %v\n", r.Result.Data))
		}
	} else {
		sb.WriteString("Status: Failed\n")
		if r.Result.Error != "" {
			sb.WriteString(fmt.Sprintf("Error: %s\n", r.Result.Error))
		}
	}

	return sb.String()
}

type ControlFlowManager struct {
	config         ControlFlowConfig
	validator      *Validator
	executor       *ParallelExecutor
	repairAttempts sync.Map
}

func NewControlFlowManager(config ControlFlowConfig) *ControlFlowManager {
	if config.MaxRepairAttempts <= 0 {
		config.MaxRepairAttempts = 2
	}
	if config.MaxToolIterations <= 0 {
		config.MaxToolIterations = 5
	}
	if config.MaxConcurrency <= 0 {
		config.MaxConcurrency = 10
	}
	if config.ToolTimeout <= 0 {
		config.ToolTimeout = 30 * time.Second
	}

	return &ControlFlowManager{
		config:    config,
		validator: NewValidator(),
		executor: NewParallelExecutor(ParallelExecutorConfig{
			MaxConcurrency: config.MaxConcurrency,
			Timeout:        config.ToolTimeout,
		}),
	}
}

func NewControlFlowManagerWithDefaults() *ControlFlowManager {
	return NewControlFlowManager(DefaultControlFlowConfig())
}

func (m *ControlFlowManager) ProcessToolCalls(ctx context.Context, toolCalls []ToolCall, registry *Registry, toolCtx *ToolContext) []ProcessedToolResult {
	if len(toolCalls) == 0 {
		return []ProcessedToolResult{}
	}

	results := make([]ProcessedToolResult, len(toolCalls))

	if m.config.EnableParallelExecution && len(toolCalls) > 1 {
		var wg sync.WaitGroup
		for i, call := range toolCalls {
			wg.Add(1)
			go func(idx int, c ToolCall) {
				defer wg.Done()
				results[idx] = m.processSingleCall(ctx, c, registry, toolCtx)
			}(i, call)
		}
		wg.Wait()
	} else {
		for i, call := range toolCalls {
			results[i] = m.processSingleCall(ctx, call, registry, toolCtx)
		}
	}

	return results
}

func (m *ControlFlowManager) processSingleCall(ctx context.Context, call ToolCall, registry *Registry, toolCtx *ToolContext) ProcessedToolResult {
	result := ProcessedToolResult{
		ID:   call.ID,
		Name: call.Name,
	}

	attempts := m.getRepairAttempts(call.ID)
	result.RepairAttempts = attempts

	if attempts >= m.config.MaxRepairAttempts {
		result.Error = fmt.Errorf("max repair attempts (%d) exceeded for tool call %s", m.config.MaxRepairAttempts, call.ID)
		result.NeedsRepair = false
		result.Success = false
		return result
	}

	validatedCall, validationErrors := m.ValidateToolCall(call, registry)
	if len(validationErrors) > 0 {
		result.ValidationErrors = validationErrors
		result.NeedsRepair = true
		result.Success = false
		m.incrementRepairAttempts(call.ID)
		return result
	}

	result.NeedsRepair = false
	result.ValidatedParams = validatedCall.Arguments

	tool, ok := registry.Get(call.Name)
	if !ok {
		result.Error = fmt.Errorf("tool %s not found", call.Name)
		result.Success = false
		return result
	}

	execCtx, cancel := context.WithTimeout(ctx, m.config.ToolTimeout)
	defer cancel()

	toolResult, err := tool.Execute(execCtx, validatedCall.Arguments, toolCtx)
	if err != nil {
		result.Error = err
		result.Success = false
		return result
	}

	result.Result = toolResult
	result.Success = toolResult.Success
	return result
}

func (m *ControlFlowManager) ValidateToolCall(call ToolCall, registry *Registry) (ToolCall, ValidationErrors) {
	if !m.config.EnableValidation {
		return call, nil
	}

	tool, ok := registry.Get(call.Name)
	if !ok {
		return call, ValidationErrors{{
			Field:   "name",
			Message: fmt.Sprintf("tool '%s' not found in registry", call.Name),
			Code:    "tool_not_found",
			Value:   call.Name,
		}}
	}

	schema := tool.Parameters()
	validatedParams, errors := m.validator.Validate(schema, call.Arguments)

	validatedCall := ToolCall{
		ID:        call.ID,
		Name:      call.Name,
		Arguments: validatedParams,
	}

	return validatedCall, errors
}

func (m *ControlFlowManager) BuildRepairMessage(call ToolCall, errors ValidationErrors) string {
	if len(errors) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("The tool call '")
	sb.WriteString(call.Name)
	sb.WriteString("' has validation errors that need to be fixed:\n\n")

	for i, err := range errors {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, err.ToLLMMessage()))
	}

	sb.WriteString("\nPlease correct the parameters and try again.")

	tool, ok := call.Name, true
	if ok {
		sb.WriteString("\n\nExpected parameter schema for '")
		sb.WriteString(tool)
		sb.WriteString("':\n")
	}

	return sb.String()
}

func (m *ControlFlowManager) BuildDetailedRepairMessage(call ToolCall, errors ValidationErrors, registry *Registry) string {
	if len(errors) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("The tool call '")
	sb.WriteString(call.Name)
	sb.WriteString("' has validation errors:\n\n")

	for i, err := range errors {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, err.ToLLMMessage()))
	}

	tool, ok := registry.Get(call.Name)
	if ok {
		schema := tool.Parameters()
		sb.WriteString("\nExpected parameters:\n")
		sb.WriteString(m.formatSchemaHints(schema))
	}

	sb.WriteString("\nPlease fix these errors and provide corrected parameters.")

	return sb.String()
}

func (m *ControlFlowManager) formatSchemaHints(schema ParameterSchema) string {
	var sb strings.Builder

	if len(schema.Required) > 0 {
		sb.WriteString(fmt.Sprintf("Required fields: %s\n", strings.Join(schema.Required, ", ")))
	}

	for name, prop := range schema.Properties {
		sb.WriteString(fmt.Sprintf("- %s (%s)", name, prop.Type))
		if prop.Description != "" {
			sb.WriteString(fmt.Sprintf(": %s", prop.Description))
		}
		if len(prop.Enum) > 0 {
			sb.WriteString(fmt.Sprintf(" [allowed values: %s]", strings.Join(prop.Enum, ", ")))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m *ControlFlowManager) getRepairAttempts(callID string) int {
	if val, ok := m.repairAttempts.Load(callID); ok {
		return val.(int)
	}
	return 0
}

func (m *ControlFlowManager) incrementRepairAttempts(callID string) int {
	newVal := 0
	for {
		existing, loaded := m.repairAttempts.LoadOrStore(callID, 1)
		if !loaded {
			return 1
		}
		oldVal := existing.(int)
		newVal = oldVal + 1
		if m.repairAttempts.CompareAndSwap(callID, oldVal, newVal) {
			return newVal
		}
	}
}

func (m *ControlFlowManager) ResetRepairAttempts(callID string) {
	m.repairAttempts.Delete(callID)
}

func (m *ControlFlowManager) ResetAllRepairAttempts() {
	m.repairAttempts = sync.Map{}
}

func (m *ControlFlowManager) GetConfig() ControlFlowConfig {
	return m.config
}

func (m *ControlFlowManager) SetConfig(config ControlFlowConfig) {
	m.config = config
	m.executor.SetMaxConcurrency(config.MaxConcurrency)
	m.executor.SetTimeout(config.ToolTimeout)
}

func (m *ControlFlowManager) GetValidator() *Validator {
	return m.validator
}

func (m *ControlFlowManager) GetExecutor() *ParallelExecutor {
	return m.executor
}

func (m *ControlFlowManager) CanRetry(callID string) bool {
	return m.getRepairAttempts(callID) < m.config.MaxRepairAttempts
}

func (m *ControlFlowManager) ProcessWithRetry(ctx context.Context, toolCalls []ToolCall, registry *Registry, toolCtx *ToolContext, maxIterations int) []ProcessedToolResult {
	if maxIterations <= 0 {
		maxIterations = m.config.MaxToolIterations
	}

	var allResults []ProcessedToolResult
	currentCalls := toolCalls

	for iteration := 0; iteration < maxIterations && len(currentCalls) > 0; iteration++ {
		results := m.ProcessToolCalls(ctx, currentCalls, registry, toolCtx)
		allResults = append(allResults, results...)

		var needsRetry []ToolCall
		for _, result := range results {
			if result.NeedsRepair && m.CanRetry(result.ID) {
				needsRetry = append(needsRetry, ToolCall{
					ID:        result.ID,
					Name:      result.Name,
					Arguments: make(map[string]any),
				})
			}
		}

		if len(needsRetry) == 0 {
			break
		}
		currentCalls = needsRetry
	}

	return allResults
}

type RepairRequest struct {
	CallID        string           `json:"call_id"`
	ToolName      string           `json:"tool_name"`
	OriginalCall  ToolCall         `json:"original_call"`
	Errors        ValidationErrors `json:"errors"`
	RepairMessage string           `json:"repair_message"`
}

func (m *ControlFlowManager) CreateRepairRequest(result ProcessedToolResult) *RepairRequest {
	if !result.NeedsRepair {
		return nil
	}

	return &RepairRequest{
		CallID:        result.ID,
		ToolName:      result.Name,
		Errors:        result.ValidationErrors,
		RepairMessage: result.ValidationErrors.ToLLMMessage(),
	}
}
