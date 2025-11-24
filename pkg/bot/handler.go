package bot

import (
	"encoding/json"
	"fmt"
	"log"
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
	}

	// Start background goroutines
	go h.clearInactiveUsers()
	go h.maintainMemories()

	return h
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

	// Get recent context (Rolling Chat Context)
	recentMsgs := h.getRecentMessages(m.Author.ID)

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
		// Even if not replying, we might want to add to recent context?
		// For now, let's only add if we reply or are involved.
		// Actually, if we don't reply, we should probably NOT add it to context
		// unless we want to track "overheard" conversations.
		// Let's stick to adding only when we reply for now to keep context clean.
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

	// 1. Generate Embedding for current message
	// We use the user's message as the query for retrieval
	emb, err := h.embeddingClient.Embed(m.Content)
	if err != nil {
		log.Printf("Error generating embedding: %v", err)
	}

	// 2. Search Memory (RAG)
	var retrievedMemories string
	if emb != nil {
		matches, err := h.memoryStore.Search(m.Author.ID, emb, 5) // Top 5 relevant memories
		if err != nil {
			log.Printf("Error searching memory: %v", err)
		} else if len(matches) > 0 {
			retrievedMemories = "Relevant past memories:\n- " + strings.Join(matches, "\n- ")
		}
	}

	// 3. Prepare Context (Rolling Window)
	// We already fetched recentMsgs above.
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

	// 4. Prepare Emojis
	var emojiText string
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

	// 5. Construct Prompt
	// [System Prompt]
	// [Retrieved Memories]
	// [Rolling Chat Context]
	// [Current User Message] (handled by appending as user message)

	// Fetch User Profile
	facts, err := h.memoryStore.GetFacts(m.Author.ID)
	if err != nil {
		log.Printf("Error fetching user profile: %v", err)
	}
	profileText := "No known facts yet."
	if len(facts) > 0 {
		profileText = "- " + strings.Join(facts, "\n- ")
	}

	systemPrompt := fmt.Sprintf(SystemPrompt, displayName, profileText)
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

	h.sendSplitMessage(s, m.ChannelID, reply, m.Reference())

	// 7. Async Updates
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()

		// Add to Rolling Context
		h.addRecentMessage(m.Author.ID, "user", m.Content)
		h.addRecentMessage(m.Author.ID, "assistant", reply)

		// Background Memory Extraction
		h.extractMemories(m.Author.ID, displayName, m.Content, reply)
	}()
}

func (h *Handler) extractMemories(userId string, userName string, userMessage string, botReply string) {
	// 1. Fetch existing facts
	existingFacts, err := h.memoryStore.GetFacts(userId)
	if err != nil {
		log.Printf("Error fetching facts for extraction: %v", err)
		return
	}

	// 2. Construct Prompt for Extraction
	currentProfile := "None"
	if len(existingFacts) > 0 {
		currentProfile = "- " + strings.Join(existingFacts, "\n- ")
	}

	extractionPrompt := fmt.Sprintf(`Current Profile:
%s

New Interaction:
%s: "%s"
Marin: "%s"

Task: Analyze the interaction and update the user's profile.
- Extract ONLY permanent, explicit facts about the user (e.g., name, job, location, strong preferences).
- Ignore opinions, temporary states, and trivial details.
- If the user contradicts a previous fact (e.g., moved cities), REMOVE the old fact and ADD the new one.
- Return a JSON object with "add" (list of strings) and "remove" (list of strings).
- If no changes, return empty lists.

Output JSON:`, currentProfile, userName, userMessage, botReply)

	messages := []cerebras.Message{
		{Role: "system", Content: "You are a memory manager. Output ONLY JSON."},
		{Role: "user", Content: extractionPrompt},
	}

	// 3. Call LLM
	resp, err := h.cerebrasClient.ChatCompletion(messages)
	if err != nil {
		log.Printf("Error extracting memories: %v", err)
		return
	}

	// 4. Parse JSON
	// Clean up response (sometimes LLMs add markdown code blocks)
	jsonStr := strings.TrimSpace(resp)
	if strings.HasPrefix(jsonStr, "```json") {
		jsonStr = strings.TrimPrefix(jsonStr, "```json")
		jsonStr = strings.TrimSuffix(jsonStr, "```")
	} else if strings.HasPrefix(jsonStr, "```") {
		jsonStr = strings.TrimPrefix(jsonStr, "```")
		jsonStr = strings.TrimSuffix(jsonStr, "```")
	}
	jsonStr = strings.TrimSpace(jsonStr)

	type Delta struct {
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
	}

	var delta Delta
	if err := json.Unmarshal([]byte(jsonStr), &delta); err != nil {
		log.Printf("Error parsing memory delta JSON: %v. Response: %s", err, resp)
		return
	}

	// 5. Apply Delta
	if len(delta.Add) > 0 || len(delta.Remove) > 0 {
		log.Printf("Applying memory delta for user %s: +%v, -%v", userId, delta.Add, delta.Remove)
		if err := h.memoryStore.ApplyDelta(userId, delta.Add, delta.Remove); err != nil {
			log.Printf("Error applying memory delta: %v", err)
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
	defer h.lastMessageMu.Unlock()
	h.lastMessageTimes[userID] = time.Now()
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

func (h *Handler) WaitForReady() {
	h.wg.Wait()
}
