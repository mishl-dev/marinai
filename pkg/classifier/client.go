package classifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	apiKey string
	apiURL string
	client *http.Client
}

func NewClient(apiKey, apiURL string) *Client {
	return &Client{
		apiKey: apiKey,
		apiURL: apiURL,
		client: &http.Client{},
	}
}

type Request struct {
	Inputs     string     `json:"inputs"`
	Parameters Parameters `json:"parameters"`
}

type Parameters struct {
	CandidateLabels []string `json:"candidate_labels"`
	MultiLabel      bool     `json:"multi_label,omitempty"`
}

type ClassificationResult struct {
	Label string  `json:"label"`
	Score float64 `json:"score"`
}

// Classify returns the top label and its score for the given text
func (c *Client) Classify(text string, labels []string) (string, float64, error) {
	reqBody := Request{
		Inputs: text,
		Parameters: Parameters{
			CandidateLabels: labels,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("api status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var apiResp []ClassificationResult
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", 0, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(apiResp) == 0 {
		return "", 0, fmt.Errorf("empty response from classifier")
	}

	// Return the top label and its score (first element has highest score)
	return apiResp[0].Label, apiResp[0].Score, nil
}
