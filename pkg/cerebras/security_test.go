package cerebras

import (
	"strings"
	"testing"
)

func TestAPIErrorTruncation(t *testing.T) {
	// Create a huge body
	hugeBody := strings.Repeat("A", 5000)

	err := &APIError{
		StatusCode: 400,
		Body:       hugeBody,
	}

	errMsg := err.Error()

	// Verification: The error message should now be truncated
	if len(errMsg) < 1000 && strings.Contains(errMsg, "(truncated)") {
		t.Logf("Success: Error message length is %d and contains truncation marker", len(errMsg))
	} else {
		t.Errorf("Failure: Error message length is %d", len(errMsg))
	}
}
