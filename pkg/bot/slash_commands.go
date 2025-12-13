package bot

import (
	"log"

	"github.com/bwmarrin/discordgo"
)

// SlashCommands defines all available slash commands
var SlashCommands = []*discordgo.ApplicationCommand{
	{
		Name:        "reset",
		Description: "Permanently delete all your conversation history and memories",
	},
	{
		Name:        "memory",
		Description: "View what Marin knows about you and current memory stats",
	},
	{
		Name:        "resent",
		Description: "Resend the last message Marin sent (useful if you accidentally deleted it)",
	},
}

// SlashCommandHandlers maps command names to their handler functions
var SlashCommandHandlers = map[string]func(h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate){
	"reset":  handleResetCommand,
	"memory": handleMemoryCommand,
	"resent": handleResentCommand,
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

func handleMemoryCommand(h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	var userID string
	if i.Member != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	} else {
		return
	}

	// Fetch facts
	facts, err := h.memoryStore.GetFacts(userID)
	if err != nil {
		log.Printf("Error getting facts: %v", err)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Error fetching memories.", Flags: discordgo.MessageFlagsEphemeral},
		})
		return
	}

	// Fetch reminders
	// This is a bit inefficient as it gets all reminders then filters, but okay for now
	// allReminders, err := h.memoryStore.GetDueReminders()
	// Actually GetDueReminders only gets DUE ones. We might want ALL pending reminders for the user.
	// But the interface doesn't support that yet.
	// Let's just show facts for now.

	content := "**🧠 Marin's Memory of You**\n\n"
	if len(facts) == 0 {
		content += "_I don't know much about you yet! Chat with me more so I can remember things._"
	} else {
		content += "**Facts:**\n"
		for _, f := range facts {
			content += "• " + f + "\n"
		}
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func handleResentCommand(h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	lastResponse := h.GetLastResponse(i.ChannelID)

	if lastResponse == "" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "I haven't said anything here recently to resend!",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: lastResponse,
		},
	})
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
