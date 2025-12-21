package bot

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

// getUserFromInteraction extracts the user ID and name from an interaction
// It handles both guild (Member) and DM (User) contexts
// Returns userID, userName, and error if user cannot be determined
func getUserFromInteraction(i *discordgo.InteractionCreate) (string, string, error) {
	if i.Member != nil {
		userName := i.Member.User.Username
		if i.Member.User.GlobalName != "" {
			userName = i.Member.User.GlobalName
		}
		return i.Member.User.ID, userName, nil
	}

	if i.User != nil {
		userName := i.User.Username
		if i.User.GlobalName != "" {
			userName = i.User.GlobalName
		}
		return i.User.ID, userName, nil
	}

	return "", "", fmt.Errorf("could not determine user from interaction")
}
