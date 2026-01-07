package bot

import (
	"log"
	"math/rand"
	"time"

	"github.com/bwmarrin/discordgo"
)

// StatusEntry represents a possible Discord status
type StatusEntry struct {
	Text  string
	Emoji string
}

// TimeBasedStatuses maps time periods to possible statuses (randomized)
var TimeBasedStatuses = map[string][]StatusEntry{
	"morning": { // 6am - 11am
		{Text: "just woke up...", Emoji: "😴"},
		{Text: "morning coffee", Emoji: "☕"},
		{Text: "doing my makeup rn", Emoji: "💄"},
		{Text: "picking today's outfit", Emoji: "👗"},
		{Text: "breakfast time~", Emoji: "🍳"},
	},
	"afternoon": { // 11am - 5pm
		{Text: "working on a cosplay", Emoji: "🧵"},
		{Text: "lunch break!", Emoji: "🍱"},
		{Text: "photoshoot today", Emoji: "📸"},
		{Text: "editing pics", Emoji: "🖥️"},
		{Text: "shopping for fabric", Emoji: "🛍️"},
		{Text: "at the studio", Emoji: "🎬"},
	},
	"evening": { // 5pm - 9pm
		{Text: "making dinner", Emoji: "🍳"},
		{Text: "watching anime", Emoji: "📺"},
		{Text: "gaming rn", Emoji: "🎮"},
		{Text: "streaming later maybe", Emoji: "🎬"},
		{Text: "just finished a shoot", Emoji: "📷"},
	},
	"night": { // 9pm - 12am
		{Text: "late night vibes", Emoji: "🌙"},
		{Text: "binge watching stuff", Emoji: "📺"},
		{Text: "sewing... one more stitch", Emoji: "🪡"},
		{Text: "thinking about cosplay ideas", Emoji: "💭"},
		{Text: "winding down", Emoji: "🌸"},
	},
	"latenight": { // 12am - 6am
		{Text: "still up lol", Emoji: "👀"},
		{Text: "late night grind", Emoji: "🌙"},
		{Text: "insomnia hours", Emoji: "🌙"},
		{Text: "3am thoughts", Emoji: "💭"},
		{Text: "night owl mode", Emoji: "🦉"},
	},
}

// getTimeOfDay returns the time period based on hour
func getTimeOfDay(hour int) string {
	switch {
	case hour >= 6 && hour < 11:
		return "morning"
	case hour >= 11 && hour < 17:
		return "afternoon"
	case hour >= 17 && hour < 21:
		return "evening"
	case hour >= 21 && hour < 24:
		return "night"
	default: // 0-6
		return "latenight"
	}
}

// pickRandomStatus selects a random status for the given time period
func pickRandomStatus(period string) StatusEntry {
	statuses := TimeBasedStatuses[period]
	if len(statuses) == 0 {
		return StatusEntry{Text: "vibing", Emoji: "✨"}
	}
	return statuses[rand.Intn(len(statuses))]
}

func (h *Handler) runDailyRoutine() {
	// Update status every 30 minutes for variety
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		if h.session == nil {
			continue
		}

		// Tokyo time for Marin
		loc := time.FixedZone("Asia/Tokyo", 9*60*60)
		now := time.Now().In(loc)
		hour := now.Hour()

		period := getTimeOfDay(hour)
		status := pickRandomStatus(period)

		err := h.session.UpdateStatusComplex(discordgo.UpdateStatusData{
			Activities: []*discordgo.Activity{
				{
					Name:  "Custom Status",
					Type:  discordgo.ActivityTypeCustom,
					State: status.Text,
					Emoji: discordgo.Emoji{Name: status.Emoji},
				},
			},
			Status: "idle",
			AFK:    true,
		})

		if err != nil {
			log.Printf("Error updating status: %v", err)
		} else {
			log.Printf("Status updated: %s %s (period: %s)", status.Emoji, status.Text, period)
		}
	}
}
