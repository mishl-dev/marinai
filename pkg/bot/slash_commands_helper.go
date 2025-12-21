package bot

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
)

// getUserFromInteraction extracts the userID and userName from an interaction.
// It prioritizes GlobalName over Username.
func getUserFromInteraction(i *discordgo.InteractionCreate) (string, string, error) {
	if i.Member != nil {
		name := i.Member.User.GlobalName
		if name == "" {
			name = i.Member.User.Username
		}
		return i.Member.User.ID, name, nil
	}
	if i.User != nil {
		name := i.User.GlobalName
		if name == "" {
			name = i.User.Username
		}
		return i.User.ID, name, nil
	}
	return "", "", fmt.Errorf("could not determine user ID")
}
