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
	`
	_, err := s.client.Query(query, map[string]interface{}{})
	return err
}

// Emoji Cache

func (s *SurrealStore) GetCachedEmojis(guildID string) ([]string, error) {
	query := `SELECT emojis FROM type::thing("guild_cache", $guild_id);`
	result, err := s.client.Query(query, map[string]interface{}{"guild_id": guildID})
	if err != nil {
		return nil, err
	}

	resSlice, ok := result.([]interface{})
	if !ok || len(resSlice) == 0 {
		return nil, nil // Not found is not an error
	}

	queryRes := resSlice[0]
	var emojis []string

	if resMap, ok := queryRes.(map[string]interface{}); ok {
		if val, ok := resMap["result"]; ok {
			if rows, ok := val.([]interface{}); ok && len(rows) > 0 {
				if row, ok := rows[0].(map[string]interface{}); ok {
					if e, ok := row["emojis"].([]interface{}); ok {
						for _, item := range e {
							if str, ok := item.(string); ok {
								emojis = append(emojis, str)
							}
						}
					}
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

func (s *SurrealStore) GetAllKnownUsers() ([]string, error) {
	query := `SELECT user_id FROM user_profiles;`
	result, err := s.client.Query(query, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	rows, ok := result.([]interface{})
	if !ok {
		return []string{}, nil
	}

	var users []string
	for _, row := range rows {
		if rowMap, ok := row.(map[string]interface{}); ok {
			if userID, ok := rowMap["user_id"].(string); ok {
				users = append(users, userID)
			}
		}
	}
	return users, nil
}

func (s *SurrealStore) DeleteUserData(userId string) error {
	query := `
		DELETE memories WHERE user_id = $user_id;
		DELETE recent_messages WHERE user_id = $user_id;
		DELETE user_profiles WHERE user_id = $user_id;
	`
	_, err := s.client.Query(query, map[string]interface{}{"user_id": userId})
	return err
}

// Profile management

func (s *SurrealStore) GetFacts(userId string) ([]string, error) {
	// Direct record lookup: user_profiles:<userId>
	// Note: In SurrealDB, record IDs are `table:id`. We assume userId is safe to use as ID part.
	// If userId contains special chars, we might need to escape or hash it, but for Discord IDs (snowflakes) it's fine.
	query := `SELECT facts FROM type::thing("user_profiles", $user_id);`

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
