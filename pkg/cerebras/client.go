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

		start := time.Now()
		content, err := c.makeRequest(reqBody)
		duration := time.Since(start)

		if err == nil {
			// Success: Received a 200 OK and valid content
			log.Printf("Model %s success (took %v, %d chars)", modelConf.ID, duration, len(content))
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

// Classify uses llama3.1-8b (fast model) to classify text into one of the provided labels
// Returns the best matching label and a confidence score (0.0-1.0)
func (c *Client) Classify(text string, labels []string) (string, float64, error) {
	// Build the labels list for the prompt
	labelsStr := ""
	for i, label := range labels {
		labelsStr += fmt.Sprintf("%d. %s\n", i+1, label)
	}

	prompt := fmt.Sprintf(`Classify this text into exactly ONE of the following categories:

%s
Text to classify: "%s"

Output a JSON object with:
- "label": the exact category name that best matches
- "confidence": a number from 0.0 to 1.0 indicating how confident you are

Output ONLY valid JSON. Example: {"label": "neutral", "confidence": 0.85}`, labelsStr, text)

	messages := []Message{
		{Role: "system", Content: "You are a text classifier. Output only valid JSON."},
		{Role: "user", Content: prompt},
	}

	// Use llama3.1-8b specifically for classification (fast and sufficient)
	reqBody := Request{
		Model:       "llama3.1-8b",
		Stream:      false,
		MaxTokens:   100, // Classification only needs short output
		Temperature: 0.1, // Low temperature for consistent classification
		TopP:        0.9,
		Messages:    messages,
	}

	resp, err := c.makeRequest(reqBody)
	if err != nil {
		return labels[0], 0.5, err // Fallback to first label
	}

	// Parse the JSON response
	responseText := strings.TrimSpace(resp)

	// Strip markdown code blocks if present
	if strings.HasPrefix(responseText, "```") {
		lines := strings.Split(responseText, "\n")
		if len(lines) >= 2 {
			responseText = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	responseText = strings.TrimSpace(responseText)

	var result struct {
		Label      string  `json:"label"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		// If JSON parsing fails, return the first label as fallback
		return labels[0], 0.5, nil
	}

	return result.Label, result.Confidence, nil
}
