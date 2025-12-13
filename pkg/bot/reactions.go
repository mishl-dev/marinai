package bot

import (
	"log"
	"math/rand"
)

func (h *Handler) evaluateReaction(s Session, channelID, messageID, content string) {
	// Simple heuristic: if the message is too short, ignore
	if len(content) < 5 {
		return
	}

	labels := []string{
		"happy, celebratory, good news, excitement",
		"funny, hilarious, joke, meme",
		"sad, disappointing, bad news, sympathy",
		"cute, adorable, wholesome",
		"neutral, boring, question, statement",
	}

	label, score, err := h.classifierClient.Classify(content, labels)
	if err != nil {
		return
	}

	// Only react if confidence is high
	if score < 0.85 {
		return
	}

	var emoji string
	switch label {
	case "happy, celebratory, good news, excitement":
		emoji = "🎉"
	case "funny, hilarious, joke, meme":
		emoji = "😂"
	case "sad, disappointing, bad news, sympathy":
		emoji = "🥺"
	case "cute, adorable, wholesome":
		emoji = "✨"
	default:
		return
	}

	// Add random chance (don't react to EVERYTHING) - 50%
	if rand.Float64() < 0.5 {
		if err := s.MessageReactionAdd(channelID, messageID, emoji); err != nil {
			log.Printf("Error adding reaction: %v", err)
		}
	}
}
