package bot

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"marinai/pkg/cerebras"
	"marinai/pkg/memory"

	"github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock Cerebras Client
type mockCerebrasClient struct {
	ChatCompletionFunc func(messages []cerebras.Message) (string, error)
}

func (m *mockCerebrasClient) ChatCompletion(messages []cerebras.Message) (string, error) {
	if m.ChatCompletionFunc != nil {
		return m.ChatCompletionFunc(messages)
	}
	return "Default mock response", nil
}

// Mock Embedding Client
type mockEmbeddingClient struct {
	EmbedFunc func(text string) ([]float32, error)
}

func (m *mockEmbeddingClient) Embed(text string) ([]float32, error) {
	if m.EmbedFunc != nil {
		return m.EmbedFunc(text)
	}
	return []float32{0.1, 0.2, 0.3}, nil
}

// Mock Memory Store
type mockMemoryStore struct {
	AddFunc                     func(userID string, text string, vector []float32) error
	SearchFunc                  func(userID string, queryVector []float32, limit int) ([]string, error)
	AddRecentMessageFunc        func(userID, role, message string) error
	GetRecentMessagesFunc       func(userID string) ([]memory.RecentMessageItem, error)
	ClearRecentMessagesFunc     func(userID string) error
	DeleteUserDataFunc          func(userID string) error
	GetFactsFunc                func(userID string) ([]string, error)
	ApplyDeltaFunc              func(userID string, adds []string, removes []string) error
	DeleteFactsFunc             func(userID string) error
	GetCachedEmojisFunc         func(guildID string) ([]string, error)
	SetCachedEmojisFunc         func(guildID string, emojis []string) error
	GetCachedClassificationFunc func(text string, model string) (string, float64, error)
	SetCachedClassificationFunc func(text string, model string, label string, score float64) error
	GetAffectionFunc            func(userID string) (int, error)
	AddAffectionFunc            func(userID string, amount int) error
	SetAffectionFunc            func(userID string, amount int) error
}

func (m *mockMemoryStore) Add(userID string, text string, vector []float32) error {
	if m.AddFunc != nil {
		return m.AddFunc(userID, text, vector)
	}
	return nil
}

func (m *mockMemoryStore) DeleteFacts(userID string) error {
	if m.DeleteFactsFunc != nil {
		return m.DeleteFactsFunc(userID)
	}
	return nil
}

func (m *mockMemoryStore) Search(userID string, queryVector []float32, limit int) ([]string, error) {
	if m.SearchFunc != nil {
		return m.SearchFunc(userID, queryVector, limit)
	}
	return []string{"retrieved memory 1", "retrieved memory 2"}, nil
}

func (m *mockMemoryStore) AddRecentMessage(userID, role, message string) error {
	if m.AddRecentMessageFunc != nil {
		return m.AddRecentMessageFunc(userID, role, message)
	}
	return nil
}

func (m *mockMemoryStore) GetRecentMessages(userID string) ([]memory.RecentMessageItem, error) {
	if m.GetRecentMessagesFunc != nil {
		return m.GetRecentMessagesFunc(userID)
	}
	return []memory.RecentMessageItem{{Role: "user", Text: "recent message 1"}, {Role: "assistant", Text: "recent message 2"}}, nil
}

func (m *mockMemoryStore) ClearRecentMessages(userID string) error {
	if m.ClearRecentMessagesFunc != nil {
		return m.ClearRecentMessagesFunc(userID)
	}
	return nil
}

func (m *mockMemoryStore) DeleteUserData(userID string) error {
	if m.DeleteUserDataFunc != nil {
		return m.DeleteUserDataFunc(userID)
	}
	return nil
}

func (m *mockMemoryStore) GetFacts(userID string) ([]string, error) {
	if m.GetFactsFunc != nil {
		return m.GetFactsFunc(userID)
	}
	return []string{}, nil
}

func (m *mockMemoryStore) ApplyDelta(userID string, adds []string, removes []string) error {
	if m.ApplyDeltaFunc != nil {
		return m.ApplyDeltaFunc(userID, adds, removes)
	}
	return nil
}

func (m *mockMemoryStore) GetCachedEmojis(guildID string) ([]string, error) {
	if m.GetCachedEmojisFunc != nil {
		return m.GetCachedEmojisFunc(guildID)
	}
	return nil, nil
}

func (m *mockMemoryStore) SetCachedEmojis(guildID string, emojis []string) error {
	if m.SetCachedEmojisFunc != nil {
		return m.SetCachedEmojisFunc(guildID, emojis)
	}
	return nil
}

func (m *mockMemoryStore) GetCachedClassification(text string, model string) (string, float64, error) {
	if m.GetCachedClassificationFunc != nil {
		return m.GetCachedClassificationFunc(text, model)
	}
	return "", 0, nil
}

func (m *mockMemoryStore) SetCachedClassification(text string, model string, label string, score float64) error {
	if m.SetCachedClassificationFunc != nil {
		return m.SetCachedClassificationFunc(text, model, label, score)
	}
	return nil
}

func (m *mockMemoryStore) GetAllKnownUsers() ([]string, error) {
	return []string{}, nil
}

func (m *mockMemoryStore) EnsureUser(userID string) error {
	return nil
}

func (m *mockMemoryStore) AddReminder(userID string, text string, dueAt int64) error {
	return nil
}

func (m *mockMemoryStore) GetDueReminders() ([]memory.Reminder, error) {
	return []memory.Reminder{}, nil
}

func (m *mockMemoryStore) UpdateReminder(reminder memory.Reminder) error {
	return nil
}

func (m *mockMemoryStore) DeleteReminder(id string) error {
	return nil
}

func (m *mockMemoryStore) DeleteOldReminders(ageLimit time.Duration) error {
	return nil
}

func (m *mockMemoryStore) GetState(key string) (string, error) {
	return "", nil
}

func (m *mockMemoryStore) SetState(key, value string) error {
	return nil
}

func (m *mockMemoryStore) HasPendingDM(userID string) (bool, error) {
	return false, nil
}

func (m *mockMemoryStore) GetPendingDMInfo(userID string) (time.Time, int, bool, error) {
	return time.Time{}, 0, false, nil
}

func (m *mockMemoryStore) SetPendingDM(userID string, sentAt time.Time) error {
	return nil
}

func (m *mockMemoryStore) ClearPendingDM(userID string) error {
	return nil
}

func (m *mockMemoryStore) GetLastInteraction(userID string) (time.Time, error) {
	return time.Time{}, nil
}

func (m *mockMemoryStore) SetLastInteraction(userID string, timestamp time.Time) error {
	return nil
}

func (m *mockMemoryStore) GetAffection(userID string) (int, error) {
	if m.GetAffectionFunc != nil {
		return m.GetAffectionFunc(userID)
	}
	return 0, nil
}

func (m *mockMemoryStore) AddAffection(userID string, amount int) error {
	if m.AddAffectionFunc != nil {
		return m.AddAffectionFunc(userID, amount)
	}
	return nil
}

func (m *mockMemoryStore) SetAffection(userID string, amount int) error {
	if m.SetAffectionFunc != nil {
		return m.SetAffectionFunc(userID, amount)
	}
	return nil
}

func (m *mockMemoryStore) GetStreak(userID string) (int, error) {
	return 0, nil
}

func (m *mockMemoryStore) UpdateStreak(userID string) (int, bool) {
	return 0, false
}

func (m *mockMemoryStore) GetFirstInteraction(userID string) (time.Time, error) {
	return time.Time{}, nil
}

func (m *mockMemoryStore) SetFirstInteraction(userID string, timestamp time.Time) error {
	return nil
}

// Mock Discord Session
type mockDiscordSession struct {
	ChannelMessageSendFunc        func(channelID string, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelMessageSendReplyFunc   func(channelID string, content string, reference *discordgo.MessageReference, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelMessageSendComplexFunc func(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelTypingFunc             func(channelID string, options ...discordgo.RequestOption) error
	UserFunc                      func(userID string) (*discordgo.User, error)
	ChannelFunc                   func(channelID string, options ...discordgo.RequestOption) (*discordgo.Channel, error)
	GuildEmojisFunc               func(guildID string, options ...discordgo.RequestOption) ([]*discordgo.Emoji, error)
}

func (m *mockDiscordSession) ChannelMessageSend(channelID string, content string, options ...discordgo.RequestOption) (*discordgo.Message, error) {
	if m.ChannelMessageSendFunc != nil {
		return m.ChannelMessageSendFunc(channelID, content, options...)
	}
	return &discordgo.Message{}, nil
}

func (m *mockDiscordSession) ChannelMessageSendReply(channelID string, content string, reference *discordgo.MessageReference, options ...discordgo.RequestOption) (*discordgo.Message, error) {
	if m.ChannelMessageSendReplyFunc != nil {
		return m.ChannelMessageSendReplyFunc(channelID, content, reference, options...)
	}
	return &discordgo.Message{}, nil
}

func (m *mockDiscordSession) ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error) {
	if m.ChannelMessageSendComplexFunc != nil {
		return m.ChannelMessageSendComplexFunc(channelID, data, options...)
	}
	return &discordgo.Message{}, nil
}

func (m *mockDiscordSession) ChannelTyping(channelID string, options ...discordgo.RequestOption) error {
	if m.ChannelTypingFunc != nil {
		return m.ChannelTypingFunc(channelID, options...)
	}
	return nil
}

func (m *mockDiscordSession) User(userID string) (*discordgo.User, error) {
	if m.UserFunc != nil {
		return m.UserFunc(userID)
	}
	return &discordgo.User{Username: "testuser"}, nil
}

func (m *mockDiscordSession) Channel(channelID string, options ...discordgo.RequestOption) (*discordgo.Channel, error) {
	if m.ChannelFunc != nil {
		return m.ChannelFunc(channelID, options...)
	}
	return &discordgo.Channel{Type: discordgo.ChannelTypeDM}, nil
}

func (m *mockDiscordSession) GuildEmojis(guildID string, options ...discordgo.RequestOption) ([]*discordgo.Emoji, error) {
	if m.GuildEmojisFunc != nil {
		return m.GuildEmojisFunc(guildID, options...)
	}
	return []*discordgo.Emoji{}, nil
}

func (m *mockDiscordSession) UserChannelCreate(recipientID string, options ...discordgo.RequestOption) (*discordgo.Channel, error) {
	return &discordgo.Channel{ID: "dm_channel"}, nil
}

func (m *mockDiscordSession) MessageReactionAdd(channelID, messageID, emojiID string) error {
	return nil
}

func (m *mockDiscordSession) UpdateStatusComplex(usd discordgo.UpdateStatusData) error {
	return nil
}

func TestMessageFlow(t *testing.T) {
	// Setup
	mockCerebras := &mockCerebrasClient{}
	mockEmbedding := &mockEmbeddingClient{}
	mockMemory := &mockMemoryStore{}
	mockSession := &mockDiscordSession{}

	mockGemini := &MockGeminiClient{}
	handler := NewHandler(mockCerebras, mockEmbedding, mockGemini, mockMemory, 0, 7, 20, 24)
	handler.SetBotID("testbot")

	// Spies
	var searchCalled bool
	var getRecentMessagesCalled bool
	var addRecentMessageCalls int
	var applyDeltaCalled bool
	var addedFacts []string
	var finalPrompt string

	mockMemory.SearchFunc = func(userID string, queryVector []float32, limit int) ([]string, error) {
		searchCalled = true
		return []string{"retrieved memory"}, nil
	}

	mockMemory.GetRecentMessagesFunc = func(userID string) ([]memory.RecentMessageItem, error) {
		getRecentMessagesCalled = true
		return []memory.RecentMessageItem{{Role: "user", Text: "rolling context"}}, nil
	}

	mockCerebras.ChatCompletionFunc = func(messages []cerebras.Message) (string, error) {
		var promptBuilder strings.Builder
		isMemoryEvaluation := false
		isSentimentAnalysis := false
		userMessage := ""
		for _, msg := range messages {
			var role string
			switch msg.Role {
			case "system":
				role = "System"
			case "user":
				role = "User"
				userMessage = msg.Content
			default:
				role = "Unknown"
			}
			promptBuilder.WriteString(fmt.Sprintf("[%s]\n%s\n", role, msg.Content))

			if strings.Contains(msg.Content, "Task: Analyze the interaction and output a JSON object with") {
				isMemoryEvaluation = true
			}
			// Detect sentiment analysis calls from analyzeInteractionBehavior
			if strings.Contains(msg.Content, "Based on HOW THE INTERACTION WENT") ||
				strings.Contains(msg.Content, "Analyze this conversation between a user and Marin") {
				isSentimentAnalysis = true
			}
		}
		// Only capture finalPrompt for the main conversation, not for memory or sentiment analysis
		if !isMemoryEvaluation && !isSentimentAnalysis {
			finalPrompt = promptBuilder.String()
		}

		if isMemoryEvaluation {
			if strings.Contains(userMessage, "Please remember") {
				// Return JSON for new extraction logic
				return `{"add": ["User asked to remember this important fact"], "remove": []}`, nil
			}
			if strings.Contains(userMessage, "I love to code") {
				return `{"add": ["User loves to code"], "remove": []}`, nil
			}
			return `{"add": [], "remove": []}`, nil
		}

		// Return neutral sentiment for sentiment analysis
		if isSentimentAnalysis {
			return `{"sentiment": "neutral"}`, nil
		}

		return "This is a standard response.", nil
	}

	mockMemory.AddRecentMessageFunc = func(userID, role, message string) error {
		addRecentMessageCalls++
		return nil
	}

	mockMemory.ApplyDeltaFunc = func(userID string, adds []string, removes []string) error {
		applyDeltaCalled = true
		addedFacts = adds
		return nil
	}

	mockSession.ChannelTypingFunc = func(channelID string, options ...discordgo.RequestOption) error {
		return nil
	}

	mockSession.ChannelMessageSendFunc = func(channelID, content string, options ...discordgo.RequestOption) (*discordgo.Message, error) {
		return &discordgo.Message{}, nil
	}

	mockSession.ChannelMessageSendReplyFunc = func(channelID, content string, reference *discordgo.MessageReference, options ...discordgo.RequestOption) (*discordgo.Message, error) {
		return &discordgo.Message{}, nil
	}

	mockSession.ChannelMessageSendComplexFunc = func(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error) {
		return &discordgo.Message{}, nil
	}

	// Test Case 1: Standard message
	t.Run("Standard flow", func(t *testing.T) {
		// Reset spies
		searchCalled = false
		getRecentMessagesCalled = false
		addRecentMessageCalls = 0
		applyDeltaCalled = false
		finalPrompt = ""

		// Trigger
		handler.HandleMessage(mockSession, &discordgo.MessageCreate{
			Message: &discordgo.Message{
				Author:  &discordgo.User{ID: "user123", Username: "testuser"},
				Content: "Hello, this is a user message.",
				Mentions: []*discordgo.User{
					{ID: "testbot"},
				},
			},
		})
		handler.WaitForReady()

		// Assert
		assert.True(t, searchCalled, "Expected Search to be called on memory store")
		assert.True(t, getRecentMessagesCalled, "Expected GetRecentMessages to be called on memory store")

		assert.Contains(t, strings.ToLower(finalPrompt), "[system]", "Final prompt should contain the System Prompt")
		assert.Contains(t, finalPrompt, "retrieved memory", "Final prompt should contain the Retrieved Memories")
		assert.Contains(t, finalPrompt, "rolling context", "Final prompt should contain the Rolling Chat Context")
		assert.Contains(t, finalPrompt, "Hello, this is a user message.", "Final prompt should contain the Current User Message")

		assert.Equal(t, 2, addRecentMessageCalls, "Expected AddRecentMessage to be called exactly twice")
		assert.False(t, applyDeltaCalled, "Expected ApplyDelta NOT to be called for standard message")
	})

	// Test Case 2: Memorable message
	t.Run("Memorable flow", func(t *testing.T) {
		// Reset spies
		applyDeltaCalled = false
		addRecentMessageCalls = 0
		searchCalled = false
		getRecentMessagesCalled = false
		finalPrompt = ""

		// Trigger
		handler.HandleMessage(mockSession, &discordgo.MessageCreate{
			Message: &discordgo.Message{
				Author:  &discordgo.User{ID: "user123", Username: "testuser"},
				Content: "Please remember this important fact.",
				Mentions: []*discordgo.User{
					{ID: "testbot"},
				},
			},
		})
		handler.WaitForReady()

		// Assert
		assert.True(t, applyDeltaCalled, "Expected ApplyDelta to be called")
	})

	// Test Case 3: Memory with quotes
	t.Run("Memory with quotes", func(t *testing.T) {
		// Reset spies
		applyDeltaCalled = false
		addedFacts = nil

		// Trigger
		handler.HandleMessage(mockSession, &discordgo.MessageCreate{
			Message: &discordgo.Message{
				Author:  &discordgo.User{ID: "user123", Username: "testuser"},
				Content: "I love to code.", // User message that will trigger memory
				Mentions: []*discordgo.User{
					{ID: "testbot"},
				},
			},
		})
		handler.WaitForReady()

		// Assert
		require.True(t, applyDeltaCalled, "Expected ApplyDelta to be called")

		expectedMemory := "User loves to code"
		require.NotEmpty(t, addedFacts, "Expected addedFacts not to be empty")
		assert.Equal(t, expectedMemory, addedFacts[0], "Memory mismatch")
	})
}
