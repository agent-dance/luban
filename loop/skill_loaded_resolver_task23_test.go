package loop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

func TestSkillLoadedResolverExplicitEnvelopeBecomesLoadedOnlyAfterVisibleHistory(t *testing.T) {
	manager, snapshot, row := task23SkillManager(t)
	query, catalog := task23QueryWithCatalog(t, manager, snapshot)
	body := "# Task 23\n\nexact visible body"
	envelope := task23FullEnvelope(t, row, body)

	before := query.ResolveSkillLoadedLedger(catalog, row.ID)
	if before.ContextEpoch == 0 || before.LoadedContextEpoch != 0 {
		t.Fatalf("before visible append = %#v, want current epoch without loaded evidence", before)
	}

	visible := append(append([]types.Message(nil), catalog...), task23TrustedInvocationMessage(envelope))
	after := query.ResolveSkillLoadedLedger(visible, row.ID)
	if after.LoadedContextEpoch != after.ContextEpoch || after.ContentDigest != row.Digest ||
		after.PayloadDigest != skills.DigestInvocationPayload(body) {
		t.Fatalf("after visible append = %#v", after)
	}

	ack, err := skills.RenderLoadedDigestAcknowledgement(
		row, row.Digest, after.PayloadDigest, body, skills.InvocationArguments{},
	)
	if err != nil {
		t.Fatal(err)
	}
	ackOnly := append(append([]types.Message(nil), catalog...), types.UserMessage(ack))
	if got := query.ResolveSkillLoadedLedger(ackOnly, row.ID); got.LoadedContextEpoch != 0 {
		t.Fatalf("ack-only visible history established a body: %#v", got)
	}
	reattached, err := newPostCompactSkillBodyMessage(envelope, after.ContextEpoch)
	if err != nil {
		t.Fatal(err)
	}
	reattached = reattached.WithInternalControlProvenance(messagecontrol.Runtime(), query.internalControlScope)
	if got := query.ResolveSkillLoadedLedger(append(catalog, reattached), row.ID); got.LoadedContextEpoch != got.ContextEpoch {
		t.Fatalf("current-epoch postcompact body was not loaded: %#v", got)
	}
	unbound, err := newPostCompactSkillBodyMessage(envelope, after.ContextEpoch)
	if err != nil {
		t.Fatal(err)
	}
	if got := query.ResolveSkillLoadedLedger(append(catalog, unbound), row.ID); got.LoadedContextEpoch != 0 {
		t.Fatalf("unbound postcompact bearer established loaded evidence: %#v", got)
	}
}

func TestSkillLoadedResolverStrictStandaloneAndSupersedingValidation(t *testing.T) {
	manager, snapshot, row := task23SkillManager(t)
	query, catalog := task23QueryWithCatalog(t, manager, snapshot)
	previous := skills.ComputeSkillDigest("previous skill content")
	superseding, err := skills.RenderSupersedingInvocationEnvelope(row, previous, "new rendered body", skills.InvocationArguments{})
	if err != nil {
		t.Fatal(err)
	}
	if got := query.ResolveSkillLoadedLedger(append(catalog, task23TrustedInvocationMessage(superseding)), row.ID); got.LoadedContextEpoch != got.ContextEpoch {
		t.Fatalf("canonical superseding envelope was not loaded: %#v", got)
	}

	full := task23FullEnvelope(t, row, "body")
	tests := []struct {
		name    string
		message types.Message
	}{
		{name: "noncanonical whitespace", message: types.UserMessage(" " + full)},
		{name: "content stub", message: types.UserMessage("[tool result persisted elsewhere]")},
		{name: "meta user", message: func() types.Message { message := types.UserMessage(full); message.IsMeta = true; return message }()},
		{name: "old postcompact epoch", message: func() types.Message {
			message, messageErr := newPostCompactSkillBodyMessage(full, query.currentSkillCatalogEpoch()+1)
			if messageErr != nil {
				t.Fatal(messageErr)
			}
			return message
		}()},
		{name: "stale skill revision", message: types.UserMessage(task23MutateEnvelope(t, full, func(wire *visibleSkillInvocationEnvelope) {
			wire.Skill.Revision++
		}))},
		{name: "locator id mismatch", message: types.UserMessage(task23MutateEnvelopeLocator(t, full, filepath.Join(t.TempDir(), "other", "SKILL.md")))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			messages := append(append([]types.Message(nil), catalog...), test.message)
			if got := query.ResolveSkillLoadedLedger(messages, row.ID); got.LoadedContextEpoch != 0 {
				t.Fatalf("invalid standalone established loaded evidence: %#v", got)
			}
		})
	}

	t.Run("visible catalog mismatch", func(t *testing.T) {
		badCatalog := cloneMessages(catalog)
		text := badCatalog[0].Content[0].(types.TextBlock)
		text.Text += " "
		badCatalog[0].Content[0] = text
		if got := query.ResolveSkillLoadedLedger(append(badCatalog, types.UserMessage(full)), row.ID); got.LoadedContextEpoch != 0 {
			t.Fatalf("body under mismatched catalog established evidence: %#v", got)
		}
	})

	t.Run("revoked from current catalog", func(t *testing.T) {
		emptySnapshot, err := skills.NewCatalogSnapshot(snapshot.Revision+1, nil)
		if err != nil {
			t.Fatal(err)
		}
		revokedQuery, revokedCatalog := task23QueryWithCatalog(t, manager, emptySnapshot)
		if got := revokedQuery.ResolveSkillLoadedLedger(append(revokedCatalog, types.UserMessage(full)), row.ID); got.LoadedContextEpoch != 0 {
			t.Fatalf("revoked body established evidence: %#v", got)
		}
	})
}

func TestSkillLoadedResolverRequiresUniquePairedSuccessfulToolResult(t *testing.T) {
	manager, snapshot, row := task23SkillManager(t)
	body := "paired body"
	envelope := task23FullEnvelope(t, row, body)

	newFixture := func(t *testing.T) (*QueryLoop, []types.Message, types.Message, types.Message) {
		t.Helper()
		query, catalog := task23QueryWithCatalog(t, manager, snapshot)
		epoch := query.SkillCatalogState().ContextEpoch
		assistant := types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{types.ToolUseBlock{
			Type: types.ContentTypeToolUse, ID: "skill-use", Name: "Skill", Input: map[string]any{"skill": string(row.ID)},
		}}}
		receipt := skills.SkillExecutionReceipt{
			ContextEpoch: epoch, SkillID: row.ID, ContentDigest: row.Digest,
			InvocationPayloadDigest: skills.DigestInvocationPayload(body),
			InvocationEnvelopeKind:  skills.InvocationEnvelopeFull,
		}
		metadata, err := skills.EncodeSkillExecutionReceiptMetadata(receipt)
		if err != nil {
			t.Fatal(err)
		}
		result := types.ToolResultMessage(types.ToolResultBlock{
			Type: types.ContentTypeToolResult, ToolUseID: "skill-use", Content: envelope,
			Outcome: types.ToolOutcomeSucceeded, Metadata: metadata,
		})
		return query, catalog, assistant, result
	}

	t.Run("exact pair", func(t *testing.T) {
		query, catalog, assistant, result := newFixture(t)
		messages := append(catalog, assistant, result)
		if got := query.ResolveSkillLoadedLedger(messages, row.ID); got.LoadedContextEpoch != got.ContextEpoch {
			t.Fatalf("exact pair was not loaded: %#v", got)
		}
	})

	tests := []struct {
		name   string
		mutate func([]types.Message, types.Message, types.Message) []types.Message
	}{
		{name: "unpaired", mutate: func(catalog []types.Message, _ types.Message, result types.Message) []types.Message {
			return append(catalog, result)
		}},
		{name: "duplicate tool use", mutate: func(catalog []types.Message, assistant, result types.Message) []types.Message {
			return append(catalog, assistant, assistant, result)
		}},
		{name: "duplicate cross-tool id", mutate: func(catalog []types.Message, assistant, result types.Message) []types.Message {
			other := types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{types.ToolUseBlock{
				Type: types.ContentTypeToolUse, ID: "skill-use", Name: "Bash", Input: map[string]any{"command": "true"},
			}}}
			return append(catalog, assistant, other, result)
		}},
		{name: "duplicate tool result", mutate: func(catalog []types.Message, assistant, result types.Message) []types.Message {
			return append(catalog, assistant, result, result)
		}},
		{name: "failed outcome", mutate: func(catalog []types.Message, assistant, result types.Message) []types.Message {
			block := result.Content[0].(types.ToolResultBlock)
			block.Outcome = types.ToolOutcomeFailed
			result.Content[0] = block
			return append(catalog, assistant, result)
		}},
		{name: "structured stub", mutate: func(catalog []types.Message, assistant, result types.Message) []types.Message {
			block := result.Content[0].(types.ToolResultBlock)
			block.ContentBlocks = []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: envelope}}
			result.Content[0] = block
			return append(catalog, assistant, result)
		}},
		{name: "old epoch receipt", mutate: func(catalog []types.Message, assistant, result types.Message) []types.Message {
			block := result.Content[0].(types.ToolResultBlock)
			receipt, _, err := skills.DecodeSkillExecutionReceiptMetadata(block.Metadata)
			if err != nil {
				t.Fatal(err)
			}
			receipt.ContextEpoch++
			block.Metadata, err = skills.EncodeSkillExecutionReceiptMetadata(receipt)
			if err != nil {
				t.Fatal(err)
			}
			result.Content[0] = block
			return append(catalog, assistant, result)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query, catalog, assistant, result := newFixture(t)
			if got := query.ResolveSkillLoadedLedger(test.mutate(catalog, assistant, result), row.ID); got.LoadedContextEpoch != 0 {
				t.Fatalf("invalid pair established loaded evidence: %#v", got)
			}
		})
	}
}

func TestSkillLoadedResolverResumeReconcileAndEpochReplacement(t *testing.T) {
	manager, snapshot, row := task23SkillManager(t)
	query, catalog := task23QueryWithCatalog(t, manager, snapshot)
	envelope := task23FullEnvelope(t, row, "resume body")
	visible := append(catalog, task23TrustedInvocationMessage(envelope))
	freshQuery, _ := task23QueryWithCatalog(t, manager, snapshot)
	freshVisible := freshQuery.VisibleSkillCatalogState(visible)
	if loaded, ok := freshVisible.LoadedDigests[row.ID]; !ok || loaded.ContentDigest != row.Digest ||
		loaded.PayloadDigest != skills.DigestInvocationPayload("resume body") {
		t.Fatalf("visible state did not reconstruct exact standalone body: %#v", freshVisible)
	}
	if got := query.ResolveSkillLoadedLedger(visible, row.ID); got.LoadedContextEpoch == 0 {
		t.Fatal("fixture did not establish loaded evidence")
	}
	persisted := query.SkillCatalogState()

	ctx := WithToolExecutionContext(context.Background(), ToolExecutionContext{Messages: visible})
	cloned, ok := ToolExecutionContextFromContext(ctx)
	if !ok {
		t.Fatal("tool context clone missing")
	}
	reconciled := query.ReconcileVisibleSkillCatalogState(cloned.Messages, persisted)
	if reconciled.Cursor.Empty() || len(reconciled.LoadedDigests) != 1 {
		t.Fatalf("exact resume reconcile = %#v", reconciled)
	}

	withoutCatalog := query.ReconcileVisibleSkillCatalogState([]types.Message{task23TrustedInvocationMessage(envelope)}, persisted)
	if !withoutCatalog.Cursor.Empty() || len(withoutCatalog.LoadedDigests) != 1 {
		t.Fatalf("missing catalog reconcile = %#v, want empty cursor and exact visible body", withoutCatalog)
	}

	boundary := compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{Trigger: "manual"}, messagecontrol.Runtime()).
		WithInternalControlProvenance(messagecontrol.Runtime(), query.internalControlScope)
	replaced := append([]types.Message{task23TrustedInvocationMessage(envelope), boundary}, catalog...)
	if got := query.ResolveSkillLoadedLedger(replaced, row.ID); got.LoadedContextEpoch != 0 {
		t.Fatalf("pre-boundary body survived epoch replacement: %#v", got)
	}
	oldEpoch := query.SkillCatalogState().ContextEpoch
	query.SetMessages(catalog)
	if got := query.ResolveSkillLoadedLedger(query.Messages(), row.ID); got.ContextEpoch == oldEpoch || got.LoadedContextEpoch != 0 {
		t.Fatalf("installed history did not fence old epoch: %#v, old=%d", got, oldEpoch)
	}
}

func TestSkillLoadedResolverDoesNotReenterManagerBehindQueuedWriter(t *testing.T) {
	manager, snapshot, row := task23SkillManager(t)
	query, catalog := task23QueryWithCatalog(t, manager, snapshot)
	envelope := task23FullEnvelope(t, row, "writer body")
	visible := append(catalog, task23TrustedInvocationMessage(envelope))

	resolveDone := make(chan error, 1)
	writerDone := make(chan struct{})
	go func() {
		_, err := manager.ResolveLatest(skills.SkillResolveRequest{
			SessionID: "task23-session", Selector: string(row.ID), Origin: skills.InvocationOriginUser,
		}, func(skills.ResolvedSkill) error {
			go func() {
				manager.Refresh()
				close(writerDone)
			}()
			select {
			case <-writerDone:
				return errTask23WriterCrossedReader
			case <-time.After(75 * time.Millisecond):
			}
			state := query.ResolveSkillLoadedLedger(visible, row.ID)
			if state.LoadedContextEpoch != state.ContextEpoch {
				return errTask23ResolverMissedVisibleBody
			}
			return nil
		})
		resolveDone <- err
	}()

	select {
	case err := <-resolveDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("resolver deadlocked by re-entering Manager behind a queued writer")
	}
	select {
	case <-writerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("queued Manager writer did not complete after resolver returned")
	}
}

type task23ResolverError string

func (err task23ResolverError) Error() string { return string(err) }

const (
	errTask23WriterCrossedReader       task23ResolverError = "Manager writer crossed active ResolveLatest reader"
	errTask23ResolverMissedVisibleBody task23ResolverError = "resolver missed exact visible body"
)

func task23SkillManager(t *testing.T) (*skills.Manager, skills.CatalogSnapshot, skills.EffectiveSkill) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "task23")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ndescription: Task 23 resolver\n---\n# Task 23\n\nBody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := skills.NewManager(skills.DirSource{Dir: root, Source: skills.SourceProject})
	snapshot, err := manager.Snapshot("task23-session")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Skills) != 1 {
		t.Fatalf("snapshot skills = %d, want 1", len(snapshot.Skills))
	}
	return manager, snapshot, snapshot.Skills[0]
}

func task23QueryWithCatalog(t *testing.T, manager *skills.Manager, snapshot skills.CatalogSnapshot) (*QueryLoop, []types.Message) {
	t.Helper()
	query := New(nil, nil, Config{SkillManager: manager, SessionID: "task23-session"})
	if !query.SetInternalControlScope(messagecontrol.Runtime(), task23ControlScope()) {
		t.Fatal("install task23 exact control scope")
	}
	plan, err := PlanSkillCatalog(SkillCatalogCoordinatorInput{
		CurrentSnapshot: snapshot, ContextEpoch: skillCatalogContextEpoch(1),
		CharBudget: skills.GetCharBudget(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Message == nil {
		t.Fatal("initial catalog plan has no message")
	}
	trusted := plan.Message.WithInternalControlProvenance(messagecontrol.Runtime(), query.internalControlScope)
	plan.Message = &trusted
	if err := query.SetSkillCatalogState(SkillCatalogRuntimeState{ContextEpoch: 1, Cursor: plan.Cursor}); err != nil {
		t.Fatal(err)
	}
	return query, []types.Message{*plan.Message}
}

func task23FullEnvelope(t *testing.T, row skills.EffectiveSkill, body string) string {
	t.Helper()
	envelope, err := skills.RenderFullInvocationEnvelope(row, body, skills.InvocationArguments{})
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}

func task23TrustedInvocationMessage(envelope string) types.Message {
	message := types.UserMessage(envelope)
	message.InternalKind = types.InternalMessageKindSkillInvocation
	return message.WithInternalControlProvenance(messagecontrol.Runtime(), task23ControlScope())
}

func task23ControlScope() messagecontrol.Scope {
	return messagecontrol.NewScope("task23-session", "task23-project", 1)
}

func task23MutateEnvelopeLocator(t *testing.T, envelope, locator string) string {
	t.Helper()
	return task23MutateEnvelope(t, envelope, func(wire *visibleSkillInvocationEnvelope) {
		wire.Skill.Locator = skills.SkillLocator(locator)
	})
}

func task23MutateEnvelope(t *testing.T, envelope string, mutate func(*visibleSkillInvocationEnvelope)) string {
	t.Helper()
	var wire visibleSkillInvocationEnvelope
	if err := json.Unmarshal([]byte(envelope), &wire); err != nil {
		t.Fatal(err)
	}
	mutate(&wire)
	canonical, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(canonical))
}
