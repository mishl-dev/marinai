package bot

import (
	"log"
	"marinai/pkg/memory"
	"time"
)

// maintainMemories runs periodically to age facts and trigger summarization
func (h *Handler) maintainMemories() {
	// Wait for the maintenance interval before first run
	ticker := time.NewTicker(h.maintenanceInterval)
	defer ticker.Stop()

	for range ticker.C {
		h.activeUsersMu.RLock()
		users := make([]string, 0, len(h.activeUsers))
		for userID := range h.activeUsers {
			users = append(users, userID)
		}
		h.activeUsersMu.RUnlock()

		if len(users) == 0 {
			log.Println("[Memory Maintenance] No active users to maintain")
			continue
		}

		log.Printf("[Memory Maintenance] Starting maintenance for %d users", len(users))

		for _, userID := range users {
			// Cast memoryStore to *SurrealStore to access maintenance methods
			if surrealStore, ok := h.memoryStore.(*memory.SurrealStore); ok {
				archivedCount, summarized, err := surrealStore.MaintainUserProfile(
					userID,
					h.embeddingClient,
					h.llmClient,
					h.factAgingDays,
					h.factSummarizationThreshold,
				)

				if err != nil {
					log.Printf("[Memory Maintenance] Error maintaining profile for user %s: %v", userID, err)
					continue
				}

				if archivedCount > 0 || summarized {
					log.Printf("[Memory Maintenance] User %s: archived %d facts, summarized: %v", userID, archivedCount, summarized)
				}
			}
		}

		log.Println("[Memory Maintenance] Maintenance cycle complete")
	}
}

// trackActiveUser marks a user as active for memory maintenance
func (h *Handler) trackActiveUser(userID string) {
	h.activeUsersMu.Lock()
	h.activeUsers[userID] = true
	h.activeUsersMu.Unlock()
}
