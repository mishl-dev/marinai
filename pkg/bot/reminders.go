package bot

import (
	"fmt"
	"log"
	
	"marinai/pkg/memory"
	"time"
)

type ReminderRequest struct {
	Text         string `json:"text"`
	DelaySeconds int64  `json:"delay_seconds"`
}

func (h *Handler) checkReminders() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		if h.session == nil {
			continue
		}

		reminders, err := h.memoryStore.GetDueReminders()
		if err != nil {
			log.Printf("Error getting reminders: %v", err)
			continue
		}

		for _, r := range reminders {
			// Process each reminder
			if err := h.processReminder(r); err != nil {
				log.Printf("Error processing reminder %s: %v. Retrying in 1 hour.", r.ID, err)
				// Retry in 1 hour to avoid loop
				r.DueAt += 3600
				if updateErr := h.memoryStore.UpdateReminder(r); updateErr != nil {
					log.Printf("Error updating reminder %s: %v", r.ID, updateErr)
				}
				continue
			}

			// Delete reminder after successful processing
			if err := h.memoryStore.DeleteReminder(r.ID); err != nil {
				log.Printf("Error deleting reminder %s: %v", r.ID, err)
			}
		}
	}
}

func (h *Handler) processReminder(r memory.Reminder) error {
	// 1. Generate contextual message
	user, err := h.session.User(r.UserID)
	userName := "User"
	if err == nil {
		userName = user.Username
		if user.GlobalName != "" {
			userName = user.GlobalName
		}
	}

	prompt := fmt.Sprintf(`You are Marin Kitagawa. You are reminding %s about: "%s".

	Rules:
	- EXTREMELY SHORT messages (1-2 sentences MAX).
	- mostly lowercase, casual typing.
	- ABSOLUTELY NO EMOJIS OR EMOTICONS. Express yourself with words only.
	- NO ROLEPLAY (*actions*). This is text, not a roleplay server.
	- NEVER start a message with "Oh,", "Ah,", or "Hmm,".
	- NEVER use asterisks for actions.
	- Sound natural, like a real text message.
	- Don't say "I just remembered" or "You have an event".
	- Just act like a friend checking in or reminding them.`, userName, r.Text)

	messages := []memory.LLMMessage{
		{Role: "system", Content: "You are Marin Kitagawa."},
		{Role: "user", Content: prompt},
	}

	reply, err := h.llmClient.ChatCompletion(messages)
	if err != nil {
		return fmt.Errorf("error generating message: %w", err)
	}

	// 2. Send DM
	ch, err := h.session.UserChannelCreate(r.UserID)
	if err != nil {
		return fmt.Errorf("error creating DM: %w", err)
	}

	_, err = h.session.ChannelMessageSend(ch.ID, reply)
	if err != nil {
		return fmt.Errorf("error sending message: %w", err)
	}

	log.Printf("Sent reminder to %s about '%s'", userName, r.Text)
	return nil
}
