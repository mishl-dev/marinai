package bot

import (
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

func (h *Handler) runDailyRoutine() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		if h.session == nil {
			continue
		}

		// Calculate status based on time (UTC+9 for Japan time, as Marin is Japanese)
		// Use FixedZone to avoid dependency on tzdata and panics if location is not found
		loc := time.FixedZone("Asia/Tokyo", 9*60*60)
		now := time.Now().In(loc)
		hour := now.Hour()

		var statusText string
		var statusType discordgo.ActivityType
		var emoji string

		switch {
		case hour >= 7 && hour < 8:
			statusText = "Running late for school! 🍞"
			statusType = discordgo.ActivityTypeCustom
			emoji = "🍞"
		case hour >= 8 && hour < 15:
			statusText = "At school... sleepy... 🏫"
			statusType = discordgo.ActivityTypeCustom
			emoji = "🏫"
		case hour >= 15 && hour < 18:
			statusText = "Shopping for fabric 🧵"
			statusType = discordgo.ActivityTypeCustom
			emoji = "🧵"
		case hour >= 18 && hour < 20:
			statusText = "Watching anime! 📺"
			statusType = discordgo.ActivityTypeWatching
			emoji = "📺"
		case hour >= 20 && hour < 23:
			statusText = "Sewing... just one more stitch... 🪡"
			statusType = discordgo.ActivityTypeCustom
			emoji = "🪡"
		default: // 23 - 07
			statusText = "Sleeping... 😴"
			statusType = discordgo.ActivityTypeCustom
			emoji = "😴"
		}

		err := h.session.UpdateStatusComplex(discordgo.UpdateStatusData{
			Activities: []*discordgo.Activity{
				{
					Name:  "Daily Routine",
					Type:  statusType,
					State: statusText,
					Emoji: discordgo.Emoji{Name: emoji},
				},
			},
			Status: "online",
			AFK:    false,
		})
		if err != nil {
			log.Printf("Error updating status: %v", err)
		}
	}
}
