package cerebras

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	apiURL = "https://api.cerebras.ai/v1/chat/completions"
)

// thinkRegex matches <think>...</think> content, including newlines.
// (?s) enables the dot (.) to match new lines.
var thinkRegex = regexp.MustCompile(`(?s)<think>.*?</think>`)

// ModelConfig defines the ID and context limits for the prioritized list.
type ModelConfig struct {
	ID     string
	MaxCtx int
}

var PrioritizedModels = []ModelConfig{
	{ID: "llama-3.3-70b", MaxCtx: 65536},
	{ID: "zai-glm-4.6", MaxCtx: 64000},
	{ID: "llama3.1-8b", MaxCtx: 8192},
	{ID: "qwen-3-235b-a22b-instruct-2507", MaxCtx: 65536},
	{ID: "qwen-3-32b", MaxCtx: 65536},
	{ID: "gpt-oss-120b", MaxCtx: 65536},
}

type Client struct {
	apiKey      string
	client      *http.Client
	temperature float64
	topP        float64
	models      []ModelConfig
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model       string    `json:"model"`
	Stream      bool      `json:"stream"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature"`
	TopP        float64   `json:"top_p"`
	Messages    []Message `json:"messages"`
}

type Response struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// APIError captures non-200 responses to allow inspection of the status code.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("api status %d: %s", e.StatusCode, e.Body)
}

func NewClient(apiKey string, temperature, topP float64, models []ModelConfig) *Client {
	if len(models) == 0 {
		models = PrioritizedModels
	}
	return &Client{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
		temperature: temperature,
		topP:        topP,
		models:      models,
	}
}

// ChatCompletion attempts to get a response.
// If the API returns ANY non-2xx status (429, 500, 400, etc.), it cycles to the next model.
func (c *Client) ChatCompletion(messages []Message) (string, error) {
	var lastErr error

	for _, modelConf := range c.models {
		log.Printf("Attempting to use model: %s", modelConf.ID)
		reqBody := Request{
			Model:       modelConf.ID,
			Stream:      false,
			MaxTokens:   2000,
			Temperature: c.temperature,
			TopP:        c.topP,
			Messages:    messages,
		}

		content, err := c.makeRequest(reqBody)

		if err == nil {
			// Success: Received a 200 OK and valid content
			return content, nil
		}

		// Capture the error and cycle to the next model
		if apiErr, ok := err.(*APIError); ok {
			lastErr = fmt.Errorf("model %s failed with status %d: %w", modelConf.ID, apiErr.StatusCode, apiErr)
		} else {
			lastErr = fmt.Errorf("model %s network error: %w", modelConf.ID, err)
		}

		// Continue to the next model in the loop
	}

	// If we reach here, all models failed
	return "", fmt.Errorf("all models exhausted. Last error: %w", lastErr)
}

func (c *Client) makeRequest(reqBody Request) (string, error) {
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	// If status code is not 2xx (e.g., 200, 201), return an APIError.
	// This triggers the loop in ChatCompletion to try the next model.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(bodyBytes),
		}
	}

	var apiResp Response
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	content := apiResp.Choices[0].Message.Content

	// Remove <think> tags and their content from the response
	content = thinkRegex.ReplaceAllString(content, "")

	// Optional: Trim whitespace that might result from removing the tags
	content = strings.TrimSpace(content)

	// Remove surrounding quotes if present
	if len(content) >= 2 && strings.HasPrefix(content, "\"") && strings.HasSuffix(content, "\"") {
		content = content[1 : len(content)-1]
		content = strings.TrimSpace(content)
	}

	return content, nil
}
