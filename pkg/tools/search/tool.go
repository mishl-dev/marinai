package search

import (
	"context"
	"fmt"

	"marinai/pkg/tools"
)

const ToolName = "web_search"

type SearchTool struct {
	client *DuckDuckGoClient
}

func NewSearchTool() *SearchTool {
	return &SearchTool{
		client: NewDuckDuckGoClient(),
	}
}

func (t *SearchTool) Name() string {
	return ToolName
}

func (t *SearchTool) Description() string {
	return "Search the web for information. Returns a list of search results with titles, URLs, and snippets."
}

func (t *SearchTool) Parameters() tools.ParameterSchema {
	return tools.ParameterSchema{
		Type: "object",
		Properties: map[string]tools.PropertySchema{
			"query": {
				Type:        "string",
				Description: "The search query",
			},
			"max_results": {
				Type:        "integer",
				Description: "Maximum number of results to return (default: 10)",
			},
		},
		Required: []string{"query"},
	}
}

func (t *SearchTool) Execute(ctx context.Context, params map[string]any, toolCtx *tools.ToolContext) (tools.Result, error) {
	query, ok := params["query"].(string)
	if !ok {
		return tools.Result{}, fmt.Errorf("query parameter must be a string")
	}

	opts := DefaultOptions()

	if maxResults, ok := params["max_results"].(float64); ok {
		opts.MaxResults = int(maxResults)
	}

	results, err := t.client.Search(ctx, query, opts)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	return tools.Result{
		Success: true,
		Data:    results,
	}, nil
}
