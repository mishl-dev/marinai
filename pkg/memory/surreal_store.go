package memory

import (
	"encoding/json"
	"fmt"
	"log"
	"marinai/pkg/surreal"
	"reflect"
	"strings"
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
	queries := []string{
		// -- Tables --
		"DEFINE TABLE IF NOT EXISTS memories SCHEMAFULL",
		"DEFINE TABLE IF NOT EXISTS user_profiles SCHEMAFULL",
		"DEFINE TABLE IF NOT EXISTS guild_cache SCHEMAFULL",
		"DEFINE TABLE IF NOT EXISTS recent_messages SCHEMAFULL",
		"DEFINE TABLE IF NOT EXISTS reminders SCHEMAFULL",
		"DEFINE TABLE IF NOT EXISTS bot_state SCHEMAFULL",
		"DEFINE TABLE IF NOT EXISTS pending_dm SCHEMAFULL",

		// -- Memories --
		"DEFINE FIELD IF NOT EXISTS user_id ON memories TYPE string",
		"DEFINE FIELD IF NOT EXISTS text ON memories TYPE string",
		"DEFINE FIELD IF NOT EXISTS timestamp ON memories TYPE int",
		"DEFINE FIELD IF NOT EXISTS vector ON memories TYPE array<float> ASSERT array::len($value) == 2048",
		"DEFINE INDEX IF NOT EXISTS vector_idx ON memories FIELDS vector MTREE DIMENSION 2048 DIST COSINE",

		// -- User Profiles --
		"DEFINE FIELD IF NOT EXISTS user_id ON user_profiles TYPE string",
		"DEFINE FIELD IF NOT EXISTS facts ON user_profiles TYPE array<object>",
		"DEFINE FIELD IF NOT EXISTS facts[*].text ON user_profiles TYPE string",
		"DEFINE FIELD IF NOT EXISTS facts[*].created_at ON user_profiles TYPE int",
		"DEFINE FIELD IF NOT EXISTS last_updated ON user_profiles TYPE int",
		// We use int DEFAULT 0 to avoid NONE issues
		"DEFINE FIELD IF NOT EXISTS last_interaction ON user_profiles TYPE int DEFAULT 0",
		"DEFINE FIELD IF NOT EXISTS first_interaction ON user_profiles TYPE int DEFAULT 0",
		"DEFINE FIELD IF NOT EXISTS affection ON user_profiles TYPE int DEFAULT 0",
		"DEFINE FIELD IF NOT EXISTS streak ON user_profiles TYPE int DEFAULT 0",
		"DEFINE FIELD IF NOT EXISTS last_streak_date ON user_profiles TYPE string DEFAULT ''",

		// -- Guild Cache --
		"DEFINE FIELD IF NOT EXISTS emojis ON guild_cache TYPE array<string>",
		"DEFINE FIELD IF NOT EXISTS last_updated ON guild_cache TYPE int",

		// -- Recent Messages --
		"DEFINE FIELD IF NOT EXISTS user_id ON recent_messages TYPE string",
		"DEFINE FIELD IF NOT EXISTS role ON recent_messages TYPE string",
		"DEFINE FIELD IF NOT EXISTS text ON recent_messages TYPE string",
		"DEFINE FIELD IF NOT EXISTS timestamp ON recent_messages TYPE int",

		// -- Reminders --
		"DEFINE FIELD IF NOT EXISTS user_id ON reminders TYPE string",
		"DEFINE FIELD IF NOT EXISTS text ON reminders TYPE string",
		"DEFINE FIELD IF NOT EXISTS due_at ON reminders TYPE int",
		"DEFINE FIELD IF NOT EXISTS created_at ON reminders TYPE int",

		// -- Bot State --
		"DEFINE FIELD IF NOT EXISTS value ON bot_state TYPE string",
		"DEFINE FIELD IF NOT EXISTS updated_at ON bot_state TYPE int",

		// -- Pending DM --
		"DEFINE FIELD IF NOT EXISTS user_id ON pending_dm TYPE string",
		"DEFINE FIELD IF NOT EXISTS sent_at ON pending_dm TYPE int",

		// -- Migrations --
		"UPDATE user_profiles SET last_interaction = 0 WHERE last_interaction IS NONE",
		"UPDATE user_profiles SET first_interaction = 0 WHERE first_interaction IS NONE",
		"UPDATE user_profiles SET affection = 0 WHERE affection IS NONE",
		"UPDATE user_profiles SET streak = 0 WHERE streak IS NONE",
		"UPDATE user_profiles SET last_streak_date = '' WHERE last_streak_date IS NONE",
	}

	for _, q := range queries {
		if _, err := s.client.Query(q, map[string]interface{}{}); err != nil {
			// Log warning but continue, as "already exists" is a common harmless error here
			// fmt.Printf("Init Warning: %v (Query: %s)\n", err, q)
		}
	}
	return nil
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

func (s *SurrealStore) detectDuplicate(userID string, vector []float32, threshold float64) (bool, float64, string, error) {
	rows, err := s.client.VectorSearch("memories", "vector", vector, 1, map[string]interface{}{
		"user_id": userID,
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

func (s *SurrealStore) Add(userID string, text string, vector []float32) error {
	const duplicateThreshold = 0.8

	isDup, sim, existingText, err := s.detectDuplicate(userID, vector, duplicateThreshold)
	if err != nil {
		log.Printf("[DEBUG] Error checking for duplicates: %v", err)
	} else if isDup {
		return fmt.Errorf(
			"duplicate memory detected (similarity: %.4f): existing='%s', new='%s'",
			sim, existingText, text,
		)
	}

	item := SurrealMemoryItem{
		UserID:    userID,
		Text:      text,
		Embedding: vector,
		Timestamp: time.Now().Unix(),
	}

	_, err = s.client.Create("memories", item)
	return err
}

func (s *SurrealStore) Search(userID string, queryVector []float32, limit int) ([]string, error) {
	log.Printf("[DEBUG] Search called: userID=%s, vectorLen=%d, limit=%d", userID, len(queryVector), limit)

	// Use the client's VectorSearch method to avoid raw queries in the store
	rows, err := s.client.VectorSearch("memories", "vector", queryVector, limit, map[string]interface{}{
		"user_id": userID,
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

func (s *SurrealStore) AddRecentMessage(userID, role, message string) error {
	// Use a map or a local struct since we need UserID which is not in the interface struct
	item := map[string]interface{}{
		"user_id":   userID,
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
	_, err = s.client.Query(query, map[string]interface{}{"user_id": userID})
	return err
}

func (s *SurrealStore) GetRecentMessages(userID string) ([]RecentMessageItem, error) {
	// Include 'timestamp' in SELECT since we're ordering by it
	query := `
		SELECT role, text, timestamp FROM recent_messages
		WHERE user_id = $user_id
		ORDER BY timestamp ASC;
	`

	result, err := s.client.Query(query, map[string]interface{}{"user_id": userID})
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

func (s *SurrealStore) ClearRecentMessages(userID string) error {
	query := `DELETE recent_messages WHERE user_id = $user_id;`
	_, err := s.client.Query(query, map[string]interface{}{"user_id": userID})
	return err
}

// Reminders

func (s *SurrealStore) AddReminder(userID string, text string, dueAt int64) error {
	reminder := Reminder{
		UserID:    userID,
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
				} else if table, ok := idMap["Table"].(string); ok {
					// Handle split Table/ID format
					if idVal, ok := idMap["ID"].(string); ok {
						r.ID = table + ":" + idVal
					}
				}
			} else {
				// Check for struct using reflection (common for driver-specific types)
				val := reflect.ValueOf(rowMap["id"])
				if val.Kind() == reflect.Struct {
					// Try to get Table and ID fields
					tableField := val.FieldByName("Table")
					idField := val.FieldByName("ID")
					if tableField.IsValid() && idField.IsValid() {
						r.ID = fmt.Sprintf("%v:%v", tableField.Interface(), idField.Interface())
					} else {
						// Fallback if fields don't match expected names
						r.ID = fmt.Sprintf("%v", rowMap["id"])
					}
				} else {
					// Fallback
					r.ID = fmt.Sprintf("%v", rowMap["id"])
				}
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
	// id is typically "reminders:<uuid>" or just "<uuid>"
	// We need to use type::thing() to ensure it's treated as a record ID
	var tb, key string

	if strings.Contains(id, ":") {
		parts := strings.SplitN(id, ":", 2)
		tb = parts[0]
		key = parts[1]
	} else {
		tb = "reminders"
		key = id
	}

	query := `DELETE type::thing($tb, $key);`
	_, err := s.client.Query(query, map[string]interface{}{
		"tb":  tb,
		"key": key,
	})
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

func (s *SurrealStore) EnsureUser(userID string) error {
	query := `
		INSERT INTO user_profiles (id, user_id, facts, last_updated, last_interaction)
		VALUES (type::thing("user_profiles", $user_id), $user_id, [], time::unix(), 0)
		ON DUPLICATE KEY UPDATE last_updated = time::unix();
	`
	_, err := s.client.Query(query, map[string]interface{}{
		"user_id": userID,
	})
	return err
}

func (s *SurrealStore) DeleteUserData(userID string) error {
	query := `
		DELETE memories WHERE user_id = $user_id;
		DELETE recent_messages WHERE user_id = $user_id;
		DELETE user_profiles WHERE user_id = $user_id;
		DELETE reminders WHERE user_id = $user_id;
	`
	_, err := s.client.Query(query, map[string]interface{}{"user_id": userID})
	return err
}

// Profile management

func (s *SurrealStore) GetFacts(userID string) ([]string, error) {
	// Direct record lookup: user_profiles:<userID>
	// Note: In SurrealDB, record IDs are `table:id`. We assume userID is safe to use as ID part.
	// If userID contains special chars, we might need to escape or hash it, but for Discord IDs (snowflakes) it's fine.
	query := `SELECT facts FROM user_profiles WHERE id = type::thing("user_profiles", $user_id);`

	result, err := s.client.Query(query, map[string]interface{}{"user_id": userID})
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

func (s *SurrealStore) ApplyDelta(userID string, adds []string, removes []string) error {
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
		INSERT INTO user_profiles (id, user_id, facts, last_updated, last_interaction) 
		VALUES (type::thing("user_profiles", $user_id), $user_id, [], time::unix(), 0) 
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
		"user_id": userID,
	})
	return err
}

func (s *SurrealStore) DeleteFacts(userID string) error {
	query := `DELETE type::thing("user_profiles", $user_id);`
	_, err := s.client.Query(query, map[string]interface{}{"user_id": userID})
	return err
}

// Pending DM tracking (Duolingo-style)

// HasPendingDM checks if the user has an unanswered boredom DM
func (s *SurrealStore) HasPendingDM(userID string) (bool, error) {
	query := `SELECT * FROM pending_dm WHERE id = type::thing("pending_dm", $user_id);`
	result, err := s.client.Query(query, map[string]interface{}{"user_id": userID})
	if err != nil {
		return false, err
	}

	rows, ok := result.([]interface{})
	if !ok || len(rows) == 0 {
		return false, nil
	}

	return true, nil
}

// SetPendingDM marks that a boredom DM was sent and is awaiting response
func (s *SurrealStore) SetPendingDM(userID string, sentAt time.Time) error {
	query := `
		INSERT INTO pending_dm (id, user_id, sent_at)
		VALUES (type::thing("pending_dm", $user_id), $user_id, $sent_at)
		ON DUPLICATE KEY UPDATE sent_at = $sent_at;
	`
	_, err := s.client.Query(query, map[string]interface{}{
		"user_id": userID,
		"sent_at": sentAt.Unix(),
	})
	return err
}

// ClearPendingDM removes the pending DM flag (user responded)
func (s *SurrealStore) ClearPendingDM(userID string) error {
	query := `DELETE type::thing("pending_dm", $user_id);`
	_, err := s.client.Query(query, map[string]interface{}{"user_id": userID})
	return err
}

// GetLastInteraction returns when the user last interacted with the bot
func (s *SurrealStore) GetLastInteraction(userID string) (time.Time, error) {
	query := `SELECT last_interaction FROM user_profiles WHERE id = type::thing("user_profiles", $user_id);`
	result, err := s.client.Query(query, map[string]interface{}{"user_id": userID})
	if err != nil {
		return time.Time{}, err
	}

	rows, ok := result.([]interface{})
	if !ok || len(rows) == 0 {
		return time.Time{}, nil // No record = never interacted
	}

	if row, ok := rows[0].(map[string]interface{}); ok {
		var timestamp int64
		switch t := row["last_interaction"].(type) {
		case float64:
			timestamp = int64(t)
		case int64:
			timestamp = t
		case int:
			timestamp = int64(t)
		case nil:
			return time.Time{}, nil // Field is nil = never set
		}
		if timestamp > 0 {
			return time.Unix(timestamp, 0), nil
		}
	}

	return time.Time{}, nil
}

// SetLastInteraction updates the user's last interaction timestamp
func (s *SurrealStore) SetLastInteraction(userID string, timestamp time.Time) error {
	query := `
		INSERT INTO user_profiles (id, user_id, facts, last_updated, last_interaction)
		VALUES (type::thing("user_profiles", $user_id), $user_id, [], time::unix(), $timestamp)
		ON DUPLICATE KEY UPDATE last_interaction = $timestamp, last_updated = time::unix();
	`
	_, err := s.client.Query(query, map[string]interface{}{
		"user_id":   userID,
		"timestamp": timestamp.Unix(),
	})
	return err
}

// Affection System

// GetAffection returns the affection level for a user (0-10000)
func (s *SurrealStore) GetAffection(userID string) (int, error) {
	// Query by user_id field which is simpler than constructing Record ID
	query := `SELECT affection FROM user_profiles WHERE user_id = $user_id;`
	result, err := s.client.Query(query, map[string]interface{}{"user_id": userID})
	if err != nil {
		fmt.Printf("GetAffection Error for %s: %v\n", userID, err)
		return 0, err
	}

	rows, ok := result.([]interface{})
	if !ok || len(rows) == 0 {
		fmt.Printf("GetAffection: No record found for %s\n", userID)
		return 0, nil // No record = 0 affection
	}

	if row, ok := rows[0].(map[string]interface{}); ok {
		// Log the raw value to debug type issues
		// fmt.Printf("GetAffection Raw Value for %s: %v\n", userID, row["affection"])
		
		switch a := row["affection"].(type) {
		case float64:
			return int(a), nil
		case float32:
			return int(a), nil
		case int64:
			return int(a), nil
		case uint64:
			return int(a), nil
		case int:
			return a, nil
		case nil:
			return 0, nil
		default:
			fmt.Printf("GetAffection: Unknown type for %s: %T\n", userID, a)
		}
	}

	return 0, nil
}

// AddAffection adds to the user's affection level (clamped to 0-100000)
func (s *SurrealStore) AddAffection(userID string, amount int) error {
	// Get current affection
	current, err := s.GetAffection(userID)
	if err != nil {
		current = 0
	}

	// Calculate new value, clamped to 0-100000
	newValue := current + amount
	if newValue < 0 {
		newValue = 0
	}
	if newValue > 100000 {
		newValue = 100000
	}

	return s.SetAffection(userID, newValue)
}

// SetAffection sets the user's affection level directly
func (s *SurrealStore) SetAffection(userID string, amount int) error {
	// Clamp to 0-100000
	if amount < 0 {
		amount = 0
	}
	if amount > 100000 {
		amount = 100000
	}

	query := `
		INSERT INTO user_profiles (id, user_id, facts, last_updated, affection)
		VALUES (type::thing("user_profiles", $user_id), $user_id, [], time::unix(), $affection)
		ON DUPLICATE KEY UPDATE affection = $affection, last_updated = time::unix();
	`
	_, err := s.client.Query(query, map[string]interface{}{
		"user_id":   userID,
		"affection": amount,
	})
	return err
}

// ==========================================
// STREAK SYSTEM
// ==========================================

// GetStreak returns the current streak for a user
func (s *SurrealStore) GetStreak(userID string) (int, error) {
	query := `SELECT streak FROM user_profiles WHERE user_id = $user_id;`
	result, err := s.client.Query(query, map[string]interface{}{"user_id": userID})
	if err != nil {
		return 0, err
	}

	rows, ok := result.([]interface{})
	if !ok || len(rows) == 0 {
		return 0, nil
	}

	if row, ok := rows[0].(map[string]interface{}); ok {
		switch a := row["streak"].(type) {
		case float64:
			return int(a), nil
		case int64:
			return int(a), nil
		case int:
			return a, nil
		case nil:
			return 0, nil
		}
	}

	return 0, nil
}

// UpdateStreak updates the user's daily streak
// Returns (new streak count, whether streak was broken)
func (s *SurrealStore) UpdateStreak(userID string) (int, bool) {
	today := time.Now().Format("2006-01-02")

	// Get current streak info
	query := `SELECT streak, last_streak_date FROM user_profiles WHERE user_id = $user_id;`
	result, err := s.client.Query(query, map[string]interface{}{"user_id": userID})
	if err != nil {
		return 0, false
	}

	var currentStreak int
	var lastDate string

	rows, ok := result.([]interface{})
	if ok && len(rows) > 0 {
		if row, ok := rows[0].(map[string]interface{}); ok {
			switch a := row["streak"].(type) {
			case float64:
				currentStreak = int(a)
			case int64:
				currentStreak = int(a)
			case int:
				currentStreak = a
			}
			if d, ok := row["last_streak_date"].(string); ok {
				lastDate = d
			}
		}
	}

	// If already updated today, return current streak
	if lastDate == today {
		return currentStreak, false
	}

	// Check if streak continues or breaks
	streakBroken := false
	newStreak := 1

	if lastDate != "" {
		lastStreakTime, err := time.Parse("2006-01-02", lastDate)
		if err == nil {
			daysSince := int(time.Since(lastStreakTime).Hours() / 24)
			if daysSince == 1 {
				// Streak continues!
				newStreak = currentStreak + 1
			} else if daysSince > 1 {
				// Streak broken
				streakBroken = true
				newStreak = 1
			}
		}
	}

	// Update streak in database
	updateQuery := `
		INSERT INTO user_profiles (id, user_id, facts, last_updated, streak, last_streak_date)
		VALUES (type::thing("user_profiles", $user_id), $user_id, [], time::unix(), $streak, $date)
		ON DUPLICATE KEY UPDATE streak = $streak, last_streak_date = $date, last_updated = time::unix();
	`
	s.client.Query(updateQuery, map[string]interface{}{
		"user_id": userID,
		"streak":  newStreak,
		"date":    today,
	})

	return newStreak, streakBroken
}

// GetFirstInteraction returns when the user first interacted with the bot
func (s *SurrealStore) GetFirstInteraction(userID string) (time.Time, error) {
	query := `SELECT first_interaction FROM user_profiles WHERE user_id = $user_id;`
	result, err := s.client.Query(query, map[string]interface{}{"user_id": userID})
	if err != nil {
		return time.Time{}, err
	}

	rows, ok := result.([]interface{})
	if !ok || len(rows) == 0 {
		return time.Time{}, nil
	}

	if row, ok := rows[0].(map[string]interface{}); ok {
		var timestamp int64
		switch t := row["first_interaction"].(type) {
		case float64:
			timestamp = int64(t)
		case int64:
			timestamp = t
		case int:
			timestamp = int64(t)
		case nil:
			return time.Time{}, nil
		}
		if timestamp > 0 {
			return time.Unix(timestamp, 0), nil
		}
	}

	return time.Time{}, nil
}

// SetFirstInteraction sets the user's first interaction timestamp (only if not already set)
func (s *SurrealStore) SetFirstInteraction(userID string, timestamp time.Time) error {
	// Only set if not already set (first_interaction == 0)
	query := `
		UPDATE user_profiles SET first_interaction = $timestamp
		WHERE user_id = $user_id AND (first_interaction IS NONE OR first_interaction = 0);
	`
	_, err := s.client.Query(query, map[string]interface{}{
		"user_id":   userID,
		"timestamp": timestamp.Unix(),
	})
	return err
}
