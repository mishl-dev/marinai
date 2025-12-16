package embedding

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Embed_Success(t *testing.T) {
	expectedEmbedding := []float32{0.1, 0.2, 0.3}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		// Verify request body
		var reqBody map[string][]string
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)
		assert.Equal(t, []string{"hello"}, reqBody["texts"])

		// Send response
		resp := map[string][][]float32{
			"embeddings": {expectedEmbedding},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL)
	embedding, err := client.Embed("hello")

	require.NoError(t, err)
	assert.Equal(t, expectedEmbedding, embedding)
}

func TestClient_Embed_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL)
	_, err := client.Embed("hello")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "api returned error status: 500")
}

func TestClient_Embed_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{invalid-json"))
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL)
	embedding, err := client.Embed("hello")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode response")
	assert.Nil(t, embedding)
}

func TestClient_Embed_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string][][]float32{
			"embeddings": {},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL)
	embedding, err := client.Embed("hello")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no embeddings returned")
	assert.Nil(t, embedding)
}
