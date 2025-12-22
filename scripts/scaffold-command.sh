#!/bin/bash
# scripts/scaffold-command.sh
# Scaffolds a new slash command in pkg/bot/slash_commands.go
# Usage: ./scripts/scaffold-command.sh <command_name> <description>

set -e

CMD_NAME="$1"
CMD_DESC="$2"

if [[ -z "$CMD_NAME" || -z "$CMD_DESC" ]]; then
    echo "Usage: $0 <command_name> <description>"
    echo "Example: $0 ping \"Replies with Pong\""
    exit 1
fi

TARGET_FILE="pkg/bot/slash_commands.go"

if [ ! -f "$TARGET_FILE" ]; then
    echo "Error: $TARGET_FILE not found. Run from repo root."
    exit 1
fi

# Check for duplicates
if grep -q "Name:[[:space:]]*\"$CMD_NAME\"" "$TARGET_FILE"; then
    echo "⚠️  Command '$CMD_NAME' already exists in $TARGET_FILE"
    exit 0
fi

echo "🔨 Scaffolding command '$CMD_NAME'..."

# Capitalize for handler name (e.g., test -> handleTestCommand)
# macOS/BSD compliant capitalization
FIRST_CHAR=$(echo "${CMD_NAME:0:1}" | tr '[:lower:]' '[:upper:]')
REST_CHARS="${CMD_NAME:1}"
HANDLER_NAME="handle${FIRST_CHAR}${REST_CHARS}Command"

# Use awk to insert the configuration and handler mapping
# We use a temp file to ensure atomic write
tmp_file=$(mktemp)

awk -v name="$CMD_NAME" -v desc="$CMD_DESC" -v handler="$HANDLER_NAME" '
    # Insert into SlashCommands slice
    # Match the line starting with "var SlashCommands =" and set flag
    /var SlashCommands =/ { in_commands = 1 }
    # Inside the block, match the closing brace at the start of the line
    in_commands && /^}/ {
        print "\t{"
        printf "\t\tName:        \"%s\",\n", name
        printf "\t\tDescription: \"%s\",\n", desc
        print "\t},"
        in_commands = 0
    }

    # Insert into SlashCommandHandlers map
    # Match the line starting with "var SlashCommandHandlers =" and set flag
    /var SlashCommandHandlers =/ { in_handlers = 1 }
    # Inside the block, match the closing brace at the start of the line
    in_handlers && /^}/ {
        printf "\t\"%s\":     %s,\n", name, handler
        in_handlers = 0
    }

    { print }
' "$TARGET_FILE" > "$tmp_file"

# Append the handler function at the end
cat <<EOF >> "$tmp_file"

// $HANDLER_NAME handles the /$CMD_NAME slash command
func $HANDLER_NAME(h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	// TODO: Implement command logic
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Command '$CMD_NAME' executed! (Not implemented yet)",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})

	if err != nil {
		log.Printf("Error responding to $CMD_NAME command: %v", err)
	}
}
EOF

# Move temp file to target
mv "$tmp_file" "$TARGET_FILE"

# Format the file
echo "🎨 Formatting code..."
if command -v go &> /dev/null; then
    go fmt "$TARGET_FILE"
else
    echo "⚠️  'go' command not found. Skipping formatting."
fi

echo "✅ Created command '$CMD_NAME' with handler $HANDLER_NAME"
