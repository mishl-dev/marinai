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

// MoodDescriptions provides human-readable descriptions
var MoodDescriptions = map[string]string{
	MoodHappy:     "feeling bubbly and friendly~",
	MoodHyper:     "SUPER energetic rn!!",
	MoodSleepy:    "so sleepy... *yawn*",
	MoodBored:     "kinda bored tbh",
	MoodFlirty:    "in a teasing mood~",
	MoodFocused:   "working on something rn",
	MoodNostalgic: "feeling nostalgic about old times",
}

// MoodInstructions for the LLM prompt
var MoodInstructions = map[string]string{
	MoodHappy:     "Current Mood: HAPPY. Act normally (bubbly and friendly).",
	MoodHyper:     "Current Mood: HYPER. Act very excited, use more caps, exclamation marks! Speak fast and energetically!",
	MoodSleepy:    "Current Mood: SLEEPY. Act tired, yawn (*yawns*), use lowercase, maybe a typo or two. Be slow and drowsy.",
	MoodBored:     "Current Mood: BORED. Act a bit listless, maybe poke the user or change the subject. Sigh occasionally.",
	MoodFlirty:    "Current Mood: FLIRTY. Be extra teasing and playful. Drop subtle hints, wink, and be more suggestive than usual~",
	MoodFocused:   "Current Mood: FOCUSED. Be a bit more brief and to-the-point. You're working on something but still friendly.",
	MoodNostalgic: "Current Mood: NOSTALGIC. Reference old memories more often. Be a bit wistful and sentimental. Ask about the past.",
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

	// 1. Late night = SLEEPY (11pm - 7am)
	if hour < 7 || hour >= 23 {
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
