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
		log.Println("Session not set, skipping loneliness check")
		return false
	}

	// 2. Get pending DMs (Batch 1) - Small number of users
	pendingDMs, err := h.memoryStore.GetPendingDMs()
	if err != nil {
		log.Printf("Error getting pending DMs: %v", err)
		return false
	}

	// 3. Get inactive users (Batch 2) - Users who haven't interacted in 24h
	cutoff := time.Now().Add(-inactivityThreshold)
	inactiveUsers, err := h.memoryStore.GetInactiveUsers(cutoff)
	if err != nil {
		log.Printf("Error getting inactive users: %v", err)
		return false
	}

	// 4. Combine and Filter candidates using exponential backoff
	eligibleMap := make(map[string]bool)

	// A. Process Pending DMs (Existing tracks)
	for userID, info := range pendingDMs {
		// Cap at 4 DMs
		if info.DMCount >= 4 {
			log.Printf("User %s: max DMs reached (%d), giving up until they respond", userID, info.DMCount)
			continue
		}

		// Check backoff
		backoffDuration := getBackoffDuration(info.DMCount)
		timeSinceDM := time.Since(info.SentAt)

		if timeSinceDM < backoffDuration {
			// Not enough time has passed
			log.Printf("User %s: backoff active (DM #%d, wait %.1f more hours)",
				userID, info.DMCount, (backoffDuration - timeSinceDM).Hours())
			continue
		}

		// Backoff expired, eligible!
		log.Printf("User %s: backoff expired (DM #%d), eligible for next DM", userID, info.DMCount)
		eligibleMap[userID] = true
	}

	// B. Process Inactive Users (New tracks)
	// These users have no pending DM (or we would have seen them above) AND are inactive
	for _, userID := range inactiveUsers {
		// If they already have a pending DM, they are handled in loop A
		if _, exists := pendingDMs[userID]; exists {
			continue
		}
		// If not in pending, and inactive > 24h (guaranteed by query), they are eligible
		eligibleMap[userID] = true
	}

	if len(eligibleMap) == 0 {
		log.Println("No eligible users for boredom DM (all in backoff or were active recently)")
		return false
	}

	var eligibleUsers []string
	for userID := range eligibleMap {
		eligibleUsers = append(eligibleUsers, userID)
	}

	if len(eligibleUsers) == 0 {
		log.Println("No eligible users for boredom DM (all in backoff or were active recently)")
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

	prompt := fmt.Sprintf(`
# PERSONA
<role>
You are Marin Kitagawa. You haven't talked to anyone in a while and you feel a bit lonely/bored. You decide to text your friend, %s.
</role>

# CONTEXT
<vibe>
%s
</vibe>
<attempt>
%s
</attempt>
<memory>
Things you remember about them:
%s
</memory>

# TARGET
<task>
Write a spontaneous message to start a conversation. Your tone must match your relationship level (%s) AND the attempt number.
</task>

# REQUIREMENTS
<style>
- Sound natural and spontaneous. 
- Reference what you're doing or ask about their day/hobbies.
- If they've ignored previous messages, be playful, dramatic, or pouting as per the attempt context.
- NO preamble. Don't say "User Profile" or "System".
</style>
<formatting>
- STRICTLY LOWERCASE ONLY. No capital letters.
- NO periods or punctuation at the end of messages.
- EXTREMELY SHORT (1 sentence).
- ABSOLUTELY NO EMOJIS OR ROLEPLAY (*actions*).
</formatting>

Just output the message text.`, userName, relationshipInstruction, attemptContext, profileText, level.Name)

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
