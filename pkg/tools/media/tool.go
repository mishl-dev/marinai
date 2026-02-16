package media

import (
	"context"
	"encoding/base64"
	"fmt"

	"marinai/pkg/tools"
)

const (
	ImageToolName      = "image_compress"
	PDFToolName        = "pdf_extract"
	TranscribeToolName = "audio_transcribe"
)

type ImageTool struct {
	processor *ImageProcessor
}

func NewImageTool() *ImageTool {
	return &ImageTool{
		processor: NewImageProcessor(DefaultCompressionOptions()),
	}
}

func (t *ImageTool) Name() string {
	return ImageToolName
}

func (t *ImageTool) Description() string {
	return "Compress and resize images. Returns compressed image data and metadata."
}

func (t *ImageTool) Parameters() tools.ParameterSchema {
	return tools.ParameterSchema{
		Type: "object",
		Properties: map[string]tools.PropertySchema{
			"image_data": {
				Type:        "string",
				Description: "Base64-encoded image data",
			},
			"quality": {
				Type:        "integer",
				Description: "Compression quality (1-100, default: 75)",
			},
			"max_width": {
				Type:        "integer",
				Description: "Maximum width in pixels (default: 1920)",
			},
			"max_height": {
				Type:        "integer",
				Description: "Maximum height in pixels (default: 1080)",
			},
		},
		Required: []string{"image_data"},
	}
}

func (t *ImageTool) Execute(ctx context.Context, params map[string]any, toolCtx *tools.ToolContext) (tools.Result, error) {
	dataB64, ok := params["image_data"].(string)
	if !ok {
		return tools.Result{}, fmt.Errorf("image_data must be a string")
	}

	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return tools.Result{}, fmt.Errorf("decode base64: %w", err)
	}

	opts := DefaultCompressionOptions()
	if quality, ok := params["quality"].(float64); ok {
		opts.Quality = int(quality)
	}
	if maxWidth, ok := params["max_width"].(float64); ok {
		opts.MaxWidth = int(maxWidth)
	}
	if maxHeight, ok := params["max_height"].(float64); ok {
		opts.MaxHeight = int(maxHeight)
	}

	processor := NewImageProcessor(opts)
	compressed, info, err := processor.Compress(data)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	return tools.Result{
		Success: true,
		Data: map[string]interface{}{
			"image_data": base64.StdEncoding.EncodeToString(compressed),
			"info":       info,
		},
	}, nil
}

type PDFTool struct {
	parser *PDFParser
}

func NewPDFTool() *PDFTool {
	return &PDFTool{
		parser: NewPDFParser(),
	}
}

func (t *PDFTool) Name() string {
	return PDFToolName
}

func (t *PDFTool) Description() string {
	return "Extract text from PDF documents. Returns extracted text and metadata."
}

func (t *PDFTool) Parameters() tools.ParameterSchema {
	return tools.ParameterSchema{
		Type: "object",
		Properties: map[string]tools.PropertySchema{
			"pdf_data": {
				Type:        "string",
				Description: "Base64-encoded PDF data",
			},
			"start_page": {
				Type:        "integer",
				Description: "Start page (optional)",
			},
			"end_page": {
				Type:        "integer",
				Description: "End page (optional)",
			},
		},
		Required: []string{"pdf_data"},
	}
}

func (t *PDFTool) Execute(ctx context.Context, params map[string]any, toolCtx *tools.ToolContext) (tools.Result, error) {
	dataB64, ok := params["pdf_data"].(string)
	if !ok {
		return tools.Result{}, fmt.Errorf("pdf_data must be a string")
	}

	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return tools.Result{}, fmt.Errorf("decode base64: %w", err)
	}

	var text string
	var info *PDFInfo

	startPage, hasStart := params["start_page"].(float64)
	endPage, hasEnd := params["end_page"].(float64)

	if hasStart || hasEnd {
		start := 1
		end := 0
		if hasStart {
			start = int(startPage)
		}
		if hasEnd {
			end = int(endPage)
		}
		text, err = t.parser.ExtractPageRange(data, start, end)
	} else {
		text, info, err = t.parser.ExtractText(data)
	}

	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	return tools.Result{
		Success: true,
		Data: map[string]interface{}{
			"text": text,
			"info": info,
		},
	}, nil
}

type TranscribeTool struct {
	transcriber *AudioTranscriber
}

func NewTranscribeTool() *TranscribeTool {
	return &TranscribeTool{
		transcriber: NewAudioTranscriber(""),
	}
}

func NewTranscribeToolWithImage(dockerImage string) *TranscribeTool {
	return &TranscribeTool{
		transcriber: NewAudioTranscriberWithImage("", dockerImage),
	}
}

func (t *TranscribeTool) Name() string {
	return TranscribeToolName
}

func (t *TranscribeTool) Description() string {
	return "Transcribe audio to text using Whisper in a Docker container. Returns transcript text. Requires Docker to be running."
}

func (t *TranscribeTool) Parameters() tools.ParameterSchema {
	return tools.ParameterSchema{
		Type: "object",
		Properties: map[string]tools.PropertySchema{
			"audio_data": {
				Type:        "string",
				Description: "Base64-encoded audio data (WAV preferred)",
			},
			"language": {
				Type:        "string",
				Description: "Audio language code (default: en)",
			},
		},
		Required: []string{"audio_data"},
	}
}

func (t *TranscribeTool) Execute(ctx context.Context, params map[string]any, toolCtx *tools.ToolContext) (tools.Result, error) {
	dataB64, ok := params["audio_data"].(string)
	if !ok {
		return tools.Result{}, fmt.Errorf("audio_data must be a string")
	}

	data, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return tools.Result{}, fmt.Errorf("decode base64: %w", err)
	}

	if lang, ok := params["language"].(string); ok {
		t.transcriber.SetLanguage(lang)
	}

	result, err := t.transcriber.Transcribe(data)
	if err != nil {
		return tools.Result{Success: false, Error: err.Error()}, nil
	}

	return tools.Result{
		Success: true,
		Data:    result,
	}, nil
}
