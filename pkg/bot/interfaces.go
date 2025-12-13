package bot

import (
	"github.com/bwmarrin/discordgo"
	"marinai/pkg/cerebras"
)

// Session interface abstracts discordgo.Session for testing
type Session interface {
	ChannelMessageSend(channelID string, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelMessageSendReply(channelID string, content string, reference *discordgo.MessageReference, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelTyping(channelID string, options ...discordgo.RequestOption) (err error)
	User(userID string) (*discordgo.User, error)
	Channel(channelID string, options ...discordgo.RequestOption) (*discordgo.Channel, error)
	GuildEmojis(guildID string, options ...discordgo.RequestOption) ([]*discordgo.Emoji, error)
	UserChannelCreate(recipientID string, options ...discordgo.RequestOption) (*discordgo.Channel, error)
	MessageReactionAdd(channelID, messageID, emojiID string) error
	UpdateStatusComplex(usd discordgo.UpdateStatusData) error
}

// DiscordSession adapts discordgo.Session to the Session interface
type DiscordSession struct {
	*discordgo.Session
}

func (s *DiscordSession) User(userID string) (*discordgo.User, error) {
	return s.Session.User(userID)
}

func (s *DiscordSession) Channel(channelID string, options ...discordgo.RequestOption) (*discordgo.Channel, error) {
	return s.Session.Channel(channelID, options...)
}

func (s *DiscordSession) GuildEmojis(guildID string, options ...discordgo.RequestOption) ([]*discordgo.Emoji, error) {
	return s.Session.GuildEmojis(guildID, options...)
}

func (s *DiscordSession) UserChannelCreate(recipientID string, options ...discordgo.RequestOption) (*discordgo.Channel, error) {
	return s.Session.UserChannelCreate(recipientID, options...)
}

func (s *DiscordSession) MessageReactionAdd(channelID, messageID, emojiID string) error {
	return s.Session.MessageReactionAdd(channelID, messageID, emojiID)
}

func (s *DiscordSession) UpdateStatusComplex(usd discordgo.UpdateStatusData) error {
	return s.Session.UpdateStatusComplex(usd)
}

type CerebrasClient interface {
	ChatCompletion(messages []cerebras.Message) (string, error)
}

type EmbeddingClient interface {
	Embed(text string) ([]float32, error)
}

type Classifier interface {
	Classify(text string, labels []string) (string, float64, error)
}

// VisionClient interface for image understanding
type VisionClient interface {
	DescribeImageFromURL(imageURL string) (*ImageDescription, error)
}

// ImageDescription represents the result of analyzing an image
// This mirrors vision.ImageDescription for interface compatibility
type ImageDescription struct {
	Description string
	IsNSFW      bool
	Error       error
}
