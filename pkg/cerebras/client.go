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
	"sync"
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

// KeyState tracks the health of an API key
type KeyState struct {
	Key          string
	FailureCount int
	LastUsed     time.Time
	LastSuccess  time.Time
}

type Client struct {
	keys        []*KeyState
	keyMu       sync.RWMutex
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

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type Response struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
}

// APIError captures non-200 responses to allow inspection of the status code.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("api status %d: %s", e.StatusCode, e.Body)
}

// NewClient creates a client with support for multiple API keys (comma-separated)
// Keys are rotated based on failure count (least failures first)
func NewClient(apiKeys string, temperature, topP float64, models []ModelConfig) *Client {
	if len(models) == 0 {
		models = PrioritizedModels
	}

	// Parse comma-separated keys
	keyStrings := strings.Split(apiKeys, ",")
	keys := make([]*KeyState, 0, len(keyStrings))
	for _, k := range keyStrings {
		k = strings.TrimSpace(k)
		if k != "" {
			keys = append(keys, &KeyState{
				Key:          k,
				FailureCount: 0,
				LastUsed:     time.Time{},
				LastSuccess:  time.Time{},
			})
		}
	}

	if len(keys) == 0 {
		log.Println("Warning: No API keys provided")
	} else {
		log.Printf("Loaded %d Cerebras API key(s)", len(keys))
	}

	return &Client{
		keys: keys,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
		temperature: temperature,
		topP:        topP,
		models:      models,
	}
}

// getBestKey returns the API key with the least failures
func (c *Client) getBestKey() *KeyState {
	c.keyMu.RLock()
	defer c.keyMu.RUnlock()

	if len(c.keys) == 0 {
		return nil
	}

	// Find key with least failures
	best := c.keys[0]
	for _, k := range c.keys[1:] {
		if k.FailureCount < best.FailureCount {
			best = k
		}
	}
	return best
}

// recordSuccess marks a key as successful
func (c *Client) recordSuccess(key *KeyState) {
	c.keyMu.Lock()
	defer c.keyMu.Unlock()
	key.LastSuccess = time.Now()
	key.LastUsed = time.Now()
	// Reduce failure count on success (gradual recovery)
	if key.FailureCount > 0 {
		key.FailureCount--
	}
}

// recordFailure marks a key as failed
func (c *Client) recordFailure(key *KeyState) {
	c.keyMu.Lock()
	defer c.keyMu.Unlock()
	key.FailureCount++
	key.LastUsed = time.Now()
}

// ChatCompletion attempts to get a response.
// Uses intelligent key rotation (least failures first) and model fallback.
func (c *Client) ChatCompletion(messages []Message) (string, error) {
	var lastErr error

	// Get the best API key (least failures)
	keyState := c.getBestKey()
	if keyState == nil {
		return "", fmt.Errorf("no API keys configured")
	}

	for _, modelConf := range c.models {
		log.Printf("Attempting model: %s (key failures: %d)", modelConf.ID, keyState.FailureCount)
		reqBody := Request{
			Model:       modelConf.ID,
			Stream:      false,
			MaxTokens:   2000,
			Temperature: c.temperature,
			TopP:        c.topP,
			Messages:    messages,
		}

		start := time.Now()
		content, usage, err := c.makeRequestWithKey(reqBody, keyState.Key)
		duration := time.Since(start)

		if err == nil {
			// Success: Received a 200 OK and valid content
			c.recordSuccess(keyState)
			log.Printf("Model %s success (took %v, input_tokens=%d, output_tokens=%d, total_tokens=%d)",
				modelConf.ID, duration, usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
			return content, nil
		}

		// Check if it's a rate limit or auth error - try another key
		if apiErr, ok := err.(*APIError); ok {
			if apiErr.StatusCode == 429 || apiErr.StatusCode == 401 || apiErr.StatusCode == 403 {
				c.recordFailure(keyState)
				// Try to get another key
				nextKey := c.getBestKey()
				if nextKey != nil && nextKey != keyState {
					log.Printf("Key rate limited/auth failed, trying another key...")
					keyState = nextKey
					// Retry same model with new key
					content, usage, err = c.makeRequestWithKey(reqBody, keyState.Key)
					if err == nil {
						c.recordSuccess(keyState)
						log.Printf("Model %s success with alternate key (took %v, input_tokens=%d, output_tokens=%d, total_tokens=%d)",
							modelConf.ID, time.Since(start), usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
						return content, nil
					}
				}
			}
			lastErr = fmt.Errorf("model %s failed with status %d: %w", modelConf.ID, apiErr.StatusCode, apiErr)
		} else {
			lastErr = fmt.Errorf("model %s network error: %w", modelConf.ID, err)
		}

		// Continue to the next model in the loop
	}

	// All models failed, record failure on the key
	c.recordFailure(keyState)
	return "", fmt.Errorf("all models exhausted. Last error: %w", lastErr)
}

func (c *Client) makeRequestWithKey(reqBody Request, apiKey string) (string, Usage, error) {
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", Usage{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", Usage{}, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", Usage{}, fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	// If status code is not 2xx (e.g., 200, 201), return an APIError.
	// This triggers the loop in ChatCompletion to try the next model.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", Usage{}, &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(bodyBytes),
		}
	}

	var apiResp Response
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", Usage{}, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return "", Usage{}, fmt.Errorf("no choices in response")
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

	return content, apiResp.Usage, nil
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

	// Get best key for the request
	keyState := c.getBestKey()
	if keyState == nil {
		return labels[0], 0.5, fmt.Errorf("no API keys configured")
	}

	resp, _, err := c.makeRequestWithKey(reqBody, keyState.Key)
	if err != nil {
		c.recordFailure(keyState)
		return labels[0], 0.5, err // Fallback to first label
	}
	c.recordSuccess(keyState)

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
