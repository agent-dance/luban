package compact

import (
	"slices"
	"testing"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

func TestModelContextEstimateIncludesProtocolAndToolPayloadAndExposesUnknownOverhead(t *testing.T) {
	cw := &ContextWindow{Counter: &CharBasedCounter{}}
	estimate := cw.EstimateMessagesDetailed([]types.Message{{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "abcdefgh"},
			types.ToolUseBlock{Type: types.ContentTypeToolUse, Name: "Read", Input: map[string]any{"file_path": "/tmp/example"}},
		},
	}}, ModelContextOverhead{})

	if estimate.MessageContentTokens != 2 || estimate.ToolPayloadTokens == 0 {
		t.Fatalf("content/tool components = %+v", estimate)
	}
	if estimate.KnownTotalTokens <= estimate.MessageContentTokens+estimate.ToolPayloadTokens {
		t.Fatalf("protocol framing was omitted: %+v", estimate)
	}
	if estimate.Complete {
		t.Fatalf("missing system/tool-schema overhead was silently presented as complete: %+v", estimate)
	}
	for _, kind := range []TokenOverheadKind{TokenOverheadSystemPrompt, TokenOverheadToolSchema} {
		if !slices.Contains(estimate.UnknownOverheads, kind) {
			t.Fatalf("missing fail-visible overhead %q in %+v", kind, estimate)
		}
	}
}

func TestModelContextEstimateCanProveAllOverheadComponents(t *testing.T) {
	cw := &ContextWindow{Counter: &CharBasedCounter{}}
	system, schemas, media := 11, 13, 17
	estimate := cw.EstimateMessagesDetailed([]types.Message{{
		Role:    types.RoleUser,
		Content: []types.ContentBlock{types.ImageBlock{Type: types.ContentTypeImage}},
	}}, ModelContextOverhead{SystemPromptTokens: &system, ToolSchemaTokens: &schemas, MediaTokens: &media})
	if !estimate.Complete || len(estimate.UnknownOverheads) != 0 {
		t.Fatalf("proved overheads not complete: %+v", estimate)
	}
	if estimate.KnownTotalTokens < system+schemas+media {
		t.Fatalf("known total omitted configured overhead: %+v", estimate)
	}
}

func TestModelContextEstimateFailsVisibleWhenToolPayloadCannotBeEncoded(t *testing.T) {
	zero := 0
	cw := &ContextWindow{Counter: &CharBasedCounter{}}
	estimate := cw.EstimateMessagesDetailed([]types.Message{{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{types.ToolUseBlock{
			Type: types.ContentTypeToolUse, Name: "Broken", Input: map[string]any{"unsupported": make(chan int)},
		}},
	}}, ModelContextOverhead{SystemPromptTokens: &zero, ToolSchemaTokens: &zero})
	if estimate.Complete || !slices.Contains(estimate.UnknownOverheads, TokenOverheadToolPayload) {
		t.Fatalf("unencodable tool payload was silently counted as zero: %+v", estimate)
	}
}

func TestProviderRequestEstimateAccountsForSystemToolsMediaAndProtocol(t *testing.T) {
	cw := &ContextWindow{Counter: &CharBasedCounter{}}
	estimate := cw.EstimateProviderRequest(provider.Params{
		System: "abcdefgh",
		Tools: []types.ToolDefinition{{
			Name: "Read", Description: "read a file",
			InputSchema: types.JSONSchema{Type: "object", Properties: map[string]any{"path": map[string]any{"type": "string"}}},
		}},
		Messages: []types.Message{{Role: types.RoleUser, Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "request"},
			types.ImageBlock{Type: types.ContentTypeImage},
		}}},
	})
	if !estimate.Complete || len(estimate.UnknownOverheads) != 0 {
		t.Fatalf("provider request estimate incomplete: %+v", estimate)
	}
	wantKinds := map[TokenOverheadKind]bool{
		TokenOverheadSystemPrompt: false, TokenOverheadToolSchema: false,
		TokenOverheadMedia: false, TokenOverheadProtocol: false,
	}
	for _, overhead := range estimate.Overheads {
		if _, ok := wantKinds[overhead.Kind]; ok {
			wantKinds[overhead.Kind] = overhead.Tokens > 0
		}
	}
	for kind, accounted := range wantKinds {
		if !accounted {
			t.Fatalf("request component %q missing from %+v", kind, estimate)
		}
	}
	if estimate.KnownTotalTokens < EstimatedMediaTokensPerBlock {
		t.Fatalf("media budget omitted: %+v", estimate)
	}
}
