#!/bin/bash
set -e

# Toolsmith: Scaffold new slash command
# Usage: ./scripts/scaffold-command.sh <command_name> "<description>"

if [ "$#" -ne 2 ]; then
    echo "Usage: $0 <command_name> \"<description>\""
    exit 1
fi

CMD_NAME=$1
CMD_DESC=$2
FILE="pkg/bot/slash_commands.go"

# Validation
if [[ ! "$CMD_NAME" =~ ^[a-z0-9_]+$ ]]; then
    echo "Error: Command name must be lowercase alphanumeric (underscores allowed)."
    exit 1
fi

if grep -q "Name:.*\"$CMD_NAME\"" "$FILE"; then
    echo "Error: Command '$CMD_NAME' already exists in $FILE."
    exit 1
fi

echo "🔨 Scaffolding command '$CMD_NAME'..."

# Construct function name (e.g. handlePingCommand)
# Capitalize first letter logic compatible with older bash
FIRST_CHAR=$(echo "${CMD_NAME:0:1}" | tr '[:lower:]' '[:upper:]')
REST_CHARS="${CMD_NAME:1}"
FUNC_NAME="handle${FIRST_CHAR}${REST_CHARS}Command"

# Helper for cross-platform in-place editing
# macOS requires empty string argument for -i, GNU sed does not accept it.
run_sed() {
    local expression=$1
    local file=$2
    if [[ "$OSTYPE" == "darwin"* ]]; then
        sed -i '' "$expression" "$file"
    else
        sed -i "$expression" "$file"
    fi
}

# 1. Add to SlashCommands list
# Insert after the opening brace of SlashCommands
# We use a temporary file to avoid sed -i compatibility issues entirely
TEMP_FILE=$(mktemp)

# Insert command definition
awk -v cmd="$CMD_NAME" -v desc="$CMD_DESC" '
/var SlashCommands = \[\]\*discordgo.ApplicationCommand{/ {
    print $0
    printf "\t{\n\t\tName:        \"%s\",\n\t\tDescription: \"%s\",\n\t},\n", cmd, desc
    next
}
{ print }
' "$FILE" > "$TEMP_FILE"
mv "$TEMP_FILE" "$FILE"

# 2. Add to SlashCommandHandlers map
# Insert handler mapping
# Note: func is a keyword in some awk versions (like gawk), so we use func_name
awk -v cmd="$CMD_NAME" -v func_name="$FUNC_NAME" '
/var SlashCommandHandlers = map\[string\]func\(h \*Handler, s \*discordgo.Session, i \*discordgo.InteractionCreate\){/ {
    print $0
    printf "\t\"%s\":     %s,\n", cmd, func_name
    next
}
{ print }
' "$FILE" > "$TEMP_FILE"
mv "$TEMP_FILE" "$FILE"

# 3. Append handler function stub
cat <<EOF >> "$FILE"

// $FUNC_NAME handles the /$CMD_NAME slash command
func $FUNC_NAME(h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Respond to the interaction
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Command /$CMD_NAME executed!",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})

	if err != nil {
		log.Printf("Error responding to $CMD_NAME command: %v", err)
	}
}
EOF

# 4. Format code
echo "🎨 Formatting code..."
go fmt "$FILE"

echo "✅ Command /$CMD_NAME created successfully!"
echo "👉 Handler: $FUNC_NAME in $FILE"
