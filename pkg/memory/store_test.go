package memory

import (
	"os"
	"testing"
)

func TestFileStore(t *testing.T) {
	// Setup temporary directory
	tmpDir, err := os.MkdirTemp("", "marinai_store_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewFileStore(tmpDir)
	userId := "test_user"

	// Test Add
	err = store.Add(userId, "Hello world", []float32{1.0, 0.0, 0.0})
	if err != nil {
		t.Errorf("Failed to add item: %v", err)
	}

	err = store.Add(userId, "Pizza is good", []float32{0.0, 1.0, 0.0})
	if err != nil {
		t.Errorf("Failed to add second item: %v", err)
	}

	// Test Search (Exact match)
	results, err := store.Search(userId, []float32{1.0, 0.0, 0.0}, 1)
	if err != nil {
		t.Errorf("Failed to search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if results[0] != "Hello world" {
		t.Errorf("Expected 'Hello world', got '%s'", results[0])
	}

	// Test Search (Similarity)
	// Vector {0.1, 0.9, 0.0} should be closer to {0.0, 1.0, 0.0} than {1.0, 0.0, 0.0}
	results, err = store.Search(userId, []float32{0.1, 0.9, 0.0}, 1)
	if err != nil {
		t.Errorf("Failed to search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if results[0] != "Pizza is good" {
		t.Errorf("Expected 'Pizza is good', got '%s'", results[0])
	}

	// Test Recent Messages
	err = store.AddRecentMessage(userId, "user", "Test message 1")
	if err != nil {
		t.Errorf("Failed to add recent message: %v", err)
	}

	err = store.AddRecentMessage(userId, "assistant", "Test message 2")
	if err != nil {
		t.Errorf("Failed to add second recent message: %v", err)
	}

	recent, err := store.GetRecentMessages(userId)
	if err != nil {
		t.Errorf("Failed to get recent messages: %v", err)
	}
	if len(recent) != 2 {
		t.Errorf("Expected 2 recent messages, got %d", len(recent))
	}
	if len(recent) > 0 {
		if recent[0].Text != "Test message 1" || recent[0].Role != "user" {
			t.Errorf("Unexpected first message: %+v", recent[0])
		}
	}

	// Test Clear Recent Messages
	err = store.ClearRecentMessages(userId)
	if err != nil {
		t.Errorf("Failed to clear recent messages: %v", err)
	}

	recent, err = store.GetRecentMessages(userId)
	if err != nil {
		t.Errorf("Failed to get recent messages after clear: %v", err)
	}
	if len(recent) != 0 {
		t.Errorf("Expected 0 recent messages after clear, got %d", len(recent))
	}

	// Test Delete User Data
	err = store.DeleteUserData(userId)
	if err != nil {
		t.Errorf("Failed to delete user data: %v", err)
	}

	results, err = store.Search(userId, []float32{1.0, 0.0, 0.0}, 1)
	if err != nil {
		t.Errorf("Failed to search after delete: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results after delete, got %d", len(results))
	}
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
			// Allow small float error
			if got < tt.want-0.0001 || got > tt.want+0.0001 {
				t.Errorf("cosineSimilarity() = %v, want %v", got, tt.want)
			}
		})
	}
}
