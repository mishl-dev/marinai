package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockRecordID simulates the struct returned by the SurrealDB driver
type MockRecordID struct {
	Table string
	ID    string
}

func TestIDExtractionWithReflection(t *testing.T) {
	// We now test the internal helper function `extractID` directly.
	// Note: Since we are in package `memory`, we can access unexported functions.

	// Case 1: ID is a simple string
	rowStringID := map[string]interface{}{"id": "reminders:123"}
	assert.Equal(t, "reminders:123", extractID(rowStringID), "Case 1 (String) failed")

	// Case 2: ID is a map (e.g., from JSON unmarshal)
	rowMapID := map[string]interface{}{
		"id": map[string]interface{}{
			"Table": "reminders",
			"ID":    "abc",
		},
	}
	assert.Equal(t, "reminders:abc", extractID(rowMapID), "Case 2 (Map) failed")

	// Case 3: ID is a struct (SurrealDB driver behavior)
	mockStruct := MockRecordID{Table: "reminders", ID: "28qa9te5wuz8m1akx1xr"}
	rowStructID := map[string]interface{}{"id": mockStruct}

	expected := "reminders:28qa9te5wuz8m1akx1xr"
	assert.Equal(t, expected, extractID(rowStructID), "Case 3 (Struct) failed")
}
