package gemini

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	geminiAPIURL = "https://generativelanguage.googleapis.com/v1beta/models/gemini-flash-lite-latest:generateContent"
)

// Client handles image understanding via Gemini Vision API
type Client struct {
	apiKey string
	client *http.Client
}

// NewClient creates a new Gemini Vision client
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

// ImageDescription represents the result of analyzing an image
type ImageDescription struct {
	Description string
	IsNSFW      bool
	Error       error
}

// Request types for Gemini API
type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text       string          `json:"text,omitempty"`
	InlineData *geminiFileData `json:"inline_data,omitempty"`
}

type geminiFileData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason  string `json:"finishReason"`
		SafetyRatings []struct {
			Category    string `json:"category"`
			Probability string `json:"probability"`
		} `json:"safetyRatings"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason   string `json:"blockReason,omitempty"`
		SafetyRatings []struct {
			Category    string `json:"category"`
			Probability string `json:"probability"`
		} `json:"safetyRatings"`
	} `json:"promptFeedback,omitempty"`
}

// DescribeImage analyzes an image and returns a description suitable for the main LLM
func (c *Client) DescribeImage(imageData []byte, mimeType string) (*ImageDescription, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("gemini API key not configured")
	}

	// Validate MIME type
	validMimes := map[string]bool{
		"image/png":  true,
		"image/jpeg": true,
		"image/webp": true,
		"image/heic": true,
		"image/heif": true,
		"image/gif":  true,
	}
	if !validMimes[mimeType] {
		return nil, fmt.Errorf("unsupported image type: %s", mimeType)
	}

	// Base64 encode the image
	encodedImage := base64.StdEncoding.EncodeToString(imageData)

	// Build the request
	prompt := `You are helping describe an image for a chatbot.
Describe this image in 2-3 natural sentences as if you're telling a friend what you see.
Focus on:
- What's in the image (people, objects, scenes, text)
- The mood/vibe (cute, aesthetic, funny, etc.)

Keep it casual and brief. Don't start with "This image shows" - just describe it naturally.
If it's a selfie or person photo, describe their appearance/outfit/vibe.
If there's text in the image, mention what it says.`

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: prompt},
					{
						InlineData: &geminiFileData{
							MimeType: mimeType,
							Data:     encodedImage,
						},
					},
				},
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make the request
	url := fmt.Sprintf("%s?key=%s", geminiAPIURL, c.apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Check for content blocking
	if geminiResp.PromptFeedback.BlockReason != "" {
		return &ImageDescription{
			Description: "",
			IsNSFW:      true,
			Error:       fmt.Errorf("content blocked: %s", geminiResp.PromptFeedback.BlockReason),
		}, nil
	}

	// Extract the description
	if len(geminiResp.Candidates) == 0 {
		return nil, fmt.Errorf("no response candidates returned")
	}

	candidate := geminiResp.Candidates[0]

	// Check if response was blocked by safety
	if candidate.FinishReason == "SAFETY" {
		return &ImageDescription{
			Description: "",
			IsNSFW:      true,
			Error:       nil,
		}, nil
	}

	if len(candidate.Content.Parts) == 0 {
		return nil, fmt.Errorf("no content parts in response")
	}

	description := strings.TrimSpace(candidate.Content.Parts[0].Text)

	return &ImageDescription{
		Description: description,
		IsNSFW:      false,
		Error:       nil,
	}, nil
}

// DescribeImageFromURL fetches an image from a URL and describes it
func (c *Client) DescribeImageFromURL(imageURL string) (*ImageDescription, error) {
	// Fetch the image
	resp, err := c.client.Get(imageURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch image (status %d)", resp.StatusCode)
	}

	// Read the image data
	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}

	// Determine MIME type from Content-Type header or URL
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		// Try to guess from URL
		if strings.HasSuffix(imageURL, ".png") {
			contentType = "image/png"
		} else if strings.HasSuffix(imageURL, ".gif") {
			contentType = "image/gif"
		} else if strings.HasSuffix(imageURL, ".webp") {
			contentType = "image/webp"
		} else {
			contentType = "image/jpeg" // Default assumption
		}
	}

	// Strip any charset or extra params from content type
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}

	return c.DescribeImage(imageData, contentType)
}

// Classify uses Gemini to classify text into one of the provided labels
// Returns the best matching label and a confidence score (0.0-1.0)
func (c *Client) Classify(text string, labels []string) (string, float64, error) {
	if c.apiKey == "" {
		return "", 0, fmt.Errorf("gemini API key not configured")
	}

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

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: prompt},
				},
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s?key=%s", geminiAPIURL, c.apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("gemini API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
		return "", 0, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", 0, fmt.Errorf("no response from Gemini")
	}

	// Parse the JSON response
	responseText := strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text)

	// Strip markdown code blocks if present
	if strings.HasPrefix(responseText, "```") {
		// Find first newline
		if idx := strings.Index(responseText, "\n"); idx != -1 {
			// Find last newline
			if lastIdx := strings.LastIndex(responseText, "\n"); lastIdx > idx {
				responseText = responseText[idx+1 : lastIdx]
			}
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
