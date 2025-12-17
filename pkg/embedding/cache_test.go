package embedding

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCachedClient_Embed_HitMiss(t *testing.T) {
	var requestCount int32
	expectedEmbedding := []float32{0.5, 0.6, 0.7}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		resp := map[string][][]float32{
			"embeddings": {expectedEmbedding},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL)
	cachedClient := NewCachedClient(client, 10)

	// First call - Cache Miss
	emb1, err := cachedClient.Embed("test request")
	require.NoError(t, err)
	assert.Equal(t, expectedEmbedding, emb1)
	assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount), "Should trigger HTTP request on cache miss")

	// Second call - Cache Hit
	emb2, err := cachedClient.Embed("test request")
	require.NoError(t, err)
	assert.Equal(t, expectedEmbedding, emb2)
	assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount), "Should NOT trigger HTTP request on cache hit")

	hits, misses, size := cachedClient.Stats()
	assert.Equal(t, 1, hits)
	assert.Equal(t, 1, misses)
	assert.Equal(t, 1, size)
}

func TestCachedClient_LRU(t *testing.T) {
	// Set maxSize to 2
	maxSize := 2

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock response based on input text to differentiate results
		var reqBody map[string][]string
		json.NewDecoder(r.Body).Decode(&reqBody)
		text := reqBody["texts"][0]

		var val float32
		if text == "one" {
			val = 1.0
		}
		if text == "two" {
			val = 2.0
		}
		if text == "three" {
			val = 3.0
		}

		resp := map[string][][]float32{
			"embeddings": {{val}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL)
	cachedClient := NewCachedClient(client, maxSize)

	// 1. Fill cache
	cachedClient.Embed("one")
	cachedClient.Embed("two")

	_, _, size := cachedClient.Stats()
	assert.Equal(t, 2, size)

	// 2. Access "one" to make it most recently used
	cachedClient.Embed("one")

	// 3. Add "three" - should evict "two" (because "one" was just accessed)
	cachedClient.Embed("three")

	// 4. Verify "two" is gone (misses should increment if we request it)
	// Reset stats for clarity (or just check misses count)

	// We expect "two" to trigger a new request if it was evicted
	// Currently misses = 3 (one, two, three)
	// Hits = 1 (one)

	cachedClient.Embed("two") // Should be a miss now

	hits, misses, size := cachedClient.Stats()

	// Hits: 1 (re-accessing "one")
	// Misses: 4 (one, two, three, two-again)
	assert.Equal(t, 1, hits)
	assert.Equal(t, 4, misses)
	assert.Equal(t, 2, size)
}

func TestCachedClient_Clear(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string][][]float32{
			"embeddings": {{0.0}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", server.URL)
	cachedClient := NewCachedClient(client, 10)

	cachedClient.Embed("test")
	_, _, size := cachedClient.Stats()
	assert.Equal(t, 1, size)

	cachedClient.Clear()
	_, _, size = cachedClient.Stats()
	assert.Equal(t, 0, size)
}
