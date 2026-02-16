package bot

import(
	"marinai/pkg/memory"
	"fmt"
	"log"
	
	"strings"

	"github.com/bwmarrin/discordgo"
)

// getRelevantEmojis returns a list of formatted emoji strings (e.g. "<:name:id>")
// It checks the cache first, and if missing, fetches from Discord and uses LLM to filter.
func (h *Handler) getRelevantEmojis(guildID string, s Session) []string {
	// 1. Check cache first
	cached, err := h.memoryStore.GetCachedEmojis(guildID)
	if err == nil && len(cached) > 0 {
		// Verify if cached items are in "name:id" format (new format)
		if strings.Contains(cached[0], ":") {
			formatted := make([]string, 0, len(cached))
			for _, item := range cached {
				parts := strings.Split(item, ":")
				if len(parts) == 2 {
					name := parts[0]
					id := parts[1]
					formatted = append(formatted, fmt.Sprintf("<:%s:%s>", name, id))
				}
			}
			return formatted
		}
		// If old format (just names), we fall through to fetch/update
	}

	// 2. Fetch emojis from Discord (only if cache miss or old format)
	emojis, err := s.GuildEmojis(guildID)
	if err != nil || len(emojis) == 0 {
		return []string{}
	}

	// Build emoji list for filtering (names only)
	emojiNames := make([]string, 0, len(emojis))
	nameToEmoji := make(map[string]*discordgo.Emoji, len(emojis))
	for _, emoji := range emojis {
		emojiNames = append(emojiNames, emoji.Name)
		nameToEmoji[emoji.Name] = emoji
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

	messages := []memory.LLMMessage{
		{Role: "system", Content: "You are an emoji filter for a character AI."},
		{Role: "user", Content: filterPrompt},
	}

	resp, err := h.llmClient.ChatCompletion(messages)
	if err != nil {
		log.Printf("Error filtering emojis: %v", err)
		// If filtering fails, return first 10 emojis as fallback
		if len(emojiNames) > 10 {
			emojiNames = emojiNames[:10]
		}

		// Fallback caching
		cacheItems := make([]string, 0, len(emojiNames))
		result := make([]string, 0, len(emojiNames))
		for _, name := range emojiNames {
			if e, ok := nameToEmoji[name]; ok {
				cacheItems = append(cacheItems, fmt.Sprintf("%s:%s", e.Name, e.ID))
				result = append(result, fmt.Sprintf("<:%s:%s>", e.Name, e.ID))
			}
		}
		_ = h.memoryStore.SetCachedEmojis(guildID, cacheItems)
		return result
	}

	// Parse response
	var filteredNames []string
	if strings.TrimSpace(resp) != "NONE" {
		// Split by comma and clean up
		parts := strings.Split(resp, ",")
		for _, name := range parts {
			cleaned := strings.TrimSpace(name)
			if cleaned != "" {
				filteredNames = append(filteredNames, cleaned)
			}
		}
	}

	// 3. Map names back to IDs and Cache
	cacheItems := make([]string, 0, len(filteredNames))
	result := make([]string, 0, len(filteredNames)) // format: "<:name:id>"

	for _, name := range filteredNames {
		if e, ok := nameToEmoji[name]; ok {
			cacheItems = append(cacheItems, fmt.Sprintf("%s:%s", e.Name, e.ID))
			result = append(result, fmt.Sprintf("<:%s:%s>", e.Name, e.ID))
		}
	}

	// Cache the result (name:id pairs)
	if err := h.memoryStore.SetCachedEmojis(guildID, cacheItems); err != nil {
		log.Printf("Error caching emojis: %v", err)
	}

	return result
}
