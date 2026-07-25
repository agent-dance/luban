package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

const (
	task19SkillDigestA skills.SkillDigest = "sha256:1919191919191919191919191919191919191919191919191919191919191919"
	task19SkillDigestB skills.SkillDigest = "sha256:2929292929292929292929292929292929292929292929292929292929292929"
)

type task19ProviderTurn struct {
	events []types.StreamEvent
	err    error
}

type task19RecordingProvider struct {
	mu              sync.Mutex
	turns           []task19ProviderTurn
	calls           []provider.Params
	dynamicToolName string
}

func (p *task19RecordingProvider) Name() string    { return "task19-recording" }
func (p *task19RecordingProvider) ModelID() string { return "task19-model" }

func (p *task19RecordingProvider) CreateStream(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	index := len(p.calls)
	params.Messages = append([]types.Message(nil), params.Messages...)
	p.calls = append(p.calls, params)
	var turn task19ProviderTurn
	if p.dynamicToolName != "" {
		toolUseID := fmt.Sprintf("task19_tool_%03d", index)
		turn.events = task19ToolEvents(toolUseID, p.dynamicToolName, nil, fmt.Sprintf("task19_response_%03d", index))
	} else if index < len(p.turns) {
		turn = p.turns[index]
	}
	p.mu.Unlock()
	if turn.err != nil {
		return nil, turn.err
	}
	stream := make(chan types.StreamEvent, len(turn.events))
	go func() {
		defer close(stream)
		for _, event := range turn.events {
			select {
			case <-ctx.Done():
				return
			case stream <- event:
			}
		}
	}()
	return stream, nil
}

func (p *task19RecordingProvider) Calls() []provider.Params {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.Params(nil), p.calls...)
}

func TestSkillCatalogQueryOrderingAcrossRunEntryPoints(t *testing.T) {
	tests := []struct {
		name       string
		invoke     func(*QueryLoop) error
		prefix     []types.Message
		assertUser func(*testing.T, types.Message)
	}{
		{
			name: "Run",
			invoke: func(q *QueryLoop) error {
				return q.Run(context.Background(), "plain current user", func(stream.Event) {})
			},
			assertUser: func(t *testing.T, message types.Message) {
				if message.GetText() != "plain current user" {
					t.Fatalf("current user = %q", message.GetText())
				}
			},
		},
		{
			name: "RunWithContent",
			invoke: func(q *QueryLoop) error {
				return q.RunWithContent(context.Background(), []types.ContentBlock{
					types.TextBlock{Type: types.ContentTypeText, Text: "multimodal current user"},
					types.ImageBlock{Type: types.ContentTypeImage, Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: "AA=="}},
				}, func(stream.Event) {})
			},
			assertUser: func(t *testing.T, message types.Message) {
				if message.GetText() != "multimodal current user" || len(message.Content) != 2 {
					t.Fatalf("multimodal current user = %#v", message)
				}
				if _, ok := message.Content[1].(types.ImageBlock); !ok {
					t.Fatalf("multimodal block = %T", message.Content[1])
				}
			},
		},
		{
			name: "RunPrepared",
			prefix: []types.Message{
				types.UserMessage("older user"),
				types.AssistantMessage("older assistant"),
			},
			invoke: func(q *QueryLoop) error { return q.RunPrepared(context.Background(), func(stream.Event) {}) },
			assertUser: func(t *testing.T, message types.Message) {
				if message.GetText() != "prepared current user" {
					t.Fatalf("prepared current user = %q", message.GetText())
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, _ := task19SkillManager(t, "initial summary", "initial body")
			recording := &task19RecordingProvider{turns: []task19ProviderTurn{{events: task19TextEvents("done", "")}}}
			q := New(recording, registry.New(), Config{MaxTurns: 2, MaxTokens: 1024, SessionID: "task19-ordering", SkillManager: manager})
			if test.name == "RunPrepared" {
				messages := append([]types.Message(nil), test.prefix...)
				messages = append(messages, types.UserMessage("prepared current user"))
				q.SetMessages(messages)
			}
			if err := test.invoke(q); err != nil {
				t.Fatalf("%s: %v", test.name, err)
			}
			calls := recording.Calls()
			if len(calls) != 1 {
				t.Fatalf("provider calls = %d, want 1", len(calls))
			}
			messages := calls[0].Messages
			if len(messages) != len(test.prefix)+2 {
				t.Fatalf("provider messages = %#v, want prefix + catalog + current user", messages)
			}
			if len(test.prefix) > 0 && !reflect.DeepEqual(messages[:len(test.prefix)], test.prefix) {
				t.Fatalf("older prefix changed\n got: %#v\nwant: %#v", messages[:len(test.prefix)], test.prefix)
			}
			assertTask19CatalogMessage(t, messages[len(test.prefix)], types.DeveloperMessageKindSkillCatalogSnapshot)
			currentUser := messages[len(test.prefix)+1]
			if currentUser.Role != types.RoleUser {
				t.Fatalf("message after catalog role = %q, want user", currentUser.Role)
			}
			test.assertUser(t, currentUser)
		})
	}
}

func TestSkillCatalogRestoredHistoryPreservesCatalogPrefix(t *testing.T) {
	manager, _ := task19SkillManager(t, "restored summary", "restored body")
	controlScope := messagecontrol.NewScope("task19-source", "task19-project", 1)
	sourceProvider := &task19RecordingProvider{turns: []task19ProviderTurn{{events: task19TextEvents("source answer", "source-response")}}}
	source := New(sourceProvider, registry.New(), Config{
		MaxTurns: 2, MaxTokens: 1024, SessionID: "task19-source", SkillManager: manager,
	})
	if !source.SetInternalControlScope(messagecontrol.Runtime(), controlScope) {
		t.Fatal("failed to install authoritative source scope")
	}
	if err := source.Run(context.Background(), "hello", func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	history := source.Messages()
	if got := task19CatalogMessageCount(history); got != 1 {
		t.Fatalf("source catalog count = %d, want 1", got)
	}
	historyJSON := task19MessagesJSON(t, history)

	forkProvider := &task19RecordingProvider{turns: []task19ProviderTurn{{events: task19TextEvents("fork answer", "fork-response")}}}
	fork := New(forkProvider, registry.New(), Config{
		MaxTurns: 2, MaxTokens: 1024, SessionID: "task19-source", SkillManager: manager,
	})
	if !fork.SetInternalControlScope(messagecontrol.Runtime(), controlScope) {
		t.Fatal("failed to install authoritative resume scope")
	}
	beforeEpoch := fork.SkillCatalogState().ContextEpoch
	fork.SetMessages(history)
	if got := fork.SkillCatalogState().ContextEpoch; got != beforeEpoch+1 {
		t.Fatalf("restored history epoch = %d, want %d", got, beforeEpoch+1)
	}
	if err := fork.Run(context.Background(), "fork question", func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}

	calls := forkProvider.Calls()
	if len(calls) != 1 {
		t.Fatalf("fork provider calls = %d, want 1", len(calls))
	}
	messages := calls[0].Messages
	if len(messages) != len(history)+1 {
		t.Fatalf("fork request messages = %d, want restored prefix + current user (%d)", len(messages), len(history)+1)
	}
	if got := task19MessagesJSON(t, messages[:len(history)]); got != historyJSON {
		t.Fatalf("fork request changed restored prefix\n got: %s\nwant: %s", got, historyJSON)
	}
	if got := task19CatalogMessageCount(messages); got != 1 {
		t.Fatalf("fork request catalog count = %d, want 1", got)
	}
	if messages[len(history)].GetText() != "fork question" {
		t.Fatalf("fork request tail = %#v", messages[len(history):])
	}
	state := fork.SkillCatalogState()
	if state.Cursor.Empty() || state.Cursor.ContextEpoch != skillCatalogContextEpoch(state.ContextEpoch) {
		t.Fatalf("fork restored cursor = %#v", state)
	}
}

func TestSkillCatalogDeltaOrderingPreservesPrefixAndNoChange(t *testing.T) {
	manager, skillFile := task19SkillManager(t, "summary v1", "body v1")
	recording := &task19RecordingProvider{turns: []task19ProviderTurn{
		{events: task19TextEvents("answer one", "")},
		{events: task19TextEvents("answer two", "")},
		{events: task19TextEvents("answer three", "")},
	}}
	q := New(recording, registry.New(), Config{MaxTurns: 2, MaxTokens: 1024, SessionID: "task19-delta", SkillManager: manager})

	if err := q.Run(context.Background(), "user one", func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	beforeDelta := q.Messages()
	beforeDeltaJSON := task19MessagesJSON(t, beforeDelta)
	task19WriteSkill(t, skillFile, "summary v2", "body v2")
	if _, err := manager.RefreshSnapshot("task19-delta"); err != nil {
		t.Fatal(err)
	}
	if err := q.Run(context.Background(), "user two", func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}

	calls := recording.Calls()
	if len(calls) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(calls))
	}
	second := calls[1].Messages
	if len(second) != len(beforeDelta)+2 {
		t.Fatalf("changed request messages = %d, want %d", len(second), len(beforeDelta)+2)
	}
	if got := task19MessagesJSON(t, second[:len(beforeDelta)]); got != beforeDeltaJSON {
		t.Fatalf("changed request rewrote old history bytes\n got: %s\nwant: %s", got, beforeDeltaJSON)
	}
	assertTask19CatalogMessage(t, second[len(beforeDelta)], types.DeveloperMessageKindSkillCatalogDelta)
	if second[len(beforeDelta)+1].GetText() != "user two" {
		t.Fatalf("delta is not immediately before current user: %#v", second[len(beforeDelta):])
	}

	beforeNoChange := q.Messages()
	beforeNoChangeJSON := task19MessagesJSON(t, beforeNoChange)
	if err := q.Run(context.Background(), "user three", func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	calls = recording.Calls()
	third := calls[2].Messages
	if len(third) != len(beforeNoChange)+1 {
		t.Fatalf("no-change request appended extra messages: got %d want %d", len(third), len(beforeNoChange)+1)
	}
	if got := task19MessagesJSON(t, third[:len(beforeNoChange)]); got != beforeNoChangeJSON {
		t.Fatalf("no-change request rewrote old history bytes\n got: %s\nwant: %s", got, beforeNoChangeJSON)
	}
	if third[len(beforeNoChange)].GetText() != "user three" {
		t.Fatalf("no-change tail = %#v", third[len(beforeNoChange):])
	}
	if got, want := task19CatalogMessageCount(third), task19CatalogMessageCount(beforeNoChange); got != want {
		t.Fatalf("no-change catalog count = %d, want unchanged %d", got, want)
	}
}

func TestSkillCatalogRetryOrderingReusesPreparedRequest(t *testing.T) {
	manager, _ := task19SkillManager(t, "retry summary", "retry body")
	recording := &task19RecordingProvider{turns: []task19ProviderTurn{
		{}, // Empty response: the loop immediately retries the same prepared request.
		{events: task19TextEvents("retry recovered", "")},
	}}
	q := New(recording, registry.New(), Config{MaxTurns: 2, MaxTokens: 1024, SessionID: "task19-retry", SkillManager: manager})
	if err := q.Run(context.Background(), "retry current user", func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	calls := recording.Calls()
	if len(calls) != 2 {
		t.Fatalf("provider calls = %d, want empty-response retry", len(calls))
	}
	if got, want := task19MessagesJSON(t, calls[1].Messages), task19MessagesJSON(t, calls[0].Messages); got != want {
		t.Fatalf("retry rebuilt or reordered prepared messages\nsecond: %s\n first: %s", got, want)
	}
	if len(calls[0].Messages) != 2 {
		t.Fatalf("prepared retry messages = %#v", calls[0].Messages)
	}
	assertTask19CatalogMessage(t, calls[0].Messages[0], types.DeveloperMessageKindSkillCatalogSnapshot)
	if calls[0].Messages[1].GetText() != "retry current user" {
		t.Fatalf("prepared retry user = %#v", calls[0].Messages[1])
	}
}

func TestSkillCatalogVisibleReceiptCommitsAfterExactEnvelopeAppend(t *testing.T) {
	recording := &task19RecordingProvider{turns: []task19ProviderTurn{
		{events: task19ToolEvents("task19_skill_use", "Skill", nil, "")},
		{events: task19TextEvents("done", "")},
	}}
	reg := registry.New()
	tool := &task19ReceiptSkillTool{skill: task19EnvelopeSkill(task19SkillDigestA)}
	reg.Register(tool)
	q := New(recording, reg, Config{MaxTurns: 3, MaxTokens: 1024, SessionID: "task19-receipt"})
	tool.query = q
	if err := q.Run(context.Background(), "load skill", func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	loaded := q.SkillLoadedLedgerState(tool.skill.ID)
	if loaded.LoadedContextEpoch != loaded.ContextEpoch || loaded.ContentDigest != tool.skill.Digest || loaded.PayloadDigest != skills.DigestInvocationPayload(tool.body()) {
		t.Fatalf("visible exact receipt was not committed: %#v", loaded)
	}
}

func TestSkillCatalogVisibleReceiptRejectsInvisibleFailedCancelledStaleAndReplaced(t *testing.T) {
	skill := task19EnvelopeSkill(task19SkillDigestA)
	body := "task19 exact visible body"
	envelope, err := skills.RenderFullInvocationEnvelope(skill, body, skills.InvocationArguments{})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		mutate    func(*QueryLoop, *types.ToolResultBlock, *[]types.Message, *skills.SkillExecutionReceipt)
		wantError bool
	}{
		{name: "invisible", mutate: func(_ *QueryLoop, _ *types.ToolResultBlock, visible *[]types.Message, _ *skills.SkillExecutionReceipt) {
			*visible = nil
		}},
		{name: "failed", mutate: func(_ *QueryLoop, result *types.ToolResultBlock, visible *[]types.Message, _ *skills.SkillExecutionReceipt) {
			result.IsError, result.Outcome = true, types.ToolOutcomeFailed
			*visible = []types.Message{types.ToolResultMessage(*result)}
		}},
		{name: "cancelled", mutate: func(_ *QueryLoop, result *types.ToolResultBlock, visible *[]types.Message, _ *skills.SkillExecutionReceipt) {
			result.IsError, result.Outcome = true, types.ToolOutcomeCancelled
			*visible = []types.Message{types.ToolResultMessage(*result)}
		}},
		{name: "stale epoch", mutate: func(q *QueryLoop, _ *types.ToolResultBlock, _ *[]types.Message, _ *skills.SkillExecutionReceipt) {
			q.installVisibleHistory([]types.Message{types.UserMessage("replacement")})
		}},
		{name: "aggregate replacement", mutate: func(_ *QueryLoop, result *types.ToolResultBlock, visible *[]types.Message, _ *skills.SkillExecutionReceipt) {
			replaced := *result
			replaced.Content = "<persisted-output>replacement stub</persisted-output>"
			*visible = []types.Message{types.ToolResultMessage(replaced)}
		}},
		{name: "forged receipt metadata without envelope", mutate: func(_ *QueryLoop, result *types.ToolResultBlock, visible *[]types.Message, _ *skills.SkillExecutionReceipt) {
			result.Content = "not a skill invocation envelope"
			*visible = []types.Message{types.ToolResultMessage(*result)}
		}},
		{name: "envelope receipt identity mismatch", mutate: func(_ *QueryLoop, result *types.ToolResultBlock, visible *[]types.Message, receipt *skills.SkillExecutionReceipt) {
			receipt.SkillID = "skill:project:/repo/.agents/skills/other"
			result.Metadata = task19ReceiptMetadata(t, *receipt)
			*visible = []types.Message{types.ToolResultMessage(*result)}
		}},
		{name: "tampered body digest", mutate: func(_ *QueryLoop, result *types.ToolResultBlock, visible *[]types.Message, _ *skills.SkillExecutionReceipt) {
			var wire map[string]any
			if err := json.Unmarshal([]byte(result.Content), &wire); err != nil {
				t.Fatal(err)
			}
			wire["body"] = "tampered after digest"
			encoded, err := json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			result.Content = string(encoded)
			*visible = []types.Message{types.ToolResultMessage(*result)}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			q := New(nil, nil, Config{})
			receipt := skills.SkillExecutionReceipt{
				ContextEpoch:            q.SkillCatalogState().ContextEpoch,
				SkillID:                 skill.ID,
				ContentDigest:           skill.Digest,
				InvocationPayloadDigest: skills.DigestInvocationPayload(body),
				InvocationEnvelopeKind:  skills.InvocationEnvelopeFull,
			}
			result := types.ToolResultBlock{
				Type:      types.ContentTypeToolResult,
				ToolUseID: "task19_skill_use",
				Content:   envelope,
				Metadata:  task19ReceiptMetadata(t, receipt),
				Outcome:   types.ToolOutcomeSucceeded,
			}
			visible := []types.Message{types.ToolResultMessage(result)}
			test.mutate(q, &result, &visible, &receipt)
			err := q.commitVisibleSkillExecutionReceipts(visible, []types.ToolUseBlock{{ID: result.ToolUseID, Name: "Skill"}}, []types.ToolResultBlock{result})
			if test.wantError && err == nil {
				t.Fatal("expected receipt validation error")
			}
			if !test.wantError && err != nil {
				t.Fatal(err)
			}
			if got := q.SkillLoadedLedgerState(skill.ID); got.LoadedContextEpoch != 0 {
				t.Fatalf("rejected receipt mutated loaded ledger: %#v", got)
			}
		})
	}
}

func TestSkillCatalogVisibleReceiptRejectsResultStoreStubAndAlreadyLoadedBootstrap(t *testing.T) {
	skill := task19EnvelopeSkill(task19SkillDigestA)
	body := strings.Repeat("large skill body ", 200)
	full, err := skills.RenderFullInvocationEnvelope(skill, body, skills.InvocationArguments{})
	if err != nil {
		t.Fatal(err)
	}
	q := New(nil, nil, Config{})
	receipt := skills.SkillExecutionReceipt{
		ContextEpoch:            q.SkillCatalogState().ContextEpoch,
		SkillID:                 skill.ID,
		ContentDigest:           skill.Digest,
		InvocationPayloadDigest: skills.DigestInvocationPayload(body),
		InvocationEnvelopeKind:  skills.InvocationEnvelopeFull,
	}
	original := types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: "task19_persisted_skill",
		Content:   full,
		Metadata:  task19ReceiptMetadata(t, receipt),
		Outcome:   types.ToolOutcomeSucceeded,
	}
	original.Metadata["maxResultSizeChars"] = "1"
	store := compact.NewResultStore(t.TempDir())
	processed, err := store.ProcessResultForTool(original, "Skill")
	if err != nil {
		t.Fatal(err)
	}
	if processed.Content == original.Content || !strings.Contains(processed.Content, "persisted-output") {
		t.Fatalf("test did not produce a persisted stub: %q", processed.Content)
	}
	if err := q.commitVisibleSkillExecutionReceipts(
		[]types.Message{types.ToolResultMessage(processed)},
		[]types.ToolUseBlock{{ID: processed.ToolUseID, Name: "Skill"}},
		[]types.ToolResultBlock{processed},
	); err != nil {
		t.Fatal(err)
	}
	if got := q.SkillLoadedLedgerState(skill.ID); got.LoadedContextEpoch != 0 {
		t.Fatalf("persisted envelope stub bootstrapped ledger: %#v", got)
	}

	ack, err := skills.RenderLoadedDigestAcknowledgement(skill, skill.Digest, receipt.InvocationPayloadDigest, body, skills.InvocationArguments{})
	if err != nil {
		t.Fatal(err)
	}
	ackReceipt := receipt
	ackReceipt.InvocationEnvelopeKind = skills.InvocationEnvelopeAlreadyLoaded
	ackResult := types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: "task19_loaded_ack",
		Content:   ack,
		Metadata:  task19ReceiptMetadata(t, ackReceipt),
		Outcome:   types.ToolOutcomeSucceeded,
	}
	if err := q.commitVisibleSkillExecutionReceipts(
		[]types.Message{types.ToolResultMessage(ackResult)},
		[]types.ToolUseBlock{{ID: ackResult.ToolUseID, Name: "Skill"}},
		[]types.ToolResultBlock{ackResult},
	); err != nil {
		t.Fatal(err)
	}
	if got := q.SkillLoadedLedgerState(skill.ID); got.LoadedContextEpoch != 0 {
		t.Fatalf("already-loaded acknowledgement bootstrapped ledger: %#v", got)
	}

	q.skillCatalogMu.Lock()
	q.loadedSkillDigests[skill.ID] = SkillLoadedLedgerEntry{ContentDigest: skill.Digest, PayloadDigest: receipt.InvocationPayloadDigest}
	q.skillCatalogMu.Unlock()
	if err := q.commitVisibleSkillExecutionReceipts(
		[]types.Message{types.ToolResultMessage(ackResult)},
		[]types.ToolUseBlock{{ID: ackResult.ToolUseID, Name: "Skill"}},
		[]types.ToolResultBlock{ackResult},
	); err != nil {
		t.Fatal(err)
	}
	if got := q.SkillLoadedLedgerState(skill.ID); got.ContentDigest != skill.Digest || got.PayloadDigest != receipt.InvocationPayloadDigest || got.LoadedContextEpoch != got.ContextEpoch {
		t.Fatalf("valid already-loaded acknowledgement changed existing evidence: %#v", got)
	}
}

func TestSkillCatalogHistoryReplacementFenceClearsResponseChainAndPreservesPromptCacheKey(t *testing.T) {
	manager, _ := task19SkillManager(t, "compact summary", "compact body")
	recording := &task19RecordingProvider{turns: []task19ProviderTurn{{events: task19TextEvents("after compact", "")}}}
	q := New(recording, registry.New(), Config{
		MaxTurns: 2, MaxTokens: 1024, MaxContextTokens: 1000,
		SessionID: "task19-cache-key", SkillManager: manager,
	})
	q.compactor = task19Compactor{}
	q.SetMessages([]types.Message{types.UserMessage("old"), types.AssistantMessage("old answer"), types.UserMessage("tail")})
	q.skillCatalogMu.Lock()
	q.loadedSkillDigests[task19EnvelopeSkill(task19SkillDigestA).ID] = SkillLoadedLedgerEntry{
		ContentDigest: task19SkillDigestA,
		PayloadDigest: skills.DigestInvocationPayload("old loaded body"),
	}
	q.skillCatalogMu.Unlock()
	before := q.SkillCatalogState().ContextEpoch
	q.lastResponseID = "response-before-compact"
	q.lastEnvelopeFingerprint = "old-envelope"
	q.currentEnvelopeFingerprint = "in-flight-envelope"
	q.disableResponseChain = true
	if _, err := q.ForceCompact(context.Background()); err != nil {
		t.Fatal(err)
	}
	after := q.SkillCatalogState()
	if after.ContextEpoch != before+1 || after.Cursor.Empty() || len(after.LoadedDigests) != 0 {
		t.Fatalf("ForceCompact did not rebuild current visible catalog without stale bodies: before=%d after=%#v", before, after)
	}
	if q.lastResponseID != "" || q.lastEnvelopeFingerprint != "" || q.currentEnvelopeFingerprint != "" || q.disableResponseChain {
		t.Fatalf("ForceCompact retained Responses chain state: id=%q last=%q current=%q disabled=%t", q.lastResponseID, q.lastEnvelopeFingerprint, q.currentEnvelopeFingerprint, q.disableResponseChain)
	}
	if q.config.SessionID != "task19-cache-key" {
		t.Fatalf("ForceCompact changed PromptCacheKey source: %q", q.config.SessionID)
	}
	if err := q.Run(context.Background(), "after compact user", func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	calls := recording.Calls()
	if len(calls) != 1 || calls[0].PreviousResponseID != "" || calls[0].PromptCacheKey != "task19-cache-key" || !calls[0].UsePromptCache {
		t.Fatalf("post-compact request chain/cache = %#v", calls)
	}
	if len(calls[0].Messages) < 2 {
		t.Fatalf("post-compact messages = %#v", calls[0].Messages)
	}
	last := len(calls[0].Messages) - 1
	if calls[0].Messages[last].GetText() != "after compact user" {
		t.Fatalf("post-compact current user missing from tail: %#v", calls[0].Messages)
	}
	snapshotIndex := -1
	for index := last - 1; index >= 0; index-- {
		message := calls[0].Messages[index]
		if message.Role == types.RoleDeveloper && message.DeveloperMetadata != nil &&
			message.DeveloperMetadata.Kind == types.DeveloperMessageKindSkillCatalogSnapshot {
			snapshotIndex = index
			break
		}
	}
	if snapshotIndex < 0 {
		t.Fatalf("post-compact provider history missing rebuilt current snapshot: %#v", calls[0].Messages)
	}
	if got := task19CatalogMessageCount(calls[0].Messages); got != 1 {
		t.Fatalf("unchanged post-compact catalog appended a duplicate snapshot: count=%d messages=%#v", got, calls[0].Messages)
	}
}

func TestSkillCatalogPlanModeHistoryReplacementRebuildsBeforeRestartUser(t *testing.T) {
	manager, _ := task19SkillManager(t, "plan summary", "plan body")
	recording := &task19RecordingProvider{turns: []task19ProviderTurn{
		{events: task19ToolEvents("task19_exit_plan", "ExitPlanMode", nil, "response-before-restart")},
		{events: task19TextEvents("implemented", "response-after-restart")},
	}}
	reg := registry.New()
	reg.Register(task19PlanRestartTool{})
	q := New(recording, reg, Config{MaxTurns: 3, MaxTokens: 1024, SessionID: "task19-plan-cache", SkillManager: manager})
	initialEpoch := q.SkillCatalogState().ContextEpoch
	if err := q.Run(context.Background(), "original plan context", func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	calls := recording.Calls()
	if len(calls) != 2 {
		t.Fatalf("provider calls = %d, want plan turn and restarted turn", len(calls))
	}
	if calls[1].PreviousResponseID != "" {
		t.Fatalf("restart reused previous_response_id %q", calls[1].PreviousResponseID)
	}
	for index, call := range calls {
		if call.PromptCacheKey != "task19-plan-cache" || !call.UsePromptCache {
			t.Fatalf("call %d lost prompt cache affinity: key=%q enabled=%t", index, call.PromptCacheKey, call.UsePromptCache)
		}
	}
	if got := q.SkillCatalogState().ContextEpoch; got != initialEpoch+1 {
		t.Fatalf("plan restart context epoch = %d, want %d", got, initialEpoch+1)
	}
	second := calls[1].Messages
	if len(second) != 2 {
		t.Fatalf("restart request retained old history: %#v", second)
	}
	assertTask19CatalogMessage(t, second[0], types.DeveloperMessageKindSkillCatalogSnapshot)
	if second[1].GetText() != "approved plan content" || strings.Contains(task19MessagesJSON(t, second), "original plan context") {
		t.Fatalf("restart snapshot/user order = %#v", second)
	}
}

func TestSkillCatalogMessageLimitFailsClosedWithoutChangingCacheOrChain(t *testing.T) {
	manager, _ := task19SkillManager(t, "truncate summary", "truncate body")
	recording := &task19RecordingProvider{dynamicToolName: "Tick"}
	reg := registry.New()
	reg.Register(task19TickTool{})
	q := New(recording, reg, Config{MaxTurns: 252, MaxTokens: 1024, SessionID: "task19-truncate-cache", SkillManager: manager})
	err := q.Run(context.Background(), "start long tool loop", func(stream.Event) {})
	if err == nil {
		t.Fatal("long tool loop unexpectedly completed before the message bound")
	}
	var limitErr *MessageHistoryLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("long tool loop error = %T %v, want MessageHistoryLimitError", err, err)
	}
	if limitErr.MessageCount <= limitErr.Limit || limitErr.Limit != maxMessagesHardLimit {
		t.Fatalf("message limit error = %#v", limitErr)
	}
	if got := q.SkillCatalogState().ContextEpoch; got != 1 {
		t.Fatalf("fail-closed message limit changed context epoch: %d", got)
	}
	calls := recording.Calls()
	if len(calls) != 250 {
		t.Fatalf("provider calls = %d, want 250 before the 502-message state was blocked", len(calls))
	}
	for index, call := range calls {
		if call.PromptCacheKey != "task19-truncate-cache" || !call.UsePromptCache {
			t.Fatalf("call %d lost prompt cache affinity: key=%q enabled=%t", index, call.PromptCacheKey, call.UsePromptCache)
		}
		if index > 0 && call.PreviousResponseID == "" {
			t.Fatalf("fail-closed message limit cleared the established Responses chain on call %d", index)
		}
		if got := task19CatalogMessageCount(call.Messages); got > 1 {
			t.Fatalf("message limit duplicated an exact visible catalog snapshot on call %d: count=%d", index, got)
		}
	}
	visible := q.Messages()
	if len(visible) != limitErr.MessageCount || !strings.Contains(joinedMessageText(visible), "start long tool loop") {
		t.Fatalf("fail-closed message limit rewrote visible history: len=%d error=%#v", len(visible), limitErr)
	}
}

type task19ReceiptSkillTool struct {
	query *QueryLoop
	skill skills.EffectiveSkill
}

func (t *task19ReceiptSkillTool) Name() string        { return "Skill" }
func (t *task19ReceiptSkillTool) Description() string { return "task19 receipt skill" }
func (t *task19ReceiptSkillTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *task19ReceiptSkillTool) IsConcurrentSafe() bool { return true }
func (t *task19ReceiptSkillTool) body() string           { return "task19 full skill body" }
func (t *task19ReceiptSkillTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	epoch := t.query.SkillLoadedLedgerState(t.skill.ID).ContextEpoch
	envelope, err := skills.RenderFullInvocationEnvelope(t.skill, t.body(), skills.InvocationArguments{})
	if err != nil {
		return types.ToolResult{}, err
	}
	receipt := skills.SkillExecutionReceipt{
		ContextEpoch:            epoch,
		SkillID:                 t.skill.ID,
		ContentDigest:           t.skill.Digest,
		InvocationPayloadDigest: skills.DigestInvocationPayload(t.body()),
		InvocationEnvelopeKind:  skills.InvocationEnvelopeFull,
	}
	metadata, err := skills.EncodeSkillExecutionReceiptMetadata(receipt)
	if err != nil {
		return types.ToolResult{}, err
	}
	return types.ToolResult{Content: envelope, Metadata: metadata}, nil
}

type task19PlanRestartTool struct{}

func (task19PlanRestartTool) Name() string        { return "ExitPlanMode" }
func (task19PlanRestartTool) Description() string { return "task19 plan restart" }
func (task19PlanRestartTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (task19PlanRestartTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{
		Content:  "approved plan content",
		Metadata: map[string]string{"clearContext": "true", "restartExecution": "true"},
	}, nil
}

type task19TickTool struct{}

func (task19TickTool) Name() string        { return "Tick" }
func (task19TickTool) Description() string { return "task19 tick" }
func (task19TickTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (task19TickTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{Content: "tick"}, nil
}

type task19Compactor struct{}

func (task19Compactor) Compact(_ context.Context, messages []types.Message, _ int) (*compact.CompactionResult, error) {
	boundary := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{Trigger: "manual"})
	return &compact.CompactionResult{
		BoundaryMarker:       &boundary,
		SummaryMessages:      []types.Message{types.UserMessage("task19 compact summary")},
		MessagesToKeep:       []types.Message{messages[len(messages)-1]},
		PreCompactTokenCount: len(messages),
	}, nil
}

func task19SkillManager(t *testing.T, summary, body string) (*skills.Manager, string) {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "task19-skill")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "SKILL.md")
	task19WriteSkill(t, path, summary, body)
	manager := newLoopTestSkillManager(t, skills.DirSource{Dir: filepath.Dir(directory), Source: skills.SourceProject})
	binding, err := manager.SnapshotBinding("task19-session")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := binding.Snapshot
	if len(snapshot.Skills) != 1 {
		t.Fatalf("task19 manager loaded %d skills, want 1", len(snapshot.Skills))
	}
	return manager, path
}

func task19WriteSkill(t *testing.T, path, summary, body string) {
	t.Helper()
	content := fmt.Sprintf("---\ndescription: %s\n---\n# Task 19\n\n%s\n", summary, body)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func task19EnvelopeSkill(digest skills.SkillDigest) skills.EffectiveSkill {
	return skills.EffectiveSkill{
		ID:                 "skill:project:/repo/.agents/skills/task19",
		Name:               "task19",
		Summary:            "Task 19 envelope",
		Source:             skills.SourceProject,
		Locator:            "/repo/.agents/skills/task19/SKILL.md",
		Digest:             digest,
		Revision:           1,
		Visibility:         skills.VisibilityAuto,
		VisibilitySource:   skills.SkillScopeProject,
		ModelVisible:       true,
		DescriptionVisible: true,
		UserInvocable:      true,
		Executable:         true,
		Mutable:            true,
	}
}

func task19ReceiptMetadata(t *testing.T, receipt skills.SkillExecutionReceipt) map[string]string {
	t.Helper()
	metadata, err := skills.EncodeSkillExecutionReceiptMetadata(receipt)
	if err != nil {
		t.Fatal(err)
	}
	return metadata
}

func task19TextEvents(text, responseID string) []types.StreamEvent {
	stop := types.StopReasonEndTurn
	return []types.StreamEvent{
		{Type: types.EventMessageStart},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: text}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: &stop},
		{Type: types.EventMessageStop, ResponseID: responseID},
	}
}

func task19ToolEvents(toolUseID, name string, input map[string]any, responseID string) []types.StreamEvent {
	if input == nil {
		input = map[string]any{}
	}
	encoded, _ := json.Marshal(input)
	stop := types.StopReasonToolUse
	return []types.StreamEvent{
		{Type: types.EventMessageStart},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeToolUse, ID: toolUseID, Name: name}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(encoded)}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: &stop},
		{Type: types.EventMessageStop, ResponseID: responseID},
	}
}

func assertTask19CatalogMessage(t *testing.T, message types.Message, kind types.DeveloperMessageKind) {
	t.Helper()
	if message.Role != types.RoleDeveloper || !message.IsMeta || message.DeveloperMetadata == nil || message.DeveloperMetadata.Kind != kind {
		t.Fatalf("catalog message = %#v, want internal developer %q", message, kind)
	}
}

func task19CatalogMessageCount(messages []types.Message) int {
	count := 0
	for _, message := range messages {
		if message.Role == types.RoleDeveloper && message.DeveloperMetadata != nil {
			switch message.DeveloperMetadata.Kind {
			case types.DeveloperMessageKindSkillCatalogSnapshot, types.DeveloperMessageKindSkillCatalogDelta:
				count++
			}
		}
	}
	return count
}

func task19MessagesJSON(t *testing.T, messages []types.Message) string {
	t.Helper()
	encoded, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
