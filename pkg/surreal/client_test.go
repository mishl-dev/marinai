package surreal

import (
	"testing"
)

// Accessing private function for testing
func TestValidateIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Valid simple", "memories", false},
		{"Valid with underscore", "user_id", false},
		{"Valid with numbers", "field1", false},
		{"Valid with mixed case", "UserId", false},
		{"Invalid space", "user id", true},
		{"Invalid semicolon", "user;id", true},
		{"Invalid dash", "user-id", true},
		{"Invalid special char", "user$", true},
		{"Invalid SQL injection", "memories; DROP TABLE memories", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateIdentifier(tt.input); (err != nil) != tt.wantErr {
				t.Errorf("validateIdentifier() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildWhereClause(t *testing.T) {
	tests := []struct {
		name    string
		filter  map[string]interface{}
		want    string // We might need partial matching because map iteration order is random
		wantErr bool
	}{
		{
			name:    "Empty filter",
			filter:  map[string]interface{}{},
			want:    "true",
			wantErr: false,
		},
		{
			name:    "Single filter",
			filter:  map[string]interface{}{"user_id": "123"},
			want:    "user_id = $user_id",
			wantErr: false,
		},
		{
			name:    "Invalid key",
			filter:  map[string]interface{}{"user id": "123"},
			want:    "",
			wantErr: true,
		},
		{
			name:    "Injection key",
			filter:  map[string]interface{}{"id; --": "123"},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildWhereClause(tt.filter)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildWhereClause() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("buildWhereClause() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Since VectorSearch depends on a real DB connection (Client struct has *surrealdb.DB),
// and we don't have a mock easily available for the external library or a real DB instance,
// we trust the unit tests for validation logic and the integration logic in VectorSearch
// which calls these validated functions.
