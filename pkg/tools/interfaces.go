package tools

import (
	"context"
	"time"
)

type Tool interface {
	Name() string
	Description() string
	Parameters() ParameterSchema
	Execute(ctx context.Context, params map[string]any, toolCtx *ToolContext) (Result, error)
}

type ParameterSchema struct {
	Type       string                    `json:"type"`
	Properties map[string]PropertySchema `json:"properties,omitempty"`
	Required   []string                  `json:"required,omitempty"`
	Items      *PropertySchema           `json:"items,omitempty"`
}

type PropertySchema struct {
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Enum        []string               `json:"enum,omitempty"`
	Properties  map[string]PropertySchema `json:"properties,omitempty"`
	Required    []string               `json:"required,omitempty"`
	Items       *PropertySchema        `json:"items,omitempty"`
}

type Result struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type Pipeline interface {
	AddStage(name string, processor Processor) Pipeline
	Execute(ctx context.Context, input any) (any, error)
}

type Processor func(ctx context.Context, input any) (any, error)

type BatchProcessor interface {
	ProcessBatch(ctx context.Context, items []BatchItem) []BatchResult
}

type BatchItem struct {
	ID    string
	Input any
}

type BatchResult struct {
	ID     string
	Result any
	Error  error
}

// ToolContext provides context for tool execution
type ToolContext struct {
	SessionID string
	UserID    string
	GuildID   string
	MessageID string
	Metadata  map[string]any
	StartedAt time.Time
}
