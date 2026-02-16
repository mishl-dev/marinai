package webscrape

import (
	"context"
	"fmt"

	"marinai/pkg/tools"
)

const (
	ScrapeToolName  = "web_scrape"
	YouTubeToolName = "youtube_transcript"
)

type ScrapeTool struct {
	client      *Client
	readability *ReadabilityExtractor
	turndown    *TurndownConverter
}

func NewScrapeTool() *ScrapeTool {
	return &ScrapeTool{
		client:      NewClient(),
		readability: NewReadabilityExtractor(),
		turndown:    NewTurndownConverter(),
	}
}

func (t *ScrapeTool) Name() string {
	return ScrapeToolName
}

func (t *ScrapeTool) Description() string {
	return "Fetch and extract content from a web page. Returns the main content as Markdown."
}

func (t *ScrapeTool) Parameters() tools.ParameterSchema {
	return tools.ParameterSchema{
		Type: "object",
		Properties: map[string]tools.PropertySchema{
			"url": {
				Type:        "string",
				Description: "The URL to scrape",
			},
			"format": {
				Type:        "string",
				Description: "Output format: 'markdown' (default) or 'text'",
				Enum:        []string{"markdown", "text"},
			},
		},
		Required: []string{"url"},
	}
}

func (t *ScrapeTool) Execute(ctx context.Context, params map[string]any, toolCtx *tools.ToolContext) (tools.Result, error) {
	url, ok := params["url"].(string)
	if !ok {
		return tools.Result{}, fmt.Errorf("url parameter must be a string")
	}

	page, err := t.client.Fetch(ctx, url)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	if !IsHTML(page.ContentType) {
		return tools.Result{
			Success: false,
			Error:   fmt.Sprintf("expected HTML, got %s", page.ContentType),
		}, nil
	}

	article, err := t.readability.Extract(page.HTML)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	format := "markdown"
	if f, ok := params["format"].(string); ok {
		format = f
	}

	var content string
	if format == "markdown" {
		content, err = t.turndown.Convert(article.Content)
	} else {
		content = article.TextContent
	}

	result := &ScrapedContent{
		URL:      url,
		Title:    article.Title,
		Content:  content,
		Author:   article.Author,
		SiteName: article.SiteName,
		Excerpt:  article.Excerpt,
	}

	return tools.Result{Success: true, Data: result}, nil
}

type YouTubeTool struct {
	extractor *YouTubeExtractor
}

func NewYouTubeTool() *YouTubeTool {
	return &YouTubeTool{
		extractor: NewYouTubeExtractor(),
	}
}

func (t *YouTubeTool) Name() string {
	return YouTubeToolName
}

func (t *YouTubeTool) Description() string {
	return "Get the transcript from a YouTube video. Returns timestamped text."
}

func (t *YouTubeTool) Parameters() tools.ParameterSchema {
	return tools.ParameterSchema{
		Type: "object",
		Properties: map[string]tools.PropertySchema{
			"url": {
				Type:        "string",
				Description: "YouTube video URL or video ID",
			},
			"include_timestamps": {
				Type:        "boolean",
				Description: "Include timestamps in output (default: false)",
			},
		},
		Required: []string{"url"},
	}
}

func (t *YouTubeTool) Execute(ctx context.Context, params map[string]any, toolCtx *tools.ToolContext) (tools.Result, error) {
	input, ok := params["url"].(string)
	if !ok {
		return tools.Result{}, fmt.Errorf("url parameter must be a string")
	}

	var videoID string
	if IsYouTubeURL(input) {
		var err error
		videoID, err = t.extractor.GetVideoID(input)
		if err != nil {
			return tools.Result{Success: false, Error: err.Error()}, nil
		}
	} else {
		videoID = input
	}

	transcript, err := t.extractor.GetTranscript(ctx, videoID)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	includeTimestamps := false
	if ts, ok := params["include_timestamps"].(bool); ok {
		includeTimestamps = ts
	}

	var output string
	if includeTimestamps {
		output = transcript.GetTranscriptWithTimestamps()
	} else {
		output = transcript.GetTranscriptText()
	}

	return tools.Result{
		Success: true,
		Data: map[string]interface{}{
			"video_id":   videoID,
			"title":      transcript.Title,
			"transcript": output,
		},
	}, nil
}
