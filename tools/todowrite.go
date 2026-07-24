package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

type TodoWriteTool struct{ Store *TodoStore }

// TodoWriteResult mirrors the TS TodoWriteTool result struct with old/new
// list snapshots. Carried alongside the user-facing string in metadata so
// observers can diff transitions; ToString preserves the existing prompt
// surface.
type TodoWriteResult struct {
	OldTodos                []TodoItem `json:"oldTodos"`
	NewTodos                []TodoItem `json:"newTodos"`
	VerificationNudgeNeeded bool       `json:"verificationNudgeNeeded,omitempty"`
}

func NewTodoWriteTool(store *TodoStore) *TodoWriteTool { return &TodoWriteTool{Store: store} }

func (t *TodoWriteTool) withInProcessAgentID(agentID string) types.Tool {
	clone := *t
	clone.Store = t.Store.withAgentID(agentID)
	return &clone
}

func (t *TodoWriteTool) withInProcessAgentScope(agentID, projectRoot string) types.Tool {
	clone := *t
	clone.Store = t.Store.withAgentScope(agentID, projectRoot)
	return &clone
}

func (t *TodoWriteTool) Name() string { return "TodoWrite" }

func (t *TodoWriteTool) Description() string {
	return "Write or replace the session todo list. Always include both content and activeForm for each todo."
}

func (t *TodoWriteTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"todos": map[string]any{
				"type":        "array",
				"description": "The updated todo list",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content": map[string]any{
							"type":        "string",
							"description": "Imperative task description",
						},
						"status": map[string]any{
							"type":        "string",
							"enum":        []string{"pending", "in_progress", "completed"},
							"description": "Current status of the task",
						},
						"activeForm": map[string]any{
							"type":        "string",
							"description": "Present continuous form shown while in progress",
						},
					},
					"required": []string{"content", "status", "activeForm"},
				},
			},
		},
		"todos",
	)
}

func (t *TodoWriteTool) ToolContract() types.ToolContract {
	todoSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content":    map[string]any{"type": "string"},
			"status":     map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}},
			"activeForm": map[string]any{"type": "string"},
		},
		"required": []string{"content", "status", "activeForm"},
	}
	return types.ToolContract{
		OutputSchema: &types.JSONSchema{
			Type: "object",
			Properties: map[string]any{
				"oldTodos":                map[string]any{"type": "array", "items": todoSchema},
				"newTodos":                map[string]any{"type": "array", "items": todoSchema},
				"verificationNudgeNeeded": map[string]any{"type": "boolean"},
			},
			Required: []string{"oldTodos", "newTodos"},
		},
		Strict:             true,
		ReadOnly:           false,
		ConcurrencySafe:    false,
		MaxResultSizeChars: 100_000,
	}
}

// MaxTodoListSize bounds the number of items a single TodoWrite call may
// store. TD-02: the TS schema has no explicit cap but the UI implicitly
// constrains reasonable lists; without a Go-side cap a runaway model could
// write tens of thousands of items, blowing up storage and event-loop time
// on every atomic write. 200 is generous for real workflows and still cheap
// to render.
const MaxTodoListSize = 200

func (t *TodoWriteTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	lang := i18n.DetectOrLoadLanguage()
	if _, ok := input["todos"]; !ok {
		return types.ToolResult{Content: i18n.Text(lang, i18n.KeyToolLegacyDTodoInputRequired), IsError: true}, nil
	}
	in, toolErr := parseStrictInputOrError[TodoWriteInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}

	if len(in.Todos) > MaxTodoListSize {
		return types.ToolResult{
			Content: i18n.Format(lang, i18n.KeyToolLegacyDTodoLimitExceeded, MaxTodoListSize, len(in.Todos)),
			IsError: true,
		}, nil
	}

	seenContent := make(map[string]struct{}, len(in.Todos))
	for _, item := range in.Todos {
		if strings.TrimSpace(item.Content) == "" {
			return types.ToolResult{Content: i18n.Text(lang, i18n.KeyToolLegacyDTodoContentRequired), IsError: true}, nil
		}
		if strings.TrimSpace(item.ActiveForm) == "" {
			return types.ToolResult{Content: i18n.Text(lang, i18n.KeyToolLegacyDTodoActiveFormRequired), IsError: true}, nil
		}
		switch item.Status {
		case "pending", "in_progress", "completed":
		default:
			return types.ToolResult{Content: i18n.Text(lang, i18n.KeyToolLegacyDTodoStatusInvalid), IsError: true}, nil
		}
		key := strings.TrimSpace(item.Content)
		if _, dup := seenContent[key]; dup {
			return types.ToolResult{Content: i18n.Text(lang, i18n.KeyToolLegacyDTodoContentDuplicate), IsError: true}, nil
		}
		seenContent[key] = struct{}{}
	}

	allDone := len(in.Todos) > 0
	for _, item := range in.Todos {
		if item.Status != "completed" {
			allDone = false
			break
		}
	}
	verificationNudgeNeeded := todoVerificationNudgeNeeded(in.Todos)

	// TD-04: read-modify-write under a single lock so concurrent TodoWrite
	// calls can never race on the diff snapshot.
	oldTodos, newTodos, err := t.Store.LoadAndSave(func(prior []TodoItem) ([]TodoItem, error) {
		next := append([]TodoItem{}, in.Todos...)
		if allDone && len(in.Todos) > 0 {
			next = []TodoItem{}
		}
		return next, nil
	})
	if err != nil {
		return types.ToolResult{Content: i18n.Format(lang, i18n.KeyToolLegacyDTodoSaveFailed, err), IsError: true}, nil
	}
	if oldTodos == nil {
		oldTodos = []TodoItem{}
	}

	// TD-03: surface a soft warning if any item regressed from completed to
	// pending — usually the model "forgot" earlier progress when re-issuing
	// the full list. We don't refuse the write, but note the regression so
	// observers can react.
	regressed := todoRegressions(oldTodos, newTodos)

	res := TodoWriteResult{
		OldTodos:                oldTodos,
		NewTodos:                newTodos,
		VerificationNudgeNeeded: allDone && verificationNudgeNeeded,
	}
	base := todoWriteModelContent(res)
	if len(regressed) > 0 {
		base += i18n.Format(lang, i18n.KeyToolLegacyDTodoRegressionWarning, len(regressed), strings.Join(regressed, ", "))
	}
	metadata := make(map[string]string, 2)
	if blob, err := json.Marshal(res); err == nil {
		metadata["todoWriteResult"] = string(blob)
	}
	if len(regressed) > 0 {
		metadata["regressed"] = strings.Join(regressed, ",")
	}
	return types.ToolResult{Content: base, Data: res, Metadata: metadata}, nil
}

func (t *TodoWriteTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(TodoWriteResult)
	if !ok {
		return types.ToolResultBlock{
			ToolUseID: toolUseID,
			Content:   i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolLegacyDTodoTypedResultInvalid),
			IsError:   true,
		}
	}
	return types.ToolResultBlock{ToolUseID: toolUseID, Content: todoWriteModelContent(output)}
}

func todoWriteModelContent(output TodoWriteResult) string {
	content := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolLegacyDTodoModified)
	if output.VerificationNudgeNeeded {
		content += verificationNudgeText(verificationAgentType)
	}
	return content
}

// todoRegressions returns the content fields of items that moved from a
// completed state in oldTodos to a non-completed state in newTodos. Used by
// TD-03 to warn the model when it appears to have lost progress.
func todoRegressions(oldTodos, newTodos []TodoItem) []string {
	if len(oldTodos) == 0 || len(newTodos) == 0 {
		return nil
	}
	priorByContent := make(map[string]TodoItem, len(oldTodos))
	for _, item := range oldTodos {
		priorByContent[strings.TrimSpace(item.Content)] = item
	}
	var regressions []string
	for _, item := range newTodos {
		key := strings.TrimSpace(item.Content)
		prior, ok := priorByContent[key]
		if !ok {
			continue
		}
		if prior.Status == "completed" && item.Status != "completed" {
			regressions = append(regressions, key)
		}
	}
	return regressions
}
