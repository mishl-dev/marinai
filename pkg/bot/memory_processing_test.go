package bot

import (
	"testing"
	"time"

	"marinai/pkg/cerebras"
	"marinai/pkg/memory"

	"github.com/stretchr/testify/mock"
)

// Mock objects for MemoryProcessingTest

type MockCerebrasClientForMemory struct {
	mock.Mock
}

func (m *MockCerebrasClientForMemory) ChatCompletion(messages []cerebras.Message) (string, error) {
	args := m.Called(messages)
	return args.String(0), args.Error(1)
}

func (m *MockCerebrasClientForMemory) Classify(text string, labels []string) (string, float64, error) {
	args := m.Called(text, labels)
	return args.String(0), args.Get(1).(float64), args.Error(2)
}

type MockMemoryStoreForMemory struct {
	mock.Mock
}

func (m *MockMemoryStoreForMemory) Add(userID string, text string, vector []float32) error {
	args := m.Called(userID, text, vector)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) Search(userID string, queryVector []float32, limit int) ([]string, error) {
	args := m.Called(userID, queryVector, limit)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockMemoryStoreForMemory) AddRecentMessage(userID, role, message string) error {
	args := m.Called(userID, role, message)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) GetRecentMessages(userID string) ([]memory.RecentMessageItem, error) {
	args := m.Called(userID)
	return args.Get(0).([]memory.RecentMessageItem), args.Error(1)
}

func (m *MockMemoryStoreForMemory) ClearRecentMessages(userID string) error {
	args := m.Called(userID)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) DeleteUserData(userID string) error {
	args := m.Called(userID)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) GetFacts(userID string) ([]string, error) {
	args := m.Called(userID)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockMemoryStoreForMemory) ApplyDelta(userID string, adds []string, removes []string) error {
	args := m.Called(userID, adds, removes)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) DeleteFacts(userID string) error {
	args := m.Called(userID)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) GetCachedEmojis(guildID string) ([]string, error) {
	args := m.Called(guildID)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockMemoryStoreForMemory) SetCachedEmojis(guildID string, emojis []string) error {
	args := m.Called(guildID, emojis)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) GetCachedClassification(text string, model string) (string, float64, error) {
	args := m.Called(text, model)
	return args.String(0), args.Get(1).(float64), args.Error(2)
}

func (m *MockMemoryStoreForMemory) SetCachedClassification(text string, model string, label string, score float64) error {
	args := m.Called(text, model, label, score)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) GetAllKnownUsers() ([]string, error) {
	args := m.Called()
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockMemoryStoreForMemory) EnsureUser(userID string) error {
	args := m.Called(userID)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) AddReminder(userID string, text string, dueAt int64) error {
	args := m.Called(userID, text, dueAt)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) GetDueReminders() ([]memory.Reminder, error) {
	args := m.Called()
	return args.Get(0).([]memory.Reminder), args.Error(1)
}

func (m *MockMemoryStoreForMemory) UpdateReminder(reminder memory.Reminder) error {
	args := m.Called(reminder)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) DeleteReminder(id string) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) DeleteOldReminders(ageLimit time.Duration) error {
	args := m.Called(ageLimit)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) GetState(key string) (string, error) {
	args := m.Called(key)
	return args.String(0), args.Error(1)
}

func (m *MockMemoryStoreForMemory) SetState(key, value string) error {
	args := m.Called(key, value)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) HasPendingDM(userID string) (bool, error) {
	args := m.Called(userID)
	return args.Bool(0), args.Error(1)
}

func (m *MockMemoryStoreForMemory) GetPendingDMInfo(userID string) (time.Time, int, bool, error) {
	args := m.Called(userID)
	return args.Get(0).(time.Time), args.Int(1), args.Bool(2), args.Error(3)
}

func (m *MockMemoryStoreForMemory) SetPendingDM(userID string, sentAt time.Time) error {
	args := m.Called(userID, sentAt)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) ClearPendingDM(userID string) error {
	args := m.Called(userID)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) GetLastInteraction(userID string) (time.Time, error) {
	args := m.Called(userID)
	return args.Get(0).(time.Time), args.Error(1)
}

func (m *MockMemoryStoreForMemory) SetLastInteraction(userID string, timestamp time.Time) error {
	args := m.Called(userID, timestamp)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) GetAffection(userID string) (int, error) {
	args := m.Called(userID)
	return args.Int(0), args.Error(1)
}

func (m *MockMemoryStoreForMemory) AddAffection(userID string, amount int) error {
	args := m.Called(userID, amount)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) SetAffection(userID string, amount int) error {
	args := m.Called(userID, amount)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) GetStreak(userID string) (int, error) {
	args := m.Called(userID)
	return args.Int(0), args.Error(1)
}

func (m *MockMemoryStoreForMemory) UpdateStreak(userID string) (int, bool) {
	args := m.Called(userID)
	return args.Int(0), args.Bool(1)
}

func (m *MockMemoryStoreForMemory) GetFirstInteraction(userID string) (time.Time, error) {
	args := m.Called(userID)
	return args.Get(0).(time.Time), args.Error(1)
}

func (m *MockMemoryStoreForMemory) SetFirstInteraction(userID string, timestamp time.Time) error {
	args := m.Called(userID, timestamp)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) AddDelayedThought(thought memory.DelayedThought) error {
	args := m.Called(thought)
	return args.Error(0)
}

func (m *MockMemoryStoreForMemory) GetDueDelayedThoughts() ([]memory.DelayedThought, error) {
	args := m.Called()
	return args.Get(0).([]memory.DelayedThought), args.Error(1)
}

func (m *MockMemoryStoreForMemory) HasDelayedThought(userID string) (bool, error) {
	args := m.Called(userID)
	return args.Bool(0), args.Error(1)
}

func (m *MockMemoryStoreForMemory) DeleteDelayedThought(id string) error {
	args := m.Called(id)
	return args.Error(0)
}

func TestExtractMemories_JSONParsing(t *testing.T) {
	tests := []struct {
		name           string
		llmResponse    string
		expectedAdds   []string
		expectedRemoves []string
		expectedRemind int // Count of reminders
	}{
		{
			name:        "Clean JSON",
			llmResponse: `{"add": ["Lives in Tokyo"], "remove": [], "reminders": []}`,
			expectedAdds: []string{"Lives in Tokyo"},
		},
		{
			name:        "Markdown Block JSON",
			llmResponse: "Here is the JSON:\n```json\n{\"add\": [\"Loves sushi\"], \"remove\": [], \"reminders\": []}\n```",
			expectedAdds: []string{"Loves sushi"},
		},
		{
			name:        "Markdown Block without json tag",
			llmResponse: "```\n{\"add\": [\"Hates spiders\"], \"remove\": [], \"reminders\": []}\n```",
			expectedAdds: []string{"Hates spiders"},
		},
		{
			name:        "JSON with surrounding text",
			llmResponse: "Sure, I found this:\n{\"add\": [\"Is a programmer\"], \"remove\": [], \"reminders\": []}\nHope that helps!",
			expectedAdds: []string{"Is a programmer"},
		},
		{
			name:        "Broken JSON (should fail gracefully)",
			llmResponse: `{"add": ["Broken JSON...`,
			expectedAdds: nil,
		},
		{
			name: "Reminders extraction",
			llmResponse: `{"add": [], "remove": [], "reminders": [{"text": "Buy milk", "delay_seconds": 3600}]}`,
			expectedRemind: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup Mocks
			mockCerebras := new(MockCerebrasClientForMemory)
			mockMemory := new(MockMemoryStoreForMemory)
			mockGemini := &MockGeminiClient{} // Use existing mock if available or define minimal one

			handler := NewHandler(mockCerebras, &mockEmbeddingClient{}, mockGemini, mockMemory, HandlerConfig{})

			// Expectations
			mockMemory.On("GetState", "mood").Return("", nil).Maybe() // Called by NewHandler in goroutine
			mockMemory.On("GetFacts", "user123").Return([]string{}, nil)
			mockCerebras.On("ChatCompletion", mock.Anything).Return(tt.llmResponse, nil)

			if len(tt.expectedAdds) > 0 || len(tt.expectedRemoves) > 0 {
				// The implementation passes an empty slice []string{} if removes is empty, not nil
				var expectedRemoves []string
				if tt.expectedRemoves == nil {
					expectedRemoves = []string{}
				} else {
					expectedRemoves = tt.expectedRemoves
				}
				mockMemory.On("ApplyDelta", "user123", tt.expectedAdds, expectedRemoves).Return(nil)
			}

			if tt.expectedRemind > 0 {
				mockMemory.On("AddReminder", "user123", mock.AnythingOfType("string"), mock.AnythingOfType("int64")).Return(nil)
			}

			// Execute
			handler.extractMemories("user123", "testuser", "I live in Tokyo and I love sushi.", "Cool!")

			// Verify
			if len(tt.expectedAdds) > 0 {
				var expectedRemoves []string
				if tt.expectedRemoves == nil {
					expectedRemoves = []string{}
				} else {
					expectedRemoves = tt.expectedRemoves
				}
				mockMemory.AssertCalled(t, "ApplyDelta", "user123", tt.expectedAdds, expectedRemoves)
			}
			if tt.expectedRemind > 0 {
				mockMemory.AssertCalled(t, "AddReminder", "user123", mock.AnythingOfType("string"), mock.AnythingOfType("int64"))
			}
		})
	}
}
