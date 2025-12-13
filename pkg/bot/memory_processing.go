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

func (h *Handler) addRecentMessage(userId, role, message string) {
	if err := h.memoryStore.AddRecentMessage(userId, role, message); err != nil {
		log.Printf("Error adding recent message: %v", err)
	}
}

func (h *Handler) getRecentMessages(userId string) []memory.RecentMessageItem {
	messages, err := h.memoryStore.GetRecentMessages(userId)
	if err != nil {
		log.Printf("Error getting recent messages: %v", err)
		return []memory.RecentMessageItem{}
	}
	return messages
}

func (h *Handler) extractMemories(userId string, userName string, userMessage string, botReply string) {
	// ---------------------------------------------------------
	// 1. Heuristic Filters (Save compute & reduce noise)
	// ---------------------------------------------------------
	cleanMsg := strings.TrimSpace(userMessage)

	// Filter A: Length Check
	// Ignore short, trivial messages like "Hi", "Thanks", "Ok", "Cool".
	// Long-term facts usually require a sentence structure.
	if len(cleanMsg) < 12 {
		return
	}

	// Filter B: Keyword/Subject Check
	// If the user isn't talking about themselves, we usually don't need to memorize it.
	// This skips questions like "What is the weather?" or "Write code for X".
	triggers := []string{
		"i am", "i'm", "my", "mine", // Self-identification
		"i live", "i work", "i study", // Life details
		"i like", "i love", "i hate", "i prefer", // Preferences
		"i have", "i've", // Possession/Experience
		"don't like", "dislike", // Negative preferences
		"name is", "call me", // Naming
		"remember", // Explicit instructions
	}

	hasTrigger := false
	lowerMsg := strings.ToLower(cleanMsg)
	for _, t := range triggers {
		if strings.Contains(lowerMsg, t) {
			hasTrigger = true
			break
		}
	}

	// If no self-reference keywords are found, abort.
	if !hasTrigger {
		return
	}

	log.Printf("Analyzing for memories: %s", cleanMsg)

	// ---------------------------------------------------------
	// 2. Fetch Existing Facts
	// ---------------------------------------------------------
	existingFacts, err := h.memoryStore.GetFacts(userId)
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

	// User Prompt: Focuses on specific logical constraints
	extractionPrompt := fmt.Sprintf(`Current Profile:
%s

Current Time: %s

New Interaction:
%s: "%s"
Marin: "%s"

Task: Analyze the interaction and output a JSON object with "add", "remove", and "reminders" lists.

STRICT RULES FOR MEMORY:
1. CONSERVATIVE: Bias towards returning empty lists. Only act if the information is explicitly stated and permanent.
2. PERMANENT ONLY: Save facts like Name, Job, Location, Allergies, Relationships.
3. IGNORE TEMPORARY: Do NOT save states like "I am hungry", "I am tired", "I am driving", or "I am busy".
4. IGNORE TRIVIAL: Do NOT save weak preferences or small talk (e.g., "I like that joke").
5. CONTRADICTIONS: If the user explicitly contradicts an item in 'Current Profile' (e.g., moved to a new city), add the new fact to 'add' and the old fact to 'remove'.

RULES FOR REMINDERS:
- If the user mentions a specific future event (exam, interview, trip) with a time frame, create a reminder.
- "delay_seconds": The number of seconds from the "Current Time" (provided above) until the event happens.
- "text": What the event is (e.g., "Math Exam", "Job Interview").
- If no specific time is given, do not create a reminder.

Output ONLY valid JSON.`, currentProfile, currentTimeStr, userName, userMessage, botReply)

	messages := []cerebras.Message{
		{
			Role: "system",
			// System Prompt: Sets the persona to be strict and lazy (avoids false positives)
			Content: `You are a strict Database Administrator responsible for long-term user records and scheduling.
Your goal is to keep the database clean and concise.
Reject all trivial information.
Reject all temporary states (moods, current activities) UNLESS they are future scheduled events.
Only record hard facts that will remain true for months.
Output ONLY valid JSON.`,
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
		lines := strings.Split(jsonStr, "\n")
		if len(lines) >= 2 {
			// If it starts with ```json or ```, strip the first and last lines
			// We reconstruct the middle lines
			jsonStr = strings.Join(lines[1:len(lines)-1], "\n")
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
		log.Printf("Applying memory delta for user %s: +%v, -%v", userId, delta.Add, delta.Remove)
		if err := h.memoryStore.ApplyDelta(userId, delta.Add, delta.Remove); err != nil {
			log.Printf("Error applying memory delta: %v", err)
		}
	}

	// ---------------------------------------------------------
	// 7. Add Reminders
	// ---------------------------------------------------------
	for _, r := range delta.Reminders {
		if r.DelaySeconds > 0 {
			dueAt := time.Now().Unix() + r.DelaySeconds
			log.Printf("Adding reminder for user %s: %s at %d (in %d seconds)", userId, r.Text, dueAt, r.DelaySeconds)
			if err := h.memoryStore.AddReminder(userId, r.Text, dueAt); err != nil {
				log.Printf("Error adding reminder: %v", err)
			}
		}
	}
}
