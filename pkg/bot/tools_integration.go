package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"

	"marinai/pkg/tools"
	"marinai/pkg/tools/media"
	"marinai/pkg/tools/search"
	"marinai/pkg/tools/webscrape"
)

func InitializeTools(cfg *ToolConfig) *tools.Registry {
	registry := tools.NewRegistry()

	if cfg == nil {
		return registry
	}

	if cfg.WebSearch.Enabled {
		registry.Register(search.NewSearchTool())
		log.Println("Tool registered: web_search")
	}

	if cfg.WebScrape.Enabled {
		registry.Register(webscrape.NewScrapeTool())
		registry.Register(webscrape.NewYouTubeTool())
		log.Println("Tools registered: web_scrape, youtube_transcript")
	}

	if cfg.Media.Enabled {
		registry.Register(media.NewImageTool())
		registry.Register(media.NewPDFTool())

		if cfg.Media.Audio.DockerImage != "" {
			registry.Register(media.NewTranscribeToolWithImage(cfg.Media.Audio.DockerImage))
		} else {
			registry.Register(media.NewTranscribeTool())
		}
		log.Println("Tools registered: image_compress, pdf_extract, audio_transcribe")
	}

	// Register sandbox tools
	RegisterSandboxTools(registry)

	return registry
}

type ToolConfig struct {
	WebSearch WebSearchConfig `yaml:"web_search"`
	WebScrape WebScrapeConfig `yaml:"web_scrape"`
	Media     MediaConfig     `yaml:"media"`
}

type WebSearchConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Backend    string `yaml:"backend"`
	MaxResults int    `yaml:"max_results"`
	Timeout    int    `yaml:"timeout_seconds"`
}

type WebScrapeConfig struct {
	Enabled     bool  `yaml:"enabled"`
	MaxBodySize int64 `yaml:"max_body_bytes"`
	Timeout     int   `yaml:"timeout_seconds"`
}

type MediaConfig struct {
	Enabled bool        `yaml:"enabled"`
	Image   ImageConfig `yaml:"image"`
	Audio   AudioConfig `yaml:"audio"`
	PDF     PDFConfig   `yaml:"pdf"`
}

type ImageConfig struct {
	CompressionThreshold int64 `yaml:"compression_threshold"`
	Quality              int   `yaml:"quality"`
	MaxWidth             int   `yaml:"max_width"`
	MaxHeight            int   `yaml:"max_height"`
}

type AudioConfig struct {
	DockerImage   string `yaml:"docker_image"`
	Language      string `yaml:"language"`
	MemoryLimitMB int    `yaml:"memory_limit_mb"`
}

type PDFConfig struct {
	MaxPages int `yaml:"max_pages"`
}

type ToolExecutor struct {
	registry *tools.Registry
}

func NewToolExecutor(registry *tools.Registry) *ToolExecutor {
	return &ToolExecutor{registry: registry}
}

func (e *ToolExecutor) ExecuteTool(ctx context.Context, name string, params map[string]any) (tools.Result, error) {
	return e.registry.Execute(ctx, name, params)
}

func (e *ToolExecutor) HasTools() bool {
	if e.registry == nil {
		return false
	}
	return len(e.registry.ListTools()) > 0
}

func (e *ToolExecutor) GetToolDefinitions() []Tool {
	defs := e.registry.GetToolDefinitions()
	tools := make([]Tool, len(defs))
	for i, d := range defs {
		params := make(map[string]interface{})
		for k, v := range d.Parameters.Properties {
			params[k] = map[string]interface{}{
				"type":        v.Type,
				"description": v.Description,
			}
		}
		tools[i] = Tool{
			Type: "function",
			Function: ToolFunction{
				Name:        d.Name,
				Description: d.Description,
				Parameters: map[string]interface{}{
					"type":       d.Parameters.Type,
					"properties": params,
					"required":   d.Parameters.Required,
				},
			},
		}
	}
	return tools
}

type ParsedToolCall struct {
	Name   string
	Params map[string]any
}

type ToolCallResult struct {
	Name    string
	Success bool
	Data    interface{}
	Error   string
}

var toolCallRegex = regexp.MustCompile(`\{"name":\s*"([^"]+)",\s*"params":\s*(\{[^}]*\})\}`)

func ParseToolCallsFromResponse(response string) ([]ParsedToolCall, error) {
	var calls []ParsedToolCall

	matches := toolCallRegex.FindAllStringSubmatch(response, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			name := match[1]
			paramsJSON := match[2]

			params := make(map[string]any)
			if paramsJSON != "" && paramsJSON != "{}" {
				if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
					continue
				}
			}

			calls = append(calls, ParsedToolCall{
				Name:   name,
				Params: params,
			})
		}
	}

	return calls, nil
}

func FormatToolResultsForLLM(originalResponse string, results []ToolCallResult) string {
	if len(results) == 0 {
		return originalResponse
	}

	var sb strings.Builder
	sb.WriteString(originalResponse)

	sb.WriteString("\n\n[Tool Results]\n")
	for _, r := range results {
		if r.Success {
			sb.WriteString(fmt.Sprintf("%s: SUCCESS\n", r.Name))
			if r.Data != nil {
				dataJSON, err := json.MarshalIndent(r.Data, "", "  ")
				if err == nil {
					sb.WriteString(fmt.Sprintf("Result:\n%s\n", string(dataJSON)))
				}
			}
		} else {
			sb.WriteString(fmt.Sprintf("%s: ERROR - %s\n", r.Name, r.Error))
		}
	}

	return sb.String()
}
