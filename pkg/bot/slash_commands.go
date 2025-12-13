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
}

// SlashCommandHandlers maps command names to their handler functions
var SlashCommandHandlers = map[string]func(h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate){
	"reset": handleResetCommand,
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
