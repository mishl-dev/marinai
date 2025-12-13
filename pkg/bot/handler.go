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
	classifierClient       Classifier
	embeddingClient        EmbeddingClient
	visionClient           VisionClient
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

func NewHandler(c CerebrasClient, cl Classifier, e EmbeddingClient, v VisionClient, m memory.Store, messageProcessingDelay float64, factAgingDays int, factSummarizationThreshold int, maintenanceIntervalHours float64) *Handler {
	h := &Handler{
		cerebrasClient:             c,
		classifierClient:           NewCachedClassifier(cl, 1000, "bart-large-mnli"),
		embeddingClient:            e,
		visionClient:               v,
		memoryStore:                m,
		taskAgent:                  NewTaskAgent(c, cl),
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

	return h
}

func (h *Handler) SetSession(s Session) {
	h.session = s
}

func (h *Handler) SetBotID(id string) {
	h.botID = id
}


func (h *Handler) ResetMemory(userId string) error {
	if err := h.memoryStore.ClearRecentMessages(userId); err != nil {
		log.Printf("Error clearing recent messages: %v", err)
	}
	if err := h.memoryStore.DeleteUserData(userId); err != nil {
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
	// Always reply in DMs, dedicated channels, or when mentioned
	shouldReply := isMentioned || isDM || isMarinChannel

	if !shouldReply {
		// Use Classifier to decide if Marin should respond based on her personality
		labels := []string{
			"message directly addressing Marin or Kitagawa",
			"discussion about cosplay, anime, or games",
			"discussion about fashion, makeup, or appearance",
			"discussion about romance or relationships",
			"someone sharing something they love (hobbies, interests)",
			"casual conversation or blank message without mention of marin",
		}

		label, score, err := h.classifierClient.Classify(m.Content, labels)
		if err != nil {
			log.Printf("Error classifying message: %v", err)
		} else {
			log.Printf("Reply Decision: '%s' (score: %.2f)", label, score)
			// Reply if it matches her personality triggers (not casual conversation)
			if label != "casual conversation or blank message without mention of marin" && score > 0.6 {
				shouldReply = true
			}
		}
	}

	if !shouldReply {
		// Check for proactive reactions even if not replying
		go h.evaluateReaction(s, m.ChannelID, m.ID, m.Content)
		return
	}

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

		// Record the refusal in recent memory
		h.wg.Add(1)
		go func() {
			defer h.wg.Done()
			h.addRecentMessage(m.Author.ID, "user", m.Content)
			h.addRecentMessage(m.Author.ID, "assistant", refusal)
		}()
		return
	}

	// Parallelize data gathering to reduce latency
	var (
		recentMsgs   []memory.RecentMessageItem
		matches      []string
		facts        []string
		emojiText    string
		imageContext string
	)

	var gatherWg sync.WaitGroup
	gatherWg.Add(5)

	// 1. Recent Context
	go func() {
		defer gatherWg.Done()
		recentMsgs = h.getRecentMessages(m.Author.ID)
	}()

	// 2. Search Memory (RAG)
	go func() {
		defer gatherWg.Done()
		// Generate Embedding for current message
		emb, err := h.embeddingClient.Embed(m.Content)
		if err != nil {
			log.Printf("Error generating embedding: %v", err)
			return
		}
		if emb != nil {
			var err error
			matches, err = h.memoryStore.Search(m.Author.ID, emb, 5) // Top 5 relevant memories
			if err != nil {
				log.Printf("Error searching memory: %v", err)
			}
		}
	}()

	// 3. Emojis
	go func() {
		defer gatherWg.Done()
		if channel != nil && channel.GuildID != "" {
			emojis, err := s.GuildEmojis(channel.GuildID)
			if err == nil && len(emojis) > 0 {
				relevantNames := h.filterRelevantEmojis(channel.GuildID, emojis)

				if len(relevantNames) > 0 {
					nameToEmoji := make(map[string]*discordgo.Emoji)
					for _, emoji := range emojis {
						nameToEmoji[emoji.Name] = emoji
					}

					var emojiList []string
					for _, name := range relevantNames {
						if emoji, ok := nameToEmoji[name]; ok {
							emojiList = append(emojiList, fmt.Sprintf("<:%s:%s>", emoji.Name, emoji.ID))
						}
					}

					if len(emojiList) > 0 {
						emojiText = "Available custom emojis:\n" + strings.Join(emojiList, ", ")
					}
				}
			}
		}
	}()

	// 4. Fetch User Profile
	go func() {
		defer gatherWg.Done()
		var err error
		facts, err = h.memoryStore.GetFacts(m.Author.ID)
		if err != nil {
			log.Printf("Error fetching user profile: %v", err)
		}
	}()

	// 5. Process Image Attachments (if any)
	go func() {
		defer gatherWg.Done()
		imageContext = h.processImageAttachments(m.Attachments)
	}()

	// Wait for all data
	gatherWg.Wait()

	// Prepare strings from gathered data
	var retrievedMemories string
	if len(matches) > 0 {
		retrievedMemories = "Relevant past memories:\n- " + strings.Join(matches, "\n- ")
	}

	var rollingContext string
	if len(recentMsgs) > 0 {
		var contextLines []string
		for _, msg := range recentMsgs {
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
	if len(facts) > 0 {
		profileText = "- " + strings.Join(facts, "\n- ")
	}

	h.moodMu.RLock()
	mood := h.currentMood
	h.moodMu.RUnlock()

	moodInstruction := GetMoodInstruction(mood)

	systemPrompt := fmt.Sprintf("%s\n\n%s", fmt.Sprintf(SystemPrompt, displayName, profileText), moodInstruction)
	messages := []cerebras.Message{
		{Role: "system", Content: systemPrompt},
	}
	log.Printf("Retrieved memories: %s", retrievedMemories)
	if retrievedMemories != "" {
		messages = append(messages, cerebras.Message{Role: "system", Content: retrievedMemories})
	}
	log.Printf("Rolling context: %s", rollingContext)
	if rollingContext != "" {
		messages = append(messages, cerebras.Message{Role: "system", Content: rollingContext})
	}
	if emojiText != "" {
		messages = append(messages, cerebras.Message{Role: "system", Content: emojiText})
	}

	// Build user message with image context if present
	userMessage := m.Content
	if imageContext != "" {
		if userMessage == "" {
			userMessage = imageContext
		} else {
			userMessage = imageContext + "\n" + userMessage
		}
		log.Printf("Image context added: %s", imageContext)
	}

	messages = append(messages, cerebras.Message{Role: "user", Content: userMessage})

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
	}()
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




