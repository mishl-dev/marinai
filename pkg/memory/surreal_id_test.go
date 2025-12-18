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
	// Use the package-level helper function 'extractID' which is now defined in surreal_store.go
	// Since we are in the same package (memory), we can access it directly (if unexported).

	// Case 1: ID is a simple string
	rowStringID := map[string]interface{}{"id": "reminders:123"}
	assert.Equal(t, "reminders:123", extractID(rowStringID["id"]), "Case 1 (String) failed")

	// Case 2: ID is a map (e.g., from JSON unmarshal)
	rowMapID := map[string]interface{}{
		"id": map[string]interface{}{
			"Table": "reminders",
			"ID":    "abc",
		},
	}
	assert.Equal(t, "reminders:abc", extractID(rowMapID["id"]), "Case 2 (Map) failed")

	// Case 3: ID is a struct (SurrealDB driver behavior)
	mockStruct := MockRecordID{Table: "reminders", ID: "28qa9te5wuz8m1akx1xr"}
	rowStructID := map[string]interface{}{"id": mockStruct}

	expected := "reminders:28qa9te5wuz8m1akx1xr"
	assert.Equal(t, expected, extractID(rowStructID["id"]), "Case 3 (Struct) failed")

	// Case 4: Fallback (should at least return string representation)
	rowIntID := map[string]interface{}{"id": 12345}
	assert.Equal(t, "12345", extractID(rowIntID["id"]), "Case 4 (Fallback) failed")
}

// Test extracting from nested map format used by some drivers
func TestIDExtraction_NestedMap(t *testing.T) {
	// Format: { "id": { "String": "reminders:123" } }
	rowNestedString := map[string]interface{}{
		"id": map[string]interface{}{
			"String": "reminders:123",
		},
	}
	assert.Equal(t, "reminders:123", extractID(rowNestedString["id"]))
}
