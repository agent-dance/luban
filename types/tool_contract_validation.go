package types

import (
	"encoding/json"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ToolContractPhase identifies the boundary that rejected an invocation.
// Values are stable machine identifiers and are safe to persist in audit data.
type ToolContractPhase string

const (
	ToolContractPhaseInputAdmission ToolContractPhase = "input_admission"
	ToolContractPhaseSchema         ToolContractPhase = "schema"
)

// ToolContractViolationCode classifies an input-contract failure without
// retaining or rendering the rejected value.
type ToolContractViolationCode string

const (
	ToolContractViolationRequired             ToolContractViolationCode = "required"
	ToolContractViolationUnexpectedProperty   ToolContractViolationCode = "unexpected_property"
	ToolContractViolationType                 ToolContractViolationCode = "type"
	ToolContractViolationEnum                 ToolContractViolationCode = "enum"
	ToolContractViolationMinimum              ToolContractViolationCode = "minimum"
	ToolContractViolationMaximum              ToolContractViolationCode = "maximum"
	ToolContractViolationExclusiveMinimum     ToolContractViolationCode = "exclusive_minimum"
	ToolContractViolationExclusiveMaximum     ToolContractViolationCode = "exclusive_maximum"
	ToolContractViolationMinLength            ToolContractViolationCode = "min_length"
	ToolContractViolationMaxLength            ToolContractViolationCode = "max_length"
	ToolContractViolationPattern              ToolContractViolationCode = "pattern"
	ToolContractViolationMinItems             ToolContractViolationCode = "min_items"
	ToolContractViolationMaxItems             ToolContractViolationCode = "max_items"
	ToolContractViolationOneOf                ToolContractViolationCode = "one_of"
	ToolContractViolationAnyOf                ToolContractViolationCode = "any_of"
	ToolContractViolationNot                  ToolContractViolationCode = "not"
	ToolContractViolationInvalidSchema        ToolContractViolationCode = "invalid_schema"
	ToolContractViolationValidationDepthLimit ToolContractViolationCode = "validation_depth_limit"
)

// ToolContractViolation is a value-free validation diagnostic. InstancePath
// is an RFC 6901 JSON Pointer; Keyword identifies the schema assertion. No
// rejected values or schema literals are copied into the diagnostic.
type ToolContractViolation struct {
	Phase        ToolContractPhase         `json:"phase"`
	Code         ToolContractViolationCode `json:"code"`
	InstancePath string                    `json:"instance_path"`
	Keyword      string                    `json:"keyword"`
	Retryable    bool                      `json:"retryable"`
}

const maximumToolContractValidationDepth = 128

// ValidateToolInputSchema evaluates one decoded value against a tool's JSON
// Schema contract. The returned diagnostics are detached, machine-readable,
// and contain no rejected values.
func ValidateToolInputSchema(schema JSONSchema, input any) []ToolContractViolation {
	raw, err := json.Marshal(schema)
	if err != nil {
		return []ToolContractViolation{schemaViolation("", "schema")}
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return []ToolContractViolation{schemaViolation("", "schema")}
	}
	return validateToolContractNode(normalized, input, "", 0)
}

// ValidateToolDefinitionInput applies a provider-facing definition's local
// input contract. Provider adapters and runtime admission can therefore share
// the same validator instead of independently approximating the schema.
func ValidateToolDefinitionInput(definition ToolDefinition, input map[string]any) error {
	violations := ValidateToolInputSchema(definition.InputSchema, input)
	return newToolInputValidationError(definition.Name, violations)
}

func validateToolContractNode(schema, value any, path string, depth int) []ToolContractViolation {
	if depth > maximumToolContractValidationDepth {
		return []ToolContractViolation{inputViolation(ToolContractViolationValidationDepthLimit, path, "depth")}
	}
	if allowed, ok := schema.(bool); ok {
		if allowed {
			return nil
		}
		return []ToolContractViolation{inputViolation(ToolContractViolationNot, path, "falseSchema")}
	}
	node, ok := stringMap(schema)
	if !ok {
		return []ToolContractViolation{schemaViolation(path, "schema")}
	}

	violations := make([]ToolContractViolation, 0)
	if branches, present := schemaArray(node["allOf"]); present {
		for _, branch := range branches {
			violations = append(violations, validateToolContractNode(branch, value, path, depth+1)...)
		}
	}
	if branches, present := schemaArray(node["anyOf"]); present {
		matched := false
		for _, branch := range branches {
			if len(validateToolContractNode(branch, value, path, depth+1)) == 0 {
				matched = true
				break
			}
		}
		if !matched {
			violations = append(violations, inputViolation(ToolContractViolationAnyOf, path, "anyOf"))
		}
	}
	if branches, present := schemaArray(node["oneOf"]); present {
		matches := 0
		for _, branch := range branches {
			if len(validateToolContractNode(branch, value, path, depth+1)) == 0 {
				matches++
			}
		}
		if matches != 1 {
			violations = append(violations, inputViolation(ToolContractViolationOneOf, path, "oneOf"))
		}
	}
	if negated, present := node["not"]; present {
		if len(validateToolContractNode(negated, value, path, depth+1)) == 0 {
			violations = append(violations, inputViolation(ToolContractViolationNot, path, "not"))
		}
	}

	if enum, present := node["enum"].([]any); present && !matchesToolContractEnum(value, enum) {
		violations = append(violations, inputViolation(ToolContractViolationEnum, path, "enum"))
	}

	declaredTypes, schemaTypeValid := toolContractTypes(node["type"])
	if !schemaTypeValid {
		violations = append(violations, schemaViolation(path, "type"))
		return violations
	}
	if len(declaredTypes) > 0 && !matchesAnyToolContractType(value, declaredTypes) {
		violations = append(violations, inputViolation(ToolContractViolationType, path, "type"))
		return violations
	}

	if object, isObject := stringMap(value); isObject {
		violations = append(violations, validateToolContractObject(node, object, path, depth)...)
	}
	if array, isArray := anySlice(value); isArray {
		violations = append(violations, validateToolContractArray(node, array, path, depth)...)
	}
	if text, isString := value.(string); isString {
		violations = append(violations, validateToolContractString(node, text, path)...)
	}
	if number, isNumber := toolContractNumber(value); isNumber {
		violations = append(violations, validateToolContractNumber(node, number, path)...)
	}
	return violations
}

func validateToolContractObject(schema map[string]any, value map[string]any, path string, depth int) []ToolContractViolation {
	violations := make([]ToolContractViolation, 0)
	properties, propertiesValid := stringMap(schema["properties"])
	if schema["properties"] != nil && !propertiesValid {
		violations = append(violations, schemaViolation(path, "properties"))
		properties = nil
	}
	if required, present := schemaArray(schema["required"]); present {
		requiredNames := make([]string, 0, len(required))
		for _, item := range required {
			name, ok := item.(string)
			if !ok {
				violations = append(violations, schemaViolation(path, "required"))
				continue
			}
			requiredNames = append(requiredNames, name)
		}
		sort.Strings(requiredNames)
		for _, name := range requiredNames {
			if _, exists := value[name]; !exists {
				violations = append(violations, inputViolation(ToolContractViolationRequired, appendJSONPointer(path, name), "required"))
			}
		}
	}

	propertyNames := make([]string, 0, len(value))
	for name := range value {
		propertyNames = append(propertyNames, name)
	}
	sort.Strings(propertyNames)
	for _, name := range propertyNames {
		childPath := appendJSONPointer(path, name)
		if childSchema, declared := properties[name]; declared {
			violations = append(violations, validateToolContractNode(childSchema, value[name], childPath, depth+1)...)
			continue
		}
		switch additional := schema["additionalProperties"].(type) {
		case bool:
			if !additional {
				violations = append(violations, inputViolation(ToolContractViolationUnexpectedProperty, childPath, "additionalProperties"))
			}
		case nil:
			// JSON Schema objects are open unless explicitly closed.
		default:
			violations = append(violations, validateToolContractNode(additional, value[name], childPath, depth+1)...)
		}
	}
	return violations
}

func validateToolContractArray(schema map[string]any, value []any, path string, depth int) []ToolContractViolation {
	violations := make([]ToolContractViolation, 0)
	if limit, ok := nonNegativeSchemaInteger(schema["minItems"]); ok && len(value) < limit {
		violations = append(violations, inputViolation(ToolContractViolationMinItems, path, "minItems"))
	} else if schema["minItems"] != nil && !ok {
		violations = append(violations, schemaViolation(path, "minItems"))
	}
	if limit, ok := nonNegativeSchemaInteger(schema["maxItems"]); ok && len(value) > limit {
		violations = append(violations, inputViolation(ToolContractViolationMaxItems, path, "maxItems"))
	} else if schema["maxItems"] != nil && !ok {
		violations = append(violations, schemaViolation(path, "maxItems"))
	}
	if items, present := schema["items"]; present {
		for index, item := range value {
			violations = append(violations, validateToolContractNode(items, item, appendJSONPointer(path, strconv.Itoa(index)), depth+1)...)
		}
	}
	return violations
}

func validateToolContractString(schema map[string]any, value, path string) []ToolContractViolation {
	violations := make([]ToolContractViolation, 0)
	length := utf8.RuneCountInString(value)
	if limit, ok := nonNegativeSchemaInteger(schema["minLength"]); ok && length < limit {
		violations = append(violations, inputViolation(ToolContractViolationMinLength, path, "minLength"))
	} else if schema["minLength"] != nil && !ok {
		violations = append(violations, schemaViolation(path, "minLength"))
	}
	if limit, ok := nonNegativeSchemaInteger(schema["maxLength"]); ok && length > limit {
		violations = append(violations, inputViolation(ToolContractViolationMaxLength, path, "maxLength"))
	} else if schema["maxLength"] != nil && !ok {
		violations = append(violations, schemaViolation(path, "maxLength"))
	}
	if pattern, present := schema["pattern"]; present {
		expression, ok := pattern.(string)
		if !ok {
			violations = append(violations, schemaViolation(path, "pattern"))
		} else if compiled, err := regexp.Compile(expression); err != nil {
			violations = append(violations, schemaViolation(path, "pattern"))
		} else if !compiled.MatchString(value) {
			violations = append(violations, inputViolation(ToolContractViolationPattern, path, "pattern"))
		}
	}
	return violations
}

func validateToolContractNumber(schema map[string]any, value float64, path string) []ToolContractViolation {
	violations := make([]ToolContractViolation, 0)
	for _, bound := range []struct {
		keyword string
		code    ToolContractViolationCode
		fails   func(float64, float64) bool
	}{
		{"minimum", ToolContractViolationMinimum, func(value, limit float64) bool { return value < limit }},
		{"maximum", ToolContractViolationMaximum, func(value, limit float64) bool { return value > limit }},
		{"exclusiveMinimum", ToolContractViolationExclusiveMinimum, func(value, limit float64) bool { return value <= limit }},
		{"exclusiveMaximum", ToolContractViolationExclusiveMaximum, func(value, limit float64) bool { return value >= limit }},
	} {
		raw, present := schema[bound.keyword]
		if !present {
			continue
		}
		limit, ok := toolContractNumber(raw)
		if !ok {
			violations = append(violations, schemaViolation(path, bound.keyword))
		} else if bound.fails(value, limit) {
			violations = append(violations, inputViolation(bound.code, path, bound.keyword))
		}
	}
	return violations
}

func inputViolation(code ToolContractViolationCode, path, keyword string) ToolContractViolation {
	return ToolContractViolation{Phase: ToolContractPhaseInputAdmission, Code: code, InstancePath: path, Keyword: keyword, Retryable: true}
}

func schemaViolation(path, keyword string) ToolContractViolation {
	return ToolContractViolation{Phase: ToolContractPhaseSchema, Code: ToolContractViolationInvalidSchema, InstancePath: path, Keyword: keyword, Retryable: false}
}

func appendJSONPointer(path, token string) string {
	token = strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
	return path + "/" + token
}

func schemaArray(value any) ([]any, bool) {
	if value == nil {
		return nil, false
	}
	return anySlice(value)
}

func anySlice(value any) ([]any, bool) {
	if result, ok := value.([]any); ok {
		return result, true
	}
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() || (reflected.Kind() != reflect.Array && reflected.Kind() != reflect.Slice) {
		return nil, false
	}
	result := make([]any, reflected.Len())
	for index := range result {
		result[index] = reflected.Index(index).Interface()
	}
	return result, true
}

func stringMap(value any) (map[string]any, bool) {
	if result, ok := value.(map[string]any); ok {
		return result, true
	}
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() || reflected.Kind() != reflect.Map || reflected.Type().Key().Kind() != reflect.String {
		return nil, false
	}
	result := make(map[string]any, reflected.Len())
	iterator := reflected.MapRange()
	for iterator.Next() {
		result[iterator.Key().String()] = iterator.Value().Interface()
	}
	return result, true
}

func toolContractTypes(value any) ([]string, bool) {
	if value == nil {
		return nil, true
	}
	if single, ok := value.(string); ok {
		if single == "" {
			return nil, true
		}
		return []string{single}, validToolContractType(single)
	}
	items, ok := anySlice(value)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		name, ok := item.(string)
		if !ok {
			return nil, false
		}
		if !validToolContractType(name) {
			return nil, false
		}
		result = append(result, name)
	}
	return result, true
}

func validToolContractType(name string) bool {
	switch name {
	case "null", "object", "array", "string", "boolean", "number", "integer":
		return true
	default:
		return false
	}
}

func matchesAnyToolContractType(value any, types []string) bool {
	for _, expected := range types {
		switch expected {
		case "null":
			if value == nil {
				return true
			}
		case "object":
			if _, ok := stringMap(value); ok {
				return true
			}
		case "array":
			if _, ok := anySlice(value); ok {
				return true
			}
		case "string":
			if _, ok := value.(string); ok {
				return true
			}
		case "boolean":
			if _, ok := value.(bool); ok {
				return true
			}
		case "number":
			if _, ok := toolContractNumber(value); ok {
				return true
			}
		case "integer":
			if number, ok := toolContractNumber(value); ok && math.Trunc(number) == number {
				return true
			}
		}
	}
	return false
}

func toolContractNumber(value any) (float64, bool) {
	var result float64
	switch number := value.(type) {
	case json.Number:
		parsed, err := number.Float64()
		if err != nil {
			return 0, false
		}
		result = parsed
	case float64:
		result = number
	case float32:
		result = float64(number)
	case int:
		result = float64(number)
	case int8:
		result = float64(number)
	case int16:
		result = float64(number)
	case int32:
		result = float64(number)
	case int64:
		result = float64(number)
	case uint:
		result = float64(number)
	case uint8:
		result = float64(number)
	case uint16:
		result = float64(number)
	case uint32:
		result = float64(number)
	case uint64:
		result = float64(number)
	default:
		return 0, false
	}
	return result, !math.IsNaN(result) && !math.IsInf(result, 0)
}

func nonNegativeSchemaInteger(value any) (int, bool) {
	number, ok := toolContractNumber(value)
	if !ok || number < 0 || math.Trunc(number) != number || number > float64(math.MaxInt) {
		return 0, false
	}
	return int(number), true
}

func matchesToolContractEnum(value any, enum []any) bool {
	for _, candidate := range enum {
		left, leftNumber := toolContractNumber(value)
		right, rightNumber := toolContractNumber(candidate)
		if leftNumber && rightNumber && left == right {
			return true
		}
		if reflect.DeepEqual(value, candidate) {
			return true
		}
	}
	return false
}
