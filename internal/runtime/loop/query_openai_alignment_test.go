package loop

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type previousResponseFallbackProvider struct {
	calls      []provider.Params
	failFirst  error
	streamResp []types.StreamEvent
	name       string
}

func assertOpenAIAlignmentContainsInOrder(t *testing.T, haystack string, needles []string) {
	t.Helper()
	offset := 0
	for _, needle := range needles {
		idx := strings.Index(haystack[offset:], needle)
		if idx < 0 {
			t.Fatalf("expected %q after offset %d in %q", needle, offset, haystack)
		}
		offset += idx + len(needle)
	}
}

func (p *previousResponseFallbackProvider) Name() string {
	if p.name != "" {
		return p.name
	}
	return "openai"
}
func (p *previousResponseFallbackProvider) ModelID() string { return "gpt-4o" }
func (p *previousResponseFallbackProvider) CreateStream(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.calls = append(p.calls, params)
	if len(p.calls) == 1 && p.failFirst != nil {
		return nil, p.failFirst
	}
	ch := make(chan types.StreamEvent, len(p.streamResp))
	for _, evt := range p.streamResp {
		ch <- evt
	}
	close(ch)
	return ch, nil
}

func TestQueryLoop_PreviousResponseFallbackPreservesPromptCache(t *testing.T) {
	prov := &previousResponseFallbackProvider{
		failFirst: &types.APIError{Type: "previous_response_not_found", Message: "expired"},
		streamResp: []types.StreamEvent{
			{Type: types.EventMessageStart},
			{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
			{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "ok"}},
			{Type: types.EventContentBlockStop, Index: 0},
			{Type: types.EventMessageDelta, Usage: &types.Usage{InputTokens: 100, OutputTokens: 10, CacheReadInputTokens: 80, CacheCreationInputTokens: 20}, StopReason: stopReasonPtr(types.StopReasonEndTurn)},
			{Type: types.EventMessageStop, ResponseID: "resp_new"},
		},
	}
	q := New(prov, registry.New(), Config{SessionID: "session-123", Model: "gpt-4o", MaxTurns: 1, MaxTokens: 1024})
	q.lastResponseID = "resp_old"
	q.lastEnvelopeFingerprint = envelopeFingerprint(q.providerParams(
		newQueryState(nil),
		newQueryConfigSnapshot(q.config, q.thinkingConfig),
		nil,
	))

	var events []stream.Event
	err := q.Run(context.Background(), "hello", func(evt stream.Event) { events = append(events, evt) })
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.calls) != 2 {
		t.Fatalf("CreateStream calls = %d, want 2", len(prov.calls))
	}
	if prov.calls[0].PreviousResponseID != "resp_old" {
		t.Fatalf("first PreviousResponseID = %q, want %q", prov.calls[0].PreviousResponseID, "resp_old")
	}
	if prov.calls[0].PromptCacheKey != "session-123" || !prov.calls[0].UsePromptCache {
		t.Fatalf("first call should enable prompt cache affinity, got key=%q enabled=%v", prov.calls[0].PromptCacheKey, prov.calls[0].UsePromptCache)
	}
	if prov.calls[1].PreviousResponseID != "" {
		t.Fatalf("fallback PreviousResponseID = %q, want empty", prov.calls[1].PreviousResponseID)
	}
	if prov.calls[1].PromptCacheKey != "session-123" || !prov.calls[1].UsePromptCache {
		t.Fatalf("fallback should preserve prompt cache affinity, got key=%q enabled=%v", prov.calls[1].PromptCacheKey, prov.calls[1].UsePromptCache)
	}
	if q.disableResponseChain {
		t.Fatal("disableResponseChain = true after a fresh response ID committed")
	}
	if got := q.previousResponseIDForRequest(envelopeFingerprint(prov.calls[1])); got != "resp_new" {
		t.Fatalf("previousResponseIDForRequest() = %q, want repaired chain parent", got)
	}
	if q.lastResponseID != "resp_new" {
		t.Fatalf("lastResponseID = %q, want %q", q.lastResponseID, "resp_new")
	}
	if err := q.Run(context.Background(), "follow-up", func(stream.Event) {}); err != nil {
		t.Fatalf("follow-up after chain repair: %v", err)
	}
	if len(prov.calls) != 3 || prov.calls[2].PreviousResponseID != "resp_new" {
		t.Fatalf("follow-up did not resume repaired chain: calls=%d previous=%q", len(prov.calls), prov.calls[len(prov.calls)-1].PreviousResponseID)
	}
	// The fallback should be silent — no warning events emitted.
	var errorEvents []stream.Event
	for _, evt := range events {
		if evt.Type == stream.EventError && evt.Text != "" {
			errorEvents = append(errorEvents, evt)
		}
	}
	if len(errorEvents) != 0 {
		t.Fatalf("expected no warning events for silent previous_response_id fallback, got %d: %+v", len(errorEvents), errorEvents)
	}
}

func TestQueryLoop_UsesCacheLineageIDInsteadOfForkSessionID(t *testing.T) {
	prov := &previousResponseFallbackProvider{
		streamResp: []types.StreamEvent{
			{Type: types.EventMessageStart},
			{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
			{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "ok"}},
			{Type: types.EventContentBlockStop, Index: 0},
			{Type: types.EventMessageDelta, Usage: &types.Usage{InputTokens: 10, OutputTokens: 1}, StopReason: stopReasonPtr(types.StopReasonEndTurn)},
			{Type: types.EventMessageStop},
		},
	}
	q := New(prov, registry.New(), Config{
		SessionID:      "fork-session",
		CacheLineageID: "root-session",
		Model:          "gpt-4o",
		MaxTurns:       1,
	})

	if err := q.Run(context.Background(), "hello", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.calls) != 1 {
		t.Fatalf("CreateStream calls = %d, want 1", len(prov.calls))
	}
	if got := prov.calls[0].PromptCacheKey; got != "root-session" || !prov.calls[0].UsePromptCache {
		t.Fatalf("prompt cache lineage = %q enabled=%v, want root-session/true", got, prov.calls[0].UsePromptCache)
	}
}

func TestQueryLoop_SendsSystemBlocks(t *testing.T) {
	prov := &previousResponseFallbackProvider{
		streamResp: []types.StreamEvent{
			{Type: types.EventMessageStart},
			{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
			{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "ok"}},
			{Type: types.EventContentBlockStop, Index: 0},
			{Type: types.EventMessageDelta, Usage: &types.Usage{InputTokens: 10, OutputTokens: 1}, StopReason: stopReasonPtr(types.StopReasonEndTurn)},
			{Type: types.EventMessageStop, ResponseID: "resp_new"},
		},
	}
	q := New(prov, registry.New(), Config{
		Model:    "gpt-4o",
		MaxTurns: 1,
		System:   "baseline-system",
		SystemBlocks: []prompt.SystemPromptBlock{
			{Text: "first"},
			{Text: "second"},
		},
	})

	if err := q.Run(context.Background(), "hello", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.calls) != 1 {
		t.Fatalf("CreateStream calls = %d, want 1", len(prov.calls))
	}
	if got := prov.calls[0].JoinedSystemPrompt(); got != "first\n\nsecond" {
		t.Fatalf("JoinedSystemPrompt = %q, want joined system blocks", got)
	}
	if len(prov.calls[0].SystemBlocks) != 2 {
		t.Fatalf("SystemBlocks = %d, want 2", len(prov.calls[0].SystemBlocks))
	}
}

func TestQueryLoop_AppliesSystemBlockCacheScopes(t *testing.T) {
	prov := &previousResponseFallbackProvider{
		name: "anthropic",
		streamResp: []types.StreamEvent{
			{Type: types.EventMessageStart},
			{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
			{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "ok"}},
			{Type: types.EventContentBlockStop, Index: 0},
			{Type: types.EventMessageDelta, Usage: &types.Usage{InputTokens: 10, OutputTokens: 1}, StopReason: stopReasonPtr(types.StopReasonEndTurn)},
			{Type: types.EventMessageStop, ResponseID: "resp_new"},
		},
	}
	q := New(prov, registry.New(), Config{
		Model:    "claude-sonnet-4-6",
		MaxTurns: 1,
		SystemBlocks: []prompt.SystemPromptBlock{
			{Text: "static", Name: "static", Cache: true},
		},
		SystemContext: prompt.SystemContext{
			GitStatus: "Status:\n M file.go",
		},
	})

	if err := q.Run(context.Background(), "hello", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.calls) != 1 {
		t.Fatalf("CreateStream calls = %d, want 1", len(prov.calls))
	}
	blocks := prov.calls[0].SystemBlocks
	if len(blocks) != 2 {
		t.Fatalf("SystemBlocks = %d, want static plus system context", len(blocks))
	}
	if !blocks[0].Cache || blocks[0].CacheScope != prompt.CacheScopeGlobal {
		t.Fatalf("static block cache metadata = %#v, want global cache", blocks[0])
	}
	if blocks[1].Cache || blocks[1].CacheScope != "" {
		t.Fatalf("system context block should remain uncached, got %#v", blocks[1])
	}
}

func TestQueryLoop_InjectsContextInOriginalOrder(t *testing.T) {
	prov := &previousResponseFallbackProvider{
		streamResp: []types.StreamEvent{
			{Type: types.EventMessageStart},
			{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
			{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "ok"}},
			{Type: types.EventContentBlockStop, Index: 0},
			{Type: types.EventMessageDelta, Usage: &types.Usage{InputTokens: 10, OutputTokens: 1}, StopReason: stopReasonPtr(types.StopReasonEndTurn)},
			{Type: types.EventMessageStop, ResponseID: "resp_new"},
		},
	}
	q := New(prov, registry.New(), Config{
		Model:    "gpt-4o",
		MaxTurns: 1,
		SystemBlocks: []prompt.SystemPromptBlock{
			{Text: "static", Name: "static"},
			{Text: "dynamic", Name: "dynamic"},
		},
		UserContext: prompt.UserContext{
			Instructions: "Use repo instructions",
			CurrentDate:  "Today's date is 2026-07-10.",
		},
		SystemContext: prompt.SystemContext{
			GitStatus: "Status:\n M file.go",
		},
	})

	if err := q.Run(context.Background(), "hello", func(stream.Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.calls) != 1 {
		t.Fatalf("CreateStream calls = %d, want 1", len(prov.calls))
	}
	gotMessages := prov.calls[0].Messages
	if len(gotMessages) != 2 {
		t.Fatalf("Messages = %d, want context plus user", len(gotMessages))
	}
	if !gotMessages[0].IsMeta || gotMessages[0].Role != types.RoleUser {
		t.Fatalf("first message = role %s meta %v, want meta user context", gotMessages[0].Role, gotMessages[0].IsMeta)
	}
	assertOpenAIAlignmentContainsInOrder(t, gotMessages[0].GetText(), []string{
		"<system-reminder>",
		"# instructions",
		"Use repo instructions",
		"# currentDate",
		"Today's date is 2026-07-10.",
		"</system-reminder>",
	})
	if got := gotMessages[1].GetText(); got != "hello" {
		t.Fatalf("second message = %q, want original user message", got)
	}
	gotBlocks := prov.calls[0].SystemTextBlocks()
	if len(gotBlocks) != 3 {
		t.Fatalf("SystemTextBlocks = %d, want base blocks plus system context", len(gotBlocks))
	}
	if gotBlocks[0].Text != "static" || gotBlocks[1].Text != "dynamic" {
		t.Fatalf("base system block order changed: %#v", gotBlocks)
	}
	if got := gotBlocks[2].Text; got != "gitStatus: Status:\n M file.go" {
		t.Fatalf("system context block = %q", got)
	}
}

func TestQueryLoop_PreviousResponseIDSkippedWhenRequestEnvelopeChanges(t *testing.T) {
	prov := &previousResponseFallbackProvider{
		streamResp: []types.StreamEvent{
			{Type: types.EventMessageStart},
			{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
			{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "ok"}},
			{Type: types.EventContentBlockStop, Index: 0},
			{Type: types.EventMessageDelta, Usage: &types.Usage{InputTokens: 100, OutputTokens: 10}, StopReason: stopReasonPtr(types.StopReasonEndTurn)},
			{Type: types.EventMessageStop, ResponseID: "resp_new"},
		},
	}
	q := New(prov, registry.New(), Config{SessionID: "session-123", Model: "gpt-4o", MaxTurns: 1, System: "new-system"})
	q.lastResponseID = "resp_old"
	q.lastEnvelopeFingerprint = envelopeFingerprint(provider.Params{
		Model:          "gpt-4o",
		System:         "old-system",
		PromptCacheKey: "session-123",
	})

	err := q.Run(context.Background(), "hello", func(stream.Event) {})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.calls) != 1 {
		t.Fatalf("CreateStream calls = %d, want 1", len(prov.calls))
	}
	if got := prov.calls[0].PreviousResponseID; got != "" {
		t.Fatalf("PreviousResponseID = %q, want empty when request envelope changes", got)
	}
	if got := prov.calls[0].PromptCacheKey; got != "session-123" || !prov.calls[0].UsePromptCache {
		t.Fatalf("prompt cache affinity lost: key=%q enabled=%v", prov.calls[0].PromptCacheKey, prov.calls[0].UsePromptCache)
	}
}

func TestEnvelopeFingerprintIncludesCompleteToolDefinitions(t *testing.T) {
	base := provider.Params{Tools: []types.ToolDefinition{{
		Name:        "Search",
		Description: "search version one",
		InputSchema: types.StrictObjectSchema(map[string]any{
			"query": map[string]any{"type": "string"},
		}, "query"),
	}}}
	descriptionChanged := base
	descriptionChanged.Tools = append([]types.ToolDefinition(nil), base.Tools...)
	descriptionChanged.Tools[0].Description = "search version two"
	if envelopeFingerprint(base) == envelopeFingerprint(descriptionChanged) {
		t.Fatal("envelope fingerprint ignored a tool description change")
	}

	schemaChanged := base
	schemaChanged.Tools = append([]types.ToolDefinition(nil), base.Tools...)
	schemaChanged.Tools[0].InputSchema = types.StrictObjectSchema(map[string]any{
		"query": map[string]any{"type": "string"},
		"limit": map[string]any{"type": "integer"},
	}, "query")
	if envelopeFingerprint(base) == envelopeFingerprint(schemaChanged) {
		t.Fatal("envelope fingerprint ignored a tool schema change")
	}
}

func TestIsPreviousResponseNotFound_DetectsProviderError(t *testing.T) {
	err := &types.APIError{Type: "invalid_request_error", Code: "previous_response_not_found", Message: "Previous response with id resp_deadbeef not found"}
	if !isPreviousResponseNotFound(err) {
		t.Fatal("expected previous_response_id rejection to be detected")
	}
	if isPreviousResponseNotFound(errors.New("other")) {
		t.Fatal("unexpected detection for unrelated error")
	}
}

func stopReasonPtr(sr types.StopReason) *types.StopReason { return &sr }
