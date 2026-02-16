package memory

// LLMMessage represents a message for LLM chat completions
type LLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
