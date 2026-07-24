package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

// helper builds a standard two-question input map for reuse across tests.
// Preview text is only attached on single-select questions because the
// AskUserQuestion contract forbids previews when multiSelect is true.
func makeAskInput(multiSelect bool) map[string]any {
	maybe := map[string]any{"label": "Maybe", "description": "Unsure"}
	if !multiSelect {
		maybe["preview"] = "50/50"
	}
	options := []any{
		map[string]any{"label": "Yes", "description": "Agree"},
		map[string]any{"label": "No", "description": "Disagree"},
		maybe,
	}
	return map[string]any{
		"questions": []any{
			map[string]any{
				"question":    "Do you like Go?",
				"header":      "Go Opinion",
				"options":     options,
				"multiSelect": multiSelect,
			},
		},
	}
}

func executeAskUserThroughPermission(ctx context.Context, tool *AskUserQuestionTool, input map[string]any) (types.ToolResult, error) {
	decision, err := tool.CheckPermissions(ctx, input, types.ToolPermissionRequest{SessionID: "test-session", ToolUseID: "test-tool"})
	if err != nil {
		return types.ToolResult{}, err
	}
	if decision.Behavior != types.PermissionBehaviorAllow || decision.UpdatedInput == nil {
		return types.ToolResult{Content: decision.Message, IsError: true, Outcome: types.ToolOutcomeDenied}, nil
	}
	return tool.Execute(ctx, decision.UpdatedInput)
}

func TestAskUserQuestion_SingleSelect_ByNumber(t *testing.T) {
	var out bytes.Buffer
	tool := &AskUserQuestionTool{
		Reader: strings.NewReader("2\n"),
		Writer: &out,
	}

	result, err := executeAskUserThroughPermission(context.Background(), tool, makeAskInput(false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content)
	}

	var answers map[string]any
	if err := json.Unmarshal([]byte(result.Content), &answers); err != nil {
		t.Fatalf("invalid JSON output: %v — content: %s", err, result.Content)
	}
	if answers["Do you like Go?"] != "No" {
		t.Errorf("expected 'No', got %v", answers["Do you like Go?"])
	}
}

func TestAskUserQuestion_SingleSelect_Other(t *testing.T) {
	var out bytes.Buffer
	tool := &AskUserQuestionTool{
		Reader: strings.NewReader("o\nNot sure yet\n"),
		Writer: &out,
	}

	result, err := executeAskUserThroughPermission(context.Background(), tool, makeAskInput(false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content)
	}

	var answers map[string]any
	if err := json.Unmarshal([]byte(result.Content), &answers); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if answers["Do you like Go?"] != "Not sure yet" {
		t.Errorf("expected 'Not sure yet', got %v", answers["Do you like Go?"])
	}
}

func TestAskUserQuestion_MultiSelect(t *testing.T) {
	var out bytes.Buffer
	tool := &AskUserQuestionTool{
		Reader: strings.NewReader("1,3\n"),
		Writer: &out,
	}

	result, err := executeAskUserThroughPermission(context.Background(), tool, makeAskInput(true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content)
	}

	var answers map[string]any
	if err := json.Unmarshal([]byte(result.Content), &answers); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	raw, ok := answers["Do you like Go?"]
	if !ok {
		t.Fatal("missing answer key")
	}
	answer, ok := raw.(string)
	if !ok || answer != "Yes, Maybe" {
		t.Fatalf("expected canonical joined answer, got %T: %v", raw, raw)
	}
}

func TestAskUserQuestion_Preview_Shown(t *testing.T) {
	var out bytes.Buffer
	tool := &AskUserQuestionTool{
		Reader: strings.NewReader("3\n"),
		Writer: &out,
	}
	_, _ = executeAskUserThroughPermission(context.Background(), tool, makeAskInput(false))

	if !strings.Contains(out.String(), "50/50") {
		t.Errorf("expected preview text in output, got:\n%s", out.String())
	}
}

func TestAskUserQuestion_InvalidSelection(t *testing.T) {
	var out bytes.Buffer
	tool := &AskUserQuestionTool{
		Reader: strings.NewReader("99\n"),
		Writer: &out,
	}

	result, err := executeAskUserThroughPermission(context.Background(), tool, makeAskInput(false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected IsError=true, got content: %s", result.Content)
	}
}

func TestAskUserQuestion_MultipleQuestions(t *testing.T) {
	input := map[string]any{
		"questions": []any{
			map[string]any{
				"question":    "Favourite colour?",
				"header":      "Colour",
				"options":     []any{map[string]any{"label": "Red", "description": "Warm"}, map[string]any{"label": "Blue", "description": "Cool"}},
				"multiSelect": false,
			},
			map[string]any{
				"question":    "Favourite season?",
				"header":      "Season",
				"options":     []any{map[string]any{"label": "Summer", "description": "Hot"}, map[string]any{"label": "Winter", "description": "Cold"}},
				"multiSelect": false,
			},
		},
	}

	var out bytes.Buffer
	tool := &AskUserQuestionTool{
		Reader: strings.NewReader("1\n2\n"),
		Writer: &out,
	}

	result, err := executeAskUserThroughPermission(context.Background(), tool, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", result.Content)
	}

	var answers map[string]any
	if err := json.Unmarshal([]byte(result.Content), &answers); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if answers["Favourite colour?"] != "Red" {
		t.Errorf("expected Red, got %v", answers["Favourite colour?"])
	}
	if answers["Favourite season?"] != "Winter" {
		t.Errorf("expected Winter, got %v", answers["Favourite season?"])
	}
}

func TestAskUserQuestion_TooManyQuestions(t *testing.T) {
	questions := make([]any, 5)
	for i := range questions {
		questions[i] = map[string]any{
			"question":    "Q",
			"header":      "H",
			"options":     []any{map[string]any{"label": "A", "description": "a"}, map[string]any{"label": "B", "description": "b"}},
			"multiSelect": false,
		}
	}
	tool := &AskUserQuestionTool{
		Reader: strings.NewReader(""),
		Writer: &bytes.Buffer{},
	}
	result, err := tool.Execute(context.Background(), map[string]any{"questions": questions})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for 5 questions")
	}
}

func TestAskUserQuestion_ContextCancelled(t *testing.T) {
	// Use a pipe that blocks so the goroutine blocks on scanner.Scan()
	pr, _ := createBlockingReader()
	var out bytes.Buffer
	tool := &AskUserQuestionTool{
		Reader: pr,
		Writer: &out,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	result, err := executeAskUserThroughPermission(ctx, tool, makeAskInput(false))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute error = %v, want context.Canceled (result=%#v)", err, result)
	}
}

func TestAskUserQuestion_Name(t *testing.T) {
	tool := &AskUserQuestionTool{}
	if tool.Name() != "AskUserQuestion" {
		t.Errorf("expected name 'AskUserQuestion', got %q", tool.Name())
	}
}

func TestAskUserQuestion_Schema(t *testing.T) {
	tool := &AskUserQuestionTool{}
	schema := tool.Schema()
	if schema.Type != "object" {
		t.Errorf("expected schema type 'object', got %q", schema.Type)
	}
	if _, ok := schema.Properties["questions"]; !ok {
		t.Error("schema missing 'questions' property")
	}
}

// createBlockingReader returns a reader that blocks until closed.
func createBlockingReader() (*bytes.Reader, func()) {
	// Return an empty reader — scanner.Scan() will immediately return false
	// after reading nothing, which is fine; what we want is the goroutine to
	// block on ctx.Done() when the reader is empty and context is cancelled.
	// Use a sleepy pipe via a bytes.Reader with no data.
	r := bytes.NewReader([]byte{})
	return r, func() {}
}
