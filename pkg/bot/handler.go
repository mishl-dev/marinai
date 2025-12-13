package bot

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"marinai/pkg/cerebras"
	"marinai/pkg/memory"

	"github.com/bwmarrin/discordgo"
)

// Session interface abstracts discordgo.Session for testing
type Session interface {
	ChannelMessageSend(channelID string, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelMessageSendReply(channelID string, content string, reference *discordgo.MessageReference, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelTyping(channelID string, options ...discordgo.RequestOption) (err error)
	User(userID string) (*discordgo.User, error)
	Channel(channelID string, options ...discordgo.RequestOption) (*discordgo.Channel, error)
	GuildEmojis(guildID string, options ...discordgo.RequestOption) ([]*discordgo.Emoji, error)
	UserChannelCreate(recipientID string, options ...discordgo.RequestOption) (*discordgo.Channel, error)
	MessageReactionAdd(channelID, messageID, emojiID string) error
	UpdateStatusComplex(usd discordgo.UpdateStatusData) error
}

// DiscordSession adapts discordgo.Session to the Session interface
type DiscordSession struct {
	*discordgo.Session
}

func (s *DiscordSession) User(userID string) (*discordgo.User, error) {
	return s.Session.User(userID)
}

func (s *DiscordSession) Channel(channelID string, options ...discordgo.RequestOption) (*discordgo.Channel, error) {
	return s.Session.Channel(channelID, options...)
}

func (s *DiscordSession) GuildEmojis(guildID string, options ...discordgo.RequestOption) ([]*discordgo.Emoji, error) {
	return s.Session.GuildEmojis(guildID, options...)
}

func (s *DiscordSession) UserChannelCreate(recipientID string, options ...discordgo.RequestOption) (*discordgo.Channel, error) {
	return s.Session.UserChannelCreate(recipientID, options...)
}

func (s *DiscordSession) MessageReactionAdd(channelID, messageID, emojiID string) error {
	return s.Session.MessageReactionAdd(channelID, messageID, emojiID)
}

func (s *DiscordSession) UpdateStatusComplex(usd discordgo.UpdateStatusData) error {
	return s.Session.UpdateStatusComplex(usd)
}

type CerebrasClient interface {
	ChatCompletion(messages []cerebras.Message) (string, error)
}

type EmbeddingClient interface {
	Embed(text string) ([]float32, error)
}

type Classifier interface {
	Classify(text string, labels []string) (string, float64, error)
}

type Handler struct {
	cerebrasClient         CerebrasClient
	classifierClient       Classifier
	embeddingClient        EmbeddingClient
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

	// Last Response Tracking (ChannelID -> Content)
	lastResponses   map[string]string
	lastResponsesMu sync.RWMutex
}

func NewHandler(c CerebrasClient, cl Classifier, e EmbeddingClient, m memory.Store, messageProcessingDelay float64, factAgingDays int, factSummarizationThreshold int, maintenanceIntervalHours float64) *Handler {
	h := &Handler{
		cerebrasClient:             c,
		classifierClient:           NewCachedClassifier(cl, 1000, "bart-large-mnli"),
		embeddingClient:            e,
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
		lastResponses:              make(map[string]string),
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

	return h
}

func (h *Handler) SetSession(s Session) {
	h.session = s
}

func (h *Handler) SetBotID(id string) {
	h.botID = id
}

func (h *Handler) addRecentMessage(userId, role, message string) {
	if err := h.memoryStore.AddRecentMessage(userId, role, message); err != nil {
		log.Printf("Error adding recent message: %v", err)
	}
}

func (h *Handler) getRecentMessages(userId string) []memory.RecentMessageItem {
	messages, err := h.memoryStore.GetRecentMessages(userId)
	if err != nil {
		log.Printf("Error getting recent messages: %v", err)
		return []memory.RecentMessageItem{}
	}
	return messages
}

// filterRelevantEmojis uses LLM to filter emojis that are relevant to Marin's character
// Results are cached in SurrealDB per guild to avoid redundant LLM calls
func (h *Handler) filterRelevantEmojis(guildID string, emojis []*discordgo.Emoji) []string {
	if len(emojis) == 0 {
		return []string{}
	}

	// Check cache first
	cached, err := h.memoryStore.GetCachedEmojis(guildID)
	if err == nil && cached != nil {
		return cached
	}

	// Build emoji list for filtering
	var emojiNames []string
	for _, emoji := range emojis {
		emojiNames = append(emojiNames, emoji.Name)
	}

	// If there are too many emojis, just take the first 50 to avoid token limits
	if len(emojiNames) > 50 {
		emojiNames = emojiNames[:50]
	}

	filterPrompt := fmt.Sprintf(`You are filtering custom Discord emojis for Marin Kitagawa, a bubbly cosplayer and otaku who loves anime, games, and fashion.

Emoji names to filter: %s

Select ONLY emojis that are relevant to Marin's character and interests:
- Cosplay, sewing, fabric, costumes
- Anime, manga, games, magical girls
- Fashion, makeup, nails, gyaru style
- Romance, hearts, love, blushing
- Emotions (excited, happy, crying, embarrassed, etc.)
- Food (ramen, burgers, sweets)

EXCLUDE emojis related to:
- Boring office stuff
- overly serious or dark themes (unless it's cool dark fantasy)
- Random/nonsensical names

Return ONLY the emoji names that should be kept, separated by commas. If none are relevant, return "NONE".`, strings.Join(emojiNames, ", "))

	messages := []cerebras.Message{
		{Role: "system", Content: "You are an emoji filter for a character AI."},
		{Role: "user", Content: filterPrompt},
	}

	resp, err := h.cerebrasClient.ChatCompletion(messages)
	if err != nil {
		log.Printf("Error filtering emojis: %v", err)
		// If filtering fails, return first 10 emojis as fallback
		if len(emojiNames) > 10 {
			return emojiNames[:10]
		}
		return emojiNames
	}

	// Parse response
	var result []string
	if strings.TrimSpace(resp) == "NONE" {
		result = []string{}
	} else {
		// Split by comma and clean up
		filtered := strings.Split(resp, ",")
		for _, name := range filtered {
			cleaned := strings.TrimSpace(name)
			if cleaned != "" {
				result = append(result, cleaned)
			}
		}
	}

	// Cache the result
	if err := h.memoryStore.SetCachedEmojis(guildID, result); err != nil {
		log.Printf("Error caching emojis: %v", err)
	}

	return result
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

	// Decision Logic: Should I reply?
	// Always reply in DMs, otherwise use decision logic
	shouldReply := isMentioned || isDM

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
		recentMsgs []memory.RecentMessageItem
		matches    []string
		facts      []string
		emojiText  string
	)

	var gatherWg sync.WaitGroup
	gatherWg.Add(4)

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

	moodInstruction := ""
	switch mood {
	case "HYPER":
		moodInstruction = "Current Mood: HYPER. Act very excited, use more caps, exclamation marks, and emojis! Speak fast!"
	case "SLEEPY":
		moodInstruction = "Current Mood: SLEEPY. Act tired, yawn ( *yawns* ), use lowercase, maybe a typo or two. Be slow."
	case "BORED":
		moodInstruction = "Current Mood: BORED. Act a bit listless, maybe poke the user or change the subject. Sigh."
	default:
		moodInstruction = "Current Mood: HAPPY. Act normally (bubbly and friendly)."
	}

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

	messages = append(messages, cerebras.Message{Role: "user", Content: m.Content})

	// 6. Generate Reply
	reply, err := h.cerebrasClient.ChatCompletion(messages)
	if err != nil {
		log.Printf("Error getting completion: %v", err)
		h.sendSplitMessage(s, m.ChannelID, "(I'm having a headache... try again later.)", m.Reference())
		return
	}

	// Save last response for /resent command
	h.lastResponsesMu.Lock()
	h.lastResponses[m.ChannelID] = reply
	h.lastResponsesMu.Unlock()

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

func (h *Handler) extractMemories(userId string, userName string, userMessage string, botReply string) {
	// ---------------------------------------------------------
	// 1. Heuristic Filters (Save compute & reduce noise)
	// ---------------------------------------------------------
	cleanMsg := strings.TrimSpace(userMessage)

	// Filter A: Length Check
	// Ignore short, trivial messages like "Hi", "Thanks", "Ok", "Cool".
	// Long-term facts usually require a sentence structure.
	if len(cleanMsg) < 12 {
		return
	}

	// Filter B: Keyword/Subject Check
	// If the user isn't talking about themselves, we usually don't need to memorize it.
	// This skips questions like "What is the weather?" or "Write code for X".
	triggers := []string{
		"i am", "i'm", "my", "mine", // Self-identification
		"i live", "i work", "i study", // Life details
		"i like", "i love", "i hate", "i prefer", // Preferences
		"i have", "i've", // Possession/Experience
		"don't like", "dislike", // Negative preferences
		"name is", "call me", // Naming
		"remember", // Explicit instructions
	}

	hasTrigger := false
	lowerMsg := strings.ToLower(cleanMsg)
	for _, t := range triggers {
		if strings.Contains(lowerMsg, t) {
			hasTrigger = true
			break
		}
	}

	// If no self-reference keywords are found, abort.
	if !hasTrigger {
		return
	}

	log.Printf("Analyzing for memories: %s", cleanMsg)

	// ---------------------------------------------------------
	// 2. Fetch Existing Facts
	// ---------------------------------------------------------
	existingFacts, err := h.memoryStore.GetFacts(userId)
	if err != nil {
		log.Printf("Error fetching facts for extraction: %v", err)
		return
	}

	// ---------------------------------------------------------
	// 3. Construct Prompts (Strict & Conservative)
	// ---------------------------------------------------------
	currentProfile := "None"
	if len(existingFacts) > 0 {
		currentProfile = "- " + strings.Join(existingFacts, "\n- ")
	}

	// Current time for reminder calculation
	now := time.Now().UTC()
	currentTimeStr := now.Format("Monday, 2006-01-02 15:04 UTC")

	// User Prompt: Focuses on specific logical constraints
	extractionPrompt := fmt.Sprintf(`Current Profile:
%s

Current Time: %s

New Interaction:
%s: "%s"
Marin: "%s"

Task: Analyze the interaction and output a JSON object with "add", "remove", and "reminders" lists.

STRICT RULES FOR MEMORY:
1. CONSERVATIVE: Bias towards returning empty lists. Only act if the information is explicitly stated and permanent.
2. PERMANENT ONLY: Save facts like Name, Job, Location, Allergies, Relationships.
3. IGNORE TEMPORARY: Do NOT save states like "I am hungry", "I am tired", "I am driving", or "I am busy".
4. IGNORE TRIVIAL: Do NOT save weak preferences or small talk (e.g., "I like that joke").
5. CONTRADICTIONS: If the user explicitly contradicts an item in 'Current Profile' (e.g., moved to a new city), add the new fact to 'add' and the old fact to 'remove'.

RULES FOR REMINDERS:
- If the user mentions a specific future event (exam, interview, trip) with a time frame, create a reminder.
- "delay_seconds": The number of seconds from the "Current Time" (provided above) until the event happens.
- "text": What the event is (e.g., "Math Exam", "Job Interview").
- If no specific time is given, do not create a reminder.

Output ONLY valid JSON.`, currentProfile, currentTimeStr, userName, userMessage, botReply)

	messages := []cerebras.Message{
		{
			Role: "system",
			// System Prompt: Sets the persona to be strict and lazy (avoids false positives)
			Content: `You are a strict Database Administrator responsible for long-term user records and scheduling.
Your goal is to keep the database clean and concise. 
Reject all trivial information. 
Reject all temporary states (moods, current activities) UNLESS they are future scheduled events.
Only record hard facts that will remain true for months. 
Output ONLY valid JSON.`,
		},
		{
			Role:    "user",
			Content: extractionPrompt,
		},
	}

	// ---------------------------------------------------------
	// 4. Call LLM
	// ---------------------------------------------------------
	resp, err := h.cerebrasClient.ChatCompletion(messages)
	if err != nil {
		log.Printf("Error extracting memories: %v", err)
		return
	}

	// ---------------------------------------------------------
	// 5. Parse JSON
	// ---------------------------------------------------------
	jsonStr := strings.TrimSpace(resp)
	
	// Robust markdown stripping
	if strings.HasPrefix(jsonStr, "```") {
		lines := strings.Split(jsonStr, "\n")
		if len(lines) >= 2 {
			// If it starts with ```json or ```, strip the first and last lines
			// We reconstruct the middle lines
			jsonStr = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	jsonStr = strings.TrimSpace(jsonStr)

	type ReminderRequest struct {
		Text         string `json:"text"`
		DelaySeconds int64  `json:"delay_seconds"`
	}

	type Delta struct {
		Add       []string          `json:"add"`
		Remove    []string          `json:"remove"`
		Reminders []ReminderRequest `json:"reminders"`
	}

	var delta Delta
	if err := json.Unmarshal([]byte(jsonStr), &delta); err != nil {
		log.Printf("[Memory Extraction] Failed to parse JSON: %v. Raw output: %s", err, jsonStr)
		return
	}

	log.Printf("[Memory Extraction] Delta: %+v", delta)

	// ---------------------------------------------------------
	// 6. Apply Delta
	// ---------------------------------------------------------
	if len(delta.Add) > 0 || len(delta.Remove) > 0 {
		log.Printf("Applying memory delta for user %s: +%v, -%v", userId, delta.Add, delta.Remove)
		if err := h.memoryStore.ApplyDelta(userId, delta.Add, delta.Remove); err != nil {
			log.Printf("Error applying memory delta: %v", err)
		}
	}

	// ---------------------------------------------------------
	// 7. Add Reminders
	// ---------------------------------------------------------
	for _, r := range delta.Reminders {
		if r.DelaySeconds > 0 {
			dueAt := time.Now().Unix() + r.DelaySeconds
			log.Printf("Adding reminder for user %s: %s at %d (in %d seconds)", userId, r.Text, dueAt, r.DelaySeconds)
			if err := h.memoryStore.AddReminder(userId, r.Text, dueAt); err != nil {
				log.Printf("Error adding reminder: %v", err)
			}
		}
	}
}

func (h *Handler) sendSplitMessage(s Session, channelID, content string, reference *discordgo.MessageReference) {
	// Replace \n\n with a special separator for multi-part messages
	content = strings.ReplaceAll(content, "\n\n", "---SPLIT---")
	parts := strings.Split(content, "---SPLIT---")

	isFirstPart := true
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		var err error
		if reference == nil {
			// If there's no reference, send a normal message
			_, err = s.ChannelMessageSend(channelID, part)
		} else {
			if isFirstPart {
				// The first part of a reply pings the user by default
				_, err = s.ChannelMessageSendReply(channelID, part, reference)
				isFirstPart = false
			} else {
				// Subsequent parts are sent as replies without pinging the user
				_, err = s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
					Content:   part,
					Reference: reference,
					AllowedMentions: &discordgo.MessageAllowedMentions{
						RepliedUser: false, // This prevents pinging on subsequent parts
					},
				})
			}
		}

		if err != nil {
			log.Printf("Error sending message part: %v", err)
		}

		// Add a short delay between messages for a more natural feel
		// time.Sleep(h.messageProcessingDelay) // REMOVED for performance
	}
}

func (h *Handler) updateLastMessageTime(userID string) {
	h.lastMessageMu.Lock()
	h.lastMessageTimes[userID] = time.Now()
	h.lastMessageMu.Unlock()

	h.lastGlobalMu.Lock()
	h.lastGlobalInteraction = time.Now()
	h.lastGlobalMu.Unlock()
}

func (h *Handler) clearInactiveUsers() {
	// Check for inactive users every minute
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		h.lastMessageMu.Lock()
		for userID, lastTime := range h.lastMessageTimes {
			// If user has been inactive for 30 minutes, clear their recent memory
			if time.Since(lastTime) > 30*time.Minute {
				log.Printf("User %s has been inactive for 30 minutes, clearing recent memory", userID)
				if err := h.memoryStore.ClearRecentMessages(userID); err != nil {
					log.Printf("Error clearing recent messages for inactive user %s: %v", userID, err)
				}
				// Remove from tracking map
				delete(h.lastMessageTimes, userID)
			}
		}
		h.lastMessageMu.Unlock()
	}
}

func (h *Handler) checkForLoneliness() {
	// Check every hour
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		h.performLonelinessCheck()
	}
}

func (h *Handler) checkReminders() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		if h.session == nil {
			continue
		}

		reminders, err := h.memoryStore.GetDueReminders()
		if err != nil {
			log.Printf("Error getting reminders: %v", err)
			continue
		}

		for _, r := range reminders {
			// Process each reminder
			if err := h.processReminder(r); err != nil {
				log.Printf("Error processing reminder %s: %v. Retrying in 1 hour.", r.ID, err)
				// Retry in 1 hour to avoid loop
				r.DueAt += 3600
				if updateErr := h.memoryStore.UpdateReminder(r); updateErr != nil {
					log.Printf("Error updating reminder %s: %v", r.ID, updateErr)
				}
				continue
			}

			// Delete reminder after successful processing
			if err := h.memoryStore.DeleteReminder(r.ID); err != nil {
				log.Printf("Error deleting reminder %s: %v", r.ID, err)
			}
		}
	}
}

func (h *Handler) processReminder(r memory.Reminder) error {
	// 1. Generate contextual message
	user, err := h.session.User(r.UserID)
	userName := "User"
	if err == nil {
		userName = user.Username
		if user.GlobalName != "" {
			userName = user.GlobalName
		}
	}

	prompt := fmt.Sprintf(`You are Marin Kitagawa. You are reminding %s about: "%s".

	Write a short, friendly, natural message.
	- Don't say "I just remembered" or "You have an event".
	- Just act like a friend checking in or reminding them.
	- Be bubbly and supportive.
	- Keep it casual.`, userName, r.Text)

	messages := []cerebras.Message{
		{Role: "system", Content: "You are Marin Kitagawa."},
		{Role: "user", Content: prompt},
	}

	reply, err := h.cerebrasClient.ChatCompletion(messages)
	if err != nil {
		return fmt.Errorf("error generating message: %w", err)
	}

	// 2. Send DM
	ch, err := h.session.UserChannelCreate(r.UserID)
	if err != nil {
		return fmt.Errorf("error creating DM: %w", err)
	}

	_, err = h.session.ChannelMessageSend(ch.ID, reply)
	if err != nil {
		return fmt.Errorf("error sending message: %w", err)
	}

	log.Printf("Sent reminder to %s about '%s'", userName, r.Text)
	return nil
}

func (h *Handler) evaluateReaction(s Session, channelID, messageID, content string) {
	// Simple heuristic: if the message is too short, ignore
	if len(content) < 5 {
		return
	}

	labels := []string{
		"happy, celebratory, good news, excitement",
		"funny, hilarious, joke, meme",
		"sad, disappointing, bad news, sympathy",
		"cute, adorable, wholesome",
		"neutral, boring, question, statement",
	}

	label, score, err := h.classifierClient.Classify(content, labels)
	if err != nil {
		return
	}

	// Only react if confidence is high
	if score < 0.85 {
		return
	}

	var emoji string
	switch label {
	case "happy, celebratory, good news, excitement":
		emoji = "🎉"
	case "funny, hilarious, joke, meme":
		emoji = "😂"
	case "sad, disappointing, bad news, sympathy":
		emoji = "🥺"
	case "cute, adorable, wholesome":
		emoji = "✨"
	default:
		return
	}

	// Add random chance (don't react to EVERYTHING) - 50%
	if rand.Float64() < 0.5 {
		if err := s.MessageReactionAdd(channelID, messageID, emoji); err != nil {
			log.Printf("Error adding reaction: %v", err)
		}
	}
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

		// Base routine based on time
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

		// Mood Overrides (unless sleeping)
		// We don't override sleep unless she is HYPER at night (which fits the character)
		h.moodMu.RLock()
		mood := h.currentMood
		h.moodMu.RUnlock()

		if mood == "HYPER" {
			// Hyper mood overrides everything except maybe deep sleep, but Marin stays up late
			hyperStatuses := []struct {
				text  string
				emoji string
			}{
				{"Singing Karaoke! 🎤", "🎤"},
				{"Dancing around! 💃", "💃"},
				{"Playing loud music! 🎵", "🎵"},
				{"Planning next cosplay!! ✨", "✨"},
			}
			// Pick one based on minute to change it up
			idx := time.Now().Minute() % len(hyperStatuses)
			statusText = hyperStatuses[idx].text
			statusType = discordgo.ActivityTypeCustom
			emoji = hyperStatuses[idx].emoji
		} else if mood == "BORED" && (hour >= 7 && hour < 23) {
			// Bored mood only overrides awake times
			boredStatuses := []struct {
				text  string
				emoji string
			}{
				{"Staring at the ceiling... 😑", "😑"},
				{"Rolling on the floor... 🌀", "🌀"},
				{"Sighing loudly... 💨", "💨"},
				{"Need something to do... 🤔", "🤔"},
			}
			idx := time.Now().Minute() % len(boredStatuses)
			statusText = boredStatuses[idx].text
			statusType = discordgo.ActivityTypeCustom
			emoji = boredStatuses[idx].emoji
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

// performLonelinessCheck is separated for testing
func (h *Handler) performLonelinessCheck() bool {
	// Loneliness threshold: 4 hours of no interaction
	const lonelinessThreshold = 4 * time.Hour

	// 1. Check if lonely
	h.lastGlobalMu.RLock()
	isLonely := time.Since(h.lastGlobalInteraction) > lonelinessThreshold
	h.lastGlobalMu.RUnlock()

	if !isLonely {
		return false
	}

	if h.session == nil {
		log.Println("Session not set, skipping loneliness check")
		return false
	}

	// 3. Get candidates
	// First try to find users who have been active in the last 7 days
	since := time.Now().AddDate(0, 0, -7).Unix()
	users, err := h.memoryStore.GetActiveUsers(since)
	if err != nil {
		log.Printf("Error getting active users: %v", err)
	}

	// If no active users, fall back to all known users
	if len(users) == 0 {
		users, err = h.memoryStore.GetAllKnownUsers()
		if err != nil {
			log.Printf("Error getting known users: %v", err)
			return false
		}
	}

	if len(users) == 0 {
		return false
	}

	// 4. Select a user
	// Just pick a random user
	idx := time.Now().UnixNano() % int64(len(users))
	targetUserID := users[idx]

	// 5. Generate message
	// We need a context for the user to make it personal
	facts, _ := h.memoryStore.GetFacts(targetUserID)
	profileText := "No known facts."
	if len(facts) > 0 {
		profileText = "- " + strings.Join(facts, "\n- ")
	}

	// Get display name (requires fetching user from Discord, or storing it)
	// We'll try to fetch from Discord
	user, err := h.session.User(targetUserID)
	userName := "User"
	if err == nil {
		userName = user.Username
		if user.GlobalName != "" {
			userName = user.GlobalName
		}
	}

	prompt := fmt.Sprintf(`You are Marin Kitagawa. You haven't talked to anyone in a while and you feel a bit lonely/bored.
You decide to text one of your friends, %s.

User Profile:
%s

Write a short, casual, friendly message to them to start a conversation.
- Be your usual bubbly self.
- Maybe reference something from their profile if relevant, or just say you're bored.
- Keep it under 2 sentences.
- Do NOT say "User Profile" or "System". just the message.`, userName, profileText)

	messages := []cerebras.Message{
		{Role: "system", Content: "You are Marin Kitagawa, a cosplayer and otaku."},
		{Role: "user", Content: prompt},
	}

	reply, err := h.cerebrasClient.ChatCompletion(messages)
	if err != nil {
		log.Printf("Error generating lonely message: %v", err)
		return false
	}

	// 6. Send DM
	// Create DM channel
	ch, err := h.session.UserChannelCreate(targetUserID)
	if err != nil {
		log.Printf("Error creating DM channel for %s: %v", targetUserID, err)
		return false
	}

	_, err = h.session.ChannelMessageSend(ch.ID, reply)
	if err != nil {
		log.Printf("Error sending lonely message to %s: %v", targetUserID, err)
		return false
	}

	log.Printf("Sent lonely message to %s: %s", userName, reply)

	// Update global interaction time so we don't spam
	h.lastGlobalMu.Lock()
	h.lastGlobalInteraction = time.Now()
	h.lastGlobalMu.Unlock()

	// Also update that specific user's last message time?
	h.updateLastMessageTime(targetUserID)

	return true
}

func (h *Handler) WaitForReady() {
	h.wg.Wait()
}

func (h *Handler) GetLastResponse(channelID string) string {
	h.lastResponsesMu.RLock()
	defer h.lastResponsesMu.RUnlock()
	return h.lastResponses[channelID]
}
