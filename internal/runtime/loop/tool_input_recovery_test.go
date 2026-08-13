package loop

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/i18n"
	streamevent "github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type toolInputRecoveryProvider struct {
	mu        sync.Mutex
	responses [][]types.StreamEvent
	params    []provider.Params
}

type toolInputRecoveryTool struct{}

func (*toolInputRecoveryTool) Name() string        { return "Inspect" }
func (*toolInputRecoveryTool) Description() string { return "test inspect" }
func (*toolInputRecoveryTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object", Properties: map[string]any{
		"cursor": map[string]any{"type": "string"},
		"path":   map[string]any{"type": "string"},
	}}
}
func (*toolInputRecoveryTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{Content: "inspected"}, nil
}

func (p *toolInputRecoveryProvider) Name() string    { return "recovery-test" }
func (p *toolInputRecoveryProvider) ModelID() string { return "recovery-test-model" }
func (p *toolInputRecoveryProvider) CreateStream(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	index := len(p.params)
	params.Messages = append([]types.Message(nil), params.Messages...)
	p.params = append(p.params, params)
	p.mu.Unlock()
	stream := make(chan types.StreamEvent, 16)
	go func() {
		defer close(stream)
		if index >= len(p.responses) {
			return
		}
		for _, event := range attachTestProviderCommitReceipts(p.responses[index]) {
			select {
			case stream <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return stream, nil
}

func malformedToolResponse(responseID string) []types.StreamEvent {
	return malformedToolResponseWithInput(responseID, `{"path":`)
}

func malformedToolResponseWithInput(responseID, input string) []types.StreamEvent {
	return testStreamEvents(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
			Type: types.ContentTypeToolUse, ID: "call_bad", Name: "Inspect", ProviderItemID: "fc_bad",
		}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: input}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
		types.StreamEvent{Type: types.EventMessageStop, ResponseID: responseID, ProviderContinuation: &types.ProviderContinuation{
			Protocol: "responses/test/standard", RequestedModel: "recovery-test-model",
		}},
	)
}

func TestParseToolInputJSONDiagnosesMissingRootFieldWithoutReadingValues(t *testing.T) {
	const raw = `{"cursor": , "secret":"do-not-leak", "requests":[]}`
	_, err, diagnostic := parseToolInputJSONWithDiagnostic(raw)
	if err == nil {
		t.Fatal("malformed input parsed successfully")
	}
	if diagnostic.Kind != types.ToolInputDiagnosticMissingValue || diagnostic.Field != "cursor" || diagnostic.ByteOffset != 12 {
		t.Fatalf("diagnostic = %#v, want missing cursor at byte 12", diagnostic)
	}
	if _, err, trailing := parseToolInputJSONWithDiagnostic(`{"path":"."}{"path":"."}`); err == nil || trailing.Kind != types.ToolInputDiagnosticTrailingData || trailing.ByteOffset != 13 {
		t.Fatalf("trailing payload was not rejected: err=%v diagnostic=%#v", err, trailing)
	}
}

func TestParseToolInputJSONDiagnostics(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		kind   types.ToolInputDiagnosticKind
		offset int
	}{
		{name: "unexpected eof", raw: `{"cursor":"unfinished`, kind: types.ToolInputDiagnosticUnexpectedEOF, offset: 22},
		{name: "top-level array", raw: `[]`, kind: types.ToolInputDiagnosticNonObject, offset: 1},
		{name: "top-level null", raw: `null`, kind: types.ToolInputDiagnosticNonObject, offset: 1},
		{name: "empty", raw: `  `, kind: types.ToolInputDiagnosticNonObject, offset: 1},
		{name: "generic syntax", raw: `{"cursor" true}`, kind: types.ToolInputDiagnosticInvalidJSON, offset: 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err, diagnostic := parseToolInputJSONWithDiagnostic(tt.raw)
			if err == nil || diagnostic.Kind != tt.kind || diagnostic.ByteOffset != tt.offset {
				t.Fatalf("err/diagnostic = %v/%#v", err, diagnostic)
			}
		})
	}
}

func TestParseToolInputJSONRequiresExactlyOneObject(t *testing.T) {
	for _, raw := range []string{"", "null", `{"path":"."} trailing`} {
		if input, err := parseToolInputJSON(raw); err == nil || input != nil {
			t.Fatalf("payload %q parsed as %#v/%v", raw, input, err)
		}
	}
}

func finalTextResponse(text string) []types.StreamEvent {
	return []types.StreamEvent{
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: text}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageStop, ResponseID: "response-corrected"},
	}
}

func correctedToolResponse() []types.StreamEvent {
	return testStreamEvents(
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
			Type: types.ContentTypeToolUse, ID: "call_corrected", Name: "Inspect", ProviderItemID: "fc_corrected",
		}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
			Type: "tool_state_final", ID: "call_corrected", Name: "Inspect", PartialJSON: `{"path":"."}`,
		}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
		types.StreamEvent{Type: types.EventMessageStop, ResponseID: "response-valid-tool"},
	)
}

func TestMalformedToolInputRetriesFromSanitizedFullHistory(t *testing.T) {
	p := &toolInputRecoveryProvider{responses: [][]types.StreamEvent{
		malformedToolResponse("response-invalid-parent"),
		correctedToolResponse(),
		finalTextResponse("corrected final"),
	}}
	reg := registry.New()
	reg.Register(&toolInputRecoveryTool{})
	query := New(p, reg, Config{MaxTurns: 4, MaxTokens: 256})
	var retryWarnings int
	err := query.Run(context.Background(), "inspect", func(event streamevent.Event) {
		if event.Type == streamevent.EventSystemWarning && event.RuntimeEvent != nil && event.RuntimeEvent.PrivateMetadata["reason"] == "invalid_tool_input" {
			retryWarnings++
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.params) != 3 {
		t.Fatalf("provider calls = %d, want correction, tool result, and final", len(p.params))
	}
	if p.params[1].PreviousResponseID != "" {
		t.Fatalf("invalid response reused as parent: %q", p.params[1].PreviousResponseID)
	}
	foundRecovery := false
	for _, message := range p.params[1].Messages {
		if len(message.GetInvalidToolUses()) != 0 {
			t.Fatalf("invalid tool audit leaked into model view: %#v", p.params[1].Messages)
		}
		if message.InternalKind == types.InternalMessageKindToolInputRecovery && message.IsInternalRuntimeMessage() {
			foundRecovery = true
		}
	}
	if !foundRecovery {
		t.Fatalf("trusted correction message missing: %#v", p.params[1].Messages)
	}
	if retryWarnings != 1 {
		t.Fatalf("structured recovery warnings = %d, want 1", retryWarnings)
	}
	invalidCount := 0
	for _, message := range query.Messages() {
		invalidCount += len(message.GetInvalidToolUses())
		if len(message.GetInvalidToolUses()) > 0 && message.HasProviderContinuation() {
			t.Fatal("malformed provider continuation remained attached to durable history")
		}
	}
	if invalidCount != 1 {
		t.Fatalf("durable invalid tool audits = %d, want 1", invalidCount)
	}
}

func TestMalformedToolInputRejectsTextOnlyCorrection(t *testing.T) {
	p := &toolInputRecoveryProvider{responses: [][]types.StreamEvent{
		malformedToolResponse("response-invalid"),
		finalTextResponse("claimed completion without a tool"),
	}}
	query := New(p, registry.New(), Config{MaxTurns: 4, MaxTokens: 256})
	err := query.Run(context.Background(), "inspect", func(streamevent.Event) {})
	if err == nil {
		t.Fatal("text-only correction was accepted as successful completion")
	}
	if len(p.params) != 2 {
		t.Fatalf("provider calls = %d, want one bounded correction", len(p.params))
	}
}

func TestMalformedToolInputFailsClosedAfterOneCorrection(t *testing.T) {
	p := &toolInputRecoveryProvider{responses: [][]types.StreamEvent{
		malformedToolResponse("response-invalid-1"),
		malformedToolResponse("response-invalid-2"),
	}}
	reg := registry.New()
	reg.Register(&toolInputRecoveryTool{})
	query := New(p, reg, Config{MaxTurns: 4, MaxTokens: 256})
	var repeated bool
	err := query.Run(context.Background(), "inspect", func(event streamevent.Event) {
		if event.RuntimeEvent != nil && event.RuntimeEvent.PrivateMetadata["repeated_invalid_input"] == true {
			repeated = true
			if event.RuntimeEvent.PublicKey != i18n.KeyRuntimeToolInputRecoveryRepeated {
				t.Fatalf("repeated warning key = %q", event.RuntimeEvent.PublicKey)
			}
		}
	})
	if err == nil {
		t.Fatal("repeated malformed tool input completed successfully")
	}
	if info, ok := i18n.DescribeSemanticError(err); !ok || info.Key != i18n.KeyLoopToolInputRecoveryRepeated {
		t.Fatalf("repeated recovery error = %#v, semantic=%#v/%v", err, info, ok)
	}
	if len(p.params) != 2 {
		t.Fatalf("provider calls = %d, want bounded correction", len(p.params))
	}
	invalidCount := 0
	for _, message := range query.Messages() {
		invalidCount += len(message.GetInvalidToolUses())
	}
	if invalidCount != 2 {
		t.Fatalf("durable invalid tool audits = %d, want 2", invalidCount)
	}
	if !repeated {
		t.Fatal("identical malformed input was not classified as repeated")
	}
}

func TestMalformedToolInputRecoveryPromptContainsOnlySafeDiagnostic(t *testing.T) {
	const secret = "do-not-leak"
	raw := `{"cursor": , "secret":"` + secret + `", "requests":[]}`
	p := &toolInputRecoveryProvider{responses: [][]types.StreamEvent{
		malformedToolResponseWithInput("response-invalid", raw),
		correctedToolResponse(),
		finalTextResponse("done"),
	}}
	reg := registry.New()
	reg.Register(&toolInputRecoveryTool{})
	query := New(p, reg, Config{MaxTurns: 4, MaxTokens: 256})
	if err := query.Run(context.Background(), "inspect", func(streamevent.Event) {}); err != nil {
		t.Fatal(err)
	}
	var recovery string
	for _, message := range p.params[1].Messages {
		if message.InternalKind == types.InternalMessageKindToolInputRecovery {
			recovery = message.GetText()
		}
	}
	if !strings.Contains(recovery, "cursor") || !strings.Contains(recovery, "12") {
		t.Fatalf("safe field/offset absent from recovery: %q", recovery)
	}
	for _, forbidden := range []string{secret, raw, "sha256:"} {
		if strings.Contains(recovery, forbidden) {
			t.Fatalf("recovery leaked %q: %q", forbidden, recovery)
		}
	}
	invalid := query.Messages()[1].GetInvalidToolUses()[0]
	if invalid.DiagnosticKind != types.ToolInputDiagnosticMissingValue || invalid.DiagnosticField != "cursor" || invalid.DiagnosticOffset != 12 {
		t.Fatalf("durable diagnostic = %#v", invalid)
	}
}

func TestMalformedToolInputDoesNotExposeUnknownSchemaField(t *testing.T) {
	const raw = `{"secret_field": , "cursor":"ok"}`
	p := &toolInputRecoveryProvider{responses: [][]types.StreamEvent{
		malformedToolResponseWithInput("response-invalid", raw),
		correctedToolResponse(),
		finalTextResponse("done"),
	}}
	reg := registry.New()
	reg.Register(&toolInputRecoveryTool{})
	query := New(p, reg, Config{MaxTurns: 4, MaxTokens: 256})
	if err := query.Run(context.Background(), "inspect", func(streamevent.Event) {}); err != nil {
		t.Fatal(err)
	}
	invalid := query.Messages()[1].GetInvalidToolUses()[0]
	if invalid.DiagnosticField != "" {
		t.Fatalf("unknown schema field exposed: %#v", invalid)
	}
	for _, message := range p.params[1].Messages {
		if message.InternalKind == types.InternalMessageKindToolInputRecovery && strings.Contains(message.GetText(), "secret_field") {
			t.Fatalf("unknown field leaked to model: %q", message.GetText())
		}
	}
}

func TestToolInputFailureFingerprintsAreOrderIndependentMultisets(t *testing.T) {
	first := []types.InvalidToolUseBlock{
		{Name: "Inspect", FailureKind: types.ToolInputFailureInvalidJSON, InputDigest: "sha256:a"},
		{Name: "Write", FailureKind: types.ToolInputFailureInvalidJSON, InputDigest: "sha256:b"},
	}
	reordered := []types.InvalidToolUseBlock{first[1], first[0]}
	if !equalToolInputFailureFingerprints(toolInputFailureFingerprints(first), toolInputFailureFingerprints(reordered)) {
		t.Fatal("reordered identical failures were not equal")
	}
	withDuplicate := append(append([]types.InvalidToolUseBlock(nil), reordered...), first[0])
	if equalToolInputFailureFingerprints(toolInputFailureFingerprints(first), toolInputFailureFingerprints(withDuplicate)) {
		t.Fatal("different multiset multiplicity was treated as equal")
	}
}

func TestMalformedToolInputDifferentCorrectionUsesGenericFailure(t *testing.T) {
	p := &toolInputRecoveryProvider{responses: [][]types.StreamEvent{
		malformedToolResponseWithInput("response-invalid-1", `{"cursor": ,}`),
		malformedToolResponseWithInput("response-invalid-2", `{"cursor":"unterminated}`),
	}}
	reg := registry.New()
	reg.Register(&toolInputRecoveryTool{})
	query := New(p, reg, Config{MaxTurns: 4, MaxTokens: 256})
	var failedKey i18n.Key
	var repeated any
	err := query.Run(context.Background(), "inspect", func(event streamevent.Event) {
		if event.RuntimeEvent != nil && event.RuntimeEvent.PrivateMetadata["repeated_invalid_input"] != nil {
			failedKey = event.RuntimeEvent.PublicKey
			repeated = event.RuntimeEvent.PrivateMetadata["repeated_invalid_input"]
		}
	})
	if err == nil {
		t.Fatal("different malformed correction completed")
	}
	if failedKey != i18n.KeyRuntimeToolInputRecoveryFailed || repeated != false {
		t.Fatalf("generic failure key/repeated = %q/%v", failedKey, repeated)
	}
}
