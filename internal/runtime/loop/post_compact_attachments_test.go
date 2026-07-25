package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

func TestPostCompactSkillBodyMetadataUsesLubanNamespace(t *testing.T) {
	if got, want := postCompactSkillBodyToolResultMetadataKey, "luban_post_compact_skill_body"; got != want {
		t.Fatalf("post-compact skill body metadata key = %q, want %q", got, want)
	}

	_, rows := task22SkillManager(t, 1)
	row := rows[0]
	envelope := task22FullEnvelope(t, row, "metadata namespace body")
	candidate, err := decodePostCompactSkillBody(envelope, 0)
	if err != nil {
		t.Fatal(err)
	}
	bound := bindPostCompactSkillToolResultProvenance(types.ToolResultBlock{}, candidate, 1)
	if got := bound.Metadata[postCompactSkillBodyToolResultMetadataKey]; got == "" {
		t.Fatalf("Luban post-compact metadata proof is empty: %#v", bound.Metadata)
	}
	if len(bound.Metadata) != 1 {
		t.Fatalf("post-compact metadata = %#v, want only the Luban proof key", bound.Metadata)
	}
	legacy := types.ToolResultBlock{Metadata: map[string]string{
		"claude_code_post_compact_skill_body": bound.Metadata[postCompactSkillBodyToolResultMetadataKey],
	}}
	if validPostCompactSkillToolResultProvenance(legacy, candidate, 1) {
		t.Fatal("historical Claude Code metadata key remains readable")
	}
}

func TestForceCompactRestoresRuntimeAttachments(t *testing.T) {
	p := &mockProvider{
		responses: [][]types.StreamEvent{textEvents(`{"schema":"compact-summary/v2","summary":"compact summary"}`)},
	}
	reg := registry.New()
	reg.Register(&toolSearchLoaderMockTool{})
	reg.Register(&namedMockTool{name: "TaskCreate", desc: "create task"})

	q := New(p, reg, Config{
		MaxTurns:         1,
		MaxTokens:        1024,
		MaxContextTokens: 200000,
		SessionID:        "session-1",
	})
	q.loadedToolNames["TaskCreate"] = struct{}{}

	msgs := make([]types.Message, 0, 22)
	for i := 0; i < 11; i++ {
		msgs = append(msgs, types.UserMessage("user"))
		msgs = append(msgs, types.AssistantMessage("assistant"))
	}
	q.SetMessages(msgs)

	if _, err := q.ForceCompact(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := q.Messages()
	joined := joinLoopMessages(got)
	for _, want := range []string{
		"Post-compaction deferred tools",
		"TaskCreate",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("post-compact messages missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "Post-compaction invoked skills") {
		t.Fatalf("post-compact messages retained obsolete invoked-name reminder:\n%s", joined)
	}

	summaryIdx := indexContaining(got, "compact summary")
	keepIdx := indexContainingFrom(got, "assistant", summaryIdx+1)
	attachIdx := indexContaining(got, "Post-compaction deferred tools")
	if summaryIdx < 0 || keepIdx < 0 || attachIdx < 0 {
		t.Fatalf("missing expected segments summary=%d keep=%d attachment=%d", summaryIdx, keepIdx, attachIdx)
	}
	if !(summaryIdx < keepIdx && keepIdx < attachIdx) {
		t.Fatalf("message order should be boundary, summary, kept messages, attachments; got summary=%d keep=%d attachment=%d", summaryIdx, keepIdx, attachIdx)
	}
}

func TestPostCompactSkillCatalogRebuildsCurrentSnapshot(t *testing.T) {
	dir := t.TempDir()
	writeLoopSkill(t, dir, "alpha", "Alpha skill")

	manager := newLoopTestSkillManager(t, skills.DirSource{Dir: dir, Source: skills.SourceProject})
	p := newParityFakeProvider([]parityProviderTurn{
		{Events: parityTextEvents("first")},
		{Events: parityTextEvents("second")},
	})
	q := New(p, registry.New(), Config{
		MaxTurns:       1,
		MaxTokens:      1024,
		SessionID:      "post-compact-snapshot",
		SkillManager:   manager,
		TranscriptPath: "",
	})
	q.compactor = &compact.SummaryCompactor{
		SummarizeMessages: func(_ context.Context, _ []types.Message, _ string) (string, error) {
			return "compact summary", nil
		},
		KeepRecent: 1,
	}

	if err := q.Run(context.Background(), "first turn", func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	if len(p.Calls) != 1 {
		t.Fatalf("provider calls after first run = %d, want 1", len(p.Calls))
	}
	if got := joinLoopMessages(p.Calls[0].Messages); !strings.Contains(got, "Alpha skill") {
		t.Fatalf("first provider request missing initial skill listing:\n%s", got)
	}

	if _, err := q.ForceCompact(context.Background()); err != nil {
		t.Fatal(err)
	}
	writeLoopSkill(t, dir, "beta", "Beta skill")

	if err := q.Run(context.Background(), "second turn", func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	if len(p.Calls) != 2 {
		t.Fatalf("provider calls after second run = %d, want 2", len(p.Calls))
	}
	lastMessages := p.Calls[1].Messages
	last := joinLoopMessages(lastMessages)
	if !strings.Contains(last, "Beta skill") || !strings.Contains(last, "Alpha skill") {
		t.Fatalf("post-compact provider request missing current full skill snapshot:\n%s", last)
	}
	userIndex := indexContaining(lastMessages, "second turn")
	if userIndex <= 0 || lastMessages[userIndex-1].Role != types.RoleDeveloper ||
		lastMessages[userIndex-1].DeveloperMetadata == nil ||
		lastMessages[userIndex-1].DeveloperMetadata.Kind != types.DeveloperMessageKindSkillCatalogDelta {
		t.Fatalf("post-compact change delta is not immediately before current user: %#v", lastMessages)
	}
	snapshotFound := false
	for _, message := range lastMessages[:userIndex-1] {
		if message.Role == types.RoleDeveloper && message.DeveloperMetadata != nil &&
			message.DeveloperMetadata.Kind == types.DeveloperMessageKindSkillCatalogSnapshot &&
			strings.Contains(message.GetText(), "Alpha skill") {
			snapshotFound = true
		}
	}
	if !snapshotFound {
		t.Fatalf("post-compact history missing rebuilt full snapshot before later delta: %#v", lastMessages)
	}
	if !strings.Contains(last, "Transcript reference: unavailable") {
		t.Fatalf("post-compact provider request missing transcript placeholder:\n%s", last)
	}
}

func TestSkillSummaryUpdateAndDeletionAppendCatalogDeltas(t *testing.T) {
	dir := t.TempDir()
	writeLoopSkill(t, dir, "alpha", "Original summary")

	manager := newLoopTestSkillManager(t, skills.DirSource{Dir: dir, Source: skills.SourceProject})
	p := newParityFakeProvider([]parityProviderTurn{
		{Events: parityTextEvents("first")},
		{Events: parityTextEvents("second")},
		{Events: parityTextEvents("third")},
	})
	q := New(p, registry.New(), Config{
		MaxTurns: 1, MaxTokens: 1024, SessionID: "skill-catalog-deltas", SkillManager: manager,
	})

	if err := q.Run(context.Background(), "first turn", func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	writeLoopSkill(t, dir, "alpha", "Updated skill summary with a different size")
	if err := q.Run(context.Background(), "second turn", func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	if second := joinLoopMessages(p.Calls[1].Messages); !strings.Contains(second, "Updated skill summary with a different size") {
		t.Fatalf("same-name summary update was not announced:\n%s", second)
	}

	if err := os.RemoveAll(filepath.Join(dir, "alpha")); err != nil {
		t.Fatal(err)
	}
	if err := q.Run(context.Background(), "third turn", func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	third := joinLoopMessages(p.Calls[2].Messages)
	if !strings.Contains(third, `"type":"skill_catalog_delta"`) ||
		!strings.Contains(third, `"reason":"deleted"`) || !strings.Contains(third, `"name":"alpha"`) {
		t.Fatalf("deleted skill tombstone missing:\n%s", third)
	}
}

func TestSkillCatalogPostCompactReattachesExactBodyAndRebuildsLedger(t *testing.T) {
	manager, rows := task22SkillManager(t, 1)
	row := rows[0]
	envelope := task22FullEnvelope(t, row, "task22 exact rendered body")
	original := task22SkillExchange(row, "task22_use", envelope)
	replacement := []types.Message{types.UserMessage("compacted summary"), types.UserMessage("current user")}
	q := New(nil, registry.New(), Config{
		SessionID: "task22-session", SkillManager: manager, MaxContextTokens: 20_000,
	})
	beforeEpoch := q.SkillCatalogState().ContextEpoch
	q.lastResponseID = "response-before-replacement"
	q.lastEnvelopeFingerprint = "old-envelope"
	q.currentEnvelopeFingerprint = "in-flight"
	q.disableResponseChain = true

	installed, err := q.installPostCompactVisibleHistory(original, replacement)
	if err != nil {
		t.Fatal(err)
	}
	state := q.SkillCatalogState()
	if state.ContextEpoch != beforeEpoch+1 || state.Cursor.Empty() {
		t.Fatalf("post-compact catalog state = %#v, want new epoch and current cursor", state)
	}
	loaded, ok := state.LoadedDigests[row.ID]
	if !ok || loaded.ContentDigest != row.Digest || loaded.PayloadDigest != skills.DigestInvocationPayload("task22 exact rendered body") {
		t.Fatalf("post-compact loaded ledger = %#v", state.LoadedDigests)
	}
	if q.lastResponseID != "" || q.lastEnvelopeFingerprint != "" || q.currentEnvelopeFingerprint != "" || q.disableResponseChain {
		t.Fatalf("Responses chain survived replacement: id=%q last=%q current=%q disabled=%t",
			q.lastResponseID, q.lastEnvelopeFingerprint, q.currentEnvelopeFingerprint, q.disableResponseChain)
	}
	if q.config.SessionID != "task22-session" {
		t.Fatalf("history replacement changed PromptCacheKey source: %q", q.config.SessionID)
	}
	userIndex := indexContaining(installed, "current user")
	if userIndex < 2 || installed[userIndex-2].Role != types.RoleDeveloper ||
		installed[userIndex-2].DeveloperMetadata == nil ||
		installed[userIndex-2].DeveloperMetadata.Kind != types.DeveloperMessageKindSkillCatalogSnapshot ||
		installed[userIndex-1].GetText() != envelope {
		t.Fatalf("snapshot/body/current-user order = %#v", installed)
	}
}

func TestPostCompactSkillManualForceRestoresBodyAndCurrentSnapshot(t *testing.T) {
	manager, rows := task22SkillManager(t, 1)
	row := rows[0]
	envelope := task22FullEnvelope(t, row, "manual compact body")
	messages := append(task22SkillExchange(row, "manual_use", envelope), types.AssistantMessage("tail answer"))
	q := New(nil, registry.New(), Config{
		SessionID: "task22-manual", SkillManager: manager, MaxContextTokens: 20_000,
	})
	q.compactor = &prepareCountingCompactor{summaryMessage: "manual summary"}
	q.SetMessages(messages)
	q.lastResponseID = "response-before-manual"
	before := q.SkillCatalogState().ContextEpoch

	if _, err := q.ForceCompact(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := q.Messages()
	state := q.SkillCatalogState()
	if state.ContextEpoch != before+1 || state.Cursor.Empty() || len(state.LoadedDigests) != 1 {
		t.Fatalf("manual post-compact state = %#v", state)
	}
	if task22EnvelopeOccurrenceCount(got, envelope) != 1 {
		t.Fatalf("manual compact did not reattach exactly one body: %#v", got)
	}
	snapshotFound := false
	for _, message := range got {
		if message.Role == types.RoleDeveloper && message.DeveloperMetadata != nil &&
			message.DeveloperMetadata.Kind == types.DeveloperMessageKindSkillCatalogSnapshot {
			snapshotFound = true
		}
	}
	if !snapshotFound {
		t.Fatalf("manual compact missing current full catalog snapshot: %#v", got)
	}
	if q.lastResponseID != "" || q.config.SessionID != "task22-manual" {
		t.Fatalf("manual compact chain/cache = response %q session %q", q.lastResponseID, q.config.SessionID)
	}
}

func TestPostCompactSkillRetainedBodyDoesNotDuplicateAndMissingHistoryClearsLedger(t *testing.T) {
	manager, rows := task22SkillManager(t, 1)
	row := rows[0]
	envelope := task22FullEnvelope(t, row, "retained body")
	exchange := task22SkillExchange(row, "retained_use", envelope)
	q := New(nil, registry.New(), Config{SessionID: "task22-retained", SkillManager: manager})

	installed, err := q.installPostCompactVisibleHistory(exchange, exchange)
	if err != nil {
		t.Fatal(err)
	}
	if got := task22EnvelopeOccurrenceCount(installed, envelope); got != 1 {
		t.Fatalf("retained body occurrences = %d, want 1", got)
	}
	if got := q.SkillLoadedLedgerState(row.ID); got.LoadedContextEpoch != got.ContextEpoch {
		t.Fatalf("retained body did not rebuild ledger: %#v", got)
	}

	q.skillCatalogMu.Lock()
	q.loadedSkillDigests[row.ID] = SkillLoadedLedgerEntry{ContentDigest: row.Digest, PayloadDigest: skills.DigestInvocationPayload("stale ledger")}
	q.skillCatalogMu.Unlock()
	installed, err = q.installPostCompactVisibleHistory(nil, []types.Message{types.UserMessage("no skill body survived")})
	if err != nil {
		t.Fatal(err)
	}
	if got := q.SkillLoadedLedgerState(row.ID); got.LoadedContextEpoch != 0 {
		t.Fatalf("missing visible body retained stale ledger: %#v messages=%#v", got, installed)
	}

	boundary := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{Trigger: "manual"}, messagecontrol.Runtime()).
		WithInternalControlProvenance(messagecontrol.Runtime(), q.internalControlScope)
	preBoundaryOnly := append(task22SkillExchange(row, "old_epoch_use", envelope), boundary, types.UserMessage("current epoch without body"))
	installed, err = q.installPostCompactVisibleHistory(preBoundaryOnly, []types.Message{types.UserMessage("replacement")})
	if err != nil {
		t.Fatal(err)
	}
	if task22EnvelopeOccurrenceCount(installed, envelope) != 0 || q.SkillLoadedLedgerState(row.ID).LoadedContextEpoch != 0 {
		t.Fatalf("pre-boundary body was treated as current visible evidence: messages=%#v state=%#v", installed, q.SkillCatalogState())
	}
}

func TestPostCompactSkillSupersedingEnvelopeRestoresLatestBody(t *testing.T) {
	manager, rows := task22SkillManager(t, 1)
	row := rows[0]
	previous := skills.SkillDigest("sha256:" + strings.Repeat("a", 64))
	envelope, err := skills.RenderSupersedingInvocationEnvelope(row, previous, "superseding current body", skills.InvocationArguments{})
	if err != nil {
		t.Fatal(err)
	}
	q := New(nil, registry.New(), Config{SessionID: "task22-superseding", SkillManager: manager})
	installed, err := q.installPostCompactVisibleHistory(
		task22SkillExchange(row, "superseding_use", envelope),
		[]types.Message{types.UserMessage("summary")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if task22EnvelopeOccurrenceCount(installed, envelope) != 1 {
		t.Fatalf("superseding envelope was not restored exactly once: %#v", installed)
	}
	loaded := q.SkillLoadedLedgerState(row.ID)
	if loaded.LoadedContextEpoch != loaded.ContextEpoch || loaded.ContentDigest != row.Digest ||
		loaded.PayloadDigest != skills.DigestInvocationPayload("superseding current body") {
		t.Fatalf("superseding envelope ledger = %#v", loaded)
	}
}

func TestPostCompactSkillExplicitStandaloneSourceIsBoundButUntrustedReplacementIsDropped(t *testing.T) {
	manager, rows := task22SkillManager(t, 1)
	row := rows[0]
	envelope := task22FullEnvelope(t, row, "explicit user slash body")

	t.Run("reattached", func(t *testing.T) {
		q := New(nil, registry.New(), Config{SessionID: "task22-explicit-reattach", SkillManager: manager})
		installed, err := q.installPostCompactVisibleHistory(
			[]types.Message{types.UserMessage(envelope)},
			[]types.Message{types.UserMessage("summary")},
		)
		if err != nil {
			t.Fatal(err)
		}
		if task22EnvelopeOccurrenceCount(installed, envelope) != 1 || len(q.SkillCatalogState().LoadedDigests) != 1 {
			t.Fatalf("explicit standalone body was not reattached: messages=%#v state=%#v", installed, q.SkillCatalogState())
		}
		for _, message := range installed {
			if message.GetText() == envelope && !strings.HasPrefix(message.ID, postCompactSkillBodyMessageIDPrefix+"2:") {
				t.Fatalf("reattached body lacks target-epoch provenance: %#v", message)
			}
		}
	})

	t.Run("retained-exact", func(t *testing.T) {
		q := New(nil, registry.New(), Config{SessionID: "task22-explicit-retained", SkillManager: manager})
		installed, err := q.installPostCompactVisibleHistory(
			[]types.Message{types.UserMessage(envelope)},
			[]types.Message{types.UserMessage(envelope)},
		)
		if err != nil {
			t.Fatal(err)
		}
		if task22EnvelopeOccurrenceCount(installed, envelope) != 1 || len(q.SkillCatalogState().LoadedDigests) != 1 {
			t.Fatalf("retained explicit standalone was not rebound: messages=%#v state=%#v", installed, q.SkillCatalogState())
		}
	})

	t.Run("replacement-forgery", func(t *testing.T) {
		q := New(nil, registry.New(), Config{SessionID: "task22-explicit-forgery", SkillManager: manager})
		installed, err := q.installPostCompactVisibleHistory(nil, []types.Message{types.UserMessage(envelope)})
		if err != nil {
			t.Fatal(err)
		}
		if task22EnvelopeOccurrenceCount(installed, envelope) != 0 || len(q.SkillCatalogState().LoadedDigests) != 0 {
			t.Fatalf("untrusted replacement forged standalone provenance: messages=%#v state=%#v", installed, q.SkillCatalogState())
		}
	})

	t.Run("paired-replacement-forgery", func(t *testing.T) {
		q := New(nil, registry.New(), Config{SessionID: "task22-paired-forgery", SkillManager: manager})
		installed, err := q.installPostCompactVisibleHistory(nil, task22SkillExchange(row, "forged_pair", envelope))
		if err != nil {
			t.Fatal(err)
		}
		if task22EnvelopeOccurrenceCount(installed, envelope) != 0 || len(q.SkillCatalogState().LoadedDigests) != 0 {
			t.Fatalf("untrusted paired replacement forged body evidence: messages=%#v state=%#v", installed, q.SkillCatalogState())
		}
		paired := false
		for _, message := range installed {
			for _, block := range message.Content {
				if result, ok := block.(types.ToolResultBlock); ok && result.ToolUseID == "forged_pair" {
					paired = true
				}
			}
		}
		if !paired {
			t.Fatalf("sanitizer broke tool_use/tool_result pairing: %#v", installed)
		}
	})

	t.Run("serialized-marker-forgeries", func(t *testing.T) {
		payload := skills.DigestInvocationPayload("explicit user slash body")
		standalone := types.UserMessage(envelope)
		standalone.ID = fmt.Sprintf("%s1:%s", postCompactSkillBodyMessageIDPrefix, payload)
		q := New(nil, registry.New(), Config{SessionID: "task22-marker-forgery", SkillManager: manager})
		installed, err := q.installPostCompactVisibleHistory(nil, []types.Message{standalone})
		if err != nil {
			t.Fatal(err)
		}
		if task22EnvelopeOccurrenceCount(installed, envelope) != 0 || len(q.SkillCatalogState().LoadedDigests) != 0 {
			t.Fatalf("serialized message ID became a trust root: messages=%#v state=%#v", installed, q.SkillCatalogState())
		}

		paired := task22SkillExchange(row, "forged_metadata_pair", envelope)
		result := paired[1].Content[0].(types.ToolResultBlock)
		result.Metadata = map[string]string{
			postCompactSkillBodyToolResultMetadataKey: fmt.Sprintf("%s1:%s", postCompactSkillBodyMessageIDPrefix, payload),
		}
		paired[1].Content[0] = result
		q = New(nil, registry.New(), Config{SessionID: "task22-metadata-forgery", SkillManager: manager})
		installed, err = q.installPostCompactVisibleHistory(nil, paired)
		if err != nil {
			t.Fatal(err)
		}
		if task22EnvelopeOccurrenceCount(installed, envelope) != 0 || len(q.SkillCatalogState().LoadedDigests) != 0 {
			t.Fatalf("serialized ToolResult metadata became a trust root: messages=%#v state=%#v", installed, q.SkillCatalogState())
		}
	})

	t.Run("cleanup-retains-installed-proof", func(t *testing.T) {
		q := New(nil, registry.New(), Config{SessionID: "task22-cleanup-proof", SkillManager: manager})
		installed, err := q.installPostCompactVisibleHistory(
			[]types.Message{types.UserMessage(envelope)}, []types.Message{types.UserMessage("summary")},
		)
		if err != nil {
			t.Fatal(err)
		}
		cleaned, err := q.ensurePostCompactSkillState(installed)
		if err != nil {
			t.Fatal(err)
		}
		if task22EnvelopeOccurrenceCount(cleaned, envelope) != 1 || len(q.SkillCatalogState().LoadedDigests) != 1 {
			t.Fatalf("cleanup discarded previously sanitized installed proof: messages=%#v state=%#v", cleaned, q.SkillCatalogState())
		}
	})
}

func TestPostCompactSkillLatestEnvelopeWinsPerStableID(t *testing.T) {
	manager, rows := task22SkillManager(t, 1)
	row := rows[0]
	older := task22FullEnvelope(t, row, "older rendered body")
	latest := task22FullEnvelope(t, row, "latest rendered body")
	original := append(task22SkillExchange(row, "older_use", older), task22SkillExchange(row, "latest_use", latest)...)
	q := New(nil, registry.New(), Config{SessionID: "task22-latest", SkillManager: manager})

	installed, err := q.installPostCompactVisibleHistory(original, []types.Message{types.UserMessage("summary")})
	if err != nil {
		t.Fatal(err)
	}
	if task22EnvelopeOccurrenceCount(installed, older) != 0 || task22EnvelopeOccurrenceCount(installed, latest) != 1 {
		t.Fatalf("latest-per-ID restoration = %#v", installed)
	}
	loaded := q.SkillLoadedLedgerState(row.ID)
	if loaded.PayloadDigest != skills.DigestInvocationPayload("latest rendered body") {
		t.Fatalf("latest-per-ID loaded ledger = %#v", loaded)
	}

	// A later strict envelope with a stale content digest blocks fallback to an
	// older valid body for the same stable ID.
	stale := row
	stale.Digest = skills.SkillDigest("sha256:" + strings.Repeat("c", 64))
	staleEnvelope := task22FullEnvelope(t, stale, "stale latest body")
	staleOriginal := append(task22SkillExchange(row, "valid_old_use", older), task22SkillExchange(stale, "stale_latest_use", staleEnvelope)...)
	q2 := New(nil, registry.New(), Config{SessionID: "task22-latest-stale", SkillManager: manager})
	installed, err = q2.installPostCompactVisibleHistory(staleOriginal, []types.Message{types.UserMessage("summary")})
	if err != nil {
		t.Fatal(err)
	}
	if task22EnvelopeOccurrenceCount(installed, older) != 0 || task22EnvelopeOccurrenceCount(installed, staleEnvelope) != 0 ||
		len(q2.SkillCatalogState().LoadedDigests) != 0 {
		t.Fatalf("stale latest envelope resurrected older body: messages=%#v state=%#v", installed, q2.SkillCatalogState())
	}
}

func TestSkillCatalogPostCompactBodyBudgetsSelectLatestDeterministically(t *testing.T) {
	manager, rows := task22SkillManager(t, compact.PostCompactMaxSkillBodies+2)
	q := New(nil, registry.New(), Config{SessionID: "task22-budget", SkillManager: manager})
	var original []types.Message
	envelopes := make([]string, len(rows))
	for index, row := range rows {
		envelopes[index] = task22FullEnvelope(t, row, fmt.Sprintf("body-%d", index))
		original = append(original, task22SkillExchange(row, fmt.Sprintf("budget_use_%d", index), envelopes[index])...)
	}

	first, err := q.installPostCompactVisibleHistory(original, []types.Message{types.UserMessage("budget summary")})
	if err != nil {
		t.Fatal(err)
	}
	q2 := New(nil, registry.New(), Config{SessionID: "task22-budget", SkillManager: manager})
	second, err := q2.installPostCompactVisibleHistory(original, []types.Message{types.UserMessage("budget summary")})
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, firstErr := json.Marshal(first)
	secondJSON, secondErr := json.Marshal(second)
	if firstErr != nil || secondErr != nil || !reflect.DeepEqual(firstJSON, secondJSON) {
		t.Fatalf("same history produced non-deterministic skill restoration\nfirst=%#v\nsecond=%#v", first, second)
	}
	for index, envelope := range envelopes {
		want := 0
		if index >= len(envelopes)-compact.PostCompactMaxSkillBodies {
			want = 1
		}
		if got := task22EnvelopeOccurrenceCount(first, envelope); got != want {
			t.Fatalf("envelope %d occurrences = %d, want %d", index, got, want)
		}
	}
	if got := len(q.SkillCatalogState().LoadedDigests); got != compact.PostCompactMaxSkillBodies {
		t.Fatalf("loaded ledger count = %d, want %d", got, compact.PostCompactMaxSkillBodies)
	}
}

func TestPostCompactSkillByteBudgetSkipsOversizedAndRetainsSmallerCandidate(t *testing.T) {
	manager, rows := task22SkillManager(t, 2)
	small := task22FullEnvelope(t, rows[0], "small body survives")
	oversized := task22FullEnvelope(t, rows[1], strings.Repeat("x", compact.PostCompactSkillBodyBudgetBytes+1))
	original := append(task22SkillExchange(rows[0], "small_use", small), task22SkillExchange(rows[1], "large_use", oversized)...)
	q := New(nil, registry.New(), Config{SessionID: "task22-byte-budget", SkillManager: manager})

	installed, err := q.installPostCompactVisibleHistory(original, []types.Message{types.UserMessage("summary")})
	if err != nil {
		t.Fatal(err)
	}
	if got := task22EnvelopeOccurrenceCount(installed, oversized); got != 0 {
		t.Fatalf("oversized envelope occurrences = %d, want 0", got)
	}
	if got := task22EnvelopeOccurrenceCount(installed, small); got != 1 {
		t.Fatalf("smaller envelope occurrences = %d, want 1", got)
	}
	state := q.SkillCatalogState()
	if len(state.LoadedDigests) != 1 {
		t.Fatalf("byte-budget loaded ledger = %#v, want only smaller body", state.LoadedDigests)
	}
	if _, ok := state.LoadedDigests[rows[0].ID]; !ok {
		t.Fatalf("smaller body missing from loaded ledger: %#v", state.LoadedDigests)
	}
}

type task22RegistryRaceCompactor struct {
	result *compact.CompactionResult
	mutate func()
}

func (compactor task22RegistryRaceCompactor) Compact(context.Context, []types.Message, int) (*compact.CompactionResult, error) {
	if compactor.mutate != nil {
		compactor.mutate()
	}
	return compactor.result, nil
}

func TestPostCompactSkillAttachmentSanitizerPreservesMessagesToKeepBody(t *testing.T) {
	manager, rows := task22SkillManager(t, 1)
	row := rows[0]
	envelope := task22FullEnvelope(t, row, "retained segment body")
	retained := task22SkillExchange(row, "retained_segment_use", envelope)
	binding, err := manager.SnapshotBinding("task22-retained-segment")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := binding.Snapshot
	providerSnapshot, err := postCompactCatalogSnapshotMessage(snapshot, 20_000)
	if err != nil {
		t.Fatal(err)
	}
	boundary := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{Trigger: "manual"})
	result := &compact.CompactionResult{
		BoundaryMarker:  &boundary,
		SummaryMessages: []types.Message{types.UserMessage("summary")},
		MessagesToKeep:  retained,
		Attachments:     []types.Message{providerSnapshot, types.UserMessage(envelope)},
	}
	q := New(nil, registry.New(), Config{
		SessionID: "task22-retained-segment", SkillManager: manager, MaxContextTokens: 20_000,
	})
	q.compactor = task22RegistryRaceCompactor{result: result}
	q.SetMessages(retained)

	if _, err := q.ForceCompact(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := task22EnvelopeOccurrenceCount(q.Messages(), envelope); got != 1 {
		t.Fatalf("attachment sanitization changed retained body count = %d, want 1: %#v", got, q.Messages())
	}
	loaded := q.SkillLoadedLedgerState(row.ID)
	if loaded.LoadedContextEpoch != loaded.ContextEpoch || loaded.PayloadDigest != skills.DigestInvocationPayload("retained segment body") {
		t.Fatalf("retained MessagesToKeep body did not rebuild ledger: %#v", loaded)
	}
}

func TestSkillCatalogAutomaticCompactRestoresBodyBeforeCurrentUser(t *testing.T) {
	manager, rows := task22SkillManager(t, 1)
	row := rows[0]
	envelope := task22FullEnvelope(t, row, "automatic compact body")
	messages := task22SkillExchange(row, "automatic_use", envelope)
	messages = append(messages, manyUserMessages(30)...)
	messages = append(messages, types.UserMessage("automatic current user"))
	q := New(nil, registry.New(), Config{
		SessionID: "task22-auto", SkillManager: manager, MaxContextTokens: 50_000,
	})
	q.ctxWindow.UsedInput = 40_000
	fake := &prepareCountingCompactor{summaryMessage: "automatic summary"}
	q.compactor = fake
	q.lastResponseID = "response-before-auto"
	before := q.SkillCatalogState().ContextEpoch
	state := newQueryState(messages)

	prepared, err := q.prepareMessagesForQuery(context.Background(), state, 1, 0, false, func(stream.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 || !state.AutoCompactTracking.Compacted {
		t.Fatalf("automatic compact calls=%d tracking=%#v", fake.calls, state.AutoCompactTracking)
	}
	if got := q.SkillCatalogState(); got.ContextEpoch != before+1 || got.Cursor.Empty() || len(got.LoadedDigests) != 1 {
		t.Fatalf("automatic post-compact state = %#v", got)
	}
	if q.lastResponseID != "" || q.config.SessionID != "task22-auto" {
		t.Fatalf("automatic compact chain/cache = response %q session %q", q.lastResponseID, q.config.SessionID)
	}
	userIndex := indexContaining(prepared.Messages, "automatic current user")
	if userIndex < 2 || prepared.Messages[userIndex-2].Role != types.RoleDeveloper || prepared.Messages[userIndex-1].GetText() != envelope {
		t.Fatalf("automatic snapshot/body/user order = %#v", prepared.Messages)
	}
}

func TestSkillCatalogSemanticAutoCompactReceivesFullHistoryAndReattachesBody(t *testing.T) {
	manager, rows := task22SkillManager(t, 1)
	row := rows[0]
	envelope := task22FullEnvelope(t, row, "history snip explicit body")
	messages := []types.Message{types.UserMessage("head one"), types.AssistantMessage("head two")}
	for index := 0; index < 10; index++ {
		messages = append(messages, types.UserMessage(fmt.Sprintf(
			"large middle %d %s", index, strings.Repeat("task22 middle token sequence 0123456789 ", 1_000),
		)))
	}
	messages = append(messages, types.UserMessage(envelope))
	messages = append(messages, manyUserMessages(20)...)

	q := New(nil, registry.New(), Config{
		SessionID: "task22-history-snip", SkillManager: manager, MaxContextTokens: 35_000,
	})
	fake := &prepareCountingCompactor{summaryMessage: "semantic summary"}
	q.compactor = fake
	q.lastResponseID = "response-before-snip"
	state := newQueryState(messages)
	beforeEpoch := q.SkillCatalogState().ContextEpoch
	prepared, err := q.prepareMessagesForQuery(context.Background(), state, 1, 0, false, func(stream.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 || !strings.Contains(fake.receivedTexts[0], "large middle 5") {
		t.Fatalf("semantic compactor did not receive full unsnipped history: calls=%d input=%q", fake.calls, fake.receivedTexts)
	}
	if q.SkillCatalogState().ContextEpoch != beforeEpoch+1 {
		t.Fatalf("semantic compact did not cross exactly one context epoch: before=%d after=%d", beforeEpoch, q.SkillCatalogState().ContextEpoch)
	}
	runtime := q.SkillCatalogState()
	if runtime.Cursor.Empty() || runtime.Cursor.AnnouncedSnapshot.Revision == 0 || q.lastResponseID != "" {
		t.Fatalf("semantic compact did not install a full cursor/chain fence: runtime=%#v response=%q", runtime, q.lastResponseID)
	}
	if task22EnvelopeOccurrenceCount(prepared.Messages, envelope) != 1 ||
		task22EnvelopeOccurrenceCount(state.Messages, envelope) != 1 || len(q.SkillCatalogState().LoadedDigests) != 1 {
		t.Fatalf("semantic compact failed to install/rebuild exact body: prepared=%#v state=%#v runtime=%#v",
			prepared.Messages, state.Messages, q.SkillCatalogState())
	}
	if strings.Contains(joinLoopMessages(prepared.Messages), "earlier messages were compressed") {
		t.Fatalf("implicit lossy fallback marker survived semantic compact: %#v", prepared.Messages)
	}
	stableEpoch := q.SkillCatalogState().ContextEpoch
	second, err := q.prepareMessagesForQuery(context.Background(), state, 2, 0, false, func(stream.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	if got := q.SkillCatalogState().ContextEpoch; got != stableEpoch {
		t.Fatalf("unchanged post-snip history advanced epoch again: got %d want %d messages=%#v", got, stableEpoch, second.Messages)
	}
}

func TestPrepareWithoutCompactorDoesNotSnipOrFenceResponseChain(t *testing.T) {
	messages := []types.Message{types.UserMessage("head"), types.AssistantMessage("head answer")}
	for index := 0; index < 10; index++ {
		messages = append(messages, types.UserMessage(fmt.Sprintf(
			"large middle %d %s", index, strings.Repeat("task22 no-manager middle token 0123456789 ", 1_000),
		)))
	}
	messages = append(messages, manyUserMessages(20)...)
	q := New(nil, registry.New(), Config{SessionID: "task22-no-manager-snip", MaxContextTokens: 35_000})
	q.compactor = nil
	q.lastResponseID = "response-before-no-manager-snip"
	before := q.SkillCatalogState().ContextEpoch
	state := newQueryState(messages)
	prepared, err := q.prepareMessagesForQuery(context.Background(), state, 1, 0, false, func(stream.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	if q.SkillCatalogState().ContextEpoch != before || q.lastResponseID != "response-before-no-manager-snip" {
		t.Fatalf("nil compactor changed epoch/chain state: before=%d after=%d response=%q",
			before, q.SkillCatalogState().ContextEpoch, q.lastResponseID)
	}
	joined := joinLoopMessages(prepared.Messages)
	if strings.Contains(joined, "earlier messages were compressed") || !strings.Contains(joined, "large middle 5") {
		t.Fatalf("nil compactor implicitly snipped history: %#v", prepared.Messages)
	}
}

func TestSkillCatalogProviderBookkeepingAndCacheEditsDoNotChurnEpoch(t *testing.T) {
	manager, _ := task22SkillManager(t, 1)

	t.Run("content-replacement-record", func(t *testing.T) {
		q := New(nil, registry.New(), Config{
			SessionID: "task22-content-record", SkillManager: manager, MaxContextTokens: 100_000,
		})
		message := types.UserMessage("ordinary current input")
		message.Content = append(message.Content, types.ContentReplacementBlock{
			Type: types.ContentTypeReplacement, Kind: "tool-result", ToolUseID: "old_tool", Replacement: "stored replacement",
		})
		state := newQueryState([]types.Message{message})
		before := q.SkillCatalogState().ContextEpoch
		for turn := 1; turn <= 2; turn++ {
			prepared, err := q.prepareMessagesForQuery(context.Background(), state, turn, 0, false, func(stream.Event) {})
			if err != nil {
				t.Fatal(err)
			}
			if got := q.SkillCatalogState().ContextEpoch; got != before {
				t.Fatalf("bookkeeping-only projection advanced epoch on turn %d: got %d want %d", turn, got, before)
			}
			for _, block := range prepared.Messages[0].Content {
				if _, leaked := block.(types.ContentReplacementBlock); leaked {
					t.Fatalf("content replacement record leaked to provider: %#v", prepared.Messages)
				}
			}
		}
	})

	t.Run("anthropic-cache-edits", func(t *testing.T) {
		provider := newParityFakeProvider(nil)
		provider.name = "anthropic"
		q := New(provider, registry.New(), Config{
			SessionID: "task22-cache-edits", SkillManager: manager, MaxContextTokens: 200_000,
		})
		q.microcompactCfg = compact.MicrocompactConfig{
			KeepRecent: 1, CachedEnabled: true, QuerySource: compact.MicrocompactSourceMain,
			CachedTriggerThreshold: 1, CachedKeepRecent: 1,
		}
		state := newQueryState(append(compactablePrepareMessages(3), types.UserMessage("cache current input")))
		before := q.SkillCatalogState().ContextEpoch
		for turn := 1; turn <= 2; turn++ {
			prepared, err := q.prepareMessagesForQuery(context.Background(), state, turn, 0, true, func(stream.Event) {})
			if err != nil {
				t.Fatal(err)
			}
			if got := q.SkillCatalogState().ContextEpoch; got != before {
				t.Fatalf("cache_edits projection advanced epoch on turn %d: got %d want %d", turn, got, before)
			}
			foundCacheEdits := false
			for _, message := range prepared.Messages {
				for _, block := range message.Content {
					if unknown, ok := block.(types.UnknownBlock); ok && unknown.Type == compact.ContentTypeCacheEdits {
						foundCacheEdits = true
					}
				}
			}
			if !foundCacheEdits {
				t.Fatalf("cache_edits projection missing on turn %d: %#v", turn, prepared.Messages)
			}
		}
	})
}

func TestRecoverySkillCatalogReactiveCompactRestoresBody(t *testing.T) {
	manager, rows := task22SkillManager(t, 1)
	row := rows[0]
	envelope := task22FullEnvelope(t, row, "reactive compact body")
	failed := append(task22SkillExchange(row, "reactive_use", envelope), types.UserMessage("reactive current user"))
	q := New(nil, registry.New(), Config{SessionID: "task22-reactive", SkillManager: manager})
	q.compactor = &prepareCountingCompactor{summaryMessage: "reactive summary"}
	q.lastResponseID = "response-before-reactive"
	before := q.SkillCatalogState().ContextEpoch
	state := newQueryState(failed)

	retry, err := q.recoverFromTerminalProviderFailure(context.Background(), state, failed,
		&types.APIError{Type: "prompt_too_long", Message: "prompt is too long"}, 1, func(stream.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	if !retry || state.Transition != QueryTransitionReactiveCompactRetry {
		t.Fatalf("reactive recovery = retry %t transition %s", retry, state.Transition)
	}
	if got := q.SkillCatalogState(); got.ContextEpoch != before+1 || got.Cursor.Empty() || len(got.LoadedDigests) != 1 {
		t.Fatalf("reactive post-compact state = %#v", got)
	}
	if q.lastResponseID != "" || q.config.SessionID != "task22-reactive" {
		t.Fatalf("reactive compact chain/cache = response %q session %q", q.lastResponseID, q.config.SessionID)
	}
	userIndex := indexContaining(state.Messages, "reactive current user")
	if userIndex < 2 || state.Messages[userIndex-2].Role != types.RoleDeveloper || state.Messages[userIndex-1].GetText() != envelope {
		t.Fatalf("reactive snapshot/body/user order = %#v", state.Messages)
	}
}

func TestPostCompactSkillBindingDoesNotAdoptUnboundNonSkillControl(t *testing.T) {
	q := New(nil, registry.New(), Config{
		SessionID: "task22-no-control-adoption", SkillManager: newLoopTestSkillManager(t),
	})
	unboundBoundary := compact.NewCompactBoundaryMessage(
		compact.CompactBoundaryMetadata{Trigger: "manual"}, messagecontrol.Runtime(),
	)
	prepared, err := q.preparePostCompactSkillHistory(
		[]types.Message{types.UserMessage("source")}, []types.Message{unboundBoundary, types.UserMessage("tail")},
	)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, message := range prepared {
		if message.InternalKind != types.InternalMessageKindCompactBoundary {
			continue
		}
		found = true
		if message.HasInternalControlProvenanceForScope(q.internalControlScope) {
			t.Fatal("skill binding adopted an unbound compact boundary")
		}
		if !message.HasInternalControlProvenance() {
			t.Fatal("test boundary unexpectedly lost its original unbound authenticator")
		}
		if _, bound := message.InternalControlProvenanceScope(); bound {
			t.Fatal("unbound non-skill control acquired a scope")
		}
	}
	if !found {
		t.Fatal("unbound boundary disappeared before the adoption assertion")
	}
}

func TestPostCompactSkillBindingDoesNotAdoptPreexistingUnboundSkillBody(t *testing.T) {
	manager, rows := task22SkillManager(t, 1)
	q := New(nil, registry.New(), Config{
		SessionID: "task22-no-skill-adoption", SkillManager: manager,
	})
	envelope := task22FullEnvelope(t, rows[0], "untrusted body")
	unbound, err := newPostCompactSkillBodyMessage(envelope, q.currentSkillCatalogEpoch())
	if err != nil {
		t.Fatal(err)
	}
	prepared, prepareErr := q.preparePostCompactSkillHistory(
		[]types.Message{unbound}, []types.Message{unbound, types.UserMessage("tail")},
	)
	if prepareErr != nil {
		t.Fatal(prepareErr)
	}
	for _, message := range prepared {
		if message.GetText() == envelope && message.IsTrustedSkillInvocationMessageForScope(q.internalControlScope) {
			t.Fatal("preexisting unbound skill body was adopted into the target scope")
		}
	}
}

func task22SkillManager(t *testing.T, count int) (*skills.Manager, []skills.EffectiveSkill) {
	t.Helper()
	root := t.TempDir()
	for index := 0; index < count; index++ {
		writeLoopSkill(t, root, fmt.Sprintf("task22-%02d", index), fmt.Sprintf("Task 22 skill %d", index))
	}
	manager := newLoopTestSkillManager(t, skills.DirSource{Dir: root, Source: skills.SourceProject})
	binding, err := manager.SnapshotBinding("task22-session")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := binding.Snapshot
	if len(snapshot.Skills) != count {
		t.Fatalf("manager skills = %d, want %d", len(snapshot.Skills), count)
	}
	return manager, snapshot.Skills
}

func task22FullEnvelope(t *testing.T, row skills.EffectiveSkill, body string) string {
	t.Helper()
	envelope, err := skills.RenderFullInvocationEnvelope(row, body, skills.InvocationArguments{})
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}

func task22SkillExchange(row skills.EffectiveSkill, toolUseID, envelope string) []types.Message {
	assistant := types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{types.ToolUseBlock{
		Type: types.ContentTypeToolUse, ID: toolUseID, Name: "Skill", Input: map[string]any{"skill": string(row.ID)},
	}}}
	result := types.ToolResultMessage(types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: envelope, Outcome: types.ToolOutcomeSucceeded,
	})
	return []types.Message{assistant, result}
}

func task22EnvelopeOccurrenceCount(messages []types.Message, envelope string) int {
	count := 0
	for _, message := range messages {
		for _, block := range message.Content {
			switch typed := block.(type) {
			case types.TextBlock:
				if typed.Text == envelope {
					count++
				}
			case types.ToolResultBlock:
				if typed.Content == envelope {
					count++
				}
			}
		}
	}
	return count
}

func writeLoopSkill(t *testing.T, root, name, description string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := "---\n" +
		"description: \"" + description + "\"\n" +
		"---\n\n" +
		"# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func joinLoopMessages(messages []types.Message) string {
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		parts = append(parts, msg.GetText())
	}
	return strings.Join(parts, "\n")
}

func indexContaining(messages []types.Message, needle string) int {
	return indexContainingFrom(messages, needle, 0)
}

func indexContainingFrom(messages []types.Message, needle string, start int) int {
	if start < 0 {
		start = 0
	}
	for i, msg := range messages {
		if i < start {
			continue
		}
		if strings.Contains(msg.GetText(), needle) {
			return i
		}
	}
	return -1
}
