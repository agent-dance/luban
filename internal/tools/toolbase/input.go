// Package toolbase contains domain-neutral helpers shared by tool packages.
package toolbase

import (
	"bytes"
	"encoding/json"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// ParseInput unmarshals a map input into T using the standard JSON contract.
func ParseInput[T any](input map[string]any) (T, error) {
	var result T
	data, err := json.Marshal(input)
	if err != nil {
		return result, i18n.WrapError(i18n.KeyToolSourceSinkParseMarshal, err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, i18n.WrapError(i18n.KeyToolSourceSinkParseDecode, err)
	}
	return result, nil
}

// ParseInputOrError parses input and projects decoding failures as a failed
// tool result suitable for returning from Execute.
func ParseInputOrError[T any](input map[string]any) (T, *types.ToolResult) {
	result, err := ParseInput[T](input)
	if err != nil {
		return result, invalidInputResult(err)
	}
	return result, nil
}

// ParseStrictInput decodes T and rejects fields not declared by its JSON
// contract.
func ParseStrictInput[T any](input map[string]any) (T, error) {
	var result T
	data, err := json.Marshal(input)
	if err != nil {
		return result, i18n.WrapError(i18n.KeyToolSourceSinkParseMarshal, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, i18n.WrapError(i18n.KeyToolSourceSinkParseDecode, err)
	}
	return result, nil
}

// ParseStrictInputOrError parses strict input and projects decoding failures
// as a failed tool result suitable for returning from Execute.
func ParseStrictInputOrError[T any](input map[string]any) (T, *types.ToolResult) {
	result, err := ParseStrictInput[T](input)
	if err != nil {
		return result, invalidInputResult(err)
	}
	return result, nil
}

func invalidInputResult(err error) *types.ToolResult {
	return &types.ToolResult{
		Content: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeInvalidInput, err),
		IsError: true,
		Outcome: types.ToolOutcomeFailed,
	}
}
