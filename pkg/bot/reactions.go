package bot

import (
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

func getAllCategories() []ReactionCategory {
	return ReactionCategories
}

func (h *Handler) evaluateReaction(s Session, channelID, messageID, content string) {
	if len(content) < 5 {
		return
	}

	h.moodMu.RLock()
	mood := h.currentMood
	h.moodMu.RUnlock()

	reactionChance := 0.15

	switch mood {
	case MoodHyper:
		reactionChance = 0.30
	case MoodBored:
		reactionChance = 0.25
	case MoodFlirty:
		reactionChance = 0.20
	}

	if rand.Float64() > reactionChance {
		return
	}

	categories := getAllCategories()
	if len(categories) == 0 {
		return
	}

	category := categories[rand.Intn(len(categories))]
	emoji := pickRandomEmoji(category.Emojis)
	if emoji == "" {
		return
	}

	s.MessageReactionAdd(channelID, messageID, emoji)
}
