package bot

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"marinai/pkg/cerebras"
	"marinai/pkg/memory"

	"github.com/bwmarrin/discordgo"
)

type Handler struct {
	cerebrasClient         CerebrasClient
	embeddingClient        EmbeddingClient
	geminiClient           GeminiClient
	memoryStore            memory.Store
	taskAgent              *TaskAgent
	botID                  string
	wg                     sync.WaitGroup
	lastMessageTimes       map[string]time.Time
	lastMessageMu          sync.RWMutex
	messageProcessingDelay time.Duration
	processingUsers        map[string]bool
	processingMu           sync.Mutex
	// Memory maintenance
	factAgingDays              int
	factSummarizationThreshold int
	maintenanceInterval        time.Duration
	activeUsers                map[string]bool
	activeUsersMu              sync.RWMutex

	// Loneliness logic
	lastGlobalInteraction time.Time
	lastGlobalMu          sync.RWMutex
	session               Session

	// Mood System
	currentMood    string
	messageCounter int
	moodMu         sync.RWMutex
}

type MessageContext struct {
	RecentMessages []memory.RecentMessageItem
	Matches        []string
	Facts          []string
	EmojiText      string
	ImageContext   string
	ComebackContext string // Set when user returns after boredom DMs
	TimeContext     string // Current time/date/season awareness
}

func NewHandler(c CerebrasClient, e EmbeddingClient, g GeminiClient, m memory.Store, messageProcessingDelay float64, factAgingDays int, factSummarizationThreshold int, maintenanceIntervalHours float64) *Handler {
	h := &Handler{
		cerebrasClient:             c,
		embeddingClient:            e,
		geminiClient:               g,
		memoryStore:                m,
		taskAgent:                  NewTaskAgent(c, g),
		lastMessageTimes:           make(map[string]time.Time),
		messageProcessingDelay:     time.Duration(messageProcessingDelay * float64(time.Second)),
		processingUsers:            make(map[string]bool),
		factAgingDays:              factAgingDays,
		factSummarizationThreshold: factSummarizationThreshold,
		maintenanceInterval:        time.Duration(maintenanceIntervalHours * float64(time.Hour)),
		activeUsers:                make(map[string]bool),
		lastGlobalInteraction:      time.Now(), // Initialize with current time so she doesn't feel lonely immediately
		currentMood:                "HAPPY",    // Default mood
	}

	// Validate maintenance interval to prevent panic
	if h.maintenanceInterval <= 0 {
		h.maintenanceInterval = 24 * time.Hour
	}

	// Initialize mood from DB
	go func() {
		if storedMood, err := m.GetState("mood"); err == nil && storedMood != "" {
			h.moodMu.Lock()
			h.currentMood = storedMood
			h.moodMu.Unlock()
		}
	}()

	// Start background goroutines
	go h.clearInactiveUsers()
	go h.maintainMemories()
	go h.checkForLoneliness()
	go h.runDailyRoutine()
	go h.checkReminders()
	go h.runMoodLoop()
	go h.cleanupLoop()
	go h.runAffectionDecayLoop()
	go h.runProactiveThoughtsLoop() // Agency: proactive thoughts to close friends

	return h
}

func (h *Handler) SetSession(s Session) {
	h.session = s
}

func (h *Handler) SetBotID(id string) {
	h.botID = id
}

func (h *Handler) ResetMemory(userID string) error {
	if err := h.memoryStore.ClearRecentMessages(userID); err != nil {
		log.Printf("Error clearing recent messages: %v", err)
	}
	if err := h.memoryStore.DeleteUserData(userID); err != nil {
		log.Printf("Error deleting user data: %v", err)
	}
	return nil
}

func (h *Handler) MessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	h.HandleMessage(&DiscordSession{s}, m)
}

func (h *Handler) HandleMessage(s Session, m *discordgo.MessageCreate) {
	// Ignore own messages
	if m.Author.ID == h.botID {
		return
	}

	// Increment message counter for mood logic
	h.moodMu.Lock()
	h.messageCounter++
	h.moodMu.Unlock()

	// Ignore long messages
	if len(m.Content) > 280 {
		s.ChannelMessageSendReply(m.ChannelID, "...", m.Reference())
		return
	}

	// Update last message time for the user to track activity
	h.updateLastMessageTime(m.Author.ID)
	h.trackActiveUser(m.Author.ID)

	// Check if user is already being processed
	h.processingMu.Lock()
	if h.processingUsers[m.Author.ID] {
		h.processingMu.Unlock()
		return
	}
	h.processingUsers[m.Author.ID] = true
	h.processingMu.Unlock()

	defer func() {
		h.processingMu.Lock()
		delete(h.processingUsers, m.Author.ID)
		h.processingMu.Unlock()
	}()

	// Get channel info to check if it's a DM
	channel, err := s.Channel(m.ChannelID)
	isDM := err == nil && channel.Type == discordgo.ChannelTypeDM

	// Clear any pending boredom DM since the user is now responding (Duolingo-style)
	// This makes them eligible for future boredom DMs
	if isDM {
		if err := h.memoryStore.ClearPendingDM(m.Author.ID); err != nil {
			log.Printf("Error clearing pending DM for %s: %v", m.Author.ID, err)
		}
	}

	// Update last interaction time in DB (for per-user boredom DM tracking)
	if err := h.memoryStore.SetLastInteraction(m.Author.ID, time.Now()); err != nil {
		log.Printf("Error updating last interaction for %s: %v", m.Author.ID, err)
	}

	// Check if mentioned
	isMentioned := false
	for _, user := range m.Mentions {
		if user.ID == h.botID {
			isMentioned = true
			break
		}
	}

	// If the message is a reply, ignore it unless it's a reply to the bot
	if m.MessageReference != nil {
		// To get the message being replied to, you might need to fetch it
		// For now, let's assume if it's a reply and we are not mentioned, we ignore it.
		// A more robust solution would be to check if the replied-to message was from the bot.
		if !isMentioned {
			return
		}
	}

	// Check if channel name contains "marin" - always respond in dedicated channels
	isMarinChannel := false
	if channel != nil && strings.Contains(strings.ToLower(channel.Name), "marin") {
		isMarinChannel = true
	}

	// Decision Logic: Should I reply?
	shouldReply := h.shouldReplyToMessage(m.Content, isMentioned, isDM, isMarinChannel)

	if !shouldReply {
		// Check for proactive reactions even if not replying
		go h.evaluateReaction(s, m.ChannelID, m.ID, m.Content)
		return
	}

	// Get current mood for affection multiplier
	h.moodMu.RLock()
	currentMoodForAffection := h.currentMood
	h.moodMu.RUnlock()

	// Update streak and set first interaction date
	go func() {
		// Update daily streak
		h.memoryStore.UpdateStreak(m.Author.ID)
		// Set first interaction if not already set
		h.memoryStore.SetFirstInteraction(m.Author.ID, time.Now())
	}()

	// Prepare display name
	displayName := m.Author.Username
	if m.Author.GlobalName != "" {
		displayName = m.Author.GlobalName
	}

	s.ChannelTyping(m.ChannelID)

	// Check if this is a long task request that should be refused
	isTask, refusal := h.taskAgent.CheckTask(m.Content)
	if isTask {
		h.sendSplitMessage(s, m.ChannelID, refusal, m.Reference())

		// Record the refusal in recent memory and calculate affection AFTER response
		h.wg.Add(1)
		go func() {
			defer h.wg.Done()
			h.addRecentMessage(m.Author.ID, "user", m.Content)
			h.addRecentMessage(m.Author.ID, "assistant", refusal)

			// Calculate affection based on full interaction (user message + Marin's response)
			h.UpdateAffectionForInteraction(m.Author.ID, m.Content, refusal, isMentioned, isDM, false, currentMoodForAffection)
		}()
		return
	}

	// Parallelize data gathering to reduce latency
	ctx := h.gatherMessageContext(s, m.Author.ID, m.Content, channel, m.Attachments)

	messages := h.buildConversationMessages(displayName, m.Content, ctx)

	// 6. Generate Reply
	reply, err := h.cerebrasClient.ChatCompletion(messages)
	if err != nil {
		log.Printf("Error getting completion: %v", err)
		h.sendSplitMessage(s, m.ChannelID, "(I'm having a headache... try again later.)", m.Reference())
		return
	}

	h.sendSplitMessage(s, m.ChannelID, reply, m.Reference())

	// 7. Async Updates
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()

		// Add to Rolling Context
		h.addRecentMessage(m.Author.ID, "user", m.Content)
		h.addRecentMessage(m.Author.ID, "assistant", reply)

		// Ensure User Profile Exists (for Loneliness/Agency features)
		if err := h.memoryStore.EnsureUser(m.Author.ID); err != nil {
			log.Printf("Error ensuring user profile exists: %v", err)
		}

		// Background Memory Extraction
		h.extractMemories(m.Author.ID, displayName, m.Content, reply)

		// Calculate affection based on full interaction (user message + Marin's response)
		// This happens AFTER the response so we can analyze the actual interaction dynamics
		_, milestoneMsg, randomEventMsg := h.UpdateAffectionForInteraction(m.Author.ID, m.Content, reply, isMentioned, isDM, false, currentMoodForAffection)

		// If there's a milestone or random event, send it as a follow-up DM
		if milestoneMsg != "" || randomEventMsg != "" {
			dmChannel, err := s.UserChannelCreate(m.Author.ID)
			if err == nil && dmChannel != nil {
				if milestoneMsg != "" {
					s.ChannelMessageSend(dmChannel.ID, milestoneMsg)
				}
				if randomEventMsg != "" && milestoneMsg == "" {
					// Only send random event if no milestone (to avoid spam)
					s.ChannelMessageSend(dmChannel.ID, randomEventMsg)
				}
			}
		}
	}()
}

// shouldReplyToMessage decides whether the bot should reply to a message
func (h *Handler) shouldReplyToMessage(content string, isMentioned, isDM, isMarinChannel bool) bool {
	// Always reply in DMs, dedicated channels, or when mentioned
	if isMentioned || isDM || isMarinChannel {
		return true
	}

	// Use Gemini to decide if Marin should respond based on her personality
	labels := []string{
		"message directly addressing Marin or Kitagawa",
		"discussion about cosplay, anime, or games",
		"discussion about fashion, makeup, or appearance",
		"discussion about romance or relationships",
		"someone sharing something they love (hobbies, interests)",
		"casual conversation or blank message without mention of marin",
	}

	label, score, err := h.cerebrasClient.Classify(content, labels)
	if err != nil {
		log.Printf("Error classifying message: %v", err)
		return false
	}

	log.Printf("Reply Decision: '%s' (score: %.2f)", label, score)
	// Reply if it matches her personality triggers (not casual conversation)
	if label != "casual conversation or blank message without mention of marin" && score > 0.6 {
		return true
	}

	return false
}

// buildConversationMessages constructs the message history and system prompt for the LLM
func (h *Handler) buildConversationMessages(displayName, userContent string, ctx MessageContext) []cerebras.Message {
	// Prepare strings from gathered data
	var retrievedMemories string
	if len(ctx.Matches) > 0 {
		retrievedMemories = "Relevant past memories:\n- " + strings.Join(ctx.Matches, "\n- ")
	}

	var rollingContext string
	if len(ctx.RecentMessages) > 0 {
		var contextLines []string
		for _, msg := range ctx.RecentMessages {
			// Reconstruct "Name: Content" format based on role
			content := msg.Text
			switch msg.Role {
			case "assistant":
				// Avoid double prefixing if data is mixed during migration
				if !strings.HasPrefix(content, "Marin: ") {
					content = "Marin: " + content
				}
			case "user":
				// Prepend current display name
				// We don't check for existing prefix because user names vary
				content = fmt.Sprintf("%s: %s", displayName, content)
			}
			contextLines = append(contextLines, content)
		}
		rollingContext = "Recent conversation:\n" + strings.Join(contextLines, "\n")
	}

	profileText := "No known facts yet."
	if len(ctx.Facts) > 0 {
		profileText = "- " + strings.Join(ctx.Facts, "\n- ")
	}

	h.moodMu.RLock()
	mood := h.currentMood
	h.moodMu.RUnlock()

	moodInstruction := GetMoodInstruction(mood)
	statePrompt := h.GetStateForPrompt() // Marin's current internal state

	// Build the full system prompt with time awareness
	systemPrompt := fmt.Sprintf("%s\n\n%s\n\n%s", fmt.Sprintf(SystemPrompt, displayName, profileText), moodInstruction, statePrompt)
	
	// Add time context if available
	if ctx.TimeContext != "" {
		systemPrompt = ctx.TimeContext + "\n\n" + systemPrompt
	}

	messages := []cerebras.Message{
		{Role: "system", Content: systemPrompt},
	}

	// Add comeback context if user is returning after boredom DMs
	if ctx.ComebackContext != "" {
		messages = append(messages, cerebras.Message{Role: "system", Content: ctx.ComebackContext})
		log.Printf("Comeback context: %s", ctx.ComebackContext)
	}

	log.Printf("Retrieved memories: %s", retrievedMemories)
	if retrievedMemories != "" {
		messages = append(messages, cerebras.Message{Role: "system", Content: retrievedMemories})
	}
	log.Printf("Rolling context: %s", rollingContext)
	if rollingContext != "" {
		messages = append(messages, cerebras.Message{Role: "system", Content: rollingContext})
	}
	if ctx.EmojiText != "" {
		messages = append(messages, cerebras.Message{Role: "system", Content: ctx.EmojiText})
	}

	// Build user message with image context if present
	userMessage := userContent
	if ctx.ImageContext != "" {
		if userMessage == "" {
			userMessage = ctx.ImageContext
		} else {
			userMessage = ctx.ImageContext + "\n" + userMessage
		}
		log.Printf("Image context added: %s", ctx.ImageContext)
	}

	messages = append(messages, cerebras.Message{Role: "user", Content: userMessage})

	return messages
}

// gatherMessageContext gathers all necessary context data in parallel
func (h *Handler) gatherMessageContext(s Session, userID, content string, channel *discordgo.Channel, attachments []*discordgo.MessageAttachment) MessageContext {
	var ctx MessageContext
	var wg sync.WaitGroup
	wg.Add(7) // Increased from 5 to 7 for new context types

	// 1. Recent Context
	go func() {
		defer wg.Done()
		ctx.RecentMessages = h.getRecentMessages(userID)
	}()

	// 2. Search Memory (RAG)
	go func() {
		defer wg.Done()
		// Generate Embedding for current message
		emb, err := h.embeddingClient.Embed(content)
		if err != nil {
			log.Printf("Error generating embedding: %v", err)
			return
		}
		if emb != nil {
			var err error
			ctx.Matches, err = h.memoryStore.Search(userID, emb, 5) // Top 5 relevant memories
			if err != nil {
				log.Printf("Error searching memory: %v", err)
			}
		}
	}()

	// 3. Emojis
	go func() {
		defer wg.Done()
		if channel != nil && channel.GuildID != "" {
			emojiList := h.getRelevantEmojis(channel.GuildID, s)
			if len(emojiList) > 0 {
				ctx.EmojiText = "Available custom emojis:\n" + strings.Join(emojiList, ", ")
			}
		}
	}()

	// 4. Fetch User Profile
	go func() {
		defer wg.Done()
		var err error
		ctx.Facts, err = h.memoryStore.GetFacts(userID)
		if err != nil {
			log.Printf("Error fetching user profile: %v", err)
		}
	}()

	// 5. Process Image Attachments (if any)
	go func() {
		defer wg.Done()
		ctx.ImageContext = h.processImageAttachments(attachments)
	}()

	// 6. Comeback Context - check if user is returning after boredom DMs
	go func() {
		defer wg.Done()
		_, dmCount, hasPending, err := h.memoryStore.GetPendingDMInfo(userID)
		if err == nil && hasPending && dmCount > 0 {
			// User is responding after we sent them boredom DMs!
			switch dmCount {
			case 1:
				ctx.ComebackContext = "COMEBACK: This person is replying after you DMed them once. Be happy they responded!"
			case 2:
				ctx.ComebackContext = "COMEBACK: This person is replying after you DMed them TWICE. Tease them gently about finally responding."
			case 3:
				ctx.ComebackContext = "COMEBACK: This person is replying after you DMed them THREE times! Be dramatic about how long it took them to respond."
			case 4:
				ctx.ComebackContext = "COMEBACK: This person is replying after you DMed them FOUR times! You were about to give up on them. Be relieved/happy but also give them a hard time about it."
			}
		}
	}()

	// 7. Time Context - current date/time awareness
	go func() {
		defer wg.Done()
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
		
		ctx.TimeContext = fmt.Sprintf("[Current Time: %s, %s %d - %s, %s%s]",
			timeOfDay, now.Month().String(), day, now.Weekday().String(), season, specialDay)
	}()

	wg.Wait()
	return ctx
}

func (h *Handler) cleanupLoop() {
	// Run cleanup every hour
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		// Delete reminders older than 24 hours
		if err := h.memoryStore.DeleteOldReminders(24 * time.Hour); err != nil {
			log.Printf("Error cleaning up old reminders: %v", err)
		}
	}
}
