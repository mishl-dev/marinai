package bot

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"marinai/pkg/memory"

	"github.com/bwmarrin/discordgo"
)

// MessageContext holds all the context gathered for a user message
type MessageContext struct {
	RecentMessages  []memory.RecentMessageItem
	Matches         []string
	Facts           []string
	EmojiText       string
	ImageContext    string
	ComebackContext string // Set when user returns after boredom DMs
	TimeContext     string // Current time/date/season awareness
}

// gatherMessageContext gathers all necessary context data in parallel
func (h *Handler) gatherMessageContext(s Session, userID, content string, channel *discordgo.Channel, attachments []*discordgo.MessageAttachment) MessageContext {
	var ctx MessageContext
	var wg sync.WaitGroup
	wg.Add(7)

	// 1. Recent Context
	go func() {
		defer wg.Done()
		ctx.RecentMessages = h.getRecentMessages(userID)
	}()

	// 2. Search Memory (RAG)
	go func() {
		defer wg.Done()
		var err error
		ctx.Matches, err = h.fetchMemoryMatches(userID, content)
		if err != nil {
			log.Printf("Error searching memory: %v", err)
		}
	}()

	// 3. Emojis
	go func() {
		defer wg.Done()
		if channel != nil {
			ctx.EmojiText = h.fetchEmojis(channel.GuildID, s)
		}
	}()

	// 4. Fetch User Profile
	go func() {
		defer wg.Done()
		var err error
		ctx.Facts, err = h.fetchUserProfile(userID)
		if err != nil {
			log.Printf("Error fetching user profile: %v", err)
		}
	}()

	// 5. Process Image Attachments
	go func() {
		defer wg.Done()
		ctx.ImageContext = h.processImageAttachments(attachments)
	}()

	// 6. Comeback Context
	go func() {
		defer wg.Done()
		ctx.ComebackContext = h.fetchComebackContext(userID)
	}()

	// 7. Time Context
	go func() {
		defer wg.Done()
		ctx.TimeContext = h.fetchTimeContext()
	}()

	wg.Wait()
	return ctx
}

func (h *Handler) fetchMemoryMatches(userID, content string) ([]string, error) {
	// Generate Embedding for current message
	emb, err := h.embeddingClient.Embed(content)
	if err != nil {
		log.Printf("Error generating embedding: %v", err)
		return nil, err
	}
	if emb != nil {
		return h.memoryStore.Search(userID, emb, 5)
	}
	return nil, nil
}

func (h *Handler) fetchEmojis(guildID string, s Session) string {
	if guildID != "" {
		emojiList := h.getRelevantEmojis(guildID, s)
		if len(emojiList) > 0 {
			return "Available custom emojis:\n" + strings.Join(emojiList, ", ")
		}
	}
	return ""
}

func (h *Handler) fetchUserProfile(userID string) ([]string, error) {
	return h.memoryStore.GetFacts(userID)
}

func (h *Handler) fetchComebackContext(userID string) string {
	_, dmCount, hasPending, err := h.memoryStore.GetPendingDMInfo(userID)
	if err == nil && hasPending && dmCount > 0 {
		switch dmCount {
		case 1:
			return "COMEBACK: This person is replying after you DMed them once. Be happy they responded!"
		case 2:
			return "COMEBACK: This person is replying after you DMed them TWICE. Tease them gently about finally responding."
		case 3:
			return "COMEBACK: This person is replying after you DMed them THREE times! Be dramatic about how long it took them to respond."
		case 4:
			return "COMEBACK: This person is replying after you DMed them FOUR times! You were about to give up on them. Be relieved/happy but also give them a hard time about it."
		}
	}
	return ""
}

func (h *Handler) fetchTimeContext() string {
	// Tokyo time for Marin
	loc := time.FixedZone("Asia/Tokyo", 9*60*60)
	now := time.Now().In(loc)

	// Get time of day
	hour := now.Hour()
	var timeOfDay string
	switch {
	case hour >= 5 && hour < 12:
		timeOfDay = "morning"
	case hour >= 12 && hour < 17:
		timeOfDay = "afternoon"
	case hour >= 17 && hour < 21:
		timeOfDay = "evening"
	default:
		timeOfDay = "night"
	}

	// Get season (Northern Hemisphere / Japan)
	month := now.Month()
	var season string
	switch {
	case month >= 3 && month <= 5:
		season = "spring"
	case month >= 6 && month <= 8:
		season = "summer"
	case month >= 9 && month <= 11:
		season = "autumn"
	default:
		season = "winter"
	}

	// Check for special days/holidays
	var specialDay string
	day := now.Day()
	switch {
	case month == 12 && day >= 20 && day <= 26:
		specialDay = " (Christmas season!)"
	case month == 12 && day == 31:
		specialDay = " (New Year's Eve!)"
	case month == 1 && day == 1:
		specialDay = " (New Year's Day!)"
	case month == 2 && day == 14:
		specialDay = " (Valentine's Day!)"
	case month == 3 && day == 14:
		specialDay = " (White Day!)"
	case month == 10 && day == 31:
		specialDay = " (Halloween!)"
	}

	return fmt.Sprintf("[Current Time: %s, %s %d - %s, %s%s]",
		timeOfDay, now.Month().String(), day, now.Weekday().String(), season, specialDay)
}
