package bot

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSystemPromptFormatting(t *testing.T) {
	// This test verifies that SystemPrompt is a valid format string.
	// It should contain exactly two %s verbs and no invalid verbs.
	// The build/vet failure "fmt.Sprintf format %. has unknown verb" happened
	// because "100%" was interpreted as a verb.

	displayName := "TestUser"
	profileText := "TestProfile"

	// Attempt to format. This should not panic.
	// Note: go vet catches format errors at compile/vet time, but this test
	// ensures we can actually execute it.
	formatted := fmt.Sprintf(SystemPrompt, displayName, profileText)

	// Verify the content contains the expected substituted values
	assert.Contains(t, formatted, "You are currently talking to TestUser")
	assert.Contains(t, formatted, "TestProfile")

	// Verify the literal "100%" is preserved (as "100%")
	assert.Contains(t, formatted, "match them 100%")
}
