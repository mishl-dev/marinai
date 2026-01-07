package bot

import (
	"log"
	"math/rand"
)

// ReactionCategory defines a category of sentiment with multiple emoji options
type ReactionCategory struct {
	Labels []string
	Emojis []string
}

// ReactionCategories maps sentiment categories to possible emoji reactions
// Multiple emojis per category add variety
var ReactionCategories = []ReactionCategory{
	{
		Labels: []string{"happy, celebratory, good news, excitement, achievement"},
		Emojis: []string{"🎉", "🥳", "✨", "💫", "🙌"},
	},
	{
		Labels: []string{"funny, hilarious, joke, meme, comedy"},
		Emojis: []string{"😂", "🤣", "💀", "😭"},
	},
	{
		Labels: []string{"sad, disappointing, bad news, sympathy, unfortunate"},
		Emojis: []string{"🥺", "😢", "💔", "🫂"},
	},
	{
		Labels: []string{"cute, adorable, wholesome, sweet"},
		Emojis: []string{"✨", "🥰", "💕", "🌸"},
	},
	{
		Labels: []string{"impressive, cool, amazing, skilled"},
		Emojis: []string{"🔥", "😎", "👏", "💪"},
	},
	{
		Labels: []string{"food, eating, hungry, delicious"},
		Emojis: []string{"🤤", "😋", "🍕", "🍜"},
	},
	{
		Labels: []string{"love, romantic, affection, crush"},
		Emojis: []string{"💕", "💗", "😳", "❤️"},
	},
	{
		Labels: []string{"shocked, surprised, unexpected, wow"},
		Emojis: []string{"😳", "😮", "🤯", "👀"},
	},
	{
		Labels: []string{"agreement, yes, correct, true"},
		Emojis: []string{"👍", "💯", "✅"},
	},
	{
		Labels: []string{"gaming, video games, playing"},
		Emojis: []string{"🎮", "🕹️", "⚔️"},
	},
	{
		Labels: []string{"anime, manga, cosplay, otaku"},
		Emojis: []string{"✨", "🌸", "💫", "⭐"},
	},
}

// buildLabelsForClassification extracts all unique labels for the classifier
func buildLabelsForClassification() []string {
	labels := make([]string, 0, len(ReactionCategories)+1)
	for _, cat := range ReactionCategories {
		labels = append(labels, cat.Labels...)
	}
	// Add neutral category to catch non-reactive messages
	labels = append(labels, "neutral, boring, question, statement, generic")
	return labels
}

// findCategoryForLabel finds which category a label belongs to
func findCategoryForLabel(label string) *ReactionCategory {
	for i := range ReactionCategories {
		for _, l := range ReactionCategories[i].Labels {
			if l == label {
				return &ReactionCategories[i]
			}
		}
	}
	return nil
}

// pickRandomEmoji selects a random emoji from a slice
func pickRandomEmoji(emojis []string) string {
	if len(emojis) == 0 {
		return ""
	}
	return emojis[rand.Intn(len(emojis))]
}

func (h *Handler) evaluateReaction(s Session, channelID, messageID, content string) {
	// Simple heuristic: if the message is too short, ignore
	if len(content) < 5 {
		return
	}

	labels := buildLabelsForClassification()

	label, score, err := h.cerebrasClient.Classify(content, labels)
	if err != nil {
		return
	}

	// Only react if confidence is reasonably high
	if score < 0.75 {
		return
	}

	// Check if it's a neutral/boring message
	if label == "neutral, boring, question, statement, generic" {
		return
	}

	// Find the category and pick an emoji
	category := findCategoryForLabel(label)
	if category == nil {
		return
	}

	emoji := pickRandomEmoji(category.Emojis)
	if emoji == "" {
		return
	}

	// Mood-based reaction probability adjustment
	h.moodMu.RLock()
	mood := h.currentMood
	h.moodMu.RUnlock()

	reactionChance := 0.4 // Default 40% chance

	switch mood {
	case MoodHyper:
		reactionChance = 0.7 // More reactive when hyper
	case MoodBored:
		reactionChance = 0.5 // Slightly more reactive when bored (something to do)
	case MoodFlirty:
		reactionChance = 0.6 // More reactive when flirty
	case MoodFocused:
		reactionChance = 0.25 // Less reactive when focused
	}

	if rand.Float64() < reactionChance {
		if err := s.MessageReactionAdd(channelID, messageID, emoji); err != nil {
			log.Printf("Error adding reaction: %v", err)
		}
	}
}
