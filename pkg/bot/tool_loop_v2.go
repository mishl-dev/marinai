package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"marinai/pkg/memory"
	"marinai/pkg/tools"
)

type ToolLoopV2 struct {
	controlFlow *tools.ControlFlowManager
	registry    *tools.Registry
	llmClient   LLMClient
}

func NewToolLoopV2(llmClient LLMClient, registry *tools.Registry, config tools.ControlFlowConfig) *ToolLoopV2 {
	return &ToolLoopV2{
		controlFlow: tools.NewControlFlowManager(config),
		registry:    registry,
		llmClient:   llmClient,
	}
}

func NewToolLoopV2WithDefaults(llmClient LLMClient, registry *tools.Registry) *ToolLoopV2 {
	return NewToolLoopV2(llmClient, registry, tools.DefaultControlFlowConfig())
}

func (tl *ToolLoopV2) GetControlFlow() *tools.ControlFlowManager {
	return tl.controlFlow
}

func (tl *ToolLoopV2) GetRegistry() *tools.Registry {
	return tl.registry
}

func (tl *ToolLoopV2) Execute(ctx context.Context, messages []memory.LLMMessage, botTools []Tool, result *ChatResult) ([]memory.LLMMessage, string, error) {
	maxIterations := tl.controlFlow.GetConfig().MaxToolIterations
	emptyResponseRetries := 0
	maxEmptyRetries := 2

	for iteration := 0; iteration < maxIterations; iteration++ {
		if len(result.ToolCalls) == 0 {
			log.Printf("[ToolLoopV2] Tool loop complete at iteration %d, no more tool calls", iteration+1)
			if strings.TrimSpace(result.Content) == "" {
				if emptyResponseRetries >= maxEmptyRetries {
					log.Printf("[ToolLoopV2] Max empty response retries reached, returning fallback")
					return messages, "Sorry, I couldn't process that. Please try again.", nil
				}
				emptyResponseRetries++
				log.Printf("[ToolLoopV2] Warning: LLM returned empty response (retry %d/%d)", emptyResponseRetries, maxEmptyRetries)
				messages = append(messages, memory.LLMMessage{
					Role:    "user",
					Content: "Please provide a response.",
				})
				var err error
				result, err = tl.llmClient.ChatCompletionWithTools(messages, botTools)
				if err != nil {
					return messages, "", err
				}
				iteration--
				continue
			}
			return messages, result.Content, nil
		}

		emptyResponseRetries = 0
		log.Printf("[ToolLoopV2] Tool iteration %d: processing %d tool calls", iteration+1, len(result.ToolCalls))

		toolCalls := make([]ToolCall, len(result.ToolCalls))
		for j, tc := range result.ToolCalls {
			toolCalls[j] = ToolCall{
				ID:        tc.ID,
				Name:      tc.Name,
				Arguments: tc.Arguments,
			}
		}

		messages = append(messages, memory.LLMMessage{
			Role:    "assistant",
			Content: result.Content,
		})

		convertedCalls := tl.convertToolCalls(result.ToolCalls)
		processedResults := tl.processWithRepair(ctx, convertedCalls, messages, botTools)

		messages = tl.appendToolResults(messages, processedResults)

		var err error
		result, err = tl.llmClient.ChatCompletionWithTools(messages, botTools)
		if err != nil {
			log.Printf("[ToolLoopV2] LLM call failed: %v", err)
			return messages, "", err
		}
		log.Printf("[ToolLoopV2] LLM response: content_len=%d, tool_calls=%d", len(result.Content), len(result.ToolCalls))
	}

	log.Printf("[ToolLoopV2] Max tool iterations reached")
	if strings.TrimSpace(result.Content) != "" {
		return messages, result.Content, nil
	}
	return messages, "", fmt.Errorf("max tool iterations reached with empty response")
}

func (tl *ToolLoopV2) convertToolCalls(botCalls []ToolCall) []tools.ToolCall {
	converted := make([]tools.ToolCall, 0, len(botCalls))
	for _, call := range botCalls {
		params := make(map[string]any)
		if call.Arguments != "" {
			if err := json.Unmarshal([]byte(call.Arguments), &params); err != nil {
				log.Printf("[ToolLoopV2] Failed to parse arguments for %s: %v", call.Name, err)
				params = make(map[string]any)
			}
		}

		converted = append(converted, tools.ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: params,
		})
	}
	return converted
}

func (tl *ToolLoopV2) processWithRepair(ctx context.Context, calls []tools.ToolCall, messages []memory.LLMMessage, botTools []Tool) []tools.ProcessedToolResult {
	toolCtx := &tools.ToolContext{
		StartedAt: time.Now(),
	}
	results := tl.controlFlow.ProcessToolCalls(ctx, calls, tl.registry, toolCtx)

	var needsRepair []*tools.ProcessedToolResult
	for i := range results {
		if results[i].NeedsRepair && tl.controlFlow.CanRetry(results[i].ID) {
			needsRepair = append(needsRepair, &results[i])
		}
	}

	if len(needsRepair) == 0 {
		return results
	}

	log.Printf("[ToolLoopV2] %d tool calls need repair", len(needsRepair))

	for _, repairResult := range needsRepair {
		originalCall := tl.findOriginalCall(calls, repairResult.ID)
		if originalCall == nil {
			continue
		}

		repairMsg := tl.controlFlow.BuildDetailedRepairMessage(*originalCall, repairResult.ValidationErrors, tl.registry)
		log.Printf("[ToolLoopV2] Repair message for %s (attempt %d): %s", repairResult.Name, repairResult.RepairAttempts+1, repairMsg)

		if !tl.controlFlow.CanRetry(repairResult.ID) {
			log.Printf("[ToolLoopV2] Max repair attempts exceeded for %s", repairResult.ID)
			repairResult.Error = fmt.Errorf("max repair attempts exceeded for tool call %s", repairResult.ID)
			repairResult.NeedsRepair = false
			repairResult.Success = false
			continue
		}

		correctedCall, corrected := tl.requestRepair(ctx, *originalCall, repairResult.ValidationErrors, messages, botTools)
		if corrected {
			toolCtx := &tools.ToolContext{StartedAt: time.Now()}
			singleResult := tl.controlFlow.ProcessToolCalls(ctx, []tools.ToolCall{correctedCall}, tl.registry, toolCtx)
			if len(singleResult) > 0 {
				for i, r := range results {
					if r.ID == repairResult.ID {
						results[i] = singleResult[0]
						break
					}
				}
			}
		}
	}

	return results
}

func (tl *ToolLoopV2) requestRepair(ctx context.Context, originalCall tools.ToolCall, errors tools.ValidationErrors, messages []memory.LLMMessage, botTools []Tool) (tools.ToolCall, bool) {
	select {
	case <-ctx.Done():
		return originalCall, false
	default:
	}

	repairPrompt := tl.buildRepairPrompt(originalCall, errors)
	repairMessages := append([]memory.LLMMessage{}, messages...)
	repairMessages = append(repairMessages, memory.LLMMessage{
		Role:    "user",
		Content: repairPrompt,
	})

	result, err := tl.llmClient.ChatCompletionWithTools(repairMessages, botTools)
	if err != nil {
		log.Printf("[ToolLoopV2] Repair request failed: %v", err)
		return originalCall, false
	}

	for _, tc := range result.ToolCalls {
		if tc.Name == originalCall.Name {
			params := make(map[string]any)
			if tc.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Arguments), &params); err != nil {
					log.Printf("[ToolLoopV2] Failed to parse corrected arguments: %v", err)
				}
			}
			log.Printf("[ToolLoopV2] Received corrected tool call for %s", tc.Name)
			return tools.ToolCall{
				ID:        tc.ID,
				Name:      tc.Name,
				Arguments: params,
			}, true
		}
	}

	log.Printf("[ToolLoopV2] No corrected tool call received in repair response")
	return originalCall, false
}

func (tl *ToolLoopV2) buildRepairPrompt(call tools.ToolCall, errors tools.ValidationErrors) string {
	var sb strings.Builder
	sb.WriteString("The previous tool call '")
	sb.WriteString(call.Name)
	sb.WriteString("' had validation errors:\n\n")

	for i, err := range errors {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, err.ToLLMMessage()))
	}

	tool, ok := tl.registry.Get(call.Name)
	if ok {
		schema := tool.Parameters()
		sb.WriteString("\nExpected parameters:\n")
		sb.WriteString(tl.formatSchemaHints(schema))
	}

	sb.WriteString("\nPlease correct the parameters and call the tool again with valid arguments.")

	return sb.String()
}

func (tl *ToolLoopV2) formatSchemaHints(schema tools.ParameterSchema) string {
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

func (tl *ToolLoopV2) findOriginalCall(calls []tools.ToolCall, id string) *tools.ToolCall {
	for i := range calls {
		if calls[i].ID == id {
			return &calls[i]
		}
	}
	return nil
}

func (tl *ToolLoopV2) appendToolResults(messages []memory.LLMMessage, results []tools.ProcessedToolResult) []memory.LLMMessage {
	for _, result := range results {
		var content string

		if result.NeedsRepair && len(result.ValidationErrors) > 0 {
			content = fmt.Sprintf("[TOOL ERROR] Tool '%s' validation failed:\n%s", result.Name, result.ValidationErrors.ToLLMMessage())
			log.Printf("[ToolLoopV2] Tool %s validation failed: %s", result.Name, result.ValidationErrors.Error())
		} else if result.Error != nil {
			content = fmt.Sprintf("[TOOL ERROR] Tool '%s' execution error: %s", result.Name, result.Error.Error())
			log.Printf("[ToolLoopV2] Tool %s execution error: %v", result.Name, result.Error)
		} else if result.Success {
			content = tl.formatSuccessResult(result)
			log.Printf("[ToolLoopV2] Tool %s succeeded", result.Name)
		} else {
			if result.Result.Error != "" {
				content = fmt.Sprintf("[TOOL ERROR] Tool '%s' failed: %s", result.Name, result.Result.Error)
			} else {
				content = fmt.Sprintf("[TOOL ERROR] Tool '%s' completed with unknown status", result.Name)
			}
			log.Printf("[ToolLoopV2] Tool %s failed: %s", result.Name, result.Result.Error)
		}

		log.Printf("[ToolLoopV2] Adding tool result as user message: name=%s, content_len=%d", result.Name, len(content))

		// Add as user message instead of tool message for better compatibility
		messages = append(messages, memory.LLMMessage{
			Role:    "user",
			Content: fmt.Sprintf("[TOOL RESULT for %s]\n%s\n[END TOOL RESULT]", result.Name, content),
		})
	}
	log.Printf("[ToolLoopV2] Total messages after tool results: %d", len(messages))
	return messages
}

func (tl *ToolLoopV2) formatSuccessResult(result tools.ProcessedToolResult) string {
	if result.Result.Data == nil {
		log.Printf("[ToolLoopV2] Tool %s has no data", result.Name)
		return fmt.Sprintf("Tool '%s' completed successfully.", result.Name)
	}

	resultJSON, err := json.Marshal(result.Result.Data)
	if err != nil {
		log.Printf("[ToolLoopV2] Tool %s marshal error: %v", result.Name, err)
		return fmt.Sprintf("Tool '%s' completed successfully with data (serialization error).", result.Name)
	}

	resultStr := string(resultJSON)
	log.Printf("[ToolLoopV2] Tool %s result length: %d bytes", result.Name, len(resultStr))

	if len(resultStr) > 8000 {
		resultStr = resultStr[:8000] + "\n...[truncated]"
	}

	return fmt.Sprintf("Tool '%s' result:\n%s", result.Name, resultStr)
}

func (tl *ToolLoopV2) ExecuteParallel(ctx context.Context, calls []ToolCall) ([]tools.ProcessedToolResult, error) {
	start := time.Now()
	converted := tl.convertToolCalls(calls)

	toolCtx := &tools.ToolContext{StartedAt: time.Now()}
	results := tl.controlFlow.ProcessToolCalls(ctx, converted, tl.registry, toolCtx)

	duration := time.Since(start)
	log.Printf("[ToolLoopV2] Parallel execution of %d tools completed in %v", len(calls), duration)

	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}
	log.Printf("[ToolLoopV2] Results: %d/%d successful", successCount, len(results))

	return results, nil
}

func (tl *ToolLoopV2) ValidateToolCall(call ToolCall) (map[string]any, tools.ValidationErrors) {
	params := make(map[string]any)
	if call.Arguments != "" {
		if err := json.Unmarshal([]byte(call.Arguments), &params); err != nil {
			log.Printf("[ToolLoopV2] Failed to parse arguments for validation: %v", err)
		}
	}

	toolsCall := tools.ToolCall{
		ID:        call.ID,
		Name:      call.Name,
		Arguments: params,
	}

	validatedCall, errors := tl.controlFlow.ValidateToolCall(toolsCall, tl.registry)
	return validatedCall.Arguments, errors
}

func (tl *ToolLoopV2) Reset() {
	tl.controlFlow.ResetAllRepairAttempts()
}
