package bot

import (
	"bytes"
	"log"
	"marinai/pkg/cerebras"
	"marinai/pkg/memory"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Helper to capture log output
func captureOutput(f func()) string {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer func() {
		log.SetOutput(os.Stderr)
	}()
	f()
	return buf.String()
}

func TestCheckReminders_Logging(t *testing.T) {
	// Setup
	mockMemory := &mockMemoryStore{}
	mockCerebras := &mockCerebrasClient{}
	mockSession := &mockDiscordSession{}

	// Create handler with mocks
	handler := NewHandler(mockCerebras, nil, nil, mockMemory, HandlerConfig{})
	handler.SetSession(mockSession)

	// Mock processReminder dependencies
	mockCerebras.ChatCompletionFunc = func(messages []cerebras.Message) (string, error) {
		return "Don't forget!", nil
	}

	// We test processReminder directly to verify the new logging logic
	t.Run("processReminder logs duration", func(t *testing.T) {
		output := captureOutput(func() {
			err := handler.processReminder(memory.Reminder{ID: "rem1", UserID: "user1", Text: "Test Reminder", DueAt: time.Now().Unix()})
			assert.NoError(t, err)
		})

		// These assertions will pass once we implement the logging changes
		// For now, they might fail or pass depending on existing logs, but we want to assert the NEW format
		assert.Contains(t, output, "[Reminders]", "Log should contain [Reminders] prefix")
		assert.Contains(t, output, "processed in", "Log should contain timing info")
	})
}
