package bot

import (
	"encoding/json"
	"fmt"
	"log"
	"marinai/pkg/cerebras"
	"marinai/pkg/memory"
	"strings"
	"time"
)

func (h *Handler) addRecentMessage(userID, role, message string) {
	if err := h.memoryStore.AddRecentMessage(userID, role, message); err != nil {
		log.Printf("Error adding recent message: %v", err)
	}
}

func (h *Handler) getRecentMessages(userID string) []memory.RecentMessageItem {
	messages, err := h.memoryStore.GetRecentMessages(userID)
	if err != nil {
		log.Printf("Error getting recent messages: %v", err)
		return []memory.RecentMessageItem{}
	}
	return messages
}

func (h *Handler) extractMemories(userID string, userName string, userMessage string, botReply string) {
	// ---------------------------------------------------------
	// 1. Basic Filter (Save compute on trivial messages)
	// ---------------------------------------------------------
	cleanMsg := strings.TrimSpace(userMessage)

	// Skip very short messages - unlikely to contain memorable info
	if len(cleanMsg) < 10 {
		return
	}

	// Let the LLM decide what's worth saving - no hardcoded keyword filters
	log.Printf("Analyzing for memories: %s", cleanMsg)

	// ---------------------------------------------------------
	// 2. Fetch Existing Facts
	// ---------------------------------------------------------
	existingFacts, err := h.memoryStore.GetFacts(userID)
	if err != nil {
		log.Printf("Error fetching facts for extraction: %v", err)
		return
	}

	// ---------------------------------------------------------
	// 3. Construct Prompts (Strict & Conservative)
	// ---------------------------------------------------------
	currentProfile := "None"
	if len(existingFacts) > 0 {
		currentProfile = "- " + strings.Join(existingFacts, "\n- ")
	}

	// Current time for reminder calculation
	now := time.Now().UTC()
	currentTimeStr := now.Format("Monday, 2006-01-02 15:04 UTC")

	// What to ignore - expanded to prevent self-learning
	exclusionList := `
- Temporary states: "I'm hungry", "I'm tired", "I'm busy right now"
- Single-use preferences: "I like that joke", "that's funny"
- Questions they asked
- Generic greetings
- Facts about Marin, the AI, or the character she is roleplaying
- Information that is clearly already known (see Current Profile)`

	// User Prompt: Focuses on extracting useful personal information
	extractionPrompt := fmt.Sprintf(`
# CONTEXT
<profile>
%s
</profile>
<time>
%s
</time>

# INTERACTION
<chat>
%s: "%s"
Marin: "%s"
</chat>

# TARGET
<task>
Analyze the interaction and output a JSON object with "add", "remove", and "reminders" lists.
</task>

# RULES
<save>
- Location: Where they live, move, or are from.
- Job/School: What they do, where they work/study.
- Hobbies: Games, shows, interests.
- Relationships: Mention of partners, pets, family.
- Preferences: Favorite things, things they hate.
- Life events: Graduations, new jobs, moves.
</save>
<ignore>
%s
</ignore>

<contradictions>
If the user says something that contradicts an existing fact, add the new fact to "add" AND the old conflicting fact to "remove".
</contradictions>

<reminders>
If they mention a specific future event with a timeframe, create a reminder with "delay_seconds" and "text".
</reminders>

# MANDATORY
Output ONLY valid JSON.
Example: {"add": ["Lives in Tokyo"], "remove": [], "reminders": []}`, currentProfile, currentTimeStr, userName, userMessage, botReply, exclusionList)

	messages := []cerebras.Message{
		{
			Role: "system",
			Content: fmt.Sprintf(`
# INSTRUCTION
<role>
You are an extraction assistant for Marin Kitagawa. Identify and save important personal facts about the USER (%s).
</role>

# CONSTRAINTS
<rules>
- Extract facts ONLY about %s. 
- DO NOT extract facts about Marin, the AI, or roleplay scenarios.
- Output ONLY valid JSON.
</rules>`, userName, userName),
		},
		{
			Role:    "user",
			Content: extractionPrompt,
		},
	}

	// ---------------------------------------------------------
	// 4. Call LLM
	// ---------------------------------------------------------
	resp, err := h.cerebrasClient.ChatCompletion(messages)
	if err != nil {
		log.Printf("Error extracting memories: %v", err)
		return
	}

	// ---------------------------------------------------------
	// 5. Parse JSON
	// ---------------------------------------------------------
	jsonStr := cleanJSONOutput(resp)

	type Delta struct {
		Add       []string          `json:"add"`
		Remove    []string          `json:"remove"`
		Reminders []ReminderRequest `json:"reminders"`
	}

	var delta Delta
	if err := json.Unmarshal([]byte(jsonStr), &delta); err != nil {
		log.Printf("[Memory Extraction] Failed to parse JSON: %v. Raw output: %s", err, jsonStr)
		return
	}

	log.Printf("[Memory Extraction] Delta: %+v", delta)

	// ---------------------------------------------------------
	// 6. Apply Delta
	// ---------------------------------------------------------
	if len(delta.Add) > 0 || len(delta.Remove) > 0 {
		log.Printf("Applying memory delta for user %s: +%v, -%v", userID, delta.Add, delta.Remove)
		if err := h.memoryStore.ApplyDelta(userID, delta.Add, delta.Remove); err != nil {
			log.Printf("Error applying memory delta: %v", err)
		}
	}

	// ---------------------------------------------------------
	// 7. Add Reminders
	// ---------------------------------------------------------
	for _, r := range delta.Reminders {
		if r.DelaySeconds > 0 {
			dueAt := time.Now().Unix() + r.DelaySeconds
			log.Printf("Adding reminder for user %s: %s at %d (in %d seconds)", userID, r.Text, dueAt, r.DelaySeconds)
			if err := h.memoryStore.AddReminder(userID, r.Text, dueAt); err != nil {
				log.Printf("Error adding reminder: %v", err)
			}
		}
	}
}

// cleanJSONOutput extracts and cleans JSON content from LLM response
func cleanJSONOutput(input string) string {
	jsonStr := strings.TrimSpace(input)

	// Robust markdown stripping
	if idx := strings.Index(jsonStr, "```"); idx != -1 {
		// Find the end of the code block, starting search after the first backticks
		if endIdx := strings.Index(jsonStr[idx+3:], "```"); endIdx != -1 {
			// Adjust endIdx relative to original string
			endIdx += idx + 3

			// Extract content inside the backticks
			blockContent := jsonStr[idx+3 : endIdx]

			// Trim "json" if it exists at the start of the block (common in LLM output)
			blockContent = strings.TrimPrefix(strings.TrimSpace(blockContent), "json")

			jsonStr = strings.TrimSpace(blockContent)
		}
	}
	// Fallback: If no markdown block is found, assume the whole string is JSON (or try to parse it)
	// But first, let's try to find the outer braces if the string has garbage around it but no markdown
	if !strings.HasPrefix(jsonStr, "{") {
		start := strings.Index(jsonStr, "{")
		end := strings.LastIndex(jsonStr, "}")
		if start != -1 && end != -1 && end > start {
			jsonStr = jsonStr[start : end+1]
		}
	}
	return jsonStr
}
