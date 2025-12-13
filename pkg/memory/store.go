package memory

import (
	"encoding/json"
	"fmt"
	"marinai/pkg/cerebras"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type MemoryItem struct {
	Text      string    `json:"text"`
	Vector    []float32 `json:"vector"`
	Timestamp int64     `json:"timestamp"` // Unix timestamp
}

// EmbeddingClient interface for generating embeddings
type EmbeddingClient interface {
	Embed(text string) ([]float32, error)
}

// CerebrasClient interface for LLM chat completions
type CerebrasClient interface {
	ChatCompletion(messages []cerebras.Message) (string, error)
}

type RecentMessageItem struct {
	Role      string `json:"role"`
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
}

type Store interface {
	Add(userId string, text string, vector []float32) error
	Search(userId string, queryVector []float32, limit int) ([]string, error)
	// Recent messages cache
	AddRecentMessage(userId, role, message string) error
	GetRecentMessages(userId string) ([]RecentMessageItem, error)
	ClearRecentMessages(userId string) error
	// User data management
	DeleteUserData(userId string) error

	// Profile management
	GetFacts(userId string) ([]string, error)
	ApplyDelta(userId string, adds []string, removes []string) error
	DeleteFacts(userId string) error

	// Emoji Cache
	GetCachedEmojis(guildID string) ([]string, error)
	SetCachedEmojis(guildID string, emojis []string) error

	// Social Features
	GetAllKnownUsers() ([]string, error)
}

type FileStore struct {
	storageDir string
	mu         sync.RWMutex
}

func NewFileStore(storageDir string) *FileStore {
	_ = os.MkdirAll(storageDir, 0755)
	return &FileStore{
		storageDir: storageDir,
	}
}

func (vs *FileStore) getUserDir(userId string) string {
	return filepath.Join(vs.storageDir, userId)
}

func (vs *FileStore) getFilePath(userId string) string {
	userDir := vs.getUserDir(userId)
	_ = os.MkdirAll(userDir, 0755) // Ensure user directory exists
	return filepath.Join(userDir, "memory.json")
}

func (vs *FileStore) load(userId string) ([]MemoryItem, error) {
	path := vs.getFilePath(userId)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []MemoryItem{}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var items []MemoryItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (vs *FileStore) save(userId string, items []MemoryItem) error {
	path := vs.getFilePath(userId)
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (vs *FileStore) Add(userId string, text string, vector []float32) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	items, err := vs.load(userId)
	if err != nil {
		return err
	}

	// Check for duplicates using embedding similarity
	const duplicateThreshold = 0.8
	for _, item := range items {
		similarity := cosineSimilarity(vector, item.Vector)
		if similarity >= duplicateThreshold {
			// This is a duplicate, skip adding
			return fmt.Errorf("duplicate memory detected (similarity: %.4f): %s", similarity, text)
		}
	}

	items = append(items, MemoryItem{
		Text:      text,
		Vector:    vector,
		Timestamp: time.Now().Unix(),
	})

	return vs.save(userId, items)
}

func (vs *FileStore) Search(userId string, queryVector []float32, limit int) ([]string, error) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	items, err := vs.load(userId)
	if err != nil {
		return nil, err
	}

	type match struct {
		Text  string
		Score float64
	}

	var matches []match
	for _, item := range items {
		score := cosineSimilarity(queryVector, item.Vector)
		matches = append(matches, match{Text: item.Text, Score: score})
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	if len(matches) > limit {
		matches = matches[:limit]
	}

	var results []string
	for _, m := range matches {
		results = append(results, m.Text)
	}

	return results, nil
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// Recent messages cache methods

func (vs *FileStore) getRecentFilePath(userId string) string {
	userDir := vs.getUserDir(userId)
	_ = os.MkdirAll(userDir, 0755) // Ensure user directory exists
	return filepath.Join(userDir, "recent.json")
}

func (vs *FileStore) loadRecentMessages(userId string) ([]RecentMessageItem, error) {
	path := vs.getRecentFilePath(userId)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []RecentMessageItem{}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var messages []RecentMessageItem
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (vs *FileStore) saveRecentMessages(userId string, messages []RecentMessageItem) error {
	path := vs.getRecentFilePath(userId)
	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// AddRecentMessage adds a message to the recent messages cache (max 15)
func (vs *FileStore) AddRecentMessage(userId, role, message string) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	messages, err := vs.loadRecentMessages(userId)
	if err != nil {
		return err
	}

	messages = append(messages, RecentMessageItem{
		Role:      role,
		Text:      message,
		Timestamp: time.Now().Unix(),
	})

	// Keep only last 15 messages
	if len(messages) > 15 {
		messages = messages[len(messages)-15:]
	}

	return vs.saveRecentMessages(userId, messages)
}

// GetRecentMessages retrieves the recent messages for a user
func (vs *FileStore) GetRecentMessages(userId string) ([]RecentMessageItem, error) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	return vs.loadRecentMessages(userId)
}

// ClearRecentMessages clears the recent messages cache for a user
func (vs *FileStore) ClearRecentMessages(userId string) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	path := vs.getRecentFilePath(userId)
	// Remove the file if it exists
	if _, err := os.Stat(path); err == nil {
		return os.Remove(path)
	}
	return nil
}

// DeleteUserData deletes all data for a user (memory + recent messages)
func (vs *FileStore) DeleteUserData(userId string) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	userDir := vs.getUserDir(userId)
	// Remove the entire user directory if it exists
	if _, err := os.Stat(userDir); err == nil {
		return os.RemoveAll(userDir)
	}
	return nil
}

// Profile management (FileStore implementation - simplified for compatibility)

func (vs *FileStore) GetFacts(userId string) ([]string, error) {
	// For FileStore, we can just return empty for now as we are moving to SurrealDB
	// Or implement a simple JSON file if needed.
	return []string{}, nil
}

func (vs *FileStore) ApplyDelta(userId string, adds []string, removes []string) error {
	return nil
}

func (vs *FileStore) DeleteFacts(userId string) error {
	return nil
}

// Emoji Cache (FileStore implementation - simplified/stub)

func (vs *FileStore) GetCachedEmojis(guildID string) ([]string, error) {
	return nil, nil
}

func (vs *FileStore) SetCachedEmojis(guildID string, emojis []string) error {
	return nil
}

func (vs *FileStore) GetAllKnownUsers() ([]string, error) {
	return []string{}, nil
}
