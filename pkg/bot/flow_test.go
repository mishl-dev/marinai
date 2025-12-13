package bot

import (
	"fmt"
	"strings"
	"testing"

	"marinai/pkg/cerebras"
	"marinai/pkg/memory"

	"github.com/bwmarrin/discordgo"
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
	AddFunc                     func(userId string, text string, vector []float32) error
	SearchFunc                  func(userId string, queryVector []float32, limit int) ([]string, error)
	AddRecentMessageFunc        func(userId, role, message string) error
	GetRecentMessagesFunc       func(userId string) ([]memory.RecentMessageItem, error)
	ClearRecentMessagesFunc     func(userId string) error
	DeleteUserDataFunc          func(userId string) error
	GetFactsFunc                func(userId string) ([]string, error)
	ApplyDeltaFunc              func(userId string, adds []string, removes []string) error
	DeleteFactsFunc             func(userId string) error
	GetCachedEmojisFunc         func(guildID string) ([]string, error)
	SetCachedEmojisFunc         func(guildID string, emojis []string) error
	GetCachedClassificationFunc func(text string, model string) (string, float64, error)
	SetCachedClassificationFunc func(text string, model string, label string, score float64) error
}

func (m *mockMemoryStore) Add(userId string, text string, vector []float32) error {
	if m.AddFunc != nil {
		return m.AddFunc(userId, text, vector)
	}
	return nil
}

func (m *mockMemoryStore) DeleteFacts(userId string) error {
	if m.DeleteFactsFunc != nil {
		return m.DeleteFactsFunc(userId)
	}
	return nil
}

func (m *mockMemoryStore) Search(userId string, queryVector []float32, limit int) ([]string, error) {
	if m.SearchFunc != nil {
		return m.SearchFunc(userId, queryVector, limit)
	}
	return []string{"retrieved memory 1", "retrieved memory 2"}, nil
}

func (m *mockMemoryStore) AddRecentMessage(userId, role, message string) error {
	if m.AddRecentMessageFunc != nil {
		return m.AddRecentMessageFunc(userId, role, message)
	}
	return nil
}

func (m *mockMemoryStore) GetRecentMessages(userId string) ([]memory.RecentMessageItem, error) {
	if m.GetRecentMessagesFunc != nil {
		return m.GetRecentMessagesFunc(userId)
	}
	return []memory.RecentMessageItem{{Role: "user", Text: "recent message 1"}, {Role: "assistant", Text: "recent message 2"}}, nil
}

func (m *mockMemoryStore) ClearRecentMessages(userId string) error {
	if m.ClearRecentMessagesFunc != nil {
		return m.ClearRecentMessagesFunc(userId)
	}
	return nil
}

func (m *mockMemoryStore) DeleteUserData(userId string) error {
	if m.DeleteUserDataFunc != nil {
		return m.DeleteUserDataFunc(userId)
	}
	return nil
}

func (m *mockMemoryStore) GetFacts(userId string) ([]string, error) {
	if m.GetFactsFunc != nil {
		return m.GetFactsFunc(userId)
	}
	return []string{}, nil
}

func (m *mockMemoryStore) ApplyDelta(userId string, adds []string, removes []string) error {
	if m.ApplyDeltaFunc != nil {
		return m.ApplyDeltaFunc(userId, adds, removes)
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

func TestMessageFlow(t *testing.T) {
	// Setup
	mockCerebras := &mockCerebrasClient{}
	mockEmbedding := &mockEmbeddingClient{}
	mockMemory := &mockMemoryStore{}
	mockSession := &mockDiscordSession{}

	handler := NewHandler(mockCerebras, &MockClassifier{}, mockEmbedding, mockMemory, 0, 7, 20, 24)
	handler.SetBotID("testbot")

	// Spies
	var searchCalled bool
	var getRecentMessagesCalled bool
	var addRecentMessageCalls int
	var applyDeltaCalled bool
	var addedFacts []string
	var finalPrompt string

	mockMemory.SearchFunc = func(userId string, queryVector []float32, limit int) ([]string, error) {
		searchCalled = true
		return []string{"retrieved memory"}, nil
	}

	mockMemory.GetRecentMessagesFunc = func(userId string) ([]memory.RecentMessageItem, error) {
		getRecentMessagesCalled = true
		return []memory.RecentMessageItem{{Role: "user", Text: "rolling context"}}, nil
	}

	mockCerebras.ChatCompletionFunc = func(messages []cerebras.Message) (string, error) {
		var promptBuilder strings.Builder
		isMemoryEvaluation := false
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
		}
		if !isMemoryEvaluation {
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

		return "This is a standard response.", nil
	}

	mockMemory.AddRecentMessageFunc = func(userId, role, message string) error {
		addRecentMessageCalls++
		return nil
	}

	mockMemory.ApplyDeltaFunc = func(userId string, adds []string, removes []string) error {
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
		if !searchCalled {
			t.Error("Expected Search to be called on memory store, but it wasn't")
		}
		if !getRecentMessagesCalled {
			t.Error("Expected GetRecentMessages to be called on memory store, but it wasn't")
		}
		if !strings.Contains(strings.ToLower(finalPrompt), "[system]") {
			t.Error("Final prompt does not contain the System Prompt")
		}
		if !strings.Contains(finalPrompt, "retrieved memory") {
			t.Error("Final prompt does not contain the Retrieved Memories")
		}
		if !strings.Contains(finalPrompt, "rolling context") {
			t.Error("Final prompt does not contain the Rolling Chat Context")
		}
		if !strings.Contains(finalPrompt, "Hello, this is a user message.") {
			t.Error("Final prompt does not contain the Current User Message")
		}
		if addRecentMessageCalls != 2 {
			t.Errorf("Expected AddRecentMessage to be called twice, but it was called %d times", addRecentMessageCalls)
		}
		if applyDeltaCalled {
			t.Error("Expected ApplyDelta not to be called for standard message, but it was")
		}
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
		if !applyDeltaCalled {
			t.Error("Expected ApplyDelta to be called, but it wasn't")
		}
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
		if !applyDeltaCalled {
			t.Error("Expected ApplyDelta to be called, but it wasn't")
		}
		expectedMemory := "User loves to code"
		if len(addedFacts) == 0 || addedFacts[0] != expectedMemory {
			t.Errorf("Expected memory to be '%s', but got '%v'", expectedMemory, addedFacts)
		}
	})
}
