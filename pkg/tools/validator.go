package tools

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
	Value   any    `json:"value,omitempty"`
}

func (e ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return e.Message
}

func (e ValidationError) ToLLMMessage() string {
	var sb strings.Builder
	if e.Field != "" {
		sb.WriteString("Field '")
		sb.WriteString(e.Field)
		sb.WriteString("': ")
	}
	sb.WriteString(e.Message)
	if e.Value != nil {
		sb.WriteString(" Got value: ")
		sb.WriteString(fmt.Sprintf("%v", e.Value))
	}
	return sb.String()
}

type ValidationErrors []ValidationError

func (errs ValidationErrors) Error() string {
	if len(errs) == 0 {
		return ""
	}
	var messages []string
	for _, e := range errs {
		messages = append(messages, e.Error())
	}
	return strings.Join(messages, "; ")
}

func (errs ValidationErrors) ToLLMMessage() string {
	if len(errs) == 0 {
		return ""
	}
	var messages []string
	for _, e := range errs {
		messages = append(messages, e.ToLLMMessage())
	}
	return strings.Join(messages, "\n")
}

type ValidatorConfig struct {
	AllowUnknownFields  bool
	CoerceTypes         bool
	RejectUnknownFields bool
}

type Validator struct {
	config ValidatorConfig
}

func NewValidator(opts ...func(*ValidatorConfig)) *Validator {
	config := ValidatorConfig{
		AllowUnknownFields:  true,
		CoerceTypes:         true,
		RejectUnknownFields: false,
	}
	for _, opt := range opts {
		opt(&config)
	}
	return &Validator{config: config}
}

func WithAllowUnknownFields(allow bool) func(*ValidatorConfig) {
	return func(c *ValidatorConfig) {
		c.AllowUnknownFields = allow
	}
}

func WithCoerceTypes(coerce bool) func(*ValidatorConfig) {
	return func(c *ValidatorConfig) {
		c.CoerceTypes = coerce
	}
}

func WithRejectUnknownFields(reject bool) func(*ValidatorConfig) {
	return func(c *ValidatorConfig) {
		c.RejectUnknownFields = reject
		c.AllowUnknownFields = !reject
	}
}

func (v *Validator) Validate(schema ParameterSchema, params map[string]any) (map[string]any, ValidationErrors) {
	var errors ValidationErrors
	result := make(map[string]any)

	if params == nil {
		params = make(map[string]any)
	}

	requiredSet := make(map[string]bool)
	for _, r := range schema.Required {
		requiredSet[r] = true
	}

	for _, fieldName := range schema.Required {
		if _, exists := params[fieldName]; !exists {
			errors = append(errors, ValidationError{
				Field:   fieldName,
				Message: "required field is missing",
				Code:    "required",
			})
		}
	}

	for key, value := range params {
		fieldPath := key

		propSchema, hasSchema := schema.Properties[key]
		if !hasSchema {
			if v.config.RejectUnknownFields {
				errors = append(errors, ValidationError{
					Field:   fieldPath,
					Message: "unknown field not allowed by schema",
					Code:    "unknown_field",
					Value:   value,
				})
			}
			result[key] = value
			continue
		}

		validated, propErrors := v.validateProperty(fieldPath, value, propSchema)
		if len(propErrors) > 0 {
			errors = append(errors, propErrors...)
		}
		result[key] = validated
	}

	return result, errors
}

func (v *Validator) validateProperty(path string, value any, schema PropertySchema) (any, ValidationErrors) {
	var errors ValidationErrors
	validated := value

	switch schema.Type {
	case "string":
		validated, errors = v.validateString(path, value, schema)
	case "integer":
		validated, errors = v.validateInteger(path, value, schema)
	case "number":
		validated, errors = v.validateNumber(path, value, schema)
	case "boolean":
		validated, errors = v.validateBoolean(path, value, schema)
	case "array":
		validated, errors = v.validateArray(path, value, schema)
	case "object":
		validated, errors = v.validateObject(path, value, schema)
	default:
		if schema.Type != "" {
			errors = append(errors, ValidationError{
				Field:   path,
				Message: fmt.Sprintf("unknown type '%s'", schema.Type),
				Code:    "unknown_type",
				Value:   value,
			})
		}
	}

	return validated, errors
}

func (v *Validator) validateString(path string, value any, schema PropertySchema) (any, ValidationErrors) {
	var errors ValidationErrors

	str, ok := value.(string)
	if !ok {
		if v.config.CoerceTypes {
			str = fmt.Sprintf("%v", value)
		} else {
			errors = append(errors, ValidationError{
				Field:   path,
				Message: fmt.Sprintf("expected string, got %s", typeName(value)),
				Code:    "type_mismatch",
				Value:   value,
			})
			return value, errors
		}
	}

	if len(schema.Enum) > 0 {
		found := false
		for _, enumVal := range schema.Enum {
			if str == enumVal {
				found = true
				break
			}
		}
		if !found {
			errors = append(errors, ValidationError{
				Field:   path,
				Message: fmt.Sprintf("value must be one of: %s", strings.Join(schema.Enum, ", ")),
				Code:    "enum_violation",
				Value:   str,
			})
		}
	}

	return str, errors
}

func (v *Validator) validateInteger(path string, value any, schema PropertySchema) (any, ValidationErrors) {
	var errors ValidationErrors
	var result int64

	switch val := value.(type) {
	case int:
		result = int64(val)
	case int8:
		result = int64(val)
	case int16:
		result = int64(val)
	case int32:
		result = int64(val)
	case int64:
		result = val
	case uint:
		result = int64(val)
	case uint8:
		result = int64(val)
	case uint16:
		result = int64(val)
	case uint32:
		result = int64(val)
	case uint64:
		if val > math.MaxInt64 {
			errors = append(errors, ValidationError{
				Field:   path,
				Message: "integer value exceeds maximum int64",
				Code:    "overflow",
				Value:   value,
			})
			return value, errors
		}
		result = int64(val)
	case float32:
		if v.config.CoerceTypes && isWholeNumber(float64(val)) {
			result = int64(val)
		} else {
			errors = append(errors, ValidationError{
				Field:   path,
				Message: fmt.Sprintf("expected integer, got float %v", val),
				Code:    "type_mismatch",
				Value:   value,
			})
			return value, errors
		}
	case float64:
		if v.config.CoerceTypes && isWholeNumber(val) {
			result = int64(val)
		} else {
			errors = append(errors, ValidationError{
				Field:   path,
				Message: fmt.Sprintf("expected integer, got float %v", val),
				Code:    "type_mismatch",
				Value:   value,
			})
			return value, errors
		}
	case string:
		if v.config.CoerceTypes {
			parsed, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				errors = append(errors, ValidationError{
					Field:   path,
					Message: fmt.Sprintf("cannot coerce string '%s' to integer", val),
					Code:    "coercion_failed",
					Value:   value,
				})
				return value, errors
			}
			result = parsed
		} else {
			errors = append(errors, ValidationError{
				Field:   path,
				Message: fmt.Sprintf("expected integer, got string '%s'", val),
				Code:    "type_mismatch",
				Value:   value,
			})
			return value, errors
		}
	case bool:
		errors = append(errors, ValidationError{
			Field:   path,
			Message: "cannot convert boolean to integer",
			Code:    "type_mismatch",
			Value:   value,
		})
		return value, errors
	default:
		errors = append(errors, ValidationError{
			Field:   path,
			Message: fmt.Sprintf("expected integer, got %s", typeName(value)),
			Code:    "type_mismatch",
			Value:   value,
		})
		return value, errors
	}

	if len(schema.Enum) > 0 {
		strVal := strconv.FormatInt(result, 10)
		found := false
		for _, enumVal := range schema.Enum {
			if strVal == enumVal {
				found = true
				break
			}
		}
		if !found {
			errors = append(errors, ValidationError{
				Field:   path,
				Message: fmt.Sprintf("value must be one of: %s", strings.Join(schema.Enum, ", ")),
				Code:    "enum_violation",
				Value:   result,
			})
		}
	}

	return result, errors
}

func (v *Validator) validateNumber(path string, value any, schema PropertySchema) (any, ValidationErrors) {
	var errors ValidationErrors
	var result float64

	switch val := value.(type) {
	case int:
		result = float64(val)
	case int8:
		result = float64(val)
	case int16:
		result = float64(val)
	case int32:
		result = float64(val)
	case int64:
		result = float64(val)
	case uint:
		result = float64(val)
	case uint8:
		result = float64(val)
	case uint16:
		result = float64(val)
	case uint32:
		result = float64(val)
	case uint64:
		result = float64(val)
	case float32:
		result = float64(val)
	case float64:
		result = val
	case string:
		if v.config.CoerceTypes {
			parsed, err := strconv.ParseFloat(val, 64)
			if err != nil {
				errors = append(errors, ValidationError{
					Field:   path,
					Message: fmt.Sprintf("cannot coerce string '%s' to number", val),
					Code:    "coercion_failed",
					Value:   value,
				})
				return value, errors
			}
			result = parsed
		} else {
			errors = append(errors, ValidationError{
				Field:   path,
				Message: fmt.Sprintf("expected number, got string '%s'", val),
				Code:    "type_mismatch",
				Value:   value,
			})
			return value, errors
		}
	case bool:
		errors = append(errors, ValidationError{
			Field:   path,
			Message: "cannot convert boolean to number",
			Code:    "type_mismatch",
			Value:   value,
		})
		return value, errors
	default:
		errors = append(errors, ValidationError{
			Field:   path,
			Message: fmt.Sprintf("expected number, got %s", typeName(value)),
			Code:    "type_mismatch",
			Value:   value,
		})
		return value, errors
	}

	if len(schema.Enum) > 0 {
		strVal := strconv.FormatFloat(result, 'f', -1, 64)
		found := false
		for _, enumVal := range schema.Enum {
			if strVal == enumVal {
				found = true
				break
			}
		}
		if !found {
			errors = append(errors, ValidationError{
				Field:   path,
				Message: fmt.Sprintf("value must be one of: %s", strings.Join(schema.Enum, ", ")),
				Code:    "enum_violation",
				Value:   result,
			})
		}
	}

	return result, errors
}

func (v *Validator) validateBoolean(path string, value any, schema PropertySchema) (any, ValidationErrors) {
	var errors ValidationErrors

	switch val := value.(type) {
	case bool:
		return val, nil
	case string:
		if v.config.CoerceTypes {
			switch strings.ToLower(val) {
			case "true", "1", "yes", "on":
				return true, nil
			case "false", "0", "no", "off":
				return false, nil
			default:
				errors = append(errors, ValidationError{
					Field:   path,
					Message: fmt.Sprintf("cannot coerce string '%s' to boolean", val),
					Code:    "coercion_failed",
					Value:   value,
				})
				return value, errors
			}
		}
		errors = append(errors, ValidationError{
			Field:   path,
			Message: fmt.Sprintf("expected boolean, got string '%s'", val),
			Code:    "type_mismatch",
			Value:   value,
		})
		return value, errors
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		if v.config.CoerceTypes {
			intVal := reflect.ValueOf(value).Int()
			return intVal != 0, nil
		}
		errors = append(errors, ValidationError{
			Field:   path,
			Message: fmt.Sprintf("expected boolean, got integer '%v'", value),
			Code:    "type_mismatch",
			Value:   value,
		})
		return value, errors
	case float32, float64:
		if v.config.CoerceTypes {
			floatVal := reflect.ValueOf(value).Float()
			return floatVal != 0, nil
		}
		errors = append(errors, ValidationError{
			Field:   path,
			Message: fmt.Sprintf("expected boolean, got number '%v'", value),
			Code:    "type_mismatch",
			Value:   value,
		})
		return value, errors
	default:
		errors = append(errors, ValidationError{
			Field:   path,
			Message: fmt.Sprintf("expected boolean, got %s", typeName(value)),
			Code:    "type_mismatch",
			Value:   value,
		})
		return value, errors
	}
}

func (v *Validator) validateArray(path string, value any, schema PropertySchema) (any, ValidationErrors) {
	var errors ValidationErrors

	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		errors = append(errors, ValidationError{
			Field:   path,
			Message: fmt.Sprintf("expected array, got %s", typeName(value)),
			Code:    "type_mismatch",
			Value:   value,
		})
		return value, errors
	}

	length := rv.Len()
	result := make([]any, length)

	itemsSchema := schema.Items
	if itemsSchema == nil {
		for i := 0; i < length; i++ {
			result[i] = rv.Index(i).Interface()
		}
		return result, nil
	}

	for i := 0; i < length; i++ {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		item := rv.Index(i).Interface()
		validated, itemErrors := v.validateProperty(itemPath, item, *itemsSchema)
		if len(itemErrors) > 0 {
			errors = append(errors, itemErrors...)
		}
		result[i] = validated
	}

	return result, errors
}

func (v *Validator) validateObject(path string, value any, schema PropertySchema) (any, ValidationErrors) {
	var errors ValidationErrors

	obj, ok := value.(map[string]any)
	if !ok {
		rv := reflect.ValueOf(value)
		if rv.Kind() == reflect.Map {
			result := make(map[string]any)
			iter := rv.MapRange()
			for iter.Next() {
				key := iter.Key()
				if key.Kind() == reflect.String {
					result[key.String()] = iter.Value().Interface()
				}
			}
			obj = result
		} else {
			errors = append(errors, ValidationError{
				Field:   path,
				Message: fmt.Sprintf("expected object, got %s", typeName(value)),
				Code:    "type_mismatch",
				Value:   value,
			})
			return value, errors
		}
	}

	if len(schema.Properties) == 0 {
		return obj, nil
	}

	result := make(map[string]any)

	requiredSet := make(map[string]bool)
	for _, r := range schema.Required {
		requiredSet[r] = true
	}

	for _, fieldName := range schema.Required {
		if _, exists := obj[fieldName]; !exists {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("%s.%s", path, fieldName),
				Message: "required field is missing",
				Code:    "required",
			})
		}
	}

	for key, val := range obj {
		fieldPath := fmt.Sprintf("%s.%s", path, key)

		propSchema, hasSchema := schema.Properties[key]
		if !hasSchema {
			if v.config.RejectUnknownFields {
				errors = append(errors, ValidationError{
					Field:   fieldPath,
					Message: "unknown field not allowed by schema",
					Code:    "unknown_field",
					Value:   val,
				})
			}
			result[key] = val
			continue
		}

		validated, propErrors := v.validateProperty(fieldPath, val, propSchema)
		if len(propErrors) > 0 {
			errors = append(errors, propErrors...)
		}
		result[key] = validated
	}

	return result, errors
}

func typeName(value any) string {
	if value == nil {
		return "null"
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "integer"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Bool:
		return "boolean"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map:
		return "object"
	case reflect.Struct:
		return "object"
	default:
		return rv.Type().String()
	}
}

func isWholeNumber(f float64) bool {
	return f == math.Trunc(f) && !math.IsInf(f, 0)
}
