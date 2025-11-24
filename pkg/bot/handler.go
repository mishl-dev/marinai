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

	// User Prompt: Focuses on specific logical constraints
	extractionPrompt := fmt.Sprintf(`Current Profile:
%s

New Interaction:
%s: "%s"
Marin: "%s"

Task: Analyze the interaction and output a JSON object with "add" and "remove" lists.

STRICT RULES FOR MEMORY:
1. CONSERVATIVE: Bias towards returning empty lists. Only act if the information is explicitly stated and permanent.
2. PERMANENT ONLY: Save facts like Name, Job, Location, Allergies, Relationships.
3. IGNORE TEMPORARY: Do NOT save states like "I am hungry", "I am tired", "I am driving", or "I am busy".
4. IGNORE TRIVIAL: Do NOT save weak preferences or small talk (e.g., "I like that joke").
5. CONTRADICTIONS: If the user explicitly contradicts an item in 'Current Profile' (e.g., moved to a new city), add the new fact to 'add' and the old fact to 'remove'.

Output ONLY valid JSON.`, currentProfile, userName, userMessage, botReply)

	messages := []cerebras.Message{
		{
			Role: "system",
			// System Prompt: Sets the persona to be strict and lazy (avoids false positives)
			Content: `You are a strict Database Administrator responsible for long-term user records. 
Your goal is to keep the database clean and concise. 
Reject all trivial information. 
Reject all temporary states (moods, current activities). 
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

	type Delta struct {
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
	}

	var delta Delta
	if err := json.Unmarshal([]byte(jsonStr), &delta); err != nil {
		// If unmarshal fails, it usually means the LLM replied with text saying "No changes".
		// We can safely ignore this.
		return
	}

	// ---------------------------------------------------------
	// 6. Apply Delta
	// ---------------------------------------------------------
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
