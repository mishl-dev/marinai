package memory

import (
	"fmt"
	"marinai/pkg/surreal"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

func TestSurrealStore_ApplyDelta(t *testing.T) {
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
	defer client.Close()

	// Create store and initialize schema
	store := NewSurrealStore(client)

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

	t.Log("✓ All memory storage tests passed!")
}

func TestSurrealStore_RecentMessages(t *testing.T) {
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
	defer client.Close()

	// Create store and initialize schema
	store := NewSurrealStore(client)

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
		} else {
			t.Errorf("storedMessages is empty, cannot verify last message")
		}

		t.Logf("✓ Successfully enforced message limit. Count: %d", len(storedMessages))
	})

	// Clean up
	if err := store.ClearRecentMessages(testUserID); err != nil {
		t.Logf("Warning: Failed to clean up after test: %v", err)
	}
}
