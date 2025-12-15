package memory

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileStore(t *testing.T) {
	// Setup temporary directory
	tmpDir, err := os.MkdirTemp("", "marinai_store_test")
	require.NoError(t, err, "Failed to create temp dir")
	defer os.RemoveAll(tmpDir)

	store := NewFileStore(tmpDir)
	userID := "test_user"

	// Test Add
	err = store.Add(userID, "Hello world", []float32{1.0, 0.0, 0.0})
	assert.NoError(t, err, "Failed to add item")

	err = store.Add(userID, "Pizza is good", []float32{0.0, 1.0, 0.0})
	assert.NoError(t, err, "Failed to add second item")

	// Test Search (Exact match)
	results, err := store.Search(userID, []float32{1.0, 0.0, 0.0}, 1)
	assert.NoError(t, err, "Failed to search")
	require.Len(t, results, 1, "Expected 1 result")
	assert.Equal(t, "Hello world", results[0], "Expected 'Hello world'")

	// Test Search (Similarity)
	// Vector {0.1, 0.9, 0.0} should be closer to {0.0, 1.0, 0.0} than {1.0, 0.0, 0.0}
	results, err = store.Search(userID, []float32{0.1, 0.9, 0.0}, 1)
	assert.NoError(t, err, "Failed to search")
	require.Len(t, results, 1, "Expected 1 result")
	assert.Equal(t, "Pizza is good", results[0], "Expected 'Pizza is good'")

	// Test Recent Messages
	err = store.AddRecentMessage(userID, "user", "Test message 1")
	assert.NoError(t, err, "Failed to add recent message")

	err = store.AddRecentMessage(userID, "assistant", "Test message 2")
	assert.NoError(t, err, "Failed to add second recent message")

	recent, err := store.GetRecentMessages(userID)
	assert.NoError(t, err, "Failed to get recent messages")
	require.Len(t, recent, 2, "Expected 2 recent messages")
	if len(recent) > 0 {
		assert.Equal(t, "Test message 1", recent[0].Text, "Unexpected first message text")
		assert.Equal(t, "user", recent[0].Role, "Unexpected first message role")
	}

	// Test Clear Recent Messages
	err = store.ClearRecentMessages(userID)
	assert.NoError(t, err, "Failed to clear recent messages")

	recent, err = store.GetRecentMessages(userID)
	assert.NoError(t, err, "Failed to get recent messages after clear")
	assert.Empty(t, recent, "Expected 0 recent messages after clear")

	// Test Delete User Data
	err = store.DeleteUserData(userID)
	assert.NoError(t, err, "Failed to delete user data")

	results, err = store.Search(userID, []float32{1.0, 0.0, 0.0}, 1)
	assert.NoError(t, err, "Failed to search after delete")
	assert.Empty(t, results, "Expected 0 results after delete")
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a    []float32
		b    []float32
		want float64
	}{
		{
			name: "Identical",
			a:    []float32{1, 0, 0},
			b:    []float32{1, 0, 0},
			want: 1.0,
		},
		{
			name: "Orthogonal",
			a:    []float32{1, 0, 0},
			b:    []float32{0, 1, 0},
			want: 0.0,
		},
		{
			name: "Opposite",
			a:    []float32{1, 0, 0},
			b:    []float32{-1, 0, 0},
			want: -1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosineSimilarity(tt.a, tt.b)
			assert.InDelta(t, tt.want, got, 0.0001, "cosineSimilarity()")
		})
	}
}
