package bot

import(
	"marinai/pkg/memory"
	
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reusing mocks from flow_test.go since they are in the same package (bot)

func TestExtractMemories(t *testing.T) {
	// Setup
	mockCerebras := &mockCerebrasClient{}
	mockEmbedding := &mockEmbeddingClient{}
	mockMemory := &mockMemoryStore{}

	// Create handler
	h := NewHandler(mockCerebras, mockEmbedding, mockMemory, HandlerConfig{
		MessageProcessingDelay:     0,
		FactAgingDays:              7,
		FactSummarizationThreshold: 20,
		MaintenanceIntervalHours:   24,
	})

	// Test data
	userID := "user123"
	userName := "TestUser"
	userMessage := "I live in Tokyo and I love sushi."
	botReply := "That's awesome! Tokyo is great."

	// Test cases
	tests := []struct {
		name                 string
		llmResponse          string
		expectDeltaCalled    bool
		expectReminderCalled bool
		expectedAdds         []string
		expectedRemoves      []string
		expectedReminders    []string // Just checking text
	}{
		{
			name:              "Valid JSON - Add Fact",
			llmResponse:       `{"add": ["Lives in Tokyo", "Loves sushi"], "remove": [], "reminders": []}`,
			expectDeltaCalled: true,
			expectedAdds:      []string{"Lives in Tokyo", "Loves sushi"},
			expectedRemoves:   []string{},
		},
		{
			name:              "Valid JSON - Remove Fact",
			llmResponse:       `{"add": ["Moved to Kyoto"], "remove": ["Lives in Tokyo"], "reminders": []}`,
			expectDeltaCalled: true,
			expectedAdds:      []string{"Moved to Kyoto"},
			expectedRemoves:   []string{"Lives in Tokyo"},
		},
		{
			name: "Valid JSON - With Reminder",
			llmResponse: `{"add": [], "remove": [], "reminders": [
				{"text": "Buy milk", "delay_seconds": 3600}
			]}`,
			expectDeltaCalled:    false, // No memory delta
			expectReminderCalled: true,
			expectedReminders:    []string{"Buy milk"},
		},
		{
			name:              "Markdown Wrapped JSON",
			llmResponse:       "```json\n{\"add\": [\"Lives in Tokyo\"], \"remove\": [], \"reminders\": []}\n```",
			expectDeltaCalled: true,
			expectedAdds:      []string{"Lives in Tokyo"},
		},
		{
			name:              "Markdown Wrapped JSON with extra text",
			llmResponse:       "Here is the JSON:\n```json\n{\"add\": [\"Lives in Tokyo\"], \"remove\": [], \"reminders\": []}\n```\nHope this helps!",
			expectDeltaCalled: true,
			expectedAdds:      []string{"Lives in Tokyo"},
		},
		{
			name:              "Invalid JSON",
			llmResponse:       `{"add": ["Broken JSON...`,
			expectDeltaCalled: false,
		},
		{
			name:              "Empty JSON",
			llmResponse:       `{"add": [], "remove": [], "reminders": []}`,
			expectDeltaCalled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Reset mocks
			var deltaAdds, deltaRemoves []string
			deltaCalled := false
			reminderCalled := false
			var reminderTexts []string

			mockMemory.ApplyDeltaFunc = func(uid string, adds []string, removes []string) error {
				assert.Equal(t, userID, uid)
				deltaAdds = adds
				deltaRemoves = removes
				deltaCalled = true
				return nil
			}

			mockMemory.AddReminderFunc = func(uid string, text string, dueAt int64) error {
				assert.Equal(t, userID, uid)
				reminderCalled = true
				reminderTexts = append(reminderTexts, text)
				return nil
			}

			// Mock Cerebras to return the test case response
			mockCerebras.ChatCompletionFunc = func(messages []memory.LLMMessage) (string, error) {
				return tc.llmResponse, nil
			}

			// Also mock GetFacts to return empty list
			mockMemory.GetFactsFunc = func(uid string) ([]string, error) {
				return []string{}, nil
			}

			// Run extraction
			h.extractMemories(userID, userName, userMessage, botReply)

			// Verify
			if tc.expectDeltaCalled {
				require.True(t, deltaCalled, "Expected ApplyDelta to be called")
				assert.Equal(t, tc.expectedAdds, deltaAdds)
				if tc.expectedRemoves != nil {
					assert.Equal(t, tc.expectedRemoves, deltaRemoves)
				}
			} else {
				assert.False(t, deltaCalled, "Expected ApplyDelta NOT to be called")
			}

			if tc.expectReminderCalled {
				require.True(t, reminderCalled, "Expected AddReminder to be called")
				assert.Equal(t, tc.expectedReminders, reminderTexts)
			} else {
				assert.False(t, reminderCalled, "Expected AddReminder NOT to be called")
			}
		})
	}
}

func TestExtractMemories_ShortMessage(t *testing.T) {
	// Setup
	mockCerebras := &mockCerebrasClient{}
	mockMemory := &mockMemoryStore{}
	// Mock other dependencies...
	mockEmbedding := &mockEmbeddingClient{}

	h := NewHandler(mockCerebras, mockEmbedding, mockMemory, HandlerConfig{
		MessageProcessingDelay:     0,
		FactAgingDays:              7,
		FactSummarizationThreshold: 20,
		MaintenanceIntervalHours:   24,
	})

	// Spy
	chatCompletionCalled := false
	mockCerebras.ChatCompletionFunc = func(messages []memory.LLMMessage) (string, error) {
		chatCompletionCalled = true
		return `{"add": [], "remove": []}`, nil
	}

	// Message shorter than 10 chars
	h.extractMemories("user1", "TestUser", "Hi", "Hello")

	assert.False(t, chatCompletionCalled, "Should skip extraction for short messages")
}
