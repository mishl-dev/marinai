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
	extractionPrompt := fmt.Sprintf(`Current Profile:
%s

Current Time: %s

New Interaction:
%s: "%s"
Marin: "%s"

Task: Analyze the interaction and output a JSON object with "add", "remove", and "reminders" lists.

WHAT TO SAVE (add to "add" list):
- Location: where they live, where they moved to, where they're from
- Job/School: what they do, where they work/study  
- Hobbies: games they play, shows they watch, things they enjoy
- Relationships: if they mention partners, pets, family
- Strong preferences: favorite things, things they hate
- Life events: graduations, new jobs, moves, major purchases

WHAT NOT TO SAVE:
%s

CONTRADICTIONS:
If the user says something that contradicts an existing fact (e.g., they moved to a new city), add the new fact to "add" AND the old conflicting fact to "remove".

REMINDERS:
If they mention a specific future event with a timeframe, create a reminder with "delay_seconds" (seconds until event) and "text" (event description).

Output ONLY valid JSON. Example: {"add": ["Lives in Tokyo"], "remove": [], "reminders": []}`, currentProfile, currentTimeStr, userName, userMessage, botReply, exclusionList)

	messages := []cerebras.Message{
		{
			Role: "system",
			Content: fmt.Sprintf(`You are a helpful assistant that extracts personal facts about the USER from conversations.
Your job is to identify and save important information about the USER (%s) that would be useful to remember.
CRITICAL: Extract facts ONLY about %s. DO NOT extract facts about Marin or the AI.
Output ONLY valid JSON with "add", "remove", and "reminders" arrays.`, userName, userName),
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
	jsonStr := strings.TrimSpace(resp)

	// Robust markdown stripping
	if strings.HasPrefix(jsonStr, "```") {
		// Find first newline
		if idx := strings.Index(jsonStr, "\n"); idx != -1 {
			// Find last newline
			if lastIdx := strings.LastIndex(jsonStr, "\n"); lastIdx > idx {
				// Slice the string between the first and last newline
				// We exclude the first newline char (idx+1)
				// We rely on strings.TrimSpace later to handle any surrounding whitespace
				jsonStr = jsonStr[idx+1 : lastIdx]
			}
		}
	}
	jsonStr = strings.TrimSpace(jsonStr)

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
