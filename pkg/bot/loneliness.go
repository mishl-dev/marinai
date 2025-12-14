package bot

import (
	"fmt"
	"log"
	"marinai/pkg/cerebras"
	"strings"
	"time"
)

func (h *Handler) checkForLoneliness() {
	// Check every hour
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		h.performLonelinessCheck()
	}
}

// performLonelinessCheck is separated for testing
// Uses Duolingo-style logic: won't DM a user if they haven't responded to the last one
func (h *Handler) performLonelinessCheck() bool {
	// Per-user inactivity threshold: 1 day without interaction
	const inactivityThreshold = 24 * time.Hour
	// Global loneliness threshold: only check when globally lonely (no interaction for 4 hours)
	const lonelinessThreshold = 4 * time.Hour

	// 1. Check if globally lonely (as a rate limiter)
	h.lastGlobalMu.RLock()
	isLonely := time.Since(h.lastGlobalInteraction) > lonelinessThreshold
	h.lastGlobalMu.RUnlock()

	if !isLonely {
		return false
	}

	if h.session == nil {
		log.Println("Session not set, skipping loneliness check")
		return false
	}

	// 2. Get all known users
	users, err := h.memoryStore.GetAllKnownUsers()
	if err != nil {
		log.Printf("Error getting known users: %v", err)
		return false
	}

	if len(users) == 0 {
		return false
	}

	// 3. Filter candidates: users who are inactive for 2+ days AND don't have pending DMs
	var eligibleUsers []string
	for _, userID := range users {
		// Check if user has a pending DM (Duolingo-style: don't spam if no response)
		hasPending, err := h.memoryStore.HasPendingDM(userID)
		if err != nil {
			log.Printf("Error checking pending DM for %s: %v", userID, err)
			continue
		}
		if hasPending {
			log.Printf("User %s has pending DM, skipping", userID)
			continue
		}

		// Check last interaction time
		lastInteraction, err := h.memoryStore.GetLastInteraction(userID)
		if err != nil {
			log.Printf("Error getting last interaction for %s: %v", userID, err)
			continue
		}

		// If never interacted (zero time), they're eligible
		// Otherwise check if they've been inactive for 2+ days
		if lastInteraction.IsZero() || time.Since(lastInteraction) > inactivityThreshold {
			eligibleUsers = append(eligibleUsers, userID)
		}
	}

	if len(eligibleUsers) == 0 {
		log.Println("No eligible users for boredom DM (all have pending DMs or were active recently)")
		return false
	}

	// 4. Select a random eligible user
	idx := time.Now().UnixNano() % int64(len(eligibleUsers))
	targetUserID := eligibleUsers[idx]

	// 5. Generate message
	facts, _ := h.memoryStore.GetFacts(targetUserID)
	profileText := "No known facts."
	if len(facts) > 0 {
		profileText = "- " + strings.Join(facts, "\n- ")
	}

	// Get relationship context
	affection, _ := h.memoryStore.GetAffection(targetUserID)
	relationshipInstruction := GetAffectionInstruction(affection)
	level := GetAffectionLevel(affection)

	// Get display name
	user, err := h.session.User(targetUserID)
	userName := "User"
	if err == nil {
		userName = user.Username
		if user.GlobalName != "" {
			userName = user.GlobalName
		}
	}

	prompt := fmt.Sprintf(`You are Marin Kitagawa. You haven't talked to anyone in a while and you feel a bit lonely/bored.
You decide to text one of your friends, %s.

%s

Things you remember about %s:
%s

Your relationship with them: %s %s

Write a short, casual message to them to start a conversation. Your tone should match your relationship level - be more reserved with strangers, more playful/flirty with close friends or special someones.

IMPORTANT INSTRUCTIONS:
- If you know specific facts about them (like their job, hobbies, what they're working on), reference those! Ask how it's going.
- Examples: "hey how's work at [company] going?", "did you finish that [thing they mentioned]?", "still playing [game]?"
- If you don't know much about them, just be your usual bubbly self and say you're bored.
- Keep it under 2 sentences.
- Sound natural, like a real text message.
- Do NOT say "User Profile" or "System" or mention that you're an AI.
- Do NOT be overly formal or use phrases like "I hope this message finds you well".

Discord Chat Style:
- Keep messages SHORT and natural. like you're actually texting
- mostly lowercase, casual typing
- you can be thoughtful or chill, but you default to playful and suggestive
- tease friends in a way that makes them flustered—that's your entertainment
- rarely use emojis. when you do, keep it simple
- no roleplay actions like *does something*

Just output the message, nothing else.`, userName, relationshipInstruction, userName, profileText, level.Emoji, level.Name)

	messages := []cerebras.Message{
		{Role: "system", Content: "You are Marin Kitagawa, a friendly cosplayer who texts like a real person. Be casual and natural. Adjust your tone based on how close you are to the person."},
		{Role: "user", Content: prompt},
	}

	reply, err := h.cerebrasClient.ChatCompletion(messages)
	if err != nil {
		log.Printf("Error generating lonely message: %v", err)
		return false
	}

	// 6. Send DM
	ch, err := h.session.UserChannelCreate(targetUserID)
	if err != nil {
		log.Printf("Error creating DM channel for %s: %v", targetUserID, err)
		return false
	}

	_, err = h.session.ChannelMessageSend(ch.ID, reply)
	if err != nil {
		log.Printf("Error sending lonely message to %s: %v", targetUserID, err)
		return false
	}

	log.Printf("Sent boredom DM to %s: %s", userName, reply)

	// 7. Mark as pending (Duolingo-style: won't send again until they respond)
	if err := h.memoryStore.SetPendingDM(targetUserID, time.Now()); err != nil {
		log.Printf("Error setting pending DM for %s: %v", targetUserID, err)
	}

	// Update global interaction time so we don't spam
	h.lastGlobalMu.Lock()
	h.lastGlobalInteraction = time.Now()
	h.lastGlobalMu.Unlock()

	// Also update in-memory tracking
	h.updateLastMessageTime(targetUserID)

	return true
}
