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

// getBackoffDuration returns the wait time based on DM count
// Schedule: 1 day -> 2 days -> 4 days -> 7 days (capped)
func getBackoffDuration(dmCount int) time.Duration {
	switch dmCount {
	case 0, 1:
		return 24 * time.Hour // 1 day
	case 2:
		return 48 * time.Hour // 2 days
	case 3:
		return 96 * time.Hour // 4 days
	default:
		return 168 * time.Hour // 7 days (capped)
	}
}

// performLonelinessCheck is separated for testing
// Uses exponential backoff: waits longer between DMs if user doesn't respond
// Schedule: 1 day -> 2 days -> 4 days -> 7 days (capped)
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
		log.Println("[Loneliness] Session not set, skipping loneliness check")
		return false
	}

	// 2. Get all known users
	users, err := h.memoryStore.GetAllKnownUsers()
	if err != nil {
		log.Printf("[Loneliness] Error getting known users: %v", err)
		return false
	}

	if len(users) == 0 {
		return false
	}

	// 3. Filter candidates using exponential backoff
	var eligibleUsers []string
	for _, userID := range users {
		// Check pending DM info for exponential backoff
		sentAt, dmCount, hasPending, err := h.memoryStore.GetPendingDMInfo(userID)
		if err != nil {
			log.Printf("[Loneliness] Error checking pending DM for %s: %v", userID, err)
			continue
		}

		if hasPending {
			// Cap at 4 DMs - after that, stop trying until user responds
			if dmCount >= 4 {
				log.Printf("[Loneliness] User %s: max DMs reached (%d), giving up until they respond", userID, dmCount)
				continue
			}

			// Check if enough time has passed based on backoff schedule
			backoffDuration := getBackoffDuration(dmCount)
			timeSinceDM := time.Since(sentAt)

			if timeSinceDM < backoffDuration {
				// Not enough time has passed, skip this user
				log.Printf("[Loneliness] User %s: backoff active (DM #%d, wait %.1f more hours)",
					userID, dmCount, (backoffDuration - timeSinceDM).Hours())
				continue
			}
			// Backoff period expired, user is eligible for another DM
			log.Printf("[Loneliness] User %s: backoff expired (DM #%d), eligible for next DM", userID, dmCount)
		}

		// Check last interaction time
		lastInteraction, err := h.memoryStore.GetLastInteraction(userID)
		if err != nil {
			log.Printf("[Loneliness] Error getting last interaction for %s: %v", userID, err)
			continue
		}

		// If never interacted (zero time), they're eligible
		// Otherwise check if they've been inactive for 1+ day
		if lastInteraction.IsZero() || time.Since(lastInteraction) > inactivityThreshold {
			eligibleUsers = append(eligibleUsers, userID)
		}
	}

	if len(eligibleUsers) == 0 {
		log.Println("[Loneliness] No eligible users for boredom DM (all in backoff or were active recently)")
		return false
	}

	// 4. Select a random eligible user
	idx := time.Now().UnixNano() % int64(len(eligibleUsers))
	targetUserID := eligibleUsers[idx]

	// 5. Get DM attempt context (for tone adjustment)
	_, currentDMCount, _, _ := h.memoryStore.GetPendingDMInfo(targetUserID)
	dmAttempt := currentDMCount + 1 // This will be the Nth DM we're sending

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

	// Build DM attempt context for tone adjustment
	var attemptContext string
	switch dmAttempt {
	case 1:
		attemptContext = "This is your FIRST time reaching out. Be casual and friendly."
	case 2:
		attemptContext = "This is your SECOND attempt - they didn't reply to your last message a few days ago. Be a little more playful or tease them gently about being quiet."
	case 3:
		attemptContext = "This is your THIRD attempt - they've ignored you twice now. You can be a bit dramatic or pouty about it. Maybe guilt-trip them playfully."
	case 4:
		attemptContext = "This is your FOURTH and FINAL attempt - they've ignored you three times. Be dramatic, maybe a bit sad or resigned. This is your last message if they don't respond."
	}

	prompt := fmt.Sprintf(`You are Marin Kitagawa. You haven't talked to anyone in a while and you feel a bit lonely/bored.
You decide to text one of your friends, %s.

%s

Things you remember about %s:
%s

Your relationship with them: %s %s

DM ATTEMPT CONTEXT:
%s

Write a short, casual message to them to start a conversation. Your tone should match your relationship level AND the attempt number.

IMPORTANT INSTRUCTIONS:
- If you know specific facts about them (like their job, hobbies, what they're working on), reference those! Ask how it's going.
- Examples: "hey how's work at [company] going?", "did you finish that [thing they mentioned]?", "still playing [game]?"
- If you don't know much about them, just be your usual bubbly self and say you're bored.
- Do NOT say "User Profile" or "System" or mention that you're an AI.
- Do NOT be overly formal or use phrases like "I hope this message finds you well".
- Adjust your tone based on the DM attempt context above!

Discord Chat Style:
- EXTREMELY SHORT messages (1-2 sentences MAX).
- mostly lowercase, casual typing.
- ABSOLUTELY NO EMOJIS OR EMOTICONS. Express yourself with words only.
- NO ROLEPLAY (*actions*). This is text, not a roleplay server.
- NEVER start a message with "Oh,", "Ah,", or "Hmm,".
- NEVER use asterisks for actions.
- Sound natural, like a real text message.
- you can be thoughtful or chill, but you default to playful and suggestive
- tease friends in a way that makes them flustered—that's your entertainment

Just output the message, nothing else.`, userName, relationshipInstruction, userName, profileText, level.Emoji, level.Name, attemptContext)

	messages := []cerebras.Message{
		{Role: "system", Content: "You are Marin Kitagawa, a friendly cosplayer who texts like a real person. Be casual and natural. Adjust your tone based on how close you are to the person."},
		{Role: "user", Content: prompt},
	}

	reply, err := h.cerebrasClient.ChatCompletion(messages)
	if err != nil {
		log.Printf("[Loneliness] Error generating lonely message: %v", err)
		return false
	}

	// 6. Send DM
	ch, err := h.session.UserChannelCreate(targetUserID)
	if err != nil {
		log.Printf("[Loneliness] Error creating DM channel for %s: %v", targetUserID, err)
		return false
	}

	_, err = h.session.ChannelMessageSend(ch.ID, reply)
	if err != nil {
		log.Printf("[Loneliness] Error sending lonely message to %s: %v", targetUserID, err)
		return false
	}

	log.Printf("[Loneliness] Sent boredom DM to %s: %s", userName, reply)

	// 7. Mark as pending (Duolingo-style: won't send again until they respond)
	if err := h.memoryStore.SetPendingDM(targetUserID, time.Now()); err != nil {
		log.Printf("[Loneliness] Error setting pending DM for %s: %v", targetUserID, err)
	}

	// Update global interaction time so we don't spam
	h.lastGlobalMu.Lock()
	h.lastGlobalInteraction = time.Now()
	h.lastGlobalMu.Unlock()

	// Also update in-memory tracking
	h.updateLastMessageTime(targetUserID)

	return true
}
