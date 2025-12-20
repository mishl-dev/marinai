package gemini

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDescribeImageFromURL(t *testing.T) {
	// Mock image server
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		// Send a dummy 1x1 PNG image
		w.Write([]byte{
			0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
			0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
			0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
			0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
			0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
			0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
		})
	}))
	defer imageServer.Close()

	// Mock Gemini API
	geminiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1beta/models/gemini-flash-lite-latest:generateContent", r.URL.Path)
		assert.Equal(t, "test-key", r.URL.Query().Get("key"))

		w.Header().Set("Content-Type", "application/json")
		response := geminiResponse{
			Candidates: []struct {
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
			}{
				{
					Content: struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					}{
						Parts: []struct {
							Text string `json:"text"`
						}{
							{Text: "A test image description."},
						},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer geminiServer.Close()

	// Initialize client
	client := NewClient("test-key")
	client.apiURL = geminiServer.URL + "/v1beta/models/gemini-flash-lite-latest:generateContent"
	client.allowLocalIPs = true // Enable testing with local server

	// Test DescribeImageFromURL (Happy Path)
	desc, err := client.DescribeImageFromURL(imageServer.URL + "/image.png")
	require.NoError(t, err)
	assert.Equal(t, "A test image description.", desc.Description)
	assert.False(t, desc.IsNSFW)
}

func TestDescribeImageFromURL_SSRF(t *testing.T) {
	// Initialize client
	client := NewClient("test-key")
	client.allowLocalIPs = false // Explicitly enable security checks

	// Test Localhost Access Blocking
	// We can use any localhost URL, it doesn't need to exist because validation happens first
	desc, err := client.DescribeImageFromURL("http://127.0.0.1:8080/image.png")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "blocked access to restricted IP")
	assert.Nil(t, desc)

	desc, err = client.DescribeImageFromURL("http://localhost:8080/image.png")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "blocked access to restricted IP")
	assert.Nil(t, desc)
}

func TestClassify(t *testing.T) {
	// Mock Gemini API
	geminiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1beta/models/gemini-flash-lite-latest:generateContent", r.URL.Path)

		var req geminiRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		// Verify prompt contains labels
		assert.Contains(t, req.Contents[0].Parts[0].Text, "Classify this text")
		assert.Contains(t, req.Contents[0].Parts[0].Text, "label1")
		assert.Contains(t, req.Contents[0].Parts[0].Text, "label2")

		w.Header().Set("Content-Type", "application/json")
		response := geminiResponse{
			Candidates: []struct {
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
			}{
				{
					Content: struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					}{
						Parts: []struct {
							Text string `json:"text"`
						}{
							{Text: `{"label": "label1", "confidence": 0.95}`},
						},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer geminiServer.Close()

	// Initialize client
	client := NewClient("test-key")
	client.apiURL = geminiServer.URL + "/v1beta/models/gemini-flash-lite-latest:generateContent"

	// Test Classify
	label, confidence, err := client.Classify("some text", []string{"label1", "label2"})
	require.NoError(t, err)
	assert.Equal(t, "label1", label)
	assert.Equal(t, 0.95, confidence)
}

func TestClassify_InvalidJSON(t *testing.T) {
	// Mock Gemini API returning invalid JSON
	geminiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := geminiResponse{
			Candidates: []struct {
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
			}{
				{
					Content: struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					}{
						Parts: []struct {
							Text string `json:"text"`
						}{
							{Text: `This is not JSON`},
						},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer geminiServer.Close()

	// Initialize client
	client := NewClient("test-key")
	client.apiURL = geminiServer.URL + "/v1beta/models/gemini-flash-lite-latest:generateContent"

	// Test Classify fallback
	labels := []string{"label1", "label2"}
	label, confidence, err := client.Classify("some text", labels)
	require.NoError(t, err)
	assert.Equal(t, labels[0], label) // Expect fallback to first label
	assert.Equal(t, 0.5, confidence)
}
