package vision

import "marinai/pkg/bot"

// Adapter wraps Client to implement bot.VisionClient interface
type Adapter struct {
	client *Client
}

// NewAdapter creates an adapter that implements bot.VisionClient
func NewAdapter(apiKey string) *Adapter {
	if apiKey == "" {
		return nil
	}
	return &Adapter{
		client: NewClient(apiKey),
	}
}

// DescribeImageFromURL implements bot.VisionClient interface
func (a *Adapter) DescribeImageFromURL(imageURL string) (*bot.ImageDescription, error) {
	result, err := a.client.DescribeImageFromURL(imageURL)
	if err != nil {
		return nil, err
	}

	// Convert vision.ImageDescription to bot.ImageDescription
	return &bot.ImageDescription{
		Description: result.Description,
		IsNSFW:      result.IsNSFW,
		Error:       result.Error,
	}, nil
}
