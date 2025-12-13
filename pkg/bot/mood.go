package bot

import (
	"log"
	"time"
)

func (h *Handler) runMoodLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		h.moodMu.Lock()
		rate := h.messageCounter
		h.messageCounter = 0 // Reset counter
		h.moodMu.Unlock()

		// Determine time
		loc := time.FixedZone("Asia/Tokyo", 9*60*60)
		now := time.Now().In(loc)
		hour := now.Hour()

		newMood := "HAPPY"

		// Mood Logic
		if rate > 20 {
			newMood = "HYPER"
		} else if hour < 7 || hour >= 23 {
			newMood = "SLEEPY"
		} else if rate < 1 && hour > 10 && hour < 20 {
			newMood = "BORED"
		}

		h.moodMu.Lock()
		if h.currentMood != newMood {
			h.currentMood = newMood
			// Async save to avoid blocking loop
			go func(mood string) {
				if err := h.memoryStore.SetState("mood", mood); err != nil {
					log.Printf("Error saving mood: %v", err)
				}
			}(newMood)
			log.Printf("Mood changed to: %s (Rate: %d, Hour: %d)", newMood, rate, hour)
		}
		h.moodMu.Unlock()
	}
}
