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
			log.Printf("User %s has been inactive for 30 minutes, clearing recent memory", userID)

			// Perform DB operation (potentially slow) outside of lock
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

func (h *Handler) WaitForReady() {
	h.wg.Wait()
}
