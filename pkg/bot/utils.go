package bot

import (
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func (h *Handler) sendSplitMessage(s Session, channelID, content string, reference *discordgo.MessageReference) {
	// Replace \n\n with a special separator for multi-part messages
	content = strings.ReplaceAll(content, "\n\n", "---SPLIT---")
	parts := strings.Split(content, "---SPLIT---")

	isFirstPart := true
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		var err error
		if reference == nil {
			// If there's no reference, send a normal message
			_, err = s.ChannelMessageSend(channelID, part)
		} else {
			if isFirstPart {
				// The first part of a reply pings the user by default
				_, err = s.ChannelMessageSendReply(channelID, part, reference)
				isFirstPart = false
			} else {
				// Subsequent parts are sent as replies without pinging the user
				_, err = s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
					Content:   part,
					Reference: reference,
					AllowedMentions: &discordgo.MessageAllowedMentions{
						RepliedUser: false, // This prevents pinging on subsequent parts
					},
				})
			}
		}

		if err != nil {
			log.Printf("Error sending message part: %v", err)
		}

		// Add a short delay between messages for a more natural feel
		// time.Sleep(h.messageProcessingDelay) // REMOVED for performance
	}
}

func (h *Handler) updateLastMessageTime(userID string) {
	h.lastMessageMu.Lock()
	h.lastMessageTimes[userID] = time.Now()
	h.lastMessageMu.Unlock()

	h.lastGlobalMu.Lock()
	h.lastGlobalInteraction = time.Now()
	h.lastGlobalMu.Unlock()
}

func (h *Handler) clearInactiveUsers() {
	// Check for inactive users every minute
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		// 1. Identify inactive users (hold lock briefly)
		var inactiveUsers []string
		h.lastMessageMu.Lock()
		for userID, lastTime := range h.lastMessageTimes {
			if time.Since(lastTime) > 30*time.Minute {
				inactiveUsers = append(inactiveUsers, userID)
			}
		}
		h.lastMessageMu.Unlock()

		// 2. Process them (no lock held during DB calls)
		for _, userID := range inactiveUsers {
			log.Printf("User %s has been inactive for 30 minutes", userID)

			// Check for continuation opportunity BEFORE clearing messages
			// This is where we queue thoughts - when the conversation genuinely ended
			h.checkContinuationOpportunity(userID)

			// Now clear the recent messages
			if err := h.memoryStore.ClearRecentMessages(userID); err != nil {
				log.Printf("Error clearing recent messages for inactive user %s: %v", userID, err)
			}

			// 3. Remove from map (re-acquire lock)
			h.lastMessageMu.Lock()
			// Double check they haven't become active in the meantime
			if lastTime, exists := h.lastMessageTimes[userID]; exists {
				if time.Since(lastTime) > 30*time.Minute {
					delete(h.lastMessageTimes, userID)
				}
			}
			h.lastMessageMu.Unlock()
		}
	}
}

// checkContinuationOpportunity checks if Marin was the last to speak and queues a continuation
func (h *Handler) checkContinuationOpportunity(userID string) {
	// Get recent messages to see who spoke last
	recentMessages, err := h.memoryStore.GetRecentMessages(userID)
	if err != nil || len(recentMessages) == 0 {
		return
	}

	// Check if the last message was from Marin (assistant)
	lastMessage := recentMessages[len(recentMessages)-1]
	if lastMessage.Role != "assistant" {
		// User spoke last, don't queue continuation
		return
	}

	// Find the last user message for context
	var lastUserMsg string
	for i := len(recentMessages) - 1; i >= 0; i-- {
		if recentMessages[i].Role == "user" {
			lastUserMsg = recentMessages[i].Text
			break
		}
	}

	if lastUserMsg == "" {
		return // No user message found
	}

	// Queue the continuation thought (this will check affection/chance internally)
	h.QueueContinuation(userID, lastUserMsg, lastMessage.Text)
}

func (h *Handler) WaitForReady() {
	h.wg.Wait()
}
