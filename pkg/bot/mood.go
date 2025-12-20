package bot

import (
	"log"
	"math/rand"
	"time"
)

// Mood constants for expanded mood system
const (
	MoodHappy     = "HAPPY"
	MoodHyper     = "HYPER"
	MoodSleepy    = "SLEEPY"
	MoodBored     = "BORED"
	MoodFlirty    = "FLIRTY"
	MoodFocused   = "FOCUSED"
	MoodNostalgic = "NOSTALGIC"
)

// MoodEmojis maps moods to their representative emojis for display
var MoodEmojis = map[string]string{
	MoodHappy:     "😊",
	MoodHyper:     "⚡",
	MoodSleepy:    "😴",
	MoodBored:     "😐",
	MoodFlirty:    "💋",
	MoodFocused:   "🎯",
	MoodNostalgic: "🌸",
}

// MoodDescriptions provides human-readable descriptions for /mood command
var MoodDescriptions = map[string]string{
	MoodHappy:     "feeling good rn~",
	MoodHyper:     "omg so much energy rn",
	MoodSleepy:    "so tired... need sleep",
	MoodBored:     "kinda bored tbh",
	MoodFlirty:    "feeling a lil spicy~",
	MoodFocused:   "working on smth rn",
	MoodNostalgic: "thinking about old times",
}

// MoodColors maps moods to their representative hex colors for Embeds
var MoodColors = map[string]int{
	MoodHappy:     0xFFD700, // Gold
	MoodHyper:     0xFF4500, // Orange Red
	MoodSleepy:    0xADD8E6, // Light Blue
	MoodBored:     0x808080, // Gray
	MoodFlirty:    0xFF69B4, // Hot Pink
	MoodFocused:   0x4169E1, // Royal Blue
	MoodNostalgic: 0xDDA0DD, // Plum
}

// MoodInstructions for the LLM prompt - must not contradict base system prompt
var MoodInstructions = map[string]string{
	MoodHappy: `Current Mood: HAPPY
You're in a good mood. Keep your usual confident, teasing energy. Still keep messages short and casual.`,

	MoodHyper: `Current Mood: HYPER
You're feeling energetic and excitable. Talk a bit faster, be more enthusiastic. Maybe repeat yourself or trail off mid-thought cuz you're so hyped. Still keep messages short.`,

	MoodSleepy: `Current Mood: SLEEPY
You are barely awake. Write VERY short messages (1-4 words). Misspell words constantly. Do NOT simply say "yawn" repeatedly. Slur your words like you're falling asleep (e.g. "sso sleepy", "cnt type rn"). You are too tired to frame complete thoughts.`,

	MoodBored: `Current Mood: BORED
You're kinda bored rn. More passive, maybe try to change the subject or poke at them for entertainment. Sigh. Still short messages.`,

	MoodFlirty: `Current Mood: FLIRTY
You're feeling extra playful and confident. Be more forward with the teasing and suggestively bold if the vibe allows, but dial it back if they aren't matching you. still keep it short.`,

	MoodFocused: `Current Mood: FOCUSED
You're working on something (cosplay probably). Bit more distracted, shorter replies, but still engage. Might mention what you're working on.`,

	MoodNostalgic: `Current Mood: NOSTALGIC
Feeling a bit sentimental. Might bring up old memories or ask about theirs. A little softer than usual but still you. Keep messages short.`,
}

// GetMoodInstruction returns the LLM instruction for the current mood
func GetMoodInstruction(mood string) string {
	if instruction, ok := MoodInstructions[mood]; ok {
		return instruction
	}
	return MoodInstructions[MoodHappy]
}

func (h *Handler) runMoodLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		h.moodMu.Lock()
		rate := h.messageCounter
		h.messageCounter = 0 // Reset counter
		h.moodMu.Unlock()

		// Determine time (Tokyo timezone for Marin)
		loc := time.FixedZone("Asia/Tokyo", 9*60*60)
		now := time.Now().In(loc)
		hour := now.Hour()
		dayOfWeek := now.Weekday()

		newMood := h.determineMood(rate, hour, dayOfWeek)

		h.moodMu.Lock()
		if h.currentMood != newMood {
			h.currentMood = newMood
			// Async save to avoid blocking loop
			go func(mood string) {
				if err := h.memoryStore.SetState("mood", mood); err != nil {
					log.Printf("Error saving mood: %v", err)
				}
			}(newMood)
			log.Printf("Mood changed to: %s %s (Rate: %d, Hour: %d)", MoodEmojis[newMood], newMood, rate, hour)
		}
		h.moodMu.Unlock()
	}
}

// determineMood calculates the mood based on various factors
func (h *Handler) determineMood(messageRate int, hour int, dayOfWeek time.Weekday) string {
	// Priority order of mood determination

	// 1. Late night nap = SLEEPY (12am - 3am)
	if hour >= 0 && hour < 3 {
		return MoodSleepy
	}

	// 2. Very high activity = HYPER
	if messageRate > 20 {
		return MoodHyper
	}

	// 3. Weekend evenings = more likely FLIRTY
	if (dayOfWeek == time.Friday || dayOfWeek == time.Saturday) && hour >= 20 && hour < 23 {
		if rand.Float64() < 0.4 { // 40% chance
			return MoodFlirty
		}
	}

	// 4. Weekday work hours with moderate activity = FOCUSED
	if dayOfWeek >= time.Monday && dayOfWeek <= time.Friday {
		if hour >= 10 && hour < 18 && messageRate >= 3 && messageRate <= 10 {
			if rand.Float64() < 0.3 { // 30% chance
				return MoodFocused
			}
		}
	}

	// 5. Sunday afternoons = NOSTALGIC
	if dayOfWeek == time.Sunday && hour >= 14 && hour < 19 {
		if rand.Float64() < 0.25 { // 25% chance
			return MoodNostalgic
		}
	}

	// 6. Low activity during daytime = BORED
	if messageRate < 1 && hour > 10 && hour < 20 {
		return MoodBored
	}

	// 7. Evening hours with some activity = FLIRTY
	if hour >= 20 && hour < 23 && messageRate >= 5 {
		if rand.Float64() < 0.3 { // 30% chance
			return MoodFlirty
		}
	}

	// Default: HAPPY
	return MoodHappy
}

// GetCurrentMood returns the current mood with emoji
func (h *Handler) GetCurrentMood() (string, string, string) {
	h.moodMu.RLock()
	defer h.moodMu.RUnlock()

	mood := h.currentMood
	emoji := MoodEmojis[mood]
	desc := MoodDescriptions[mood]

	if emoji == "" {
		emoji = MoodEmojis[MoodHappy]
	}
	if desc == "" {
		desc = MoodDescriptions[MoodHappy]
	}

	return mood, emoji, desc
}
