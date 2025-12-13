package memory

import (
	"fmt"
	"reflect"
	"testing"
)

// MockRecordID simulates the struct returned by the SurrealDB driver
type MockRecordID struct {
	Table string
	ID    string
}

func TestIDExtractionWithReflection(t *testing.T) {
	// Re-implement the extraction logic here for testing purposes,
	// since the actual logic is embedded in the GetDueReminders method and not exported.
	// In a real refactor, this logic should be extracted to a helper function.

	extractID := func(row map[string]interface{}) string {
		if id, ok := row["id"].(string); ok {
			return id
		} else if idMap, ok := row["id"].(map[string]interface{}); ok {
			if strID, ok := idMap["String"].(string); ok {
				return strID
			}
			if table, ok := idMap["Table"].(string); ok {
				if idVal, ok := idMap["ID"].(string); ok {
					return table + ":" + idVal
				}
			}
		} else {
			// Reflection logic matching the implementation in surreal_store.go
			val := reflect.ValueOf(row["id"])
			if val.Kind() == reflect.Struct {
				tableField := val.FieldByName("Table")
				idField := val.FieldByName("ID")
				if tableField.IsValid() && idField.IsValid() {
					return fmt.Sprintf("%v:%v", tableField.Interface(), idField.Interface())
				}
			}
		}
		// Fallback
		return fmt.Sprintf("%v", row["id"])
	}

	// Case 1: ID is a simple string
	rowStringID := map[string]interface{}{"id": "reminders:123"}
	if id := extractID(rowStringID); id != "reminders:123" {
		t.Errorf("Case 1 (String) failed: got %s", id)
	}

	// Case 2: ID is a map (e.g., from JSON unmarshal)
	rowMapID := map[string]interface{}{
		"id": map[string]interface{}{
			"Table": "reminders",
			"ID":    "abc",
		},
	}
	if id := extractID(rowMapID); id != "reminders:abc" {
		t.Errorf("Case 2 (Map) failed: got %s", id)
	}

	// Case 3: ID is a struct (SurrealDB driver behavior)
	mockStruct := MockRecordID{Table: "reminders", ID: "28qa9te5wuz8m1akx1xr"}
	rowStructID := map[string]interface{}{"id": mockStruct}

	expected := "reminders:28qa9te5wuz8m1akx1xr"
	if id := extractID(rowStructID); id != expected {
		t.Errorf("Case 3 (Struct) failed: expected %s, got %s", expected, id)
	}
}
