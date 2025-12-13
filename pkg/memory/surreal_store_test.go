package memory

import (
	"fmt"
	"marinai/pkg/surreal"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
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
		if err != nil {
			t.Fatalf("Failed to apply delta (add): %v", err)
		}

		// Verify facts were added
		facts, err := store.GetFacts(testUserID)
		if err != nil {
			t.Fatalf("Failed to get facts: %v", err)
		}

		if len(facts) != 2 {
			t.Errorf("Expected 2 facts, got %d: %v", len(facts), facts)
		}

		// Check if facts contain expected values
		hasLovesPizza := false
		hasLivesInTokyo := false
		for _, fact := range facts {
			if fact == "loves pizza" {
				hasLovesPizza = true
			}
			if fact == "lives in Tokyo" {
				hasLivesInTokyo = true
			}
		}

		if !hasLovesPizza || !hasLivesInTokyo {
			t.Errorf("Facts missing expected values. Got: %v", facts)
		}

		t.Logf("✓ Successfully added facts: %v", facts)
	})

	// Test 2: Add more facts
	t.Run("Add more facts", func(t *testing.T) {
		adds := []string{"works as engineer"}
		removes := []string{}

		err := store.ApplyDelta(testUserID, adds, removes)
		if err != nil {
			t.Fatalf("Failed to apply delta (add more): %v", err)
		}

		facts, err := store.GetFacts(testUserID)
		if err != nil {
			t.Fatalf("Failed to get facts: %v", err)
		}

		if len(facts) != 3 {
			t.Errorf("Expected 3 facts, got %d: %v", len(facts), facts)
		}

		t.Logf("✓ Successfully added more facts: %v", facts)
	})

	// Test 3: Remove a fact
	t.Run("Remove a fact", func(t *testing.T) {
		adds := []string{}
		removes := []string{"lives in Tokyo"}

		err := store.ApplyDelta(testUserID, adds, removes)
		if err != nil {
			t.Fatalf("Failed to apply delta (remove): %v", err)
		}

		facts, err := store.GetFacts(testUserID)
		if err != nil {
			t.Fatalf("Failed to get facts: %v", err)
		}

		if len(facts) != 2 {
			t.Errorf("Expected 2 facts after removal, got %d: %v", len(facts), facts)
		}

		// Verify "lives in Tokyo" was removed
		for _, fact := range facts {
			if fact == "lives in Tokyo" {
				t.Errorf("Fact 'lives in Tokyo' should have been removed but still exists")
			}
		}

		t.Logf("✓ Successfully removed fact. Remaining: %v", facts)
	})

	// Test 4: Update fact (remove old, add new)
	t.Run("Update fact", func(t *testing.T) {
		adds := []string{"lives in New York"}
		removes := []string{"loves pizza"}

		err := store.ApplyDelta(testUserID, adds, removes)
		if err != nil {
			t.Fatalf("Failed to apply delta (update): %v", err)
		}

		facts, err := store.GetFacts(testUserID)
		if err != nil {
			t.Fatalf("Failed to get facts: %v", err)
		}

		if len(facts) != 2 {
			t.Errorf("Expected 2 facts after update, got %d: %v", len(facts), facts)
		}

		hasNewYork := false
		for _, fact := range facts {
			if fact == "lives in New York" {
				hasNewYork = true
			}
			if fact == "loves pizza" {
				t.Errorf("Old fact 'loves pizza' should have been removed")
			}
		}

		if !hasNewYork {
			t.Errorf("New fact 'lives in New York' not found. Got: %v", facts)
		}

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
			if err := store.AddRecentMessage(testUserID, msg.Role, msg.Text); err != nil {
				t.Fatalf("Failed to add recent message: %v", err)
			}
			// Small sleep to ensure timestamps are different (SurrealDB might be fast)
			time.Sleep(10 * time.Millisecond)
		}

		// Verify messages
		storedMessages, err := store.GetRecentMessages(testUserID)
		if err != nil {
			t.Fatalf("Failed to get recent messages: %v", err)
		}

		if len(storedMessages) != 3 {
			t.Errorf("Expected 3 messages, got %d: %v", len(storedMessages), storedMessages)
		}

		// Check order and content
		for i, msg := range storedMessages {
			expected := messages[i]
			if msg.Text != expected.Text {
				t.Errorf("Message text mismatch at index %d. Expected '%s', got '%s'", i, expected.Text, msg.Text)
			}
			if msg.Role != expected.Role {
				t.Errorf("Message role mismatch at index %d. Expected '%s', got '%s'", i, expected.Role, msg.Role)
			}
		}

		t.Logf("✓ Successfully added and retrieved messages: %v", storedMessages)
	})

	// Test 2: Limit check (add more messages to trigger cleanup)
	t.Run("Limit check", func(t *testing.T) {
		// Add 15 more messages (total 18)
		for i := 0; i < 15; i++ {
			msg := fmt.Sprintf("Message %d", i)
			if err := store.AddRecentMessage(testUserID, "user", msg); err != nil {
				t.Fatalf("Failed to add message: %v", err)
			}
			time.Sleep(10 * time.Millisecond)
		}

		storedMessages, err := store.GetRecentMessages(testUserID)
		if err != nil {
			t.Fatalf("Failed to get recent messages: %v", err)
		}

		// Should be capped at 15
		if len(storedMessages) != 15 {
			t.Errorf("Expected 15 messages (limit), got %d", len(storedMessages))
		}

		// The last message added should be present
		if len(storedMessages) > 0 {
			lastMsg := storedMessages[len(storedMessages)-1]
			if lastMsg.Text != "Message 14" {
				t.Errorf("Expected last message to be 'Message 14', got '%s'", lastMsg.Text)
			}
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
	if err := store.SetState(key, value); err != nil {
		t.Fatalf("Failed to set state: %v", err)
	}

	// Get state
	got, err := store.GetState(key)
	if err != nil {
		t.Fatalf("Failed to get state: %v", err)
	}

	if got != value {
		t.Errorf("Expected state '%s', got '%s'", value, got)
	}

	// Update state
	newValue := "sad"
	if err := store.SetState(key, newValue); err != nil {
		t.Fatalf("Failed to update state: %v", err)
	}

	got, err = store.GetState(key)
	if err != nil {
		t.Fatalf("Failed to get state after update: %v", err)
	}

	if got != newValue {
		t.Errorf("Expected state '%s', got '%s'", newValue, got)
	}
}

func TestSurrealStore_Users(t *testing.T) {
	store, cleanup := setupSurrealTest(t)
	defer cleanup()

	userID := "test_user_unique_123"

	// Clean up before test (best effort)
	_ = store.DeleteUserData(userID)

	// Ensure User
	if err := store.EnsureUser(userID); err != nil {
		t.Fatalf("Failed to ensure user: %v", err)
	}

	// Check if user is in known users
	users, err := store.GetAllKnownUsers()
	if err != nil {
		t.Fatalf("Failed to get all known users: %v", err)
	}

	found := false
	for _, u := range users {
		if u == userID {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("User %s not found in known users list: %v", userID, users)
	}

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
	if err := store.AddReminder(userID, textPast, pastDue); err != nil {
		t.Fatalf("Failed to add past reminder: %v", err)
	}

	// Case 2: Add a reminder due in the future
	futureDue := time.Now().Add(1 * time.Hour).Unix()
	textFuture := "This is due in an hour"
	if err := store.AddReminder(userID, textFuture, futureDue); err != nil {
		t.Fatalf("Failed to add future reminder: %v", err)
	}

	// Get due reminders
	dueReminders, err := store.GetDueReminders()
	if err != nil {
		t.Fatalf("Failed to get due reminders: %v", err)
	}

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

	if !foundPast {
		t.Errorf("Expected to find past reminder '%s', but didn't", textPast)
	}
	if foundFuture {
		t.Errorf("Found future reminder '%s' in due list, but shouldn't have", textFuture)
	}

	// Clean up the future reminder (we need to find its ID first, but GetDueReminders didn't return it)
	// We can't easily delete it without ID. But that's okay for test environment usually.
	// However, if we want to be clean:
	// The `AddReminder` doesn't return ID. This is a design flaw in `Store` interface if we want to manage them immediately.
	// But for now we just leave it or improve `Store` later.
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
	if err := store.SetCachedEmojis(guildID, emojis); err != nil {
		t.Fatalf("Failed to set cached emojis: %v", err)
	}

	// Get emojis
	got, err := store.GetCachedEmojis(guildID)
	if err != nil {
		t.Fatalf("Failed to get cached emojis: %v", err)
	}

	if len(got) != 3 {
		t.Errorf("Expected 3 emojis, got %d", len(got))
	}

	for i, e := range emojis {
		if got[i] != e {
			t.Errorf("Expected emoji %s at index %d, got %s", e, i, got[i])
		}
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
	if err := store.Add(userID, "Memory A", vecA); err != nil {
		t.Fatalf("Failed to add Memory A: %v", err)
	}

	// Add memory B (vector at index 100)
	vecB := makeVector(100)
	if err := store.Add(userID, "Memory B", vecB); err != nil {
		t.Fatalf("Failed to add Memory B: %v", err)
	}

	// Search for A (exact match)
	results, err := store.Search(userID, vecA, 5)
	if err != nil {
		t.Fatalf("Failed to search: %v", err)
	}

	if len(results) == 0 {
		t.Errorf("Expected at least 1 result for exact match")
	} else if results[0] != "Memory A" {
		t.Errorf("Expected top result to be 'Memory A', got '%s'", results[0])
	}

	// Search for B (exact match)
	results, err = store.Search(userID, vecB, 5)
	if err != nil {
		t.Fatalf("Failed to search: %v", err)
	}

	if len(results) == 0 {
		t.Errorf("Expected at least 1 result for exact match")
	} else if results[0] != "Memory B" {
		t.Errorf("Expected top result to be 'Memory B', got '%s'", results[0])
	}

	// Search for orthogonal vector (index 500) - should return nothing or low score
	vecC := makeVector(500)
	results, err = store.Search(userID, vecC, 5)
	if err != nil {
		t.Fatalf("Failed to search: %v", err)
	}

	// The Search method has a threshold of 0.6. Orthogonal vectors (dot product 0) should be excluded.
	if len(results) > 0 {
		t.Errorf("Expected 0 results for orthogonal vector, got %v", results)
	}

	// Clean up
	_ = store.DeleteUserData(userID)
}
