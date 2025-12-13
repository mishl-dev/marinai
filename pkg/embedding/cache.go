package embedding

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// CachedClient wraps an embedding client with an in-memory LRU cache
type CachedClient struct {
	client   *Client
	cache    map[string][]float32
	order    []string // For LRU eviction
	maxSize  int
	mu       sync.RWMutex
	hits     int
	misses   int
}

// NewCachedClient creates a cached wrapper around the embedding client
func NewCachedClient(client *Client, maxSize int) *CachedClient {
	if maxSize <= 0 {
		maxSize = 500 // Default cache size
	}
	return &CachedClient{
		client:  client,
		cache:   make(map[string][]float32),
		order:   make([]string, 0, maxSize),
		maxSize: maxSize,
	}
}

// hashText creates a cache key from the text
func hashText(text string) string {
	h := sha256.New()
	h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))[:16] // Use first 16 chars for shorter keys
}

// Embed returns cached embedding or fetches from API
func (c *CachedClient) Embed(text string) ([]float32, error) {
	key := hashText(text)

	// Check cache first
	c.mu.RLock()
	if embedding, ok := c.cache[key]; ok {
		c.mu.RUnlock()
		c.mu.Lock()
		c.hits++
		// Move to end of order (most recently used)
		c.moveToEnd(key)
		c.mu.Unlock()
		return embedding, nil
	}
	c.mu.RUnlock()

	// Cache miss - fetch from API
	embedding, err := c.client.Embed(text)
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.mu.Lock()
	c.misses++
	c.set(key, embedding)
	c.mu.Unlock()

	return embedding, nil
}

// set adds an item to the cache, evicting oldest if necessary
func (c *CachedClient) set(key string, embedding []float32) {
	// If at capacity, evict oldest
	if len(c.cache) >= c.maxSize {
		oldest := c.order[0]
		delete(c.cache, oldest)
		c.order = c.order[1:]
	}

	c.cache[key] = embedding
	c.order = append(c.order, key)
}

// moveToEnd moves a key to the end of the order slice (LRU update)
func (c *CachedClient) moveToEnd(key string) {
	// Find and remove from current position
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
	// Add to end
	c.order = append(c.order, key)
}

// Stats returns cache hit/miss statistics
func (c *CachedClient) Stats() (hits, misses, size int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits, c.misses, len(c.cache)
}

// Clear empties the cache
func (c *CachedClient) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string][]float32)
	c.order = make([]string, 0, c.maxSize)
}
