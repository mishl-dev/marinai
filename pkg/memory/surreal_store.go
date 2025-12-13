package memory

import (
	"encoding/json"
	"fmt"
	"log"
	"marinai/pkg/surreal"
	"time"
)

type SurrealStore struct {
	client *surreal.Client
}

type SurrealMemoryItem struct {
	ID        string    `json:"id,omitempty"`
	UserID    string    `json:"user_id"`
	Text      string    `json:"text"`
	Embedding []float32 `json:"vector"`
	Timestamp int64     `json:"timestamp"`
}

type FactItem struct {
	Text      string `json:"text"`
	CreatedAt int64  `json:"created_at"`
}

func NewSurrealStore(client *surreal.Client) *SurrealStore {
	store := &SurrealStore{
		client: client,
	}
	if err := store.Init(); err != nil {
		// Log error but don't fail startup, as DB might be reachable later or schema exists
		// In production, you might want to handle this more strictly
		fmt.Printf("Warning: Failed to initialize SurrealDB schema: %v\n", err)
	}
	return store
}

func (s *SurrealStore) Init() error {
	query := `
		DEFINE TABLE IF NOT EXISTS memories SCHEMAFULL;
		DEFINE FIELD IF NOT EXISTS user_id ON memories TYPE string;
		DEFINE FIELD IF NOT EXISTS text ON memories TYPE string;
		DEFINE FIELD IF NOT EXISTS timestamp ON memories TYPE int;
		-- We define the vector field with 2048 dimensions
		DEFINE FIELD IF NOT EXISTS vector ON memories TYPE array<float> ASSERT array::len($value) == 2048;
		DEFINE INDEX IF NOT EXISTS vector_idx ON memories FIELDS vector MTREE DIMENSION 2048 DIST COSINE;

	// Define schema for user profiles
		DEFINE TABLE IF NOT EXISTS user_profiles SCHEMAFULL;
		DEFINE FIELD IF NOT EXISTS user_id ON user_profiles TYPE string;
		DEFINE FIELD IF NOT EXISTS facts ON user_profiles TYPE array<object>;
		DEFINE FIELD IF NOT EXISTS facts[*].text ON user_profiles TYPE string;
		DEFINE FIELD IF NOT EXISTS facts[*].created_at ON user_profiles TYPE int;
		DEFINE FIELD IF NOT EXISTS last_updated ON user_profiles TYPE int;

	// Define schema for guild cache (emojis)
		DEFINE TABLE IF NOT EXISTS guild_cache SCHEMAFULL;
		DEFINE FIELD IF NOT EXISTS emojis ON guild_cache TYPE array<string>;
		DEFINE FIELD IF NOT EXISTS last_updated ON guild_cache TYPE int;

	// Define schema for recent messages
		DEFINE TABLE IF NOT EXISTS recent_messages SCHEMAFULL;
		DEFINE FIELD IF NOT EXISTS user_id ON recent_messages TYPE string;
		DEFINE FIELD IF NOT EXISTS role ON recent_messages TYPE string;
		DEFINE FIELD IF NOT EXISTS text ON recent_messages TYPE string;
		DEFINE FIELD IF NOT EXISTS timestamp ON recent_messages TYPE int;

	// Define schema for reminders
		DEFINE TABLE IF NOT EXISTS reminders SCHEMAFULL;
		DEFINE FIELD IF NOT EXISTS user_id ON reminders TYPE string;
		DEFINE FIELD IF NOT EXISTS text ON reminders TYPE string;
		DEFINE FIELD IF NOT EXISTS due_at ON reminders TYPE int;
		DEFINE FIELD IF NOT EXISTS created_at ON reminders TYPE int;

	// Define schema for bot state
		DEFINE TABLE IF NOT EXISTS bot_state SCHEMAFULL;
		DEFINE FIELD IF NOT EXISTS value ON bot_state TYPE string;
		DEFINE FIELD IF NOT EXISTS updated_at ON bot_state TYPE int;
	`
	_, err := s.client.Query(query, map[string]interface{}{})
	return err
}

// Emoji Cache

func (s *SurrealStore) GetCachedEmojis(guildID string) ([]string, error) {
	query := `SELECT emojis FROM guild_cache WHERE id = type::thing("guild_cache", $guild_id);`
	result, err := s.client.Query(query, map[string]interface{}{"guild_id": guildID})
	if err != nil {
		return nil, err
	}

	rows, ok := result.([]interface{})
	if !ok || len(rows) == 0 {
		return nil, nil // Not found is not an error
	}

	var emojis []string
	if row, ok := rows[0].(map[string]interface{}); ok {
		if e, ok := row["emojis"].([]interface{}); ok {
			for _, item := range e {
				if str, ok := item.(string); ok {
					emojis = append(emojis, str)
				}
			}
		}
	}

	return emojis, nil
}

func (s *SurrealStore) SetCachedEmojis(guildID string, emojis []string) error {
	query := `
		INSERT INTO guild_cache (id, emojis, last_updated)
		VALUES (type::thing("guild_cache", $guild_id), $emojis, time::unix())
		ON DUPLICATE KEY UPDATE emojis = $emojis, last_updated = time::unix();
	`
	_, err := s.client.Query(query, map[string]interface{}{
		"guild_id": guildID,
		"emojis":   emojis,
	})
	return err
}

func (s *SurrealStore) detectDuplicate(userId string, vector []float32, threshold float64) (bool, float64, string, error) {
	rows, err := s.client.VectorSearch("memories", "vector", vector, 1, map[string]interface{}{
		"user_id": userId,
	})
	if err != nil {
		return false, 0, "", err
	}

	if len(rows) == 0 {
		return false, 0, "", nil
	}

	rowMap, ok := rows[0].(map[string]interface{})
	if !ok {
		return false, 0, "", fmt.Errorf("unexpected row format")
	}

	// Extract similarity
	var simScore float64
	switch v := rowMap["similarity"].(type) {
	case float64:
		simScore = v
	case float32:
		simScore = float64(v)
	}

	// Extract existing text
	existingText, _ := rowMap["text"].(string)

	if simScore >= threshold {
		return true, simScore, existingText, nil
	}

	return false, simScore, existingText, nil
}

func (s *SurrealStore) Add(userId string, text string, vector []float32) error {
	const duplicateThreshold = 0.8

	isDup, sim, existingText, err := s.detectDuplicate(userId, vector, duplicateThreshold)
	if err != nil {
		log.Printf("[DEBUG] Error checking for duplicates: %v", err)
	} else if isDup {
		return fmt.Errorf(
			"duplicate memory detected (similarity: %.4f): existing='%s', new='%s'",
			sim, existingText, text,
		)
	}

	item := SurrealMemoryItem{
		UserID:    userId,
		Text:      text,
		Embedding: vector,
		Timestamp: time.Now().Unix(),
	}

	_, err = s.client.Create("memories", item)
	return err
}

func (s *SurrealStore) Search(userId string, queryVector []float32, limit int) ([]string, error) {
	log.Printf("[DEBUG] Search called: userId=%s, vectorLen=%d, limit=%d", userId, len(queryVector), limit)

	// Use the client's VectorSearch method to avoid raw queries in the store
	rows, err := s.client.VectorSearch("memories", "vector", queryVector, limit, map[string]interface{}{
		"user_id": userId,
	})
	if err != nil {
		log.Printf("[DEBUG] VectorSearch error: %v", err)
		return nil, err
	}

	log.Printf("[DEBUG] VectorSearch returned %d rows", len(rows))

	const similarityThreshold = 0.6 // Only include memories with good similarity
	var texts []string

	for _, row := range rows {
		if rowMap, ok := row.(map[string]interface{}); ok {
			if text, ok := rowMap["text"].(string); ok {
				similarity := rowMap["similarity"]

				// Check if similarity meets threshold
				var simScore float64
				switch v := similarity.(type) {
				case float64:
					simScore = v
				case float32:
					simScore = float64(v)
				default:
					log.Printf("Unknown similarity type: %T", similarity)
					continue
				}

				if simScore >= similarityThreshold {
					log.Printf("Memory match: '%s' (similarity: %.4f)", text, simScore)
					texts = append(texts, text)
				} else {
					log.Printf("Skipping low-similarity memory: '%s' (similarity: %.4f)", text, simScore)
				}
			}
		}
	}

	return texts, nil
}

// Recent messages cache

func (s *SurrealStore) AddRecentMessage(userId, role, message string) error {
	// Use a map or a local struct since we need UserID which is not in the interface struct
	item := map[string]interface{}{
		"user_id":   userId,
		"role":      role,
		"text":      message,
		"timestamp": time.Now().UnixNano(),
	}

	_, err := s.client.Create("recent_messages", item)
	if err != nil {
		return err
	}

	// Cleanup old messages (keep last 15)
	query := `
		DELETE recent_messages
		WHERE user_id = $user_id
		AND id NOT IN (
			SELECT VALUE id FROM (
				SELECT id, timestamp FROM recent_messages
				WHERE user_id = $user_id
				ORDER BY timestamp DESC
				LIMIT 15
			)
		);
	`
	_, err = s.client.Query(query, map[string]interface{}{"user_id": userId})
	return err
}

func (s *SurrealStore) GetRecentMessages(userId string) ([]RecentMessageItem, error) {
	// Include 'timestamp' in SELECT since we're ordering by it
	query := `
		SELECT role, text, timestamp FROM recent_messages
		WHERE user_id = $user_id
		ORDER BY timestamp ASC;
	`

	result, err := s.client.Query(query, map[string]interface{}{"user_id": userId})
	if err != nil {
		return nil, err
	}

	rows, ok := result.([]interface{})
	if !ok || len(rows) == 0 {
		return []RecentMessageItem{}, nil
	}

	var messages []RecentMessageItem
	for _, row := range rows {
		if rowMap, ok := row.(map[string]interface{}); ok {
			// Manually map fields to struct to be safe
			msg := RecentMessageItem{}
			if role, ok := rowMap["role"].(string); ok {
				msg.Role = role
			}
			if text, ok := rowMap["text"].(string); ok {
				msg.Text = text
			}
			// Handle timestamp (might be float64 from JSON or int/uint from driver)
			switch t := rowMap["timestamp"].(type) {
			case float64:
				msg.Timestamp = int64(t)
			case int64:
				msg.Timestamp = t
			case uint64:
				msg.Timestamp = int64(t)
			case int:
				msg.Timestamp = int64(t)
			}

			messages = append(messages, msg)
		}
	}

	return messages, nil
}

func (s *SurrealStore) ClearRecentMessages(userId string) error {
	query := `DELETE recent_messages WHERE user_id = $user_id;`
	_, err := s.client.Query(query, map[string]interface{}{"user_id": userId})
	return err
}

// Reminders

func (s *SurrealStore) AddReminder(userId string, text string, dueAt int64) error {
	reminder := Reminder{
		UserID:    userId,
		Text:      text,
		DueAt:     dueAt,
		CreatedAt: time.Now().Unix(),
	}

	_, err := s.client.Create("reminders", reminder)
	return err
}

func (s *SurrealStore) GetDueReminders() ([]Reminder, error) {
	// Select reminders where due_at is less than current time
	query := `
		SELECT * FROM reminders WHERE due_at <= $now;
	`
	result, err := s.client.Query(query, map[string]interface{}{
		"now": time.Now().Unix(),
	})
	if err != nil {
		return nil, err
	}

	rows, ok := result.([]interface{})
	if !ok {
		return []Reminder{}, nil
	}

	var reminders []Reminder
	for _, row := range rows {
		if rowMap, ok := row.(map[string]interface{}); ok {
			r := Reminder{}
			if id, ok := rowMap["id"].(string); ok {
				r.ID = id
			} else if idMap, ok := rowMap["id"].(map[string]interface{}); ok {
				// Handle RecordID object if returned as map
				if strID, ok := idMap["String"].(string); ok {
					r.ID = strID
				}
			} else {
				// Fallback: try to print it if it's something else, or just ignore
				// SurrealDB Go driver might return specific types for RecordID
				r.ID = fmt.Sprintf("%v", rowMap["id"])
			}

			if uid, ok := rowMap["user_id"].(string); ok {
				r.UserID = uid
			}
			if txt, ok := rowMap["text"].(string); ok {
				r.Text = txt
			}

			// Handle numbers which might come as float64
			if due, ok := rowMap["due_at"].(float64); ok {
				r.DueAt = int64(due)
			} else if due, ok := rowMap["due_at"].(int64); ok {
				r.DueAt = due
			}

			if created, ok := rowMap["created_at"].(float64); ok {
				r.CreatedAt = int64(created)
			} else if created, ok := rowMap["created_at"].(int64); ok {
				r.CreatedAt = created
			}

			reminders = append(reminders, r)
		}
	}
	return reminders, nil
}

func (s *SurrealStore) UpdateReminder(reminder Reminder) error {
	query := `
		UPDATE $id SET text = $text, due_at = $due_at, user_id = $user_id;
	`
	_, err := s.client.Query(query, map[string]interface{}{
		"id":      reminder.ID,
		"text":    reminder.Text,
		"due_at":  reminder.DueAt,
		"user_id": reminder.UserID,
	})
	return err
}

func (s *SurrealStore) DeleteReminder(id string) error {
	// id is typically "reminders:<uuid>"
	query := `DELETE $id;`
	_, err := s.client.Query(query, map[string]interface{}{"id": id})
	return err
}

func (s *SurrealStore) DeleteOldReminders(ageLimit time.Duration) error {
	// Delete reminders where due_at is older than (now - ageLimit)
	cutoff := time.Now().Add(-ageLimit).Unix()
	query := `DELETE reminders WHERE due_at < $cutoff;`
	_, err := s.client.Query(query, map[string]interface{}{"cutoff": cutoff})
	return err
}

// General State

func (s *SurrealStore) GetState(key string) (string, error) {
	// key is used as the ID: bot_state:key
	// Use SELECT * to avoid ambiguity with VALUE keyword
	query := `SELECT * FROM bot_state WHERE id = type::thing("bot_state", $key);`
	result, err := s.client.Query(query, map[string]interface{}{"key": key})
	if err != nil {
		return "", err
	}

	rows, ok := result.([]interface{})
	if !ok || len(rows) == 0 {
		return "", nil
	}

	if row, ok := rows[0].(map[string]interface{}); ok {
		if val, ok := row["value"].(string); ok {
			return val, nil
		}
	}
	return "", nil
}

func (s *SurrealStore) SetState(key, value string) error {
	query := `
		INSERT INTO bot_state (id, value, updated_at)
		VALUES (type::thing("bot_state", $key), $value, time::unix())
		ON DUPLICATE KEY UPDATE value = $value, updated_at = time::unix();
	`
	_, err := s.client.Query(query, map[string]interface{}{
		"key":   key,
		"value": value,
	})
	return err
}

func (s *SurrealStore) GetAllKnownUsers() ([]string, error) {
	// Query both user_profiles and memories to ensure we find all known users
	// Use array::distinct to remove duplicates
	query := `
		LET $u1 = SELECT VALUE user_id FROM user_profiles;
		LET $u2 = SELECT VALUE user_id FROM memories;
		RETURN array::distinct(array::union($u1, $u2));
	`
	result, err := s.client.Query(query, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	// Unwrap result: Query returns []interface{}, where first element is the result of RETURN
	// Since we used multiple statements, the result might be wrapped differently depending on client
	// But `Query` helper tries to unwrap "Result" field.
	// In multi-statement query, the result is typically the result of the LAST statement.

	// If result is []interface{}, and we used RETURN, it should be the array of user IDs directly or wrapped.
	// Let's inspect typical structure.

	// Assuming s.client.Query returns the "Result" of the last query.
	// If result is []interface{}, it might be the rows.
	// But here we return an ARRAY of strings.

	// If result is []string, that's great.
	if strList, ok := result.([]string); ok {
		return strList, nil
	}

	// If result is []interface{} (generic list)
	if list, ok := result.([]interface{}); ok {
		var users []string
		for _, item := range list {
			if str, ok := item.(string); ok {
				users = append(users, str)
			}
		}
		return users, nil
	}

	return []string{}, nil
}

func (s *SurrealStore) EnsureUser(userId string) error {
	query := `
		INSERT INTO user_profiles (id, user_id, facts, last_updated)
		VALUES (type::thing("user_profiles", $user_id), $user_id, [], time::unix())
		ON DUPLICATE KEY UPDATE last_updated = time::unix();
	`
	_, err := s.client.Query(query, map[string]interface{}{
		"user_id": userId,
	})
	return err
}

func (s *SurrealStore) DeleteUserData(userId string) error {
	query := `
		DELETE memories WHERE user_id = $user_id;
		DELETE recent_messages WHERE user_id = $user_id;
		DELETE user_profiles WHERE user_id = $user_id;
		DELETE reminders WHERE user_id = $user_id;
	`
	_, err := s.client.Query(query, map[string]interface{}{"user_id": userId})
	return err
}

// Profile management

func (s *SurrealStore) GetFacts(userId string) ([]string, error) {
	// Direct record lookup: user_profiles:<userId>
	// Note: In SurrealDB, record IDs are `table:id`. We assume userId is safe to use as ID part.
	// If userId contains special chars, we might need to escape or hash it, but for Discord IDs (snowflakes) it's fine.
	query := `SELECT facts FROM user_profiles WHERE id = type::thing("user_profiles", $user_id);`

	result, err := s.client.Query(query, map[string]interface{}{"user_id": userId})
	if err != nil {
		return nil, err
	}
	// Parse result
	// Result is now []interface{} (rows)
	rows, ok := result.([]interface{})
	if !ok || len(rows) == 0 {
		return []string{}, nil
	}

	// The first row is the user profile
	row, ok := rows[0].(map[string]interface{})
	if !ok {
		return []string{}, nil
	}

	var facts []string

	// Helper to extract facts from row (now facts are objects with {text, created_at})
	extractFacts := func(row map[string]interface{}) {
		rawFacts, ok := row["facts"]
		if !ok {
			return
		}

		if f, ok := rawFacts.([]interface{}); ok {
			for _, item := range f {
				// Facts are now objects with {text: string, created_at: int}
				if factMap, ok := item.(map[string]interface{}); ok {
					if text, ok := factMap["text"].(string); ok {
						facts = append(facts, text)
					}
				} else if str, ok := item.(string); ok {
					// Backward compatibility: handle old string-based facts
					facts = append(facts, str)
				}
			}
		}
	}

	extractFacts(row)

	return facts, nil
}

func (s *SurrealStore) ApplyDelta(userId string, adds []string, removes []string) error {
	// Ensure adds and removes are not nil
	if adds == nil {
		adds = []string{}
	}
	if removes == nil {
		removes = []string{}
	}

	// Convert adds to fact objects with timestamps
	currentTime := time.Now().Unix()

	// Build fact objects with timestamps
	addsWithTimestamps := make([]FactItem, len(adds))
	for i, factText := range adds {
		addsWithTimestamps[i] = FactItem{
			Text:      factText,
			CreatedAt: currentTime,
		}
	}

	addsJson, _ := json.Marshal(addsWithTimestamps)
	removesJson, _ := json.Marshal(removes)

	// Use fmt.Sprintf to embed JSON directly into the query to avoid parameter binding issues
	// This is safe here because we are marshalling structs/slices we control, but in general be careful with injection
	query := fmt.Sprintf(`
		-- Ensure record exists
		INSERT INTO user_profiles (id, user_id, facts, last_updated) 
		VALUES (type::thing("user_profiles", $user_id), $user_id, [], time::unix()) 
		ON DUPLICATE KEY UPDATE last_updated = time::unix();

		-- Remove facts by text
		IF array::len(%s) > 0 THEN
			UPDATE type::thing("user_profiles", $user_id) SET 
				facts = array::filter(facts, |$fact| !array::includes(%s, $fact.text)),
				last_updated = time::unix();
		END;

		-- Add new facts with timestamps
		UPDATE type::thing("user_profiles", $user_id) SET 
			facts = array::union(facts, %s),
			last_updated = time::unix();
	`, string(removesJson), string(removesJson), string(addsJson))

	_, err := s.client.Query(query, map[string]interface{}{
		"user_id": userId,
	})
	return err
}

func (s *SurrealStore) DeleteFacts(userId string) error {
	query := `DELETE type::thing("user_profiles", $user_id);`
	_, err := s.client.Query(query, map[string]interface{}{"user_id": userId})
	return err
}
