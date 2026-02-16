package nvidia

import (
	"context"
	"fmt"
	"log"
	"marinai/pkg/bot"
	"marinai/pkg/memory"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

const (
	baseURL = "https://integrate.api.nvidia.com/v1"
)

type ModelConfig struct {
	ID       string
	MaxCtx   int
	MaxToken int
}

var DefaultModels = []ModelConfig{
	{ID: "z-ai/glm5", MaxCtx: 65536, MaxToken: 16384},
}

type KeyState struct {
	Key          string
	FailureCount int
	LastUsed     time.Time
	LastSuccess  time.Time
}

type Client struct {
	keys           []*KeyState
	keyMu          sync.RWMutex
	clients        map[string]openai.Client
	clientsMu      sync.RWMutex
	temperature    float64
	topP           float64
	models         []ModelConfig
	enableThinking bool
}

func NewClient(apiKeys string, temperature, topP float64, models []ModelConfig) *Client {
	if len(models) == 0 {
		models = DefaultModels
	}

	keyStrings := strings.Split(apiKeys, ",")
	keys := make([]*KeyState, 0, len(keyStrings))
	for _, k := range keyStrings {
		k = strings.TrimSpace(k)
		if k != "" {
			keys = append(keys, &KeyState{
				Key:          k,
				FailureCount: 0,
			})
		}
	}

	if len(keys) == 0 {
		log.Println("Warning: No NVIDIA API keys provided")
	} else {
		log.Printf("Loaded %d NVIDIA API key(s)", len(keys))
	}

	return &Client{
		keys:           keys,
		clients:        make(map[string]openai.Client),
		temperature:    temperature,
		topP:           topP,
		models:         models,
		enableThinking: true,
	}
}

func (c *Client) EnableThinking(enable bool) {
	c.enableThinking = enable
}

func (c *Client) getClient(key string) openai.Client {
	c.clientsMu.RLock()
	if client, ok := c.clients[key]; ok {
		c.clientsMu.RUnlock()
		return client
	}
	c.clientsMu.RUnlock()

	c.clientsMu.Lock()
	defer c.clientsMu.Unlock()

	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(key),
	)
	c.clients[key] = client
	return client
}

func (c *Client) getBestKey() *KeyState {
	c.keyMu.RLock()
	defer c.keyMu.RUnlock()

	if len(c.keys) == 0 {
		return nil
	}

	best := c.keys[0]
	for _, k := range c.keys[1:] {
		if k.FailureCount < best.FailureCount {
			best = k
		}
	}
	return best
}

func (c *Client) recordSuccess(key *KeyState) {
	c.keyMu.Lock()
	defer c.keyMu.Unlock()
	key.LastSuccess = time.Now()
	key.LastUsed = time.Now()
	if key.FailureCount > 0 {
		key.FailureCount--
	}
}

func (c *Client) recordFailure(key *KeyState) {
	c.keyMu.Lock()
	defer c.keyMu.Unlock()
	key.FailureCount++
	key.LastUsed = time.Now()
}

func (c *Client) simpleCompletion(messages []memory.LLMMessage, model ModelConfig) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	keyState := c.getBestKey()
	if keyState == nil {
		return "", fmt.Errorf("no API keys configured")
	}

	client := c.getClient(keyState.Key)

	chatMessages := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case "system":
			chatMessages[i] = openai.SystemMessage(msg.Content)
		case "assistant":
			chatMessages[i] = openai.AssistantMessage(msg.Content)
		case "user":
			chatMessages[i] = openai.UserMessage(msg.Content)
		default:
			chatMessages[i] = openai.UserMessage(msg.Content)
		}
	}

	params := openai.ChatCompletionNewParams{
		Model:       shared.ChatModel(model.ID),
		Messages:    chatMessages,
		Temperature: openai.Float(c.temperature),
		TopP:        openai.Float(c.topP),
		MaxTokens:   openai.Int(int64(model.MaxToken)),
	}

	resp, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		c.recordFailure(keyState)
		return "", err
	}

	if resp == nil || len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty response")
	}

	c.recordSuccess(keyState)
	return resp.Choices[0].Message.Content, nil
}

func (c *Client) ChatCompletion(messages []memory.LLMMessage) (string, error) {
	result, err := c.ChatCompletionWithTools(messages, nil)
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

func (c *Client) ChatCompletionWithTools(messages []memory.LLMMessage, tools []bot.Tool) (*bot.ChatResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	keyState := c.getBestKey()
	if keyState == nil {
		return nil, fmt.Errorf("no API keys configured")
	}

	var lastErr error

	for _, modelConf := range c.models {
		log.Printf("Attempting NVIDIA model: %s (key failures: %d)", modelConf.ID, keyState.FailureCount)

		client := c.getClient(keyState.Key)

		chatMessages := make([]openai.ChatCompletionMessageParamUnion, len(messages))
		for i, msg := range messages {
			switch msg.Role {
			case "system":
				chatMessages[i] = openai.SystemMessage(msg.Content)
			case "assistant":
				chatMessages[i] = openai.AssistantMessage(msg.Content)
			case "user":
				chatMessages[i] = openai.UserMessage(msg.Content)
			default:
				chatMessages[i] = openai.UserMessage(msg.Content)
			}
		}

		params := openai.ChatCompletionNewParams{
			Model:       shared.ChatModel(modelConf.ID),
			Messages:    chatMessages,
			Temperature: openai.Float(c.temperature),
			TopP:        openai.Float(c.topP),
			MaxTokens:   openai.Int(int64(modelConf.MaxToken)),
		}

		if len(tools) > 0 {
			toolParams := make([]openai.ChatCompletionToolParam, len(tools))
			for i, t := range tools {
				toolParams[i] = openai.ChatCompletionToolParam{
					Function: shared.FunctionDefinitionParam{
						Name:        t.Function.Name,
						Description: openai.String(t.Function.Description),
						Parameters:  shared.FunctionParameters(t.Function.Parameters),
					},
				}
			}
			params.Tools = toolParams
		}

		start := time.Now()

		resp, err := client.Chat.Completions.New(ctx, params)

		if err != nil {
			log.Printf("NVIDIA model %s error: %v", modelConf.ID, err)
			lastErr = err

			if isRateLimitOrAuthError(err) {
				c.recordFailure(keyState)
				nextKey := c.getBestKey()
				if nextKey != nil && nextKey != keyState {
					log.Printf("Key rate limited/auth failed, trying another key...")
					keyState = nextKey
					client = c.getClient(keyState.Key)
					params.Model = shared.ChatModel(modelConf.ID)
					resp, err = client.Chat.Completions.New(ctx, params)
					if err == nil {
						break
					}
				}
			}
			continue
		}

		if resp == nil || len(resp.Choices) == 0 {
			log.Printf("NVIDIA model %s returned empty response", modelConf.ID)
			lastErr = fmt.Errorf("empty response from model %s", modelConf.ID)
			continue
		}

		c.recordSuccess(keyState)
		duration := time.Since(start)

		choice := resp.Choices[0]
		result := &bot.ChatResult{
			Content: choice.Message.Content,
			Usage: bot.Usage{
				PromptTokens:     int(resp.Usage.PromptTokens),
				CompletionTokens: int(resp.Usage.CompletionTokens),
				TotalTokens:      int(resp.Usage.TotalTokens),
			},
		}

		if len(choice.Message.ToolCalls) > 0 {
			for _, tc := range choice.Message.ToolCalls {
				result.ToolCalls = append(result.ToolCalls, bot.ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				})
			}
		}

		log.Printf("NVIDIA model %s success (took %v, tokens: in=%d, out=%d)",
			modelConf.ID, duration, result.Usage.PromptTokens, result.Usage.CompletionTokens)

		return result, nil
	}

	c.recordFailure(keyState)
	return nil, fmt.Errorf("all NVIDIA models exhausted. Last error: %w", lastErr)
}

func (c *Client) ChatCompletionStream(messages []memory.LLMMessage, tools []bot.Tool, onChunk func(content, reasoning string, toolCalls []bot.ToolCall)) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	keyState := c.getBestKey()
	if keyState == nil {
		return fmt.Errorf("no API keys configured")
	}

	client := c.getClient(keyState.Key)
	modelConf := c.models[0]

	chatMessages := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case "system":
			chatMessages[i] = openai.SystemMessage(msg.Content)
		case "assistant":
			chatMessages[i] = openai.AssistantMessage(msg.Content)
		case "user":
			chatMessages[i] = openai.UserMessage(msg.Content)
		default:
			chatMessages[i] = openai.UserMessage(msg.Content)
		}
	}

	params := openai.ChatCompletionNewParams{
		Model:       shared.ChatModel(modelConf.ID),
		Messages:    chatMessages,
		Temperature: openai.Float(c.temperature),
		TopP:        openai.Float(c.topP),
		MaxTokens:   openai.Int(int64(modelConf.MaxToken)),
	}

	if len(tools) > 0 {
		toolParams := make([]openai.ChatCompletionToolParam, len(tools))
		for i, t := range tools {
			toolParams[i] = openai.ChatCompletionToolParam{
				Function: shared.FunctionDefinitionParam{
					Name:        t.Function.Name,
					Description: openai.String(t.Function.Description),
					Parameters:  shared.FunctionParameters(t.Function.Parameters),
				},
			}
		}
		params.Tools = toolParams
	}

	stream := client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()

	for stream.Next() {
		chunk := stream.Current()

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta
		var toolCalls []bot.ToolCall

		if len(delta.ToolCalls) > 0 {
			for _, tc := range delta.ToolCalls {
				toolCalls = append(toolCalls, bot.ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				})
			}
		}

		onChunk(delta.Content, "", toolCalls)
	}

	if err := stream.Err(); err != nil {
		c.recordFailure(keyState)
		return err
	}

	c.recordSuccess(keyState)
	return nil
}

func isRateLimitOrAuthError(err error) bool {
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "401") ||
		strings.Contains(errStr, "403") ||
		strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "unauthorized")
}

// Vision model for image description
var VisionModel = ModelConfig{ID: "moonshotai/kimi-k2.5", MaxCtx: 128000, MaxToken: 16384}

// DescribeImageFromURL describes an image from a URL using vision model
func (c *Client) DescribeImageFromURL(imageURL string) (*bot.ImageDescription, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	keyState := c.getBestKey()
	if keyState == nil {
		return nil, fmt.Errorf("no API keys configured")
	}

	client := c.getClient(keyState.Key)

	params := openai.ChatCompletionNewParams{
		Model: shared.ChatModel(VisionModel.ID),
		Messages: []openai.ChatCompletionMessageParamUnion{
			{OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfArrayOfContentParts: []openai.ChatCompletionContentPartUnionParam{
						{OfText: &openai.ChatCompletionContentPartTextParam{
							Text: "Describe this image concisely. Focus on:\n1. Main subjects/objects\n2. Setting/context\n3. Any text visible\n4. Overall mood/tone\n\nKeep description brief (2-3 sentences max).",
						}},
						{OfImageURL: &openai.ChatCompletionContentPartImageParam{
							ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
								URL: imageURL,
							},
						}},
					},
				},
			}},
		},
		MaxTokens:   openai.Int(512),
		Temperature: openai.Float(0.3),
	}

	resp, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		c.recordFailure(keyState)
		return nil, fmt.Errorf("vision API error: %w", err)
	}

	if resp == nil || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from vision model")
	}

	c.recordSuccess(keyState)

	description := strings.TrimSpace(resp.Choices[0].Message.Content)

	isNSFW := false
	lowerDesc := strings.ToLower(description)
	nsfwKeywords := []string{"nude", "naked", "explicit", "nsfw", "adult content", "sexual"}
	for _, kw := range nsfwKeywords {
		if strings.Contains(lowerDesc, kw) {
			isNSFW = true
			break
		}
	}

	return &bot.ImageDescription{
		Description: description,
		IsNSFW:      isNSFW,
	}, nil
}

// DescribeImageFromBase64 describes an image from base64 data
func (c *Client) DescribeImageFromBase64(imageB64, mimeType string) (*bot.ImageDescription, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	keyState := c.getBestKey()
	if keyState == nil {
		return nil, fmt.Errorf("no API keys configured")
	}

	client := c.getClient(keyState.Key)

	imageDataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, imageB64)

	params := openai.ChatCompletionNewParams{
		Model: shared.ChatModel(VisionModel.ID),
		Messages: []openai.ChatCompletionMessageParamUnion{
			{OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfArrayOfContentParts: []openai.ChatCompletionContentPartUnionParam{
						{OfText: &openai.ChatCompletionContentPartTextParam{
							Text: "Describe this image concisely. Focus on:\n1. Main subjects/objects\n2. Setting/context\n3. Any text visible\n4. Overall mood/tone\n\nKeep description brief (2-3 sentences max).",
						}},
						{OfImageURL: &openai.ChatCompletionContentPartImageParam{
							ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
								URL: imageDataURL,
							},
						}},
					},
				},
			}},
		},
		MaxTokens:   openai.Int(512),
		Temperature: openai.Float(0.3),
	}

	resp, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		c.recordFailure(keyState)
		return nil, fmt.Errorf("vision API error: %w", err)
	}

	if resp == nil || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from vision model")
	}

	c.recordSuccess(keyState)

	description := strings.TrimSpace(resp.Choices[0].Message.Content)

	isNSFW := false
	lowerDesc := strings.ToLower(description)
	nsfwKeywords := []string{"nude", "naked", "explicit", "nsfw", "adult content", "sexual"}
	for _, kw := range nsfwKeywords {
		if strings.Contains(lowerDesc, kw) {
			isNSFW = true
			break
		}
	}

	return &bot.ImageDescription{
		Description: description,
		IsNSFW:      isNSFW,
	}, nil
}
