package bot

type MockGeminiClient struct {
	DescribeImageFromURLFunc func(imageURL string) (*ImageDescription, error)
	ClassifyFunc             func(text string, labels []string) (string, float64, error)
}

func (m *MockGeminiClient) DescribeImageFromURL(imageURL string) (*ImageDescription, error) {
	if m.DescribeImageFromURLFunc != nil {
		return m.DescribeImageFromURLFunc(imageURL)
	}
	return &ImageDescription{Description: "A generic image description"}, nil
}

func (m *MockGeminiClient) Classify(text string, labels []string) (string, float64, error) {
	if m.ClassifyFunc != nil {
		return m.ClassifyFunc(text, labels)
	}
	// Default behavior: return the first label with high confidence
	if len(labels) > 0 {
		return labels[0], 0.9, nil
	}
	return "", 0, nil
}
