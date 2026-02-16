package tools

import (
	"fmt"
	"sync"
)

type ToolCallDelta struct {
	Index     int
	ID        string
	Name      string
	Arguments string
}

type StreamToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type accumulatedToolCall struct {
	id        string
	name      string
	arguments string
	hasID     bool
	hasName   bool
}

type StreamAccumulator struct {
	mu       sync.RWMutex
	calls    map[int]*accumulatedToolCall
	indices  []int
	warnings []string
}

func NewStreamAccumulator() *StreamAccumulator {
	return &StreamAccumulator{
		calls:   make(map[int]*accumulatedToolCall),
		indices: make([]int, 0),
	}
}

func (a *StreamAccumulator) AddChunk(delta ToolCallDelta) {
	a.mu.Lock()
	defer a.mu.Unlock()

	idx := delta.Index

	call, exists := a.calls[idx]
	if !exists {
		call = &accumulatedToolCall{}
		a.calls[idx] = call
		a.indices = append(a.indices, idx)
	}

	if delta.ID != "" {
		if call.hasID && call.id != delta.ID {
			a.warnings = append(a.warnings,
				fmt.Sprintf("tool call index %d: ID changed from %q to %q", idx, call.id, delta.ID))
		}
		call.id = delta.ID
		call.hasID = true
	}

	if delta.Name != "" {
		if call.hasName && call.name != delta.Name {
			a.warnings = append(a.warnings,
				fmt.Sprintf("tool call index %d: name changed from %q to %q", idx, call.name, delta.Name))
		}
		call.name = delta.Name
		call.hasName = true
	}

	if delta.Arguments != "" {
		call.arguments += delta.Arguments
	}
}

func (a *StreamAccumulator) GetToolCalls() []StreamToolCall {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]StreamToolCall, 0, len(a.indices))

	for _, idx := range a.indices {
		call := a.calls[idx]
		if call == nil {
			continue
		}

		if !call.hasID || call.id == "" {
			a.logWarning(fmt.Sprintf("tool call index %d: missing ID", idx))
			continue
		}

		if !call.hasName || call.name == "" {
			a.logWarning(fmt.Sprintf("tool call index %d: missing name", idx))
			continue
		}

		result = append(result, StreamToolCall{
			ID:        call.id,
			Name:      call.name,
			Arguments: call.arguments,
		})
	}

	return result
}

func (a *StreamAccumulator) GetWarnings() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	warnings := make([]string, len(a.warnings))
	copy(warnings, a.warnings)
	return warnings
}

func (a *StreamAccumulator) HasWarnings() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.warnings) > 0
}

func (a *StreamAccumulator) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.calls = make(map[int]*accumulatedToolCall)
	a.indices = make([]int, 0)
	a.warnings = make([]string, 0)
}

func (a *StreamAccumulator) Count() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.calls)
}

func (a *StreamAccumulator) logWarning(warning string) {
	a.warnings = append(a.warnings, warning)
}
