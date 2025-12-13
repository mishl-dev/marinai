package memory

import (
	"fmt"
	"marinai/pkg/surreal"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupSurrealTest handles the common setup logic for SurrealDB tests.
// It returns a store instance and a cleanup function (which closes the client).
// It skips the test if environment variables are missing.
func setupSurrealTest(t *testing.T) (*SurrealStore, func()) {
	// Load .env from project root
	if err := godotenv.Load("../../.env"); err != nil {
		t.Log("Warning: Error loading .env file")
	}

	// Get SurrealDB connection details
	surrealHost := os.Getenv("SURREAL_DB_HOST")
	surrealUser := os.Getenv("SURREAL_DB_USER")
	surrealPass := os.Getenv("SURREAL_DB_PASS")
	surrealNS := os.Getenv("SURREAL_DB_NAMESPACE")
	surrealDB := os.Getenv("SURREAL_DB_DATABASE")

	if surrealHost == "" || surrealUser == "" || surrealPass == "" {
		t.Skip("Skipping SurrealDB test: Missing environment variables")
		return nil, nil // Unreachable due to Skip
	}

	if surrealNS == "" {
		surrealNS = "marin"
	}
	if surrealDB == "" {
		surrealDB = "memory"
	}

	// Add protocol if missing
	if len(surrealHost) > 0 && surrealHost[:4] != "ws://" && surrealHost[:5] != "wss://" {
		surrealHost = "wss://" + surrealHost + "/rpc"
	}

	// Connect to SurrealDB
	client, err := surreal.NewClient(surrealHost, surrealUser, surrealPass, surrealNS, surrealDB)
	if err != nil {
		t.Fatalf("Failed to connect to SurrealDB: %v", err)
	}

	// Create store and initialize schema
	store := NewSurrealStore(client)

	return store, func() {
		client.Close()
	}
}

func TestSurrealStore_ApplyDelta(t *testing.T) {
	store, cleanup := setupSurrealTest(t)
	defer cleanup()

	// Test user ID
	testUserID := "test_user_memory_facts"

	// Clean up before test
	if err := store.DeleteFacts(testUserID); err != nil {
		t.Logf("Warning: Failed to clean up before test: %v", err)
	}

	// Test 1: Add facts to empty profile
	t.Run("Add facts to empty profile", func(t *testing.T) {
		adds := []string{"loves pizza", "lives in Tokyo"}
		removes := []string{}

		err := store.ApplyDelta(testUserID, adds, removes)
		require.NoError(t, err, "Failed to apply delta (add)")

		// Verify facts were added
		facts, err := store.GetFacts(testUserID)
		require.NoError(t, err, "Failed to get facts")

		assert.Len(t, facts, 2, "Expected 2 facts")

		// Check if facts contain expected values
		assert.Contains(t, facts, "loves pizza")
		assert.Contains(t, facts, "lives in Tokyo")

		t.Logf("✓ Successfully added facts: %v", facts)
	})

	// Test 2: Add more facts
	t.Run("Add more facts", func(t *testing.T) {
		adds := []string{"works as engineer"}
		removes := []string{}

		err := store.ApplyDelta(testUserID, adds, removes)
		require.NoError(t, err, "Failed to apply delta (add more)")

		facts, err := store.GetFacts(testUserID)
		require.NoError(t, err, "Failed to get facts")

		assert.Len(t, facts, 3, "Expected 3 facts")

		t.Logf("✓ Successfully added more facts: %v", facts)
	})

	// Test 3: Remove a fact
	t.Run("Remove a fact", func(t *testing.T) {
		adds := []string{}
		removes := []string{"lives in Tokyo"}

		err := store.ApplyDelta(testUserID, adds, removes)
		require.NoError(t, err, "Failed to apply delta (remove)")

		facts, err := store.GetFacts(testUserID)
		require.NoError(t, err, "Failed to get facts")

		assert.Len(t, facts, 2, "Expected 2 facts after removal")

		// Verify "lives in Tokyo" was removed
		assert.NotContains(t, facts, "lives in Tokyo", "Fact 'lives in Tokyo' should have been removed")

		t.Logf("✓ Successfully removed fact. Remaining: %v", facts)
	})

	// Test 4: Update fact (remove old, add new)
	t.Run("Update fact", func(t *testing.T) {
		adds := []string{"lives in New York"}
		removes := []string{"loves pizza"}

		err := store.ApplyDelta(testUserID, adds, removes)
		require.NoError(t, err, "Failed to apply delta (update)")

		facts, err := store.GetFacts(testUserID)
		require.NoError(t, err, "Failed to get facts")

		assert.Len(t, facts, 2, "Expected 2 facts after update")

		assert.Contains(t, facts, "lives in New York", "New fact 'lives in New York' not found")
		assert.NotContains(t, facts, "loves pizza", "Old fact 'loves pizza' should have been removed")

		t.Logf("✓ Successfully updated fact. Current: %v", facts)
	})

	// Clean up after test
	if err := store.DeleteFacts(testUserID); err != nil {
		t.Logf("Warning: Failed to clean up after test: %v", err)
	}
}

func TestSurrealStore_RecentMessages(t *testing.T) {
	store, cleanup := setupSurrealTest(t)
	defer cleanup()

	testUserID := "test_user_recent_messages"

	// Clean up before test
	if err := store.ClearRecentMessages(testUserID); err != nil {
		t.Logf("Warning: Failed to clean up before test: %v", err)
	}

	// Test 1: Add recent messages
	t.Run("Add recent messages", func(t *testing.T) {
		messages := []struct {
			Role string
			Text string
		}{
			{"user", "Hello world"},
			{"assistant", "How are you?"},
			{"user", "I am fine"},
		}

		for _, msg := range messages {
			err := store.AddRecentMessage(testUserID, msg.Role, msg.Text)
			require.NoError(t, err, "Failed to add recent message")
			// Small sleep to ensure timestamps are different (SurrealDB might be fast)
			time.Sleep(10 * time.Millisecond)
		}

		// Verify messages
		storedMessages, err := store.GetRecentMessages(testUserID)
		require.NoError(t, err, "Failed to get recent messages")

		assert.Len(t, storedMessages, 3, "Expected 3 messages")

		// Check order and content
		for i, msg := range storedMessages {
			expected := messages[i]
			assert.Equal(t, expected.Text, msg.Text, "Message text mismatch at index %d", i)
			assert.Equal(t, expected.Role, msg.Role, "Message role mismatch at index %d", i)
		}

		t.Logf("✓ Successfully added and retrieved messages: %v", storedMessages)
	})

	// Test 2: Limit check (add more messages to trigger cleanup)
	t.Run("Limit check", func(t *testing.T) {
		// Add 15 more messages (total 18)
		for i := 0; i < 15; i++ {
			msg := fmt.Sprintf("Message %d", i)
			err := store.AddRecentMessage(testUserID, "user", msg)
			require.NoError(t, err, "Failed to add message")
			time.Sleep(10 * time.Millisecond)
		}

		storedMessages, err := store.GetRecentMessages(testUserID)
		require.NoError(t, err, "Failed to get recent messages")

		// Should be capped at 15
		assert.Len(t, storedMessages, 15, "Expected 15 messages (limit)")

		// The last message added should be present
		if len(storedMessages) > 0 {
			lastMsg := storedMessages[len(storedMessages)-1]
			assert.Equal(t, "Message 14", lastMsg.Text, "Expected last message to be 'Message 14'")
		}

		t.Logf("✓ Successfully enforced message limit. Count: %d", len(storedMessages))
	})

	// Clean up
	if err := store.ClearRecentMessages(testUserID); err != nil {
		t.Logf("Warning: Failed to clean up after test: %v", err)
	}
}

func TestSurrealStore_State(t *testing.T) {
	store, cleanup := setupSurrealTest(t)
	defer cleanup()

	key := "test_state_key"
	// Ensure cleanup of state
	defer func() {
		query := `DELETE type::thing("bot_state", $key);`
		_, _ = store.client.Query(query, map[string]interface{}{"key": key})
	}()

	value := "happy"

	// Set state
	err := store.SetState(key, value)
	require.NoError(t, err, "Failed to set state")

	// Get state
	got, err := store.GetState(key)
	require.NoError(t, err, "Failed to get state")
	assert.Equal(t, value, got, "Expected state to be %s", value)

	// Update state
	newValue := "sad"
	err = store.SetState(key, newValue)
	require.NoError(t, err, "Failed to update state")

	got, err = store.GetState(key)
	require.NoError(t, err, "Failed to get state after update")
	assert.Equal(t, newValue, got, "Expected state to be %s", newValue)
}

func TestSurrealStore_Users(t *testing.T) {
	store, cleanup := setupSurrealTest(t)
	defer cleanup()

	userID := "test_user_unique_123"

	// Clean up before test (best effort)
	_ = store.DeleteUserData(userID)

	// Ensure User
	err := store.EnsureUser(userID)
	require.NoError(t, err, "Failed to ensure user")

	// Check if user is in known users
	users, err := store.GetAllKnownUsers()
	require.NoError(t, err, "Failed to get all known users")

	assert.Contains(t, users, userID, "User %s not found in known users list", userID)

	// Clean up
	_ = store.DeleteUserData(userID)
}

func TestSurrealStore_Reminders(t *testing.T) {
	store, cleanup := setupSurrealTest(t)
	defer cleanup()

	userID := "test_user_reminders"
	// Clean up before and after
	_ = store.DeleteUserData(userID)
	defer func() { _ = store.DeleteUserData(userID) }()

	// Case 1: Add a reminder due in the past
	pastDue := time.Now().Add(-1 * time.Hour).Unix()
	textPast := "This was due an hour ago"
	err := store.AddReminder(userID, textPast, pastDue)
	require.NoError(t, err, "Failed to add past reminder")

	// Case 2: Add a reminder due in the future
	futureDue := time.Now().Add(1 * time.Hour).Unix()
	textFuture := "This is due in an hour"
	err = store.AddReminder(userID, textFuture, futureDue)
	require.NoError(t, err, "Failed to add future reminder")

	// Get due reminders
	dueReminders, err := store.GetDueReminders()
	require.NoError(t, err, "Failed to get due reminders")

	// Check results
	foundPast := false
	foundFuture := false

	for _, r := range dueReminders {
		if r.Text == textPast && r.UserID == userID {
			foundPast = true
			// Cleanup
			_ = store.DeleteReminder(r.ID)
		}
		if r.Text == textFuture && r.UserID == userID {
			foundFuture = true
			// Cleanup (should not happen if logic is correct)
			_ = store.DeleteReminder(r.ID)
		}
	}

	assert.True(t, foundPast, "Expected to find past reminder '%s', but didn't", textPast)
	assert.False(t, foundFuture, "Found future reminder '%s' in due list, but shouldn't have", textFuture)
}

func TestSurrealStore_EmojiCache(t *testing.T) {
	store, cleanup := setupSurrealTest(t)
	defer cleanup()

	guildID := "test_guild_123"
	// Ensure cleanup
	defer func() {
		query := `DELETE type::thing("guild_cache", $id);`
		_, _ = store.client.Query(query, map[string]interface{}{"id": guildID})
	}()

	emojis := []string{"😀", "🚀", "🎉"}

	// Set emojis
	err := store.SetCachedEmojis(guildID, emojis)
	require.NoError(t, err, "Failed to set cached emojis")

	// Get emojis
	got, err := store.GetCachedEmojis(guildID)
	require.NoError(t, err, "Failed to get cached emojis")

	assert.Len(t, got, 3, "Expected 3 emojis")

	for i, e := range emojis {
		assert.Equal(t, e, got[i], "Expected emoji %s at index %d, got %s", e, i, got[i])
	}
}

func TestSurrealStore_VectorSearch(t *testing.T) {
	store, cleanup := setupSurrealTest(t)
	defer cleanup()

	userID := "test_user_vectors"

	// Helper to create 2048-dim vector
	makeVector := func(idx int) []float32 {
		v := make([]float32, 2048)
		if idx >= 0 && idx < 2048 {
			v[idx] = 1.0
		}
		return v
	}

	// Clear existing data (best effort)
	_ = store.DeleteUserData(userID)

	// Add memory A (vector at index 0)
	vecA := makeVector(0)
	err := store.Add(userID, "Memory A", vecA)
	require.NoError(t, err, "Failed to add Memory A")

	// Add memory B (vector at index 100)
	vecB := makeVector(100)
	err = store.Add(userID, "Memory B", vecB)
	require.NoError(t, err, "Failed to add Memory B")

	// Search for A (exact match)
	results, err := store.Search(userID, vecA, 5)
	require.NoError(t, err, "Failed to search")

	require.NotEmpty(t, results, "Expected at least 1 result for exact match")
	assert.Equal(t, "Memory A", results[0], "Expected top result to be 'Memory A'")

	// Search for B (exact match)
	results, err = store.Search(userID, vecB, 5)
	require.NoError(t, err, "Failed to search")

	require.NotEmpty(t, results, "Expected at least 1 result for exact match")
	assert.Equal(t, "Memory B", results[0], "Expected top result to be 'Memory B'")

	// Search for orthogonal vector (index 500) - should return nothing or low score
	vecC := makeVector(500)
	results, err = store.Search(userID, vecC, 5)
	require.NoError(t, err, "Failed to search")

	// The Search method has a threshold of 0.6. Orthogonal vectors (dot product 0) should be excluded.
	assert.Empty(t, results, "Expected 0 results for orthogonal vector, got %v", results)

	// Clean up
	_ = store.DeleteUserData(userID)
}
