package bot

import (
	"fmt"
	"log"
	"marinai/pkg/cerebras"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// filterRelevantEmojis uses LLM to filter emojis that are relevant to Marin's character
// Results are cached in SurrealDB per guild to avoid redundant LLM calls
func (h *Handler) filterRelevantEmojis(guildID string, emojis []*discordgo.Emoji) []string {
	if len(emojis) == 0 {
		return []string{}
	}

	// Check cache first
	cached, err := h.memoryStore.GetCachedEmojis(guildID)
	if err == nil && cached != nil {
		return cached
	}

	// Build emoji list for filtering
	var emojiNames []string
	for _, emoji := range emojis {
		emojiNames = append(emojiNames, emoji.Name)
	}

	// If there are too many emojis, just take the first 50 to avoid token limits
	if len(emojiNames) > 50 {
		emojiNames = emojiNames[:50]
	}

	filterPrompt := fmt.Sprintf(`You are filtering custom Discord emojis for Marin Kitagawa, a bubbly cosplayer and otaku who loves anime, games, and fashion.

Emoji names to filter: %s

Select ONLY emojis that are relevant to Marin's character and interests:
- Cosplay, sewing, fabric, costumes
- Anime, manga, games, magical girls
- Fashion, makeup, nails, gyaru style
- Romance, hearts, love, blushing
- Emotions (excited, happy, crying, embarrassed, etc.)
- Food (ramen, burgers, sweets)

EXCLUDE emojis related to:
- Boring office stuff
- overly serious or dark themes (unless it's cool dark fantasy)
- Random/nonsensical names

Return ONLY the emoji names that should be kept, separated by commas. If none are relevant, return "NONE".`, strings.Join(emojiNames, ", "))

	messages := []cerebras.Message{
		{Role: "system", Content: "You are an emoji filter for a character AI."},
		{Role: "user", Content: filterPrompt},
	}

	resp, err := h.cerebrasClient.ChatCompletion(messages)
	if err != nil {
		log.Printf("Error filtering emojis: %v", err)
		// If filtering fails, return first 10 emojis as fallback
		if len(emojiNames) > 10 {
			return emojiNames[:10]
		}
		return emojiNames
	}

	// Parse response
	var result []string
	if strings.TrimSpace(resp) == "NONE" {
		result = []string{}
	} else {
		// Split by comma and clean up
		filtered := strings.Split(resp, ",")
		for _, name := range filtered {
			cleaned := strings.TrimSpace(name)
			if cleaned != "" {
				result = append(result, cleaned)
			}
		}
	}

	// Cache the result
	if err := h.memoryStore.SetCachedEmojis(guildID, result); err != nil {
		log.Printf("Error caching emojis: %v", err)
	}

	return result
}
