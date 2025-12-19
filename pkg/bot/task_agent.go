package bot

import (
	"fmt"
	"log"
	"strings"

	"marinai/pkg/cerebras"
)

type TaskAgent struct {
	cerebrasClient CerebrasClient
	geminiClient   GeminiClient
}

func NewTaskAgent(c CerebrasClient, g GeminiClient) *TaskAgent {
	return &TaskAgent{
		cerebrasClient: c,
		geminiClient:   g,
	}
}

// CheckTask analyzes if the message is a long writing task.
// If it is, it returns true and a refusal message in character.
// If not, it returns false and an empty string.
func (ta *TaskAgent) CheckTask(userMsg string) (bool, string) {
	// 1. Classify the message
	labels := []string{
		"chat message",
		"requesting for long writing task",
	}

	label, score, err := ta.cerebrasClient.Classify(userMsg, labels)
	if err != nil {
		log.Printf("Error classifying task: %v", err)
		// Fallback to assuming it's safe if classifier fails
		return false, ""
	}

	log.Printf("Task Classification: '%s' (score: %.2f)", label, score)

	// Only refuse if it is explicitly a task request with high confidence
	if label == "requesting for long writing task" && score >= 0.9 {
		log.Printf("Detected long writing task (score: %.2f), generating refusal", score)
	} else {
		return false, ""
	}
	// 2. Generate Refusal
	// We know it's a task, so we generate a refusal.
	prompt := fmt.Sprintf(`User Message: "%s"


User is asking for a complex task. You are Marin Kitagawa.
Refuse this request.
- Be playful but firm.
- Keep it short (1-2 sentences).
- Do NOT start with "Marin:" or quotes. Just the message.

Chat Style (Discord Mode):
- Type like a normal Discord user: mostly lowercase, minimal punctuation.
- Friendly but firm.
- No emojis.
- Be brief. Maximum 1-2 sentences.`, userMsg)

	messages := []cerebras.Message{
		{Role: "system", Content: "You are Marin Kitagawa. You refuse to do boring work."},
		{Role: "user", Content: prompt},
	}

	resp, err := ta.cerebrasClient.ChatCompletion(messages)
	if err != nil {
		log.Printf("Error generating refusal: %v", err)
		return true, "Hah? Do it yourself. I'm busy." // Fallback refusal
	}

	return true, strings.TrimSpace(resp)
}
