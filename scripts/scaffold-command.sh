#!/bin/bash
set -e

# Usage: ./scripts/scaffold-command.sh <command_name> <description>
# Example: ./scripts/scaffold-command.sh ping "Responds with pong"

COMMAND_NAME=$1
DESCRIPTION=$2
FILE="pkg/bot/slash_commands.go"

if [ -z "$COMMAND_NAME" ] || [ -z "$DESCRIPTION" ]; then
    echo "Usage: $0 <command_name> <description>"
    exit 1
fi

# Validation: command name should be lowercase alphanumeric (kebab-case allowed)
if [[ ! "$COMMAND_NAME" =~ ^[a-z0-9_-]+$ ]]; then
    echo "Error: Command name must be lowercase alphanumeric (plus hyphens/underscores)."
    exit 1
fi

if [ ! -f "$FILE" ]; then
    echo "Error: File $FILE not found."
    exit 1
fi

# Check if command already exists
if grep -q "\"$COMMAND_NAME\":" "$FILE" || grep -q "Name:.*\"$COMMAND_NAME\"" "$FILE"; then
    echo "⚠️  Command '$COMMAND_NAME' already exists in $FILE. Skipping."
    exit 0
fi

# Convert command-name to CamelCase for the handler function
# e.g., "my-command" -> "MyCommand"
CAMEL_NAME=$(echo "$COMMAND_NAME" | awk -F'[-_]' '{for(i=1;i<=NF;i++) $i=toupper(substr($i,1,1)) substr($i,2)} 1' OFS='')
HANDLER_NAME="handle${CAMEL_NAME}Command"

echo "🔨 Scaffolding command '$COMMAND_NAME'..."

# Create a temporary file
TEMP_FILE=$(mktemp)

# 1. Add to SlashCommands slice
# We look for the 'var SlashCommands =' block and insert before its closing brace '}'
awk -v cmd="$COMMAND_NAME" -v desc="$DESCRIPTION" '
    /^var SlashCommands =/ { in_slash_commands = 1 }
    /^}/ && in_slash_commands {
        print "\t{"
        print "\t\tName:        \"" cmd "\","
        print "\t\tDescription: \"" desc "\","
        print "\t},"
        in_slash_commands = 0
    }
    { print }
' "$FILE" > "$TEMP_FILE" && mv "$TEMP_FILE" "$FILE"

# 2. Add to SlashCommandHandlers map
# We look for the 'var SlashCommandHandlers =' block and insert before its closing brace '}'
awk -v cmd="$COMMAND_NAME" -v handler="$HANDLER_NAME" '
    /^var SlashCommandHandlers =/ { in_handlers = 1 }
    /^}/ && in_handlers {
        print "\t\"" cmd "\":     " handler ","
        in_handlers = 0
    }
    { print }
' "$FILE" > "$TEMP_FILE" && mv "$TEMP_FILE" "$FILE"

# 3. Add handler function stub at the end of the file
cat <<EOF >> "$FILE"

// $HANDLER_NAME handles the /$COMMAND_NAME slash command
func $HANDLER_NAME(h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	// TODO: Implement logic for /$COMMAND_NAME
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Command /$COMMAND_NAME executed! 🚀",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})

	if err != nil {
		log.Printf("Error responding to $COMMAND_NAME command: %v", err)
	}
}
EOF

# Format the file
echo "✨ Formatting code..."
go fmt "$FILE"

echo "✅ Created command '$COMMAND_NAME' with handler '$HANDLER_NAME'!"
