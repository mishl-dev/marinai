package bot

import (
	"fmt"
	"strings"
	"testing"
)

func TestSystemPromptFormatting(t *testing.T) {
	// Verify that SystemPrompt is a valid format string that accepts exactly two string arguments
	// (displayName and profileText)

	// Attempt to format with two strings
	displayName := "TestUser"
	profileText := "TestProfile"

	// This should not panic or error if the format string is valid and has 2 %s verbs
	formatted := fmt.Sprintf(SystemPrompt, displayName, profileText)

	// Verify that the arguments were actually inserted
	if !strings.Contains(formatted, displayName) {
		t.Errorf("Formatted prompt should contain the display name %q", displayName)
	}
	if !strings.Contains(formatted, profileText) {
		t.Errorf("Formatted prompt should contain the profile text %q", profileText)
	}

	// Verify that there are no unescaped % signs (other than the ones for arguments)
	// We can check this by counting the occurrences of "TestUser" and "TestProfile" in the output
	// If fmt.Sprintf had fewer verbs than arguments, it would append (EXTRA string=TestProfile)
	// If it had more verbs, it would append %!s(MISSING)

	if strings.Contains(formatted, "%!") {
		t.Errorf("Formatted prompt contains formatting errors (missing arguments?): %s", formatted)
	}
	if strings.Contains(formatted, "(EXTRA") {
		t.Errorf("Formatted prompt contains extra arguments errors: %s", formatted)
	}

	// Ensure the literal 100% is present (as 100%)
	expectedLiteral := "100%"
	if !strings.Contains(formatted, expectedLiteral) {
		t.Errorf("Formatted prompt should contain literal %q, but got: ...%s...", expectedLiteral, formatted)
	}
}
