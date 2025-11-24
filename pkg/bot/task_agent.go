package bot

import (
	"fmt"
	"log"
	"marinai/pkg/cerebras"
	"strings"
)

type TaskAgent struct {
	cerebrasClient   CerebrasClient
	classifierClient Classifier
}

func NewTaskAgent(c CerebrasClient, cl Classifier) *TaskAgent {
	return &TaskAgent{
		cerebrasClient:   c,
		classifierClient: cl,
	}
}

// CheckTask analyzes if the message is a long writing task.
// If it is, it returns true and a refusal message in character.
// If not, it returns false and an empty string.
func (ta *TaskAgent) CheckTask(userMsg string) (bool, string) {
	// 1. Classify the message
	labels := []string{
		"chat message",
		"request for long writing task",
		"request for code generation",
		"request for homework help",
		"request for essay",
	}

	label, score, err := ta.classifierClient.Classify(userMsg, labels)
	if err != nil {
		log.Printf("Error classifying task: %v", err)
		// Fallback to assuming it's safe if classifier fails
		return false, ""
	}

	log.Printf("Task Classification: '%s' (score: %.2f)", label, score)

	// If it's just a chat message, we're good
	if label == "chat message" {
		return false, ""
	}

	// If confidence is low, give benefit of doubt
	// Only treat as a task if we're confident (score >= 0.5)
	if score < 0.5 {
		log.Printf("Score too low, assuming chat message")
		return false, ""
	}
	// 2. Generate Refusal
	// We know it's a task, so we generate a refusal.
	prompt := fmt.Sprintf(`User Message: "%s"


User is asking for a complex task (writing/coding/homework). You are Marin Kitagawa.
Refuse this request.
- Be playful but firm.
- Tell them you'd rather be cosplaying, watching anime, or playing games.
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
