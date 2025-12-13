package bot

import (
	"sync"
	"testing"
	"time"

	"marinai/pkg/cerebras"
	"marinai/pkg/memory"

	"github.com/bwmarrin/discordgo"
)

// MockSessionForLoneliness for testing
type MockSessionForLoneliness struct {
	SentMessages []string
}

func (m *MockSessionForLoneliness) ChannelMessageSend(channelID string, content string, options ...discordgo.RequestOption) (*discordgo.Message, error) {
	m.SentMessages = append(m.SentMessages, content)
	return &discordgo.Message{}, nil
}

func (m *MockSessionForLoneliness) ChannelMessageSendReply(channelID string, content string, reference *discordgo.MessageReference, options ...discordgo.RequestOption) (*discordgo.Message, error) {
	return nil, nil
}

func (m *MockSessionForLoneliness) ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error) {
	return nil, nil
}

func (m *MockSessionForLoneliness) ChannelTyping(channelID string, options ...discordgo.RequestOption) (err error) {
	return nil
}

func (m *MockSessionForLoneliness) User(userID string) (*discordgo.User, error) {
	return &discordgo.User{Username: "TestUser"}, nil
}

func (m *MockSessionForLoneliness) Channel(channelID string, options ...discordgo.RequestOption) (*discordgo.Channel, error) {
	return &discordgo.Channel{}, nil
}

func (m *MockSessionForLoneliness) GuildEmojis(guildID string, options ...discordgo.RequestOption) ([]*discordgo.Emoji, error) {
	return nil, nil
}

func (m *MockSessionForLoneliness) UserChannelCreate(recipientID string, options ...discordgo.RequestOption) (*discordgo.Channel, error) {
	return &discordgo.Channel{ID: "dm_channel"}, nil
}

// MockMemoryStoreForLoneliness
type MockMemoryStoreForLoneliness struct {
	Users []string
}

func (m *MockMemoryStoreForLoneliness) Add(userId string, text string, vector []float32) error { return nil }
func (m *MockMemoryStoreForLoneliness) Search(userId string, queryVector []float32, limit int) ([]string, error) {
	return nil, nil
}
func (m *MockMemoryStoreForLoneliness) AddRecentMessage(userId, role, message string) error { return nil }
func (m *MockMemoryStoreForLoneliness) GetRecentMessages(userId string) ([]memory.RecentMessageItem, error) {
	return nil, nil
}
func (m *MockMemoryStoreForLoneliness) ClearRecentMessages(userId string) error { return nil }
func (m *MockMemoryStoreForLoneliness) DeleteUserData(userId string) error      { return nil }
func (m *MockMemoryStoreForLoneliness) GetFacts(userId string) ([]string, error) {
	return []string{"User likes testing"}, nil
}
func (m *MockMemoryStoreForLoneliness) ApplyDelta(userId string, adds []string, removes []string) error {
	return nil
}
func (m *MockMemoryStoreForLoneliness) DeleteFacts(userId string) error { return nil }
func (m *MockMemoryStoreForLoneliness) GetCachedEmojis(guildID string) ([]string, error) {
	return nil, nil
}
func (m *MockMemoryStoreForLoneliness) SetCachedEmojis(guildID string, emojis []string) error { return nil }
func (m *MockMemoryStoreForLoneliness) GetAllKnownUsers() ([]string, error) {
	return m.Users, nil
}

// MockCerebrasClientForLoneliness
type MockCerebrasClientForLoneliness struct{}

func (c *MockCerebrasClientForLoneliness) ChatCompletion(messages []cerebras.Message) (string, error) {
	return "Hey! I'm lonely.", nil
}

func TestCheckForLoneliness(t *testing.T) {
	// Setup
	mockStore := &MockMemoryStoreForLoneliness{Users: []string{"user1"}}
	mockSession := &MockSessionForLoneliness{}
	mockClient := &MockCerebrasClientForLoneliness{}

	h := &Handler{
		cerebrasClient: mockClient,
		memoryStore:    mockStore,
		session:        mockSession,
		lastGlobalInteraction: time.Now().Add(-5 * time.Hour), // 5 hours ago
		lastMessageTimes: make(map[string]time.Time),
		activeUsers: make(map[string]bool),
		processingUsers: make(map[string]bool),
		lastGlobalMu: sync.RWMutex{},
		lastMessageMu: sync.RWMutex{},
	}

	// Trigger loneliness check
	triggered := h.performLonelinessCheck()
	if !triggered {
		t.Errorf("Expected loneliness check to trigger")
	}

	if len(mockSession.SentMessages) != 1 {
		t.Errorf("Expected 1 message sent, got %d", len(mockSession.SentMessages))
	}

	if mockSession.SentMessages[0] != "Hey! I'm lonely." {
		t.Errorf("Unexpected message content: %s", mockSession.SentMessages[0])
	}

	// Test not lonely
	h.lastGlobalInteraction = time.Now()
	triggered = h.performLonelinessCheck()
	if triggered {
		t.Errorf("Expected loneliness check NOT to trigger right after interaction")
	}
}
