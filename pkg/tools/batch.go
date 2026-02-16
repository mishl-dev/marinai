package tools

import (
	"context"
	"fmt"
	"sync"
)

type BatchExecutor struct {
	maxConcurrency int
}

func NewBatchExecutor(maxConcurrency int) *BatchExecutor {
	if maxConcurrency <= 0 {
		maxConcurrency = 5
	}
	return &BatchExecutor{
		maxConcurrency: maxConcurrency,
	}
}

func (e *BatchExecutor) Process(ctx context.Context, items []BatchItem, fn func(ctx context.Context, item BatchItem) BatchResult) []BatchResult {
	results := make([]BatchResult, len(items))

	var wg sync.WaitGroup
	sem := make(chan struct{}, e.maxConcurrency)

	for i, item := range items {
		wg.Add(1)
		go func(idx int, itm BatchItem) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				results[idx] = fn(ctx, itm)
			case <-ctx.Done():
				results[idx] = BatchResult{
					ID:    itm.ID,
					Error: ctx.Err(),
				}
			}
		}(i, item)
	}

	wg.Wait()
	return results
}

func (r *Registry) ProcessBatch(ctx context.Context, toolName string, items []BatchItem, executor *BatchExecutor) []BatchResult {
	tool, ok := r.Get(toolName)
	if !ok {
		results := make([]BatchResult, len(items))
		for i, item := range items {
			results[i] = BatchResult{
				ID:    item.ID,
				Error: fmt.Errorf("tool %s not found", toolName),
			}
		}
		return results
	}

	return executor.Process(ctx, items, func(ctx context.Context, item BatchItem) BatchResult {
		params, ok := item.Input.(map[string]any)
		if !ok {
			return BatchResult{
				ID:    item.ID,
				Error: fmt.Errorf("invalid input type for item %s", item.ID),
			}
		}

		result, err := tool.Execute(ctx, params, nil)
		if err != nil {
			return BatchResult{ID: item.ID, Error: err}
		}
		return BatchResult{ID: item.ID, Result: result}
	})
}
