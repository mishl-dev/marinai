package bot

import (
	"marinai/pkg/cerebras"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Reusing mocks from flow_test.go as they are in the same package

func TestTaskAgent_CheckTask(t *testing.T) {
	// Setup
	mockCerebras := &mockCerebrasClient{}
	mockGemini := &MockGeminiClient{}
	agent := NewTaskAgent(mockCerebras, mockGemini)

	tests := []struct {
		name            string
		message         string
		classifyLabel   string
		classifyScore   float64
		classifyError   error
		refusalResponse string
		refusalError    error
		expectedIsTask  bool
		expectedRefusal string
	}{
		{
			name:            "Normal chat message",
			message:         "Hi Marin, how are you?",
			classifyLabel:   "chat message",
			classifyScore:   0.95,
			expectedIsTask:  false,
			expectedRefusal: "",
		},
		{
			name:            "Ambiguous message (low score)",
			message:         "Can you write something?",
			classifyLabel:   "requesting for long writing task",
			classifyScore:   0.5, // Below 0.9 threshold
			expectedIsTask:  false,
			expectedRefusal: "",
		},
		{
			name:            "Classification error",
			message:         "Error causing message",
			classifyError:   assert.AnError,
			expectedIsTask:  false,
			expectedRefusal: "",
		},
		{
			name:            "Explicit task request",
			message:         "Write a 500 word essay about history.",
			classifyLabel:   "requesting for long writing task",
			classifyScore:   0.99,
			refusalResponse: "No way, that sounds super boring.",
			expectedIsTask:  true,
			expectedRefusal: "No way, that sounds super boring.",
		},
		{
			name:            "Task request with refusal generation error",
			message:         "Write code for me.",
			classifyLabel:   "requesting for long writing task",
			classifyScore:   0.95,
			refusalError:    assert.AnError,
			expectedIsTask:  true,
			expectedRefusal: "Hah? Do it yourself. I'm busy.", // Default fallback
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Mock Classification
			mockCerebras.ClassifyFunc = func(text string, labels []string) (string, float64, error) {
				assert.Equal(t, tc.message, text)
				assert.Contains(t, labels, "chat message")
				assert.Contains(t, labels, "requesting for long writing task")
				return tc.classifyLabel, tc.classifyScore, tc.classifyError
			}

			// Mock ChatCompletion (Refusal Generation)
			mockCerebras.ChatCompletionFunc = func(messages []cerebras.Message) (string, error) {
				// Only called if identified as task
				return tc.refusalResponse, tc.refusalError
			}

			isTask, refusal := agent.CheckTask(tc.message)

			assert.Equal(t, tc.expectedIsTask, isTask)
			assert.Equal(t, tc.expectedRefusal, refusal)
		})
	}
}
