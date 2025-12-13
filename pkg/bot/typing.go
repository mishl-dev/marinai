package bot

import (
	"math/rand"
	"time"
)

// TypingConfig controls typing simulation behavior
type TypingConfig struct {
	// Base characters per second for "typing"
	BaseCharsPerSecond float64
	// Minimum typing duration
	MinDuration time.Duration
	// Maximum typing duration
	MaxDuration time.Duration
	// Random variation factor (0.0 - 1.0)
	Variation float64
}

// DefaultTypingConfig provides sensible defaults for natural-feeling typing
var DefaultTypingConfig = TypingConfig{
	BaseCharsPerSecond: 25.0, // Average human types ~40 WPM = ~200 CPM = ~3.3 CPS, we do 25 for "thinking" time
	MinDuration:        800 * time.Millisecond,
	MaxDuration:        4 * time.Second,
	Variation:          0.3, // ±30% random variation
}

// CalculateTypingDuration determines how long to "type" based on message length and mood
func CalculateTypingDuration(messageLength int, mood string, config TypingConfig) time.Duration {
	// Base duration from message length
	baseDuration := time.Duration(float64(messageLength)/config.BaseCharsPerSecond) * time.Second

	// Mood modifiers
	moodMultiplier := 1.0
	switch mood {
	case MoodHyper:
		moodMultiplier = 0.6 // Types fast when hyper
	case MoodSleepy:
		moodMultiplier = 1.5 // Types slow when sleepy
	case MoodFlirty:
		moodMultiplier = 1.1 // Takes a moment to think of something clever
	case MoodFocused:
		moodMultiplier = 0.8 // Efficient typing
	case MoodBored:
		moodMultiplier = 1.2 // Slow, deliberate
	case MoodNostalgic:
		moodMultiplier = 1.3 // Thoughtful, takes time
	}

	adjustedDuration := time.Duration(float64(baseDuration) * moodMultiplier)

	// Add random variation
	variation := 1.0 + (rand.Float64()*2-1)*config.Variation
	adjustedDuration = time.Duration(float64(adjustedDuration) * variation)

	// Clamp to min/max
	if adjustedDuration < config.MinDuration {
		adjustedDuration = config.MinDuration
	}
	if adjustedDuration > config.MaxDuration {
		adjustedDuration = config.MaxDuration
	}

	return adjustedDuration
}

// SimulateTyping shows typing indicator for a calculated duration
// Returns immediately if duration is 0 or negative
func (h *Handler) SimulateTyping(s Session, channelID string, messageLength int) {
	h.moodMu.RLock()
	mood := h.currentMood
	h.moodMu.RUnlock()

	duration := CalculateTypingDuration(messageLength, mood, DefaultTypingConfig)

	if duration <= 0 {
		return
	}

	// Send typing indicator
	s.ChannelTyping(channelID)

	// Wait for calculated duration
	// Discord typing indicator lasts ~10 seconds, so we may need to refresh for long messages
	refreshInterval := 8 * time.Second
	elapsed := time.Duration(0)

	for elapsed < duration {
		sleepTime := duration - elapsed
		if sleepTime > refreshInterval {
			sleepTime = refreshInterval
		}
		time.Sleep(sleepTime)
		elapsed += sleepTime

		// Refresh typing indicator if we need to keep typing
		if elapsed < duration {
			s.ChannelTyping(channelID)
		}
	}
}
