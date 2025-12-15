package bot

import (
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// SlashCommands defines all available slash commands
var SlashCommands = []*discordgo.ApplicationCommand{
	{
		Name:        "reset",
		Description: "Permanently delete all your conversation history and memories",
	},
	{
		Name:        "stats",
		Description: "See what Marin remembers about you",
	},
	{
		Name:        "mood",
		Description: "Check Marin's current mood",
	},
	{
		Name:        "affection",
		Description: "Check your relationship status with Marin",
	},
}

// SlashCommandHandlers maps command names to their handler functions
var SlashCommandHandlers = map[string]func(h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate){
	"reset":     handleResetCommand,
	"stats":     handleStatsCommand,
	"mood":      handleMoodCommand,
	"affection": handleAffectionCommand,
}

// handleResetCommand handles the /reset slash command
func handleResetCommand(h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Get user ID (works for both guild and DM contexts)
	var userID string
	if i.Member != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	} else {
		log.Printf("Error: Could not determine user ID for reset command")
		return
	}

	// Reset the user's memory
	err := h.ResetMemory(userID)

	responseContent := "Memory reset! Starting fresh. 💭✨"
	if err != nil {
		log.Printf("Error resetting memory for user %s: %v", userID, err)
		responseContent = "Ugh, something went wrong trying to reset your memory... Try again later?"
	}

	// Respond to the interaction
	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: responseContent,
			Flags:   discordgo.MessageFlagsEphemeral, // Only visible to the user who ran the command
		},
	})

	if err != nil {
		log.Printf("Error responding to reset command: %v", err)
	}
}

// handleStatsCommand handles the /stats slash command - shows what Marin remembers
func handleStatsCommand(h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Get user ID
	var userID string
	var userName string
	if i.Member != nil {
		userID = i.Member.User.ID
		userName = i.Member.User.Username
		if i.Member.User.GlobalName != "" {
			userName = i.Member.User.GlobalName
		}
	} else if i.User != nil {
		userID = i.User.ID
		userName = i.User.Username
		if i.User.GlobalName != "" {
			userName = i.User.GlobalName
		}
	} else {
		log.Printf("Error: Could not determine user ID for stats command")
		return
	}

	// Get facts about the user
	facts, err := h.memoryStore.GetFacts(userID)

	if err != nil {
		log.Printf("Error getting facts for user %s: %v", userID, err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Hmm, I had trouble checking my notes... Try again?",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	var embed *discordgo.MessageEmbed

	if len(facts) == 0 {
		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("📝 Memory File: %s", userName),
			Description: "I don't have any specific notes about you yet! Chat with me more and I'll start remembering things~",
			Color:       0x00BFFF, // Deep Sky Blue
		}
	} else {
		// Format facts nicely
		factList := "• " + strings.Join(facts, "\n• ")

		// If factList is too long for one field (1024 chars), truncate safely
		if len(factList) > 1024 {
			// Convert to runes to safely slice multi-byte characters (like emojis)
			runes := []rune(factList)
			// Discord limit is 1024 characters, but we need room for "..."
			// However, Discord counts characters, not bytes (mostly).
			// To be safe against byte limits, we'll keep it under 1000 characters.
			if len(runes) > 1021 {
				factList = string(runes[:1021]) + "..."
			}
		}

		embed = &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("📝 Memory File: %s", userName),
			Description: "Here's everything I've noted down about you so far!",
			Color:       0x00BFFF, // Deep Sky Blue
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "Observations",
					Value:  factList,
					Inline: false,
				},
			},
			Footer: &discordgo.MessageEmbedFooter{
				Text: "To start fresh, use /reset",
			},
		}
	}

	// Respond to the interaction
	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})

	if err != nil {
		log.Printf("Error responding to stats command: %v", err)
	}
}

// handleMoodCommand handles the /mood slash command - shows Marin's current mood
func handleMoodCommand(h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	mood, emoji, description := h.GetCurrentMood()

	responseContent := fmt.Sprintf("%s **%s**\n\n%s", emoji, mood, description)

	// Add a little extra flavor based on mood
	switch mood {
	case MoodHyper:
		responseContent += "\n\n*bounces around excitedly*"
	case MoodSleepy:
		responseContent += "\n\n*yawns*"
	case MoodFlirty:
		responseContent += " 😏"
	case MoodNostalgic:
		responseContent += "\n\n*stares out the window wistfully*"
	case MoodFocused:
		responseContent += "\n\n*adjusts glasses*"
	case MoodBored:
		responseContent += "\n\n*sighs*"
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: responseContent,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})

	if err != nil {
		log.Printf("Error responding to mood command: %v", err)
	}
}

// handleAffectionCommand handles the /affection slash command - shows relationship status
func handleAffectionCommand(h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Get user ID
	var userID string
	var userName string
	if i.Member != nil {
		userID = i.Member.User.ID
		userName = i.Member.User.Username
		if i.Member.User.GlobalName != "" {
			userName = i.Member.User.GlobalName
		}
	} else if i.User != nil {
		userID = i.User.ID
		userName = i.User.Username
		if i.User.GlobalName != "" {
			userName = i.User.GlobalName
		}
	} else {
		log.Printf("Error: Could not determine user ID for affection command")
		return
	}

	affection, level := h.GetUserAffection(userID)
	streak, _ := h.memoryStore.GetStreak(userID)

	// Build flavor text
	var flavorText string
	switch level.Name {
	case "Stranger":
		flavorText = "we just met~ let's chat more!"
	case "Familiar Face":
		flavorText = "i think i've seen you around? hey there!"
	case "Acquaintance":
		flavorText = "i'm starting to remember you~"
	case "Casual Friend":
		flavorText = "always nice to chat with you~"
	case "Friend":
		flavorText = "we're friends now! nice"
	case "Good Friend":
		flavorText = "you're pretty cool, you know that?"
	case "Close Friend":
		flavorText = "you're like... really important to me"
	case "Best Friend":
		flavorText = "i literally think about you all the time"
	case "Soulmate":
		flavorText = "you complete me. seriously. 💕"
	case "Special Someone":
		flavorText = "...you know how i feel about you right? 💕"
	default:
		flavorText = "hello! nice to see you~"
	}

	// Create Embed
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("💕 Relationship with %s", userName),
		Description: flavorText,
		Color:       0xFF69B4, // Hot Pink
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Current Status",
				Value:  FormatAffectionDisplay(affection, streak),
				Inline: false,
			},
		},
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})

	if err != nil {
		log.Printf("Error responding to affection command: %v", err)
	}
}

// InteractionCreate handles all slash command interactions
func (h *Handler) InteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Only handle application commands (slash commands)
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	commandName := i.ApplicationCommandData().Name

	// Find and execute the appropriate handler
	if handler, ok := SlashCommandHandlers[commandName]; ok {
		handler(h, s, i)
	} else {
		log.Printf("Unknown slash command: %s", commandName)
	}
}

// RegisterSlashCommands registers all slash commands with Discord
func RegisterSlashCommands(s *discordgo.Session, guildID string) ([]*discordgo.ApplicationCommand, error) {
	log.Println("Registering slash commands...")

	registeredCommands := make([]*discordgo.ApplicationCommand, len(SlashCommands))

	for i, cmd := range SlashCommands {
		// Register globally (guildID = "") or for a specific guild
		registeredCmd, err := s.ApplicationCommandCreate(s.State.User.ID, guildID, cmd)
		if err != nil {
			log.Printf("Cannot create '%s' command: %v", cmd.Name, err)
			return nil, err
		}
		registeredCommands[i] = registeredCmd
		log.Printf("Registered command: %s", cmd.Name)
	}

	return registeredCommands, nil
}

// UnregisterSlashCommands removes all registered slash commands
func UnregisterSlashCommands(s *discordgo.Session, guildID string, commands []*discordgo.ApplicationCommand) error {
	log.Println("Unregistering slash commands...")

	for _, cmd := range commands {
		err := s.ApplicationCommandDelete(s.State.User.ID, guildID, cmd.ID)
		if err != nil {
			log.Printf("Cannot delete '%s' command: %v", cmd.Name, err)
			return err
		}
		log.Printf("Unregistered command: %s", cmd.Name)
	}

	return nil
}
