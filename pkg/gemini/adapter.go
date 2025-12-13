package gemini

import "marinai/pkg/bot"

// Adapter wraps Client to implement bot.GeminiClient interface
type Adapter struct {
	client *Client
}

// NewAdapter creates an adapter that implements bot.GeminiClient
func NewAdapter(apiKey string) *Adapter {
	if apiKey == "" {
		return nil
	}
	return &Adapter{
		client: NewClient(apiKey),
	}
}

// DescribeImageFromURL implements bot.GeminiClient interface
func (a *Adapter) DescribeImageFromURL(imageURL string) (*bot.ImageDescription, error) {
	result, err := a.client.DescribeImageFromURL(imageURL)
	if err != nil {
		return nil, err
	}

	// Convert gemini.ImageDescription to bot.ImageDescription
	return &bot.ImageDescription{
		Description: result.Description,
		IsNSFW:      result.IsNSFW,
		Error:       result.Error,
	}, nil
}

// Classify implements bot.GeminiClient interface for text classification
func (a *Adapter) Classify(text string, labels []string) (string, float64, error) {
	return a.client.Classify(text, labels)
}
