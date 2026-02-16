package memory

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// GetFactsWithTimestamps returns facts with their creation timestamps
func (s *SurrealStore) GetFactsWithTimestamps(userID string) ([]FactItem, error) {
	query := `SELECT facts FROM type::thing("user_profiles", $user_id);`

	result, err := s.client.Query(query, map[string]interface{}{"user_id": userID})
	if err != nil {
		return nil, err
	}

	// Parse result using reflection (same pattern as GetFacts)
	var factItems []FactItem

	if resSlice, ok := result.([]interface{}); ok && len(resSlice) > 0 {
		if queryRes, ok := resSlice[0].(map[string]interface{}); ok {
			if val, ok := queryRes["result"]; ok {
				if rows, ok := val.([]interface{}); ok && len(rows) > 0 {
					if row, ok := rows[0].(map[string]interface{}); ok {
						if facts, ok := row["facts"].([]interface{}); ok {
							for _, item := range facts {
								if factMap, ok := item.(map[string]interface{}); ok {
									text, _ := factMap["text"].(string)
									var createdAt int64
									switch v := factMap["created_at"].(type) {
									case float64:
										createdAt = int64(v)
									case int64:
										createdAt = v
									case int:
										createdAt = int64(v)
									}
									factItems = append(factItems, FactItem{
										Text:      text,
										CreatedAt: createdAt,
									})
								}
							}
						}
					}
				}
			}
		}
	}

	return factItems, nil
}

// ArchiveFact moves a fact from user_profiles to vector memories
func (s *SurrealStore) ArchiveFact(userID, factText string, embedding []float32) error {
	// Add to vector memories
	item := SurrealMemoryItem{
		UserID:    userID,
		Text:      factText,
		Embedding: embedding,
		Timestamp: time.Now().Unix(),
	}

	_, err := s.client.Create("memories", item)
	if err != nil {
		return fmt.Errorf("failed to archive fact to memories: %w", err)
	}

	// Remove from user_profiles
	query := `
		UPDATE type::thing("user_profiles", $user_id) SET 
			facts = array::filter(facts, |$fact| $fact.text != $fact_text),
			last_updated = time::unix();
	`

	_, err = s.client.Query(query, map[string]interface{}{
		"user_id":   userID,
		"fact_text": factText,
	})

	return err
}

// SummarizeFacts uses LLM to consolidate facts
func (s *SurrealStore) SummarizeFacts(userID string, llmClient LLMClient) ([]string, error) {
	// Get all facts
	factItems, err := s.GetFactsWithTimestamps(userID)
	if err != nil {
		return nil, err
	}

	if len(factItems) == 0 {
		return []string{}, nil
	}

	// Extract just the text
	var factTexts []string
	for _, item := range factItems {
		factTexts = append(factTexts, item.Text)
	}

	// Build summarization prompt
	targetCount := len(factTexts) / 2
	if targetCount < 1 {
		targetCount = 1
	}

	prompt := fmt.Sprintf(`You are consolidating user profile facts. Merge related facts and remove redundancy while preserving all unique information.

Current facts (%d total):
%s

Task: Consolidate these facts into approximately %d facts (50%% reduction). This keeps facts concise for semantic search.
- Combine similar facts: ["loves pizza", "favorite food is pasta"] → "enjoys Italian food, especially pizza and pasta"
- Keep distinct facts separate
- Preserve all unique information
- Use concise language
- Aim for exactly %d consolidated facts

Return ONLY a JSON array of strings with the consolidated facts. Example: ["fact 1", "fact 2", "fact 3"]`, len(factTexts), strings.Join(factTexts, "\n"), targetCount, targetCount)

	messages := []LLMMessage{
		{Role: "system", Content: "You are a fact consolidation assistant. Output ONLY JSON."},
		{Role: "user", Content: prompt},
	}

	resp, err := llmClient.ChatCompletion(messages)
	if err != nil {
		return nil, fmt.Errorf("failed to get summarization from LLM: %w", err)
	}

	// Parse JSON response
	jsonStr := strings.TrimSpace(resp)
	if strings.HasPrefix(jsonStr, "```json") {
		jsonStr = strings.TrimPrefix(jsonStr, "```json")
		jsonStr = strings.TrimSuffix(jsonStr, "```")
	} else if strings.HasPrefix(jsonStr, "```") {
		jsonStr = strings.TrimPrefix(jsonStr, "```")
		jsonStr = strings.TrimSuffix(jsonStr, "```")
	}
	jsonStr = strings.TrimSpace(jsonStr)

	var summarizedFacts []string
	if err := json.Unmarshal([]byte(jsonStr), &summarizedFacts); err != nil {
		return nil, fmt.Errorf("failed to parse summarization JSON: %w. Response: %s", err, resp)
	}

	return summarizedFacts, nil
}

// MaintainUserProfile performs aging and summarization on a user's profile
func (s *SurrealStore) MaintainUserProfile(userID string, embeddingClient EmbeddingClient, llmClient LLMClient, agingDays int, summarizationThreshold int) (archivedCount int, summarized bool, err error) {
	// Get facts with timestamps
	factItems, err := s.GetFactsWithTimestamps(userID)
	if err != nil {
		return 0, false, err
	}

	if len(factItems) == 0 {
		return 0, false, nil
	}

	// Check for facts older than agingDays
	cutoffTime := time.Now().AddDate(0, 0, -agingDays).Unix()
	var oldFacts []FactItem
	var currentFacts []FactItem

	for _, fact := range factItems {
		if fact.CreatedAt < cutoffTime {
			oldFacts = append(oldFacts, fact)
		} else {
			currentFacts = append(currentFacts, fact)
		}
	}

	// Archive old facts to vector memories
	for _, fact := range oldFacts {
		embedding, err := embeddingClient.Embed(fact.Text)
		if err != nil {
			log.Printf("Error generating embedding for fact '%s': %v", fact.Text, err)
			continue
		}

		if err := s.ArchiveFact(userID, fact.Text, embedding); err != nil {
			log.Printf("Error archiving fact '%s': %v", fact.Text, err)
			continue
		}

		archivedCount++
		log.Printf("Archived fact for user %s: %s (age: %d days)", userID, fact.Text, (time.Now().Unix()-fact.CreatedAt)/(60*60*24))
	}

	// Check if summarization is needed
	if len(currentFacts) > summarizationThreshold {
		log.Printf("User %s has %d facts, triggering summarization (threshold: %d)", userID, len(currentFacts), summarizationThreshold)

		summarizedFacts, err := s.SummarizeFacts(userID, llmClient)
		if err != nil {
			return archivedCount, false, fmt.Errorf("failed to summarize facts: %w", err)
		}

		// Replace all facts with summarized version
		// First, clear existing facts
		_, err = s.client.Query(`UPDATE type::thing("user_profiles", $user_id) SET facts = [], last_updated = time::unix();`, map[string]interface{}{
			"user_id": userID,
		})
		if err != nil {
			return archivedCount, false, fmt.Errorf("failed to clear facts for summarization: %w", err)
		}

		// Then add summarized facts with current timestamp
		for _, factText := range summarizedFacts {
			err = s.AddFactWithTimestamp(userID, factText, time.Now().Unix())
			if err != nil {
				log.Printf("Error adding summarized fact '%s': %v", factText, err)
			}
		}

		summarized = true
		log.Printf("Summarized %d facts into %d facts for user %s", len(currentFacts), len(summarizedFacts), userID)
	}

	return archivedCount, summarized, nil
}

// AddFactWithTimestamp adds a single fact with a specific timestamp
func (s *SurrealStore) AddFactWithTimestamp(userID, factText string, timestamp int64) error {
	query := `
		-- Ensure record exists
		INSERT INTO user_profiles (id, user_id, facts, last_updated) 
		VALUES (type::thing("user_profiles", $user_id), $user_id, [], time::unix()) 
		ON DUPLICATE KEY UPDATE last_updated = time::unix();

		-- Add fact with timestamp
		UPDATE type::thing("user_profiles", $user_id) SET 
			facts = array::append(facts, {text: $fact_text, created_at: $timestamp}),
			last_updated = time::unix();
	`

	_, err := s.client.Query(query, map[string]interface{}{
		"user_id":   userID,
		"fact_text": factText,
		"timestamp": timestamp,
	})

	return err
}
