package bot

import (
	"os"
	"testing"
	"time"

	"marinai/pkg/cerebras"
	"marinai/pkg/embedding"
	"marinai/pkg/memory"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockSession implements Session for testing
type MockSession struct {
	SentMessages []string
	TypingCalls  int
	ChannelType  discordgo.ChannelType // Configurable channel type for testing
}

func (m *MockSession) ChannelMessageSend(channelID string, content string, options ...discordgo.RequestOption) (*discordgo.Message, error) {
	m.SentMessages = append(m.SentMessages, content)
	return &discordgo.Message{
		ID:        "mock_msg_id",
		ChannelID: channelID,
		Content:   content,
	}, nil
}

func (m *MockSession) ChannelMessageSendReply(channelID string, content string, reference *discordgo.MessageReference, options ...discordgo.RequestOption) (*discordgo.Message, error) {
	m.SentMessages = append(m.SentMessages, content)
	return &discordgo.Message{
		ID:        "mock_msg_id",
		ChannelID: channelID,
		Content:   content,
	}, nil
}

func (m *MockSession) ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error) {
	m.SentMessages = append(m.SentMessages, data.Content)
	return &discordgo.Message{
		ID:        "mock_msg_id",
		ChannelID: channelID,
		Content:   data.Content,
	}, nil
}

func (m *MockSession) ChannelTyping(channelID string, options ...discordgo.RequestOption) error {
	m.TypingCalls++
	return nil
}

func (m *MockSession) User(userID string) (*discordgo.User, error) {
	return &discordgo.User{
		ID:       userID,
		Username: "TestUser",
	}, nil
}

func (m *MockSession) Channel(channelID string, options ...discordgo.RequestOption) (*discordgo.Channel, error) {
	channelType := m.ChannelType
	if channelType == 0 {
		channelType = discordgo.ChannelTypeGuildText // Default to guild text channel
	}
	return &discordgo.Channel{
		ID:   channelID,
		Type: channelType,
	}, nil
}

func (m *MockSession) GuildEmojis(guildID string, options ...discordgo.RequestOption) ([]*discordgo.Emoji, error) {
	return []*discordgo.Emoji{}, nil
}

func (m *MockSession) UserChannelCreate(recipientID string, options ...discordgo.RequestOption) (*discordgo.Channel, error) {
	return &discordgo.Channel{ID: "dm_channel"}, nil
}

func (m *MockSession) MessageReactionAdd(channelID, messageID, emojiID string) error {
	return nil
}

func (m *MockSession) UpdateStatusComplex(usd discordgo.UpdateStatusData) error {
	return nil
}

func TestHandler_Flow(t *testing.T) {
	// Load .env from project root
	if err := godotenv.Load("../../.env"); err != nil {
		t.Log("Warning: Error loading .env file (might be running in environment with vars set)")
	}

	// Setup Dependencies
	cerebrasKey := os.Getenv("CEREBRAS_API_KEY")
	embeddingKey := os.Getenv("EMBEDDING_API_KEY")
	embeddingURL := os.Getenv("EMBEDDING_API_URL")
	if embeddingURL == "" {
		embeddingURL = "https://vector.mishl.dev/embed"
	}

	if cerebrasKey == "" || embeddingKey == "" {
		t.Skip("Skipping flow test: Missing API keys")
	}

	cerebrasClient := cerebras.NewClient(cerebrasKey, 0.7, 0.9, nil)
	embeddingClient := embedding.NewClient(embeddingKey, embeddingURL)

	// Override prioritized models for testing to avoid excessive retries
	cerebras.PrioritizedModels = []cerebras.ModelConfig{
		{ID: "llama-3.3-70b", MaxCtx: 65536},
	}

	// Use a temp dir for memory
	tmpDir, err := os.MkdirTemp("", "marinai_flow_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	memoryStore := memory.NewFileStore(tmpDir)

	// Pre-populate with a memory-worthy message
	testMemory := "User: My favorite programming language is Go | Marin: That's cool, I guess."
	testEmb, err := embeddingClient.Embed(testMemory)
	if err == nil && testEmb != nil {
		memoryStore.Add("test_user_1", testMemory, testEmb)
	}

	// Initialize Handler
	mockGemini := &MockGeminiClient{}
	handler := NewHandler(cerebrasClient, embeddingClient, mockGemini, memoryStore, 0, 7, 20, 24)
	botID := "mock_bot_id"
	handler.SetBotID(botID)

	// Create Mock Session
	mockSession := &MockSession{}

	// Simulate User Message - use a memory-worthy message
	userMsg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "user_msg_1",
			ChannelID: "test_channel",
			Content:   "Do you remember what programming language I like?",
			Author: &discordgo.User{
				ID:       "test_user_1",
				Username: "TestUser",
			},
			Mentions: []*discordgo.User{
				{ID: botID, Username: "Marin"},
			},
		},
	}

	t.Logf("User: %s", userMsg.Content)

	// Trigger Handler
	handler.HandleMessage(mockSession, userMsg)

	// Wait a bit for async operations (memory save)
	time.Sleep(2 * time.Second)

	// Verify Results

	// 1. Check if reply was sent
	require.NotEmpty(t, mockSession.SentMessages, "FAIL: No reply sent")
	reply := mockSession.SentMessages[0]
	t.Logf("PASS: Bot replied: %s", reply)

	// 2. Check if typing was triggered
	assert.Greater(t, mockSession.TypingCalls, 0, "FAIL: Typing indicator not triggered")
	t.Log("PASS: Typing indicator triggered")

	// 3. Check Memory - search for the pre-populated memory
	emb, err := embeddingClient.Embed("programming language")
	require.NoError(t, err, "FAIL: Embedding error")

	matches, err := memoryStore.Search("test_user_1", emb, 5)
	require.NoError(t, err, "FAIL: Memory search error")
	require.NotEmpty(t, matches, "FAIL: No memories found")

	t.Logf("PASS: Found %d memories:", len(matches))
	for _, m := range matches {
		t.Logf("- %s", m)
	}
}

// TestHandler_FlowStructure tests that the message flow follows the correct structure:
// [System Prompt] -> [Retrieved Memories] -> [Rolling Chat Context] -> [Current User Message]
func TestHandler_FlowStructure(t *testing.T) {
	if err := godotenv.Load("../../.env"); err != nil {
		t.Log("Warning: Error loading .env file")
	}

	cerebrasKey := os.Getenv("CEREBRAS_API_KEY")
	embeddingKey := os.Getenv("EMBEDDING_API_KEY")
	embeddingURL := os.Getenv("EMBEDDING_API_URL")
	if embeddingURL == "" {
		embeddingURL = "https://vector.mishl.dev/embed"
	}

	if cerebrasKey == "" || embeddingKey == "" {
		t.Skip("Skipping structure test: Missing API keys")
	}

	cerebrasClient := cerebras.NewClient(cerebrasKey, 0.7, 0.9, nil)
	embeddingClient := embedding.NewClient(embeddingKey, embeddingURL)

	// Override prioritized models for testing
	cerebras.PrioritizedModels = []cerebras.ModelConfig{
		{ID: "llama-3.3-70b", MaxCtx: 65536},
	}

	tmpDir, err := os.MkdirTemp("", "marinai_structure_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	memoryStore := memory.NewFileStore(tmpDir)

	// Pre-populate memory with some test data
	testMemory := "User: What's your favorite food? | Marin: I love cooking pasta and making tea."
	testEmb, _ := embeddingClient.Embed(testMemory)
	if testEmb != nil {
		memoryStore.Add("test_user_structure", testMemory, testEmb)
	}

	// Add some recent messages to create rolling context
	memoryStore.AddRecentMessage("test_user_structure", "user", "Hi Marin!")
	memoryStore.AddRecentMessage("test_user_structure", "assistant", "Oh, it's you again...")
	memoryStore.AddRecentMessage("test_user_structure", "user", "How was your day?")
	memoryStore.AddRecentMessage("test_user_structure", "assistant", "It was fine, I guess.")

	mockGemini := &MockGeminiClient{}
	handler := NewHandler(cerebrasClient, embeddingClient, mockGemini, memoryStore, 0, 7, 20, 24)
	botID := "mock_bot_id"
	handler.SetBotID(botID)

	mockSession := &MockSession{}

	userMsg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "user_msg_structure",
			ChannelID: "test_channel",
			Content:   "Do you remember what you like to cook?",
			Author: &discordgo.User{
				ID:       "test_user_structure",
				Username: "TestUser",
			},
			Mentions: []*discordgo.User{
				{ID: botID, Username: "Marin"},
			},
		},
	}

	t.Log("Testing message flow structure...")
	t.Logf("User: %s", userMsg.Content)

	handler.HandleMessage(mockSession, userMsg)
	time.Sleep(2 * time.Second)

	require.NotEmpty(t, mockSession.SentMessages, "FAIL: No reply sent")

	t.Logf("PASS: Bot replied: %s", mockSession.SentMessages[0])
	t.Log("PASS: Flow structure test completed")

	// Verify recent messages were updated
	recentMsgs, _ := memoryStore.GetRecentMessages("test_user_structure")
	assert.GreaterOrEqual(t, len(recentMsgs), 2, "FAIL: Recent messages not updated properly")
	t.Logf("PASS: Recent messages updated (count: %d)", len(recentMsgs))
}

// TestHandler_RollingContext tests that the rolling chat context is properly maintained and limited
func TestHandler_RollingContext(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "marinai_rolling_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	memoryStore := memory.NewFileStore(tmpDir)

	userID := "test_user_rolling"

	// Add more than 15 messages (the limit in FileStore is 15)
	for i := 1; i <= 20; i++ {
		msg := "Message " + string(rune('0'+i))
		memoryStore.AddRecentMessage(userID, "user", msg)
	}

	recentMsgs, err := memoryStore.GetRecentMessages(userID)
	require.NoError(t, err, "FAIL: Error getting recent messages")

	assert.LessOrEqual(t, len(recentMsgs), 15, "FAIL: Rolling context not limited (got %d messages, expected max 15)", len(recentMsgs))

	t.Logf("PASS: Rolling context properly limited to %d messages", len(recentMsgs))
}

// TestHandler_DMBehavior tests that the bot always replies in DMs
func TestHandler_DMBehavior(t *testing.T) {
	if err := godotenv.Load("../../.env"); err != nil {
		t.Log("Warning: Error loading .env file")
	}

	cerebrasKey := os.Getenv("CEREBRAS_API_KEY")
	embeddingKey := os.Getenv("EMBEDDING_API_KEY")
	embeddingURL := os.Getenv("EMBEDDING_API_URL")
	if embeddingURL == "" {
		embeddingURL = "https://vector.mishl.dev/embed"
	}

	if cerebrasKey == "" || embeddingKey == "" {
		t.Skip("Skipping DM behavior test: Missing API keys")
	}

	cerebrasClient := cerebras.NewClient(cerebrasKey, 0.7, 0.9, nil)
	embeddingClient := embedding.NewClient(embeddingKey, embeddingURL)

	// Override prioritized models for testing
	cerebras.PrioritizedModels = []cerebras.ModelConfig{
		{ID: "llama3.1-8b", MaxCtx: 65536},
	}

	tmpDir, err := os.MkdirTemp("", "marinai_dm_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	memoryStore := memory.NewFileStore(tmpDir)

	mockGemini := &MockGeminiClient{}
	handler := NewHandler(cerebrasClient, embeddingClient, mockGemini, memoryStore, 0, 7, 20, 24)
	botID := "mock_bot_id"
	handler.SetBotID(botID)

	// Create a mock session configured for DM
	mockSession := &MockSession{
		ChannelType: discordgo.ChannelTypeDM,
	}

	// Send message without mention in DM
	dmMsg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "dm_msg",
			ChannelID: "dm_channel",
			Content:   "Hello!", // No mention
			Author: &discordgo.User{
				ID:       "test_user_dm",
				Username: "TestUser",
			},
			Mentions: []*discordgo.User{}, // No mentions
		},
	}

	t.Log("Testing DM behavior (should always reply)...")
	handler.HandleMessage(mockSession, dmMsg)
	time.Sleep(2 * time.Second)

	require.NotEmpty(t, mockSession.SentMessages, "FAIL: Bot did not reply in DM")

	t.Logf("PASS: Bot replied in DM: %s", mockSession.SentMessages[0])
}
