package tools

import (
	"testing"
)

func TestValidator_Validate_RequiredFields(t *testing.T) {
	validator := NewValidator()
	schema := ParameterSchema{
		Type: "object",
		Properties: map[string]PropertySchema{
			"name": {Type: "string"},
			"age":  {Type: "integer"},
		},
		Required: []string{"name", "age"},
	}

	_, errors := validator.Validate(schema, map[string]any{"name": "John"})
	if len(errors) == 0 {
		t.Error("expected validation error for missing required field")
	}
	if errors[0].Code != "required" {
		t.Errorf("expected 'required' error code, got '%s'", errors[0].Code)
	}
	if errors[0].Field != "age" {
		t.Errorf("expected error for 'age' field, got '%s'", errors[0].Field)
	}
}

func TestValidator_Validate_StringType(t *testing.T) {
	validator := NewValidator()
	schema := ParameterSchema{
		Type: "object",
		Properties: map[string]PropertySchema{
			"name": {Type: "string"},
		},
	}

	params, errors := validator.Validate(schema, map[string]any{"name": "John"})
	if len(errors) != 0 {
		t.Errorf("unexpected errors: %v", errors)
	}
	if params["name"] != "John" {
		t.Errorf("expected 'John', got '%v'", params["name"])
	}
}

func TestValidator_Validate_StringEnum(t *testing.T) {
	validator := NewValidator()
	schema := ParameterSchema{
		Type: "object",
		Properties: map[string]PropertySchema{
			"status": {
				Type: "string",
				Enum: []string{"active", "inactive", "pending"},
			},
		},
	}

	_, errors := validator.Validate(schema, map[string]any{"status": "unknown"})
	if len(errors) == 0 {
		t.Error("expected enum validation error")
	}
	if errors[0].Code != "enum_violation" {
		t.Errorf("expected 'enum_violation' error code, got '%s'", errors[0].Code)
	}
}

func TestValidator_Validate_IntegerCoercion(t *testing.T) {
	validator := NewValidator()
	schema := ParameterSchema{
		Type: "object",
		Properties: map[string]PropertySchema{
			"count": {Type: "integer"},
		},
	}

	params, errors := validator.Validate(schema, map[string]any{"count": "42"})
	if len(errors) != 0 {
		t.Errorf("unexpected errors: %v", errors)
	}
	if params["count"] != int64(42) {
		t.Errorf("expected int64(42), got %v (%T)", params["count"], params["count"])
	}
}

func TestValidator_Validate_IntegerFromFloat(t *testing.T) {
	validator := NewValidator()
	schema := ParameterSchema{
		Type: "object",
		Properties: map[string]PropertySchema{
			"count": {Type: "integer"},
		},
	}

	params, errors := validator.Validate(schema, map[string]any{"count": 42.0})
	if len(errors) != 0 {
		t.Errorf("unexpected errors: %v", errors)
	}
	if params["count"] != int64(42) {
		t.Errorf("expected int64(42), got %v", params["count"])
	}

	_, errors = validator.Validate(schema, map[string]any{"count": 42.5})
	if len(errors) == 0 {
		t.Error("expected error for non-whole float")
	}
}

func TestValidator_Validate_NumberCoercion(t *testing.T) {
	validator := NewValidator()
	schema := ParameterSchema{
		Type: "object",
		Properties: map[string]PropertySchema{
			"price": {Type: "number"},
		},
	}

	params, errors := validator.Validate(schema, map[string]any{"price": "19.99"})
	if len(errors) != 0 {
		t.Errorf("unexpected errors: %v", errors)
	}
	if params["price"] != 19.99 {
		t.Errorf("expected 19.99, got %v", params["price"])
	}

	params, errors = validator.Validate(schema, map[string]any{"price": 42})
	if len(errors) != 0 {
		t.Errorf("unexpected errors: %v", errors)
	}
	if params["price"] != 42.0 {
		t.Errorf("expected 42.0, got %v", params["price"])
	}
}

func TestValidator_Validate_BooleanCoercion(t *testing.T) {
	validator := NewValidator()
	schema := ParameterSchema{
		Type: "object",
		Properties: map[string]PropertySchema{
			"enabled": {Type: "boolean"},
		},
	}

	tests := []struct {
		input    any
		expected bool
	}{
		{"true", true},
		{"false", false},
		{"yes", true},
		{"no", false},
		{"1", true},
		{"0", false},
		{1, true},
		{0, false},
	}

	for _, tc := range tests {
		params, errors := validator.Validate(schema, map[string]any{"enabled": tc.input})
		if len(errors) != 0 {
			t.Errorf("input %v: unexpected errors: %v", tc.input, errors)
			continue
		}
		if params["enabled"] != tc.expected {
			t.Errorf("input %v: expected %v, got %v", tc.input, tc.expected, params["enabled"])
		}
	}
}

func TestValidator_Validate_ArrayValidation(t *testing.T) {
	validator := NewValidator()
	schema := ParameterSchema{
		Type: "object",
		Properties: map[string]PropertySchema{
			"tags": {
				Type:  "array",
				Items: &PropertySchema{Type: "string"},
			},
		},
	}

	params, errors := validator.Validate(schema, map[string]any{
		"tags": []any{"go", "test", "validator"},
	})
	if len(errors) != 0 {
		t.Errorf("unexpected errors: %v", errors)
	}
	tags, ok := params["tags"].([]any)
	if !ok {
		t.Error("expected array result")
	}
	if len(tags) != 3 {
		t.Errorf("expected 3 items, got %d", len(tags))
	}
}

func TestValidator_Validate_ArrayItemError(t *testing.T) {
	validator := NewValidator()
	schema := ParameterSchema{
		Type: "object",
		Properties: map[string]PropertySchema{
			"numbers": {
				Type:  "array",
				Items: &PropertySchema{Type: "integer"},
			},
		},
	}

	_, errors := validator.Validate(schema, map[string]any{
		"numbers": []any{1, "not-a-number", 3},
	})
	if len(errors) == 0 {
		t.Error("expected validation error for array item")
	}
	if errors[0].Field != "numbers[1]" {
		t.Errorf("expected error for 'numbers[1]', got '%s'", errors[0].Field)
	}
}

func TestValidator_Validate_NestedObject(t *testing.T) {
	validator := NewValidator()
	schema := ParameterSchema{
		Type: "object",
		Properties: map[string]PropertySchema{
			"user": {
				Type: "object",
				Properties: map[string]PropertySchema{
					"name": {Type: "string"},
					"age":  {Type: "integer"},
				},
				Required: []string{"name"},
			},
		},
	}

	params, errors := validator.Validate(schema, map[string]any{
		"user": map[string]any{
			"name": "John",
			"age":  30,
		},
	})
	if len(errors) != 0 {
		t.Errorf("unexpected errors: %v", errors)
	}
	user, ok := params["user"].(map[string]any)
	if !ok {
		t.Error("expected object result")
	}
	if user["name"] != "John" {
		t.Errorf("expected 'John', got '%v'", user["name"])
	}
}

func TestValidator_Validate_NestedObjectRequired(t *testing.T) {
	validator := NewValidator()
	schema := ParameterSchema{
		Type: "object",
		Properties: map[string]PropertySchema{
			"user": {
				Type: "object",
				Properties: map[string]PropertySchema{
					"name": {Type: "string"},
				},
				Required: []string{"name"},
			},
		},
	}

	_, errors := validator.Validate(schema, map[string]any{
		"user": map[string]any{},
	})
	if len(errors) == 0 {
		t.Error("expected validation error for nested required field")
	}
	if errors[0].Field != "user.name" {
		t.Errorf("expected error for 'user.name', got '%s'", errors[0].Field)
	}
}

func TestValidator_Validate_UnknownFieldReject(t *testing.T) {
	validator := NewValidator(WithRejectUnknownFields(true))
	schema := ParameterSchema{
		Type: "object",
		Properties: map[string]PropertySchema{
			"name": {Type: "string"},
		},
	}

	_, errors := validator.Validate(schema, map[string]any{
		"name":    "John",
		"unknown": "field",
	})
	if len(errors) == 0 {
		t.Error("expected error for unknown field")
	}
	if errors[0].Code != "unknown_field" {
		t.Errorf("expected 'unknown_field' error code, got '%s'", errors[0].Code)
	}
}

func TestValidator_Validate_UnknownFieldAllow(t *testing.T) {
	validator := NewValidator(WithAllowUnknownFields(true))
	schema := ParameterSchema{
		Type: "object",
		Properties: map[string]PropertySchema{
			"name": {Type: "string"},
		},
	}

	params, errors := validator.Validate(schema, map[string]any{
		"name":    "John",
		"unknown": "field",
	})
	if len(errors) != 0 {
		t.Errorf("unexpected errors: %v", errors)
	}
	if params["unknown"] != "field" {
		t.Error("expected unknown field to be preserved")
	}
}

func TestValidator_Validate_NoCoercion(t *testing.T) {
	validator := NewValidator(WithCoerceTypes(false))
	schema := ParameterSchema{
		Type: "object",
		Properties: map[string]PropertySchema{
			"count": {Type: "integer"},
		},
	}

	_, errors := validator.Validate(schema, map[string]any{"count": "42"})
	if len(errors) == 0 {
		t.Error("expected type mismatch error when coercion is disabled")
	}
	if errors[0].Code != "type_mismatch" {
		t.Errorf("expected 'type_mismatch' error code, got '%s'", errors[0].Code)
	}
}

func TestValidationError_ToLLMMessage(t *testing.T) {
	tests := []struct {
		err      ValidationError
		expected string
	}{
		{
			err: ValidationError{
				Field:   "name",
				Message: "required field is missing",
				Code:    "required",
			},
			expected: "Field 'name': required field is missing",
		},
		{
			err: ValidationError{
				Field:   "status",
				Message: "value must be one of: active, inactive",
				Code:    "enum_violation",
				Value:   "unknown",
			},
			expected: "Field 'status': value must be one of: active, inactive Got value: unknown",
		},
		{
			err: ValidationError{
				Message: "validation failed",
				Code:    "general",
			},
			expected: "validation failed",
		},
	}

	for _, tc := range tests {
		got := tc.err.ToLLMMessage()
		if got != tc.expected {
			t.Errorf("expected '%s', got '%s'", tc.expected, got)
		}
	}
}

func TestValidationErrors_ToLLMMessage(t *testing.T) {
	errors := ValidationErrors{
		{Field: "name", Message: "required field is missing", Code: "required"},
		{Field: "age", Message: "must be a positive integer", Code: "constraint"},
	}

	msg := errors.ToLLMMessage()
	if msg == "" {
		t.Error("expected non-empty message")
	}
	if !contains(msg, "name") || !contains(msg, "age") {
		t.Errorf("expected message to contain both fields, got: %s", msg)
	}
}

func TestValidator_Validate_NilParams(t *testing.T) {
	validator := NewValidator()
	schema := ParameterSchema{
		Type: "object",
		Properties: map[string]PropertySchema{
			"name": {Type: "string"},
		},
		Required: []string{"name"},
	}

	params, errors := validator.Validate(schema, nil)
	if len(errors) == 0 {
		t.Error("expected validation error for missing required field")
	}
	if params == nil {
		t.Error("expected non-nil params result")
	}
}

func TestValidator_Validate_EmptySchema(t *testing.T) {
	validator := NewValidator()
	schema := ParameterSchema{
		Type:       "object",
		Properties: map[string]PropertySchema{},
	}

	params, errors := validator.Validate(schema, map[string]any{"anything": "allowed"})
	if len(errors) != 0 {
		t.Errorf("unexpected errors: %v", errors)
	}
	if params["anything"] != "allowed" {
		t.Error("expected any field to be allowed with empty schema")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
