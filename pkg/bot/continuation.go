package bot

import (
	"fmt"
	"log"
	
	"marinai/pkg/memory"
	"math/rand"
	"strings"
	"time"
)

// ==========================================
// CONVERSATION CONTINUATION SYSTEM
// ==========================================
// Makes Marin feel like a real person who thinks about
// conversations after they end and comes back with new thoughts.

// ContinuationChance returns the probability of queuing a continuation based on affection
// Higher affection = more likely to think about them later
func ContinuationChance(affection int) float64 {
	level := GetAffectionLevel(affection)
	switch level.Name {
	case "Special Someone":
		return 0.35 // 35% chance
	case "Soulmate":
		return 0.30
	case "Best Friend":
		return 0.25
	case "Close Friend":
		return 0.20
	case "Good Friend":
		return 0.15
	case "Friend":
		return 0.10
	case "Casual Friend":
		return 0.05
	default:
		return 0 // Don't do this for strangers
	}
}

// ContinuationDelayRange returns min/max delay in hours based on affection
// Higher affection = might come back sooner
func ContinuationDelayRange(affection int) (minHours, maxHours float64) {
	level := GetAffectionLevel(affection)
	switch level.Name {
	case "Special Someone", "Soulmate":
		return 0.5, 3.0 // 30 mins to 3 hours
	case "Best Friend", "Close Friend":
		return 1.0, 5.0 // 1-5 hours
	case "Good Friend", "Friend":
		return 2.0, 8.0 // 2-8 hours
	default:
		return 4.0, 12.0 // 4-12 hours
	}
}

// QueueContinuation decides whether to queue a continuation thought after a conversation
func (h *Handler) QueueContinuation(userID string, userMessage string, marinReply string) {
	// Get affection to determine probability
	affection, _ := h.memoryStore.GetAffection(userID)
	chance := ContinuationChance(affection)

	if chance == 0 || rand.Float64() > chance {
		return // Not rolling continuation
	}

	// Check if already has a pending thought for this user
	hasPending, _ := h.memoryStore.HasDelayedThought(userID)
	if hasPending {
		return // Don't stack thoughts
	}

	// Determine delay
	minHours, maxHours := ContinuationDelayRange(affection)
	delayHours := minHours + rand.Float64()*(maxHours-minHours)
	scheduledAt := time.Now().Add(time.Duration(delayHours * float64(time.Hour)))

	// Generate a brief summary of the conversation topic
	summary := h.generateConvoSummary(userMessage, marinReply)

	// Create brief context from the conversation
	thought := memory.DelayedThought{
		UserID:         userID,
		ConvoSummary:   summary,
		LastUserMsg:    truncateMessage(userMessage, 200),
		LastMarinReply: truncateMessage(marinReply, 200),
		ScheduledAt:    scheduledAt.Unix(),
		CreatedAt:      time.Now().Unix(),
	}

	if err := h.memoryStore.AddDelayedThought(thought); err != nil {
		log.Printf("Error queuing continuation for %s: %v", userID, err)
		return
	}

	log.Printf("Queued continuation for %s in %.1f hours (topic: %s)", userID, delayHours, summary)
}

// generateConvoSummary creates a brief summary of what the conversation was about
func (h *Handler) generateConvoSummary(userMessage string, marinReply string) string {
	prompt := fmt.Sprintf(`Summarize this conversation exchange in 5-10 words. Focus on the TOPIC, not the emotions.

User: "%s"
Marin: "%s"

Output ONLY the brief topic summary, nothing else. Examples:
- "talking about their job stress"
- "discussing favorite anime"
- "joking about staying up late"
- "them asking about my cosplay"`, userMessage, marinReply)

	messages := []memory.LLMMessage{
		{Role: "system", Content: "You summarize conversations in 5-10 words. Output only the summary."},
		{Role: "user", Content: prompt},
	}

	reply, err := h.llmClient.ChatCompletion(messages)
	if err != nil {
		log.Printf("Error generating convo summary: %v", err)
		return "general chat" // Fallback
	}

	summary := strings.TrimSpace(reply)
	// Clean up any quotes the LLM might add
	summary = strings.Trim(summary, `"'`)

	// Truncate if too long
	if len(summary) > 50 {
		summary = summary[:47] + "..."
	}

	return summary
}

// truncateMessage cuts a message to maxLen, adding "..." if truncated
func truncateMessage(msg string, maxLen int) string {
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen-3] + "..."
}

// runContinuationLoop checks for and sends due continuation thoughts
func (h *Handler) runContinuationLoop() {
	// Check every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		h.processDueContinuations()
	}
}

// processDueContinuations finds and sends all due continuation thoughts
func (h *Handler) processDueContinuations() {
	if h.session == nil {
		return
	}

	thoughts, err := h.memoryStore.GetDueDelayedThoughts()
	if err != nil {
		log.Printf("Error getting due continuations: %v", err)
		return
	}

	for _, thought := range thoughts {
		h.sendContinuationThought(thought)
	}
}

// sendContinuationThought generates and sends a continuation message
func (h *Handler) sendContinuationThought(thought memory.DelayedThought) {
	// First, delete the thought so we don't send it again
	if err := h.memoryStore.DeleteDelayedThought(thought.ID); err != nil {
		log.Printf("Error deleting thought %s: %v", thought.ID, err)
		return
	}

	// Check if user has been active since we queued this
	// If they've talked to us again, the conversation has moved on
	lastInteraction, _ := h.memoryStore.GetLastInteraction(thought.UserID)
	if !lastInteraction.IsZero() && lastInteraction.Unix() > thought.CreatedAt {
		log.Printf("Skipping continuation for %s - they've been active since", thought.UserID)
		return
	}

	// Check for pending DMs - don't stack on top of boredom DMs
	hasPendingDM, _ := h.memoryStore.HasPendingDM(thought.UserID)
	if hasPendingDM {
		log.Printf("Skipping continuation for %s - has pending DM", thought.UserID)
		return
	}

	// Get affection for context
	affection, _ := h.memoryStore.GetAffection(thought.UserID)
	level := GetAffectionLevel(affection)

	// Get user's name
	user, err := h.session.User(thought.UserID)
	userName := "them"
	if err == nil {
		userName = user.Username
		if user.GlobalName != "" {
			userName = user.GlobalName
		}
	}

	// Generate the continuation message
	message := h.generateContinuationMessage(thought, level, userName)
	if message == "" {
		return
	}

	// Send DM
	ch, err := h.session.UserChannelCreate(thought.UserID)
	if err != nil {
		log.Printf("Error creating DM channel for continuation: %v", err)
		return
	}

	_, err = h.session.ChannelMessageSend(ch.ID, message)
	if err != nil {
		log.Printf("Error sending continuation: %v", err)
		return
	}

	log.Printf("Sent continuation to %s: %s", userName, message)

	// Mark as pending so boredom DM doesn't trigger
	h.memoryStore.SetPendingDM(thought.UserID, time.Now())
}

// generateContinuationMessage creates a personalized follow-up message
func (h *Handler) generateContinuationMessage(thought memory.DelayedThought, level AffectionLevel, userName string) string {
	// Get Marin's current state for context
	statePrompt := h.GetStateForPrompt()

	// Calculate how long ago the conversation was
	hoursAgo := time.Since(time.Unix(thought.CreatedAt, 0)).Hours()
	timeContext := "a little bit ago"
	if hoursAgo >= 6 {
		timeContext = "earlier today"
	}
	if hoursAgo >= 12 {
		timeContext = "a while ago"
	}

	prompt := fmt.Sprintf(`You are Marin Kitagawa. You talked to %s %s and you've been thinking about the conversation.

Your relationship: %s %s

Topic you were discussing: %s

What they said: "%s"
What you replied: "%s"

%s

Now you want to add something to the conversation - maybe you:
- Had a new thought about what they said
- Remembered something relevant
- Wanted to add something you forgot to say
- Just kept thinking about them and wanted to reach out

Write a SHORT message (1-2 sentences) that feels like you're coming back to continue the conversation naturally.

Rules:
- EXTREMELY SHORT messages (1-2 sentences MAX).
- mostly lowercase, casual typing.
- ABSOLUTELY NO EMOJIS OR EMOTICONS. Express yourself with words only.
- NO ROLEPLAY (*actions*). This is text, not a roleplay server.
- NEVER start a message with "Oh,", "Ah,", or "Hmm,".
- Be natural, like a real text message.
- Reference the previous conversation but don't repeat it verbatim.
- Could be a follow-up question, an afterthought, or just continuing the vibe.
- Match your energy to the relationship level.

Just output the message, nothing else.`,
		userName, timeContext, level.Emoji, level.Name,
		thought.ConvoSummary,
		thought.LastUserMsg, thought.LastMarinReply, statePrompt)

	messages := []memory.LLMMessage{
		{Role: "system", Content: "You are Marin Kitagawa continuing a conversation hours later with a follow-up thought."},
		{Role: "user", Content: prompt},
	}

	reply, err := h.llmClient.ChatCompletion(messages)
	if err != nil {
		log.Printf("Error generating continuation: %v", err)
		// Fall back to simple templates
		return h.getFallbackContinuation(level)
	}

	return strings.TrimSpace(reply)
}

// getFallbackContinuation returns a simple template message if LLM fails
func (h *Handler) getFallbackContinuation(level AffectionLevel) string {
	templates := []string{
		"wait i was just thinking about what you said earlier",
		"oh also i forgot to mention",
		"hey so about earlier...",
		"this is random but i kept thinking about our convo",
	}

	if level.MinAffection >= 65000 { // Close friend+
		templates = append(templates, []string{
			"cant stop thinking about what you said earlier lol",
			"okay so i had another thought about what we were talking about",
			"hey you. was just thinking about you",
		}...)
	}

	if level.MinAffection >= 90000 { // Soulmate+
		templates = append(templates, []string{
			"i keep replaying our conversation in my head",
			"miss talking to you already tbh",
		}...)
	}

	return templates[rand.Intn(len(templates))]
}
