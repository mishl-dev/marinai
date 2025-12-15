package cerebras

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatCompletion(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req Request
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		resp := Response{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{
					Message: struct {
						Content string `json:"content"`
					}{
						Content: "Hello world!",
					},
				},
			},
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Override apiURL for testing (this requires a small refactor or using a global var override if possible)
	// Since apiURL is a const in client.go, we cannot change it directly.
	// However, we can use a transport to hijack the request.

	client := NewClient("test-key", 0.7, 0.9, []ModelConfig{{ID: "test-model", MaxCtx: 100}})
	// Replace the internal http client's transport to route to our test server
	client.client.Transport = &mockTransport{
		TargetURL: server.URL,
	}

	messages := []Message{{Role: "user", Content: "Hi"}}
	response, err := client.ChatCompletion(messages)

	require.NoError(t, err)
	assert.Equal(t, "Hello world!", response)
}

type mockTransport struct {
	TargetURL string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Re-route request to the mock server
	u := m.TargetURL
	req.URL.Scheme = "http"
	req.URL.Host = u[7:] // remove http://
	return http.DefaultTransport.RoundTrip(req)
}
