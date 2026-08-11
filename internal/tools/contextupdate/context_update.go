package contextupdate

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/agent-dance/luban/i18n"
	contextcontract "github.com/agent-dance/luban/internal/contracts/contextupdate"
	"github.com/agent-dance/luban/types"
)

// Tool records an untrusted model proposal. It never changes history itself;
// the query loop validates and observes it in shadow mode.
type Tool struct{}

func New() *Tool { return &Tool{} }

func (*Tool) Name() string { return "ContextUpdate" }

func (*Tool) Description() string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolContextUpdateDescription)
}

func (*Tool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true, MaxResultSizeChars: 2_000}
}

func (*Tool) Schema() types.JSONSchema {
	lang := i18n.DetectOrLoadLanguage()
	return types.StrictObjectSchema(map[string]any{
		"target_index": map[string]any{"type": "integer", "minimum": 0, "description": i18n.Text(lang, i18n.KeyToolContextUpdateInputTargetIndex)},
		"target_tool":  map[string]any{"type": "string", "description": i18n.Text(lang, i18n.KeyToolContextUpdateInputTargetTool)},
		"action":       map[string]any{"type": "string", "enum": []string{"KEEP", "REWRITE", "INDEX", "DROP"}, "description": i18n.Text(lang, i18n.KeyToolContextUpdateInputAction)},
		"reason_code":  map[string]any{"type": "string", "description": i18n.Text(lang, i18n.KeyToolContextUpdateInputReasonCode)},
		"confidence":   map[string]any{"type": "number", "minimum": 0, "maximum": 1, "description": i18n.Text(lang, i18n.KeyToolContextUpdateInputConfidence)},
	}, "target_index", "target_tool", "action", "reason_code", "confidence")
}

type result struct {
	Decision contextcontract.Decision
}

func (r result) ContextUpdateDecision() contextcontract.Decision { return r.Decision }

func (tool *Tool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	if err := types.ValidateToolInput(tool, input); err != nil {
		return types.ToolResult{Content: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolContextUpdateInvalid), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}
	action := contextcontract.Action(strings.ToUpper(strings.TrimSpace(input["action"].(string))))
	targetIndex, ok := numericIndex(input["target_index"])
	if !ok {
		return types.ToolResult{Content: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolContextUpdateInvalid), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}
	confidence, ok := numericConfidence(input["confidence"])
	if !ok {
		return types.ToolResult{Content: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolContextUpdateInvalid), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}
	decision := contextcontract.Decision{
		Schema: contextcontract.SchemaVersion, TargetIndex: targetIndex, TargetTool: normalizeTargetTool(input["target_tool"].(string)), Action: action,
		ReasonCode: strings.TrimSpace(input["reason_code"].(string)), Confidence: confidence,
	}
	if !validTargetTool(decision.TargetTool) || !validReasonCode(decision.ReasonCode) || !validAction(action) {
		return types.ToolResult{Content: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolContextUpdateInvalid), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}
	receipt, _ := json.Marshal(map[string]any{"schema": contextcontract.SchemaVersion, "accepted": true, "mode": "shadow"})
	return types.ToolResult{Content: string(receipt), Data: result{Decision: decision}, Outcome: types.ToolOutcomeSucceeded}, nil
}

func normalizeTargetTool(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > len("functions.") && strings.EqualFold(value[:len("functions.")], "functions.") {
		value = value[len("functions."):]
	}
	return value
}

func numericIndex(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, typed >= 0
	case float64:
		index := int(typed)
		return index, typed >= 0 && typed == float64(index)
	default:
		return 0, false
	}
}

func validTargetTool(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func validReasonCode(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' ||
			char == '_' || char == '-' || char == '.' || char == ':' {
			continue
		}
		return false
	}
	return true
}

func numericConfidence(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, typed >= 0 && typed <= 1
	case int:
		return float64(typed), typed >= 0 && typed <= 1
	default:
		return 0, false
	}
}

func validAction(action contextcontract.Action) bool {
	switch action {
	case contextcontract.ActionKeep, contextcontract.ActionRewrite, contextcontract.ActionIndex, contextcontract.ActionDrop:
		return true
	default:
		return false
	}
}
