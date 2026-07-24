package loop_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	loopapi "github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/tools"
	"github.com/agent-dance/luban/types"
)

type task26LoopProvider struct {
	mu    sync.Mutex
	turns [][]types.StreamEvent
	calls []provider.Params
}

func (p *task26LoopProvider) Name() string    { return "task26-loop" }
func (p *task26LoopProvider) ModelID() string { return "task26-model" }

func (p *task26LoopProvider) CreateStream(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	index := len(p.calls)
	params.Messages = append([]types.Message(nil), params.Messages...)
	p.calls = append(p.calls, params)
	events := task26LoopTextEvents(fmt.Sprintf("answer-%d", index))
	if index < len(p.turns) {
		events = append([]types.StreamEvent(nil), p.turns[index]...)
	}
	p.mu.Unlock()

	stream := make(chan types.StreamEvent, len(events))
	go func() {
		defer close(stream)
		for _, event := range events {
			select {
			case <-ctx.Done():
				return
			case stream <- event:
			}
		}
	}()
	return stream, nil
}

func (p *task26LoopProvider) Calls() []provider.Params {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.Params(nil), p.calls...)
}

type task26CatalogDeltaWire struct {
	Type         string                 `json:"type"`
	FromRevision skills.CatalogRevision `json:"from_revision"`
	ToRevision   skills.CatalogRevision `json:"to_revision"`
	Upserts      []struct {
		Reason skills.CatalogUpsertReason `json:"reason"`
		Skill  struct {
			ID         skills.SkillID       `json:"id"`
			Revision   skills.SkillRevision `json:"revision"`
			Digest     skills.SkillDigest   `json:"digest"`
			Visibility skills.Visibility    `json:"visibility"`
		} `json:"skill"`
	} `json:"upserts"`
	Revokes []struct {
		ID     skills.SkillID             `json:"id"`
		Reason skills.CatalogRevokeReason `json:"reason"`
	} `json:"revokes"`
}

func TestSkillCatalogIntegrationLifecycleThroughQueryLoop(t *testing.T) {
	projectRoot := t.TempDir()
	alphaDir, alphaFile := task26WriteLoopSkill(t, projectRoot, "alpha-dir", "alpha", "alpha summary v1", "alpha body v1")
	layer := skills.NewMemorySessionOverrideLayer()
	manager := task26LoopManager(t, layer, skills.DirSource{Dir: projectRoot, Source: skills.SourceProject})

	const sessionID = "task26-lifecycle"
	initial := task26LoopSnapshot(t, manager, sessionID)
	alpha := task26FindLoopSkill(t, initial, "alpha", skills.SourceProject)
	recording := &task26LoopProvider{}
	query := loopapi.New(recording, registry.New(), loopapi.Config{
		MaxTurns: 2, MaxTokens: 1024, SessionID: sessionID, SkillManager: manager,
	})

	first := task26RunLoopTurn(t, query, recording, "user-initial")
	if len(first) != 2 {
		t.Fatalf("initial request messages = %d, want catalog plus user: %#v", len(first), first)
	}
	task26AssertLoopCatalog(t, first[0], types.DeveloperMessageKindSkillCatalogSnapshot)
	if strings.Contains(first[0].GetText(), "alpha body v1") {
		t.Fatalf("initial catalog eagerly exposed SKILL.md body: %s", first[0].GetText())
	}
	if first[1].Role != types.RoleUser || first[1].GetText() != "user-initial" {
		t.Fatalf("initial catalog was not immediately before user: %#v", first)
	}

	beforeNoChange := task26LoopMessagesJSON(t, query.Messages())
	noChange := task26RunLoopTurn(t, query, recording, "user-no-change")
	if len(noChange) != len(query.Messages())-1 {
		// query.Messages contains the provider response appended after this
		// request, so the request is exactly one message shorter.
		t.Fatalf("no-change request length = %d, durable history = %d", len(noChange), len(query.Messages()))
	}
	if got := task26LoopMessagesJSON(t, noChange[:len(noChange)-1]); got != beforeNoChange {
		t.Fatalf("no-change turn rewrote the visible prefix\n got: %s\nwant: %s", got, beforeNoChange)
	}
	if noChange[len(noChange)-1].GetText() != "user-no-change" || task26CatalogCount(noChange) != 1 {
		t.Fatalf("no-change turn appended a catalog item: %#v", noChange)
	}

	_, betaFile := task26WriteLoopSkill(t, projectRoot, "beta-dir", "beta", "beta summary v1", "beta body v1")
	addedSnapshot, err := manager.RefreshSnapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	beta := task26FindLoopSkill(t, addedSnapshot, "beta", skills.SourceProject)
	// Advance the same newly discovered ID through content and visibility
	// changes before the next sampling boundary. The model must see one latest
	// upsert, not a replay of intermediate registry events.
	task26RewriteLoopSkill(t, betaFile, "beta", "beta summary v2", "beta body v2 with changed content")
	updatedBetaSnapshot, err := manager.RefreshSnapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	updatedBeta, found := updatedBetaSnapshot.Find(beta.ID)
	if !found || updatedBeta.Digest == beta.Digest {
		t.Fatalf("beta intermediate update = %#v", updatedBeta)
	}
	latestBetaSnapshot, err := manager.SetVisibility(sessionID, skills.VisibilityOverride{
		SkillID: beta.ID, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityNameOnly,
	})
	if err != nil {
		t.Fatal(err)
	}
	latestBeta, found := latestBetaSnapshot.Find(beta.ID)
	if !found || latestBeta.Visibility != skills.VisibilityNameOnly {
		t.Fatalf("beta latest coalesced state = %#v", latestBeta)
	}
	added := task26LoopTailDelta(t, task26RunLoopTurn(t, query, recording, "user-add"), "user-add")
	task26AssertOnlyLoopUpsert(t, added, beta.ID, skills.CatalogUpsertAdded)
	if added.Upserts[0].Skill.Digest != latestBeta.Digest || added.Upserts[0].Skill.Revision != latestBeta.Revision || added.Upserts[0].Skill.Visibility != skills.VisibilityNameOnly {
		t.Fatalf("coalesced upsert exposed an intermediate state: got=%#v want=%#v", added.Upserts[0].Skill, latestBeta)
	}

	task26RewriteLoopSkill(t, alphaFile, "alpha", "alpha summary v2 is deliberately longer", "alpha body v2 is deliberately longer than the first body")
	updatedSnapshot, err := manager.RefreshSnapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	updatedAlpha, found := updatedSnapshot.Find(alpha.ID)
	if !found || updatedAlpha.Revision <= alpha.Revision || updatedAlpha.Digest == alpha.Digest {
		t.Fatalf("alpha update did not advance exact content state: before=%#v after=%#v", alpha, updatedAlpha)
	}
	updated := task26LoopTailDelta(t, task26RunLoopTurn(t, query, recording, "user-update"), "user-update")
	task26AssertOnlyLoopUpsert(t, updated, alpha.ID, skills.CatalogUpsertUpdated)

	nameOnlySnapshot, err := manager.SetVisibility(sessionID, skills.VisibilityOverride{
		SkillID: alpha.ID, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityNameOnly,
	})
	if err != nil {
		t.Fatal(err)
	}
	nameOnly, found := nameOnlySnapshot.Find(alpha.ID)
	if !found || nameOnly.Visibility != skills.VisibilityNameOnly {
		t.Fatalf("name-only state = %#v", nameOnly)
	}
	visibility := task26LoopTailDelta(t, task26RunLoopTurn(t, query, recording, "user-visibility"), "user-visibility")
	task26AssertOnlyLoopUpsert(t, visibility, alpha.ID, skills.CatalogUpsertUpdated)
	if got := visibility.Upserts[0].Skill.Visibility; got != skills.VisibilityNameOnly {
		t.Fatalf("visibility upsert = %q, want name-only", got)
	}

	disabledSnapshot, err := manager.SetVisibility(sessionID, skills.VisibilityOverride{
		SkillID: alpha.ID, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityOff,
	})
	if err != nil {
		t.Fatal(err)
	}
	disabledAlpha, found := disabledSnapshot.Find(alpha.ID)
	if !found || disabledAlpha.Visibility != skills.VisibilityOff {
		t.Fatalf("disabled alpha = %#v", disabledAlpha)
	}
	disabled := task26LoopTailDelta(t, task26RunLoopTurn(t, query, recording, "user-disable"), "user-disable")
	task26AssertOnlyLoopRevoke(t, disabled, alpha.ID, skills.CatalogRevokeDisabled)

	otherSession := task26LoopSnapshot(t, manager, "task26-other-session")
	otherAlpha, found := otherSession.Find(alpha.ID)
	if !found || otherAlpha.Visibility == skills.VisibilityOff || otherAlpha.VisibilitySource == skills.SkillScopeSession {
		t.Fatalf("session override leaked to another session: %#v", otherAlpha)
	}

	reenabledSnapshot, err := manager.ResetVisibility(sessionID, skills.SkillScopeSession, alpha.ID)
	if err != nil {
		t.Fatal(err)
	}
	reenabledAlpha, found := reenabledSnapshot.Find(alpha.ID)
	if !found || reenabledAlpha.Visibility != skills.VisibilityAuto {
		t.Fatalf("re-enabled alpha = %#v", reenabledAlpha)
	}
	reenabled := task26LoopTailDelta(t, task26RunLoopTurn(t, query, recording, "user-re-enable"), "user-re-enable")
	task26AssertOnlyLoopUpsert(t, reenabled, alpha.ID, skills.CatalogUpsertReenabled)

	if err := os.RemoveAll(alphaDir); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RefreshSnapshot(sessionID); err != nil {
		t.Fatal(err)
	}
	deleted := task26LoopTailDelta(t, task26RunLoopTurn(t, query, recording, "user-delete"), "user-delete")
	task26AssertOnlyLoopRevoke(t, deleted, alpha.ID, skills.CatalogRevokeDeleted)
}

func TestSkillCatalogIntegrationTwoRealQueryLoopsIsolateCatalogAndLoadedBodies(t *testing.T) {
	projectRoot := t.TempDir()
	_, _ = task26WriteLoopSkill(t, projectRoot, "isolated-dir", "isolated", "isolated summary", "isolated body")
	layer := skills.NewMemorySessionOverrideLayer()
	manager := task26LoopManager(t, layer, skills.DirSource{Dir: projectRoot, Source: skills.SourceProject})
	row := task26FindLoopSkill(t, task26LoopSnapshot(t, manager, "task26-real-a"), "isolated", skills.SourceProject)

	tool := &tools.SkillTool{Manager: manager}
	tool.SessionIDResolver = func(ctx context.Context) string {
		if execution, ok := loopapi.ToolExecutionContextFromContext(ctx); ok {
			return execution.SessionID
		}
		return ""
	}
	tool.LoadedLedgerResolver = func(ctx context.Context, _ string, id skills.SkillID) tools.SkillLoadedLedgerState {
		execution, ok := loopapi.ToolExecutionContextFromContext(ctx)
		if !ok {
			return tools.SkillLoadedLedgerState{}
		}
		state, resolved := execution.ResolveSkillLoadedLedger(id)
		if !resolved {
			return tools.SkillLoadedLedgerState{}
		}
		return task26ToolsLedger(state)
	}
	reg := registry.New()
	reg.Register(tool)

	providerA := &task26LoopProvider{turns: [][]types.StreamEvent{
		task26LoopToolEvents("task26-a-first", "Skill", map[string]any{
			"skill": string(row.ID), "revision": uint64(row.Revision),
		}),
		task26LoopToolEvents("task26-a-second", "Skill", map[string]any{
			"skill": string(row.ID), "revision": uint64(row.Revision),
		}),
		task26LoopTextEvents("session A done"),
	}}
	providerB := &task26LoopProvider{turns: [][]types.StreamEvent{
		task26LoopToolEvents("task26-b-first", "Skill", map[string]any{
			"skill": string(row.ID), "revision": uint64(row.Revision),
		}),
		task26LoopTextEvents("session B done"),
	}}
	queryA := loopapi.New(providerA, reg, loopapi.Config{
		MaxTurns: 4, MaxTokens: 1024, SessionID: "task26-real-a", SkillManager: manager,
	})
	queryB := loopapi.New(providerB, reg, loopapi.Config{
		MaxTurns: 3, MaxTokens: 1024, SessionID: "task26-real-b", SkillManager: manager,
	})

	if err := queryA.Run(context.Background(), "load twice in session A", func(loopapi.Event) {}); err != nil {
		t.Fatal(err)
	}
	aFirst := task26DecodeLoopEnvelope(t, task26FindLoopToolResult(t, queryA.Messages(), "task26-a-first"))
	aSecond := task26DecodeLoopEnvelope(t, task26FindLoopToolResult(t, queryA.Messages(), "task26-a-second"))
	if aFirst.Kind != skills.InvocationEnvelopeFull || aFirst.Body == nil || !strings.Contains(*aFirst.Body, "isolated body") {
		t.Fatalf("session A first invocation = %#v, want full body", aFirst)
	}
	if aSecond.Kind != skills.InvocationEnvelopeAlreadyLoaded || aSecond.Body != nil {
		t.Fatalf("session A second invocation = %#v, want already-loaded without body", aSecond)
	}
	aAfterA := queryA.SkillCatalogState()
	if aAfterA.Cursor.Empty() || len(aAfterA.LoadedDigests) != 1 {
		t.Fatalf("session A runtime state = %#v", aAfterA)
	}
	aLoaded := queryA.SkillLoadedLedgerState(row.ID)
	if aLoaded.LoadedContextEpoch != aLoaded.ContextEpoch || aLoaded.ContentDigest != row.Digest {
		t.Fatalf("session A loaded state = %#v", aLoaded)
	}

	if err := queryB.Run(context.Background(), "first load in session B", func(loopapi.Event) {}); err != nil {
		t.Fatal(err)
	}
	bFirst := task26DecodeLoopEnvelope(t, task26FindLoopToolResult(t, queryB.Messages(), "task26-b-first"))
	if bFirst.Kind != skills.InvocationEnvelopeFull || bFirst.Body == nil || !strings.Contains(*bFirst.Body, "isolated body") {
		t.Fatalf("session B reused session A body ledger: %#v", bFirst)
	}
	bAfterB := queryB.SkillCatalogState()
	if bAfterB.Cursor.Empty() || len(bAfterB.LoadedDigests) != 1 {
		t.Fatalf("session B runtime state = %#v", bAfterB)
	}
	bLoaded := queryB.SkillLoadedLedgerState(row.ID)
	if bLoaded.LoadedContextEpoch != bLoaded.ContextEpoch || bLoaded.ContentDigest != row.Digest {
		t.Fatalf("session B loaded state = %#v", bLoaded)
	}
	if got := queryA.SkillCatalogState(); !reflect.DeepEqual(got, aAfterA) {
		t.Fatalf("running session B mutated session A catalog/ledger\n before: %#v\n after: %#v", aAfterA, got)
	}
	for name, state := range map[string]loopapi.SkillCatalogRuntimeState{"A": aAfterA, "B": bAfterB} {
		wantEpoch := loopapi.SkillCatalogContextEpoch(fmt.Sprintf("context-%d", state.ContextEpoch))
		if state.Cursor.ContextEpoch != wantEpoch {
			t.Fatalf("session %s cursor epoch = %q, want %q", name, state.Cursor.ContextEpoch, wantEpoch)
		}
	}

	// Replacing B's visible history proves these are independent state owners,
	// not two handles to a shared cursor/ledger whose values merely happened to
	// compare equal at epoch 1.
	queryB.SetMessages([]types.Message{types.UserMessage("session B replacement")})
	bReplaced := queryB.SkillCatalogState()
	if bReplaced.ContextEpoch <= bAfterB.ContextEpoch || !bReplaced.Cursor.Empty() || len(bReplaced.LoadedDigests) != 0 {
		t.Fatalf("session B replacement retained its old state: before=%#v after=%#v", bAfterB, bReplaced)
	}
	if got := queryA.SkillCatalogState(); !reflect.DeepEqual(got, aAfterA) {
		t.Fatalf("replacing session B mutated session A state\n before: %#v\n after: %#v", aAfterA, got)
	}
	if got := queryA.SkillLoadedLedgerState(row.ID); got.LoadedContextEpoch != got.ContextEpoch || got.ContentDigest != row.Digest {
		t.Fatalf("session A ledger was cleared by session B replacement: %#v", got)
	}
}

func TestSkillCatalogIntegrationStableIdentityIsolationAndExecutionFence(t *testing.T) {
	projectRoot := t.TempDir()
	userRoot := t.TempDir()
	_, projectFile := task26WriteLoopSkill(t, projectRoot, "project-shared", "shared", "project summary", "project body v1")
	_, _ = task26WriteLoopSkill(t, userRoot, "user-shared", "shared", "user summary", "user body")
	layer := skills.NewMemorySessionOverrideLayer()
	manager := task26LoopManager(t, layer,
		skills.DirSource{Dir: projectRoot, Source: skills.SourceProject},
		skills.DirSource{Dir: userRoot, Source: skills.SourceUser},
	)

	const sessionA = "task26-session-a"
	initialA := task26LoopSnapshot(t, manager, sessionA)
	if len(initialA.Skills) != 2 || initialA.Skills[0].ID == initialA.Skills[1].ID {
		t.Fatalf("same-name sources were collapsed: %#v", initialA.Skills)
	}
	var winner, shadow skills.EffectiveSkill
	for _, row := range initialA.Skills {
		if row.ShadowedBy == "" {
			winner = row
		} else {
			shadow = row
		}
	}
	if winner.Source != skills.SourceProject || shadow.Source != skills.SourceUser || shadow.ShadowedBy != winner.ID {
		t.Fatalf("unexpected same-name winner/shadow: winner=%#v shadow=%#v", winner, shadow)
	}

	shadowOnly, err := manager.SetVisibility(sessionA, skills.VisibilityOverride{
		SkillID: shadow.ID, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityOff,
	})
	if err != nil {
		t.Fatal(err)
	}
	gotShadow, _ := shadowOnly.Find(shadow.ID)
	gotWinner, _ := shadowOnly.Find(winner.ID)
	if gotShadow.Visibility != skills.VisibilityOff || gotWinner.Visibility == skills.VisibilityOff {
		t.Fatalf("stable-ID shadow mutation changed the wrong row: winner=%#v shadow=%#v", gotWinner, gotShadow)
	}
	if _, err := manager.ResetVisibility(sessionA, skills.SkillScopeSession, shadow.ID); err != nil {
		t.Fatal(err)
	}

	tool := &tools.SkillTool{Manager: manager, FallbackSessionID: sessionA}
	loadedProvider := &task26LoopProvider{turns: [][]types.StreamEvent{
		task26LoopToolEvents("task26-skill-load", "Skill", map[string]any{
			"skill": string(winner.ID), "revision": uint64(winner.Revision),
		}),
		task26LoopTextEvents("loaded"),
	}}
	reg := registry.New()
	reg.Register(tool)
	var query *loopapi.QueryLoop
	tool.SessionIDResolver = func(ctx context.Context) string {
		if execution, ok := loopapi.ToolExecutionContextFromContext(ctx); ok {
			return execution.SessionID
		}
		return sessionA
	}
	tool.LoadedLedgerResolver = func(ctx context.Context, sessionID string, id skills.SkillID) tools.SkillLoadedLedgerState {
		if execution, ok := loopapi.ToolExecutionContextFromContext(ctx); ok {
			if state, resolved := execution.ResolveSkillLoadedLedger(id); resolved {
				return task26ToolsLedger(state)
			}
			return tools.SkillLoadedLedgerState{}
		}
		if sessionID != sessionA {
			return tools.SkillLoadedLedgerState{}
		}
		return task26ToolsLedger(query.ResolveSkillLoadedLedger(query.Messages(), id))
	}
	query = loopapi.New(loadedProvider, reg, loopapi.Config{
		MaxTurns: 3, MaxTokens: 1024, SessionID: sessionA, SkillManager: manager,
	})
	if err := query.Run(context.Background(), "load the shared skill", func(loopapi.Event) {}); err != nil {
		t.Fatal(err)
	}
	loaded := query.SkillLoadedLedgerState(winner.ID)
	if loaded.LoadedContextEpoch != loaded.ContextEpoch || loaded.ContentDigest != winner.Digest {
		t.Fatalf("visible full envelope did not commit the body ledger: %#v", loaded)
	}
	visibleResult := task26FindLoopToolResult(t, query.Messages(), "task26-skill-load")
	var visibleEnvelope struct {
		Kind  skills.InvocationEnvelopeKind `json:"kind"`
		Skill struct {
			ID       skills.SkillID       `json:"id"`
			Revision skills.SkillRevision `json:"revision"`
			Digest   skills.SkillDigest   `json:"digest"`
		} `json:"skill"`
		Body *string `json:"body"`
	}
	if err := json.Unmarshal([]byte(visibleResult.Content), &visibleEnvelope); err != nil {
		t.Fatalf("decode visible Skill envelope: %v\n%s", err, visibleResult.Content)
	}
	if visibleEnvelope.Kind != skills.InvocationEnvelopeFull || visibleEnvelope.Body == nil ||
		visibleEnvelope.Skill.ID != winner.ID || visibleEnvelope.Skill.Revision != winner.Revision || visibleEnvelope.Skill.Digest != winner.Digest ||
		!strings.Contains(*visibleEnvelope.Body, "project body v1") {
		t.Fatalf("on-demand envelope lost versioned identity/body: %#v", visibleEnvelope)
	}
	receipt, found, err := skills.DecodeSkillExecutionReceiptMetadata(visibleResult.Metadata)
	if err != nil || !found || receipt.SkillID != winner.ID || receipt.ContentDigest != winner.Digest || receipt.ContextEpoch != loaded.ContextEpoch {
		t.Fatalf("visible Skill receipt = %#v, found=%t err=%v", receipt, found, err)
	}

	task26RewriteLoopSkill(t, projectFile, "shared", "project summary v2", "project body v2 with a changed digest")
	updatedSnapshot, err := manager.RefreshSnapshot(sessionA)
	if err != nil {
		t.Fatal(err)
	}
	updated, found := updatedSnapshot.Find(winner.ID)
	if !found || updated.Revision == winner.Revision {
		t.Fatalf("updated winner = %#v", updated)
	}
	stale, err := tool.Invoke(context.Background(), tools.SkillInvocationRequest{
		SessionID: sessionA, Selector: string(winner.ID), ExpectedRevision: winner.Revision,
		Origin: skills.InvocationOriginUser,
	})
	if err != nil || !stale.IsError || stale.Metadata["registryOutcome"] != string(skills.SkillResolveStale) {
		t.Fatalf("stale selection crossed the latest-registry fence: result=%#v err=%v", stale, err)
	}

	disabledA, err := manager.SetVisibility(sessionA, skills.VisibilityOverride{
		SkillID: winner.ID, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityOff,
	})
	if err != nil {
		t.Fatal(err)
	}
	disabledRow, _ := disabledA.Find(winner.ID)
	denied, err := tool.Invoke(context.Background(), tools.SkillInvocationRequest{
		SessionID: sessionA, Selector: string(winner.ID), ExpectedRevision: disabledRow.Revision,
		Origin: skills.InvocationOriginUser,
	})
	if err != nil || !denied.IsError || denied.Metadata["registryOutcome"] != string(skills.SkillResolvePolicyDenied) {
		t.Fatalf("disabled skill executed: result=%#v err=%v", denied, err)
	}
	other := task26LoopSnapshot(t, manager, "task26-session-b")
	otherWinner, _ := other.Find(winner.ID)
	if otherWinner.Visibility == skills.VisibilityOff || otherWinner.VisibilitySource == skills.SkillScopeSession {
		t.Fatalf("session-a disable leaked to session-b: %#v", otherWinner)
	}
	reenabled, err := manager.ResetVisibility(sessionA, skills.SkillScopeSession, winner.ID)
	if err != nil {
		t.Fatal(err)
	}
	reenabledRow, _ := reenabled.Find(winner.ID)
	beforeReplacement := query.SkillCatalogState()
	query.SetMessages([]types.Message{types.UserMessage("replacement without catalog or skill body")})
	afterReplacement := query.SkillCatalogState()
	if afterReplacement.ContextEpoch <= beforeReplacement.ContextEpoch || !afterReplacement.Cursor.Empty() || len(afterReplacement.LoadedDigests) != 0 {
		t.Fatalf("history replacement reused invisible catalog/body state: before=%#v after=%#v", beforeReplacement, afterReplacement)
	}
	fullAgain, err := tool.Invoke(context.Background(), tools.SkillInvocationRequest{
		SessionID: sessionA, Selector: string(winner.ID), ExpectedRevision: reenabledRow.Revision,
		Origin: skills.InvocationOriginUser,
	})
	if err != nil || fullAgain.IsError {
		t.Fatalf("post-replacement invocation = %#v, %v", fullAgain, err)
	}
	var envelope struct {
		Kind skills.InvocationEnvelopeKind `json:"kind"`
		Body *string                       `json:"body"`
	}
	if err := json.Unmarshal([]byte(fullAgain.Content), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Kind != skills.InvocationEnvelopeFull || envelope.Body == nil {
		t.Fatalf("invisible ledger was reused after replacement: %#v", envelope)
	}
}

func TestSkillCatalogIntegrationMCPDiscoveryUpdateExecutionAndDisconnect(t *testing.T) {
	projectRoot := t.TempDir()
	_, _ = task26WriteLoopSkill(t, projectRoot, "local-dir", "local", "local summary", "local body")
	layer := skills.NewMemorySessionOverrideLayer()
	manager := task26LoopManager(t, layer, skills.DirSource{Dir: projectRoot, Source: skills.SourceProject})
	const sessionID = "task26-mcp"
	recording := &task26LoopProvider{}
	query := loopapi.New(recording, registry.New(), loopapi.Config{
		MaxTurns: 2, MaxTokens: 1024, SessionID: sessionID, SkillManager: manager,
	})
	_ = task26RunLoopTurn(t, query, recording, "before mcp discovery")

	prompt := skills.MCPPrompt{
		Server: "task26-server", Name: "remote-review", Description: "remote summary v1",
		WhenToUse: "when reviewing", Body: "remote body v1",
	}
	manager.RegisterMCPPrompts([]skills.MCPPrompt{prompt})
	addedSnapshot, err := manager.RefreshSnapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	mcpRow := task26FindLoopSkill(t, addedSnapshot, prompt.QualifiedName(), skills.SourceMCP)
	added := task26LoopTailDelta(t, task26RunLoopTurn(t, query, recording, "after mcp discovery"), "after mcp discovery")
	task26AssertOnlyLoopUpsert(t, added, mcpRow.ID, skills.CatalogUpsertAdded)

	prompt.Description = "remote summary v2"
	prompt.Body = "remote body v2 with changed content"
	manager.RegisterMCPPrompts([]skills.MCPPrompt{prompt})
	updatedSnapshot, err := manager.RefreshSnapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	updatedRow, found := updatedSnapshot.Find(mcpRow.ID)
	if !found || updatedRow.Revision <= mcpRow.Revision || updatedRow.Digest == mcpRow.Digest {
		t.Fatalf("MCP update did not preserve identity and advance content: before=%#v after=%#v", mcpRow, updatedRow)
	}
	updated := task26LoopTailDelta(t, task26RunLoopTurn(t, query, recording, "after mcp update"), "after mcp update")
	task26AssertOnlyLoopUpsert(t, updated, mcpRow.ID, skills.CatalogUpsertUpdated)

	tool := &tools.SkillTool{Manager: manager, FallbackSessionID: sessionID}
	result, err := tool.Invoke(context.Background(), tools.SkillInvocationRequest{
		SessionID: sessionID, Selector: string(updatedRow.ID), ExpectedRevision: updatedRow.Revision,
		Origin: skills.InvocationOriginUser,
	})
	if err != nil || result.IsError || !strings.Contains(result.Content, prompt.Body) {
		t.Fatalf("MCP skill did not execute through the authoritative registry: result=%#v err=%v", result, err)
	}

	manager.RegisterMCPPrompts(nil)
	if _, err := manager.RefreshSnapshot(sessionID); err != nil {
		t.Fatal(err)
	}
	disconnected := task26LoopTailDelta(t, task26RunLoopTurn(t, query, recording, "after mcp disconnect"), "after mcp disconnect")
	task26AssertOnlyLoopRevoke(t, disconnected, mcpRow.ID, skills.CatalogRevokeDeleted)
	rejected, err := tool.Invoke(context.Background(), tools.SkillInvocationRequest{
		SessionID: sessionID, Selector: string(mcpRow.ID), Origin: skills.InvocationOriginUser,
	})
	if err != nil || !rejected.IsError || rejected.Metadata["registryOutcome"] != string(skills.SkillResolveNotFound) {
		t.Fatalf("disconnected MCP skill remained executable: result=%#v err=%v", rejected, err)
	}
}

func task26LoopManager(t *testing.T, layer *skills.MemorySessionOverrideLayer, dirs ...skills.DirSource) *skills.Manager {
	t.Helper()
	settings := t.TempDir()
	store, err := skills.NewFileOverrideStoreAt(skills.OverrideStorePaths{
		UserSettings:    filepath.Join(settings, "user", "settings.json"),
		ProjectSettings: filepath.Join(settings, "project", "settings.json"),
	}, nil, layer)
	if err != nil {
		t.Fatal(err)
	}
	return skills.NewManagerWithOverrideStore(store, dirs...)
}

func task26WriteLoopSkill(t *testing.T, root, directory, name, summary, body string) (string, string) {
	t.Helper()
	dir := filepath.Join(root, directory)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "SKILL.md")
	task26RewriteLoopSkill(t, path, name, summary, body)
	return dir, path
}

func task26RewriteLoopSkill(t *testing.T, path, name, summary, body string) {
	t.Helper()
	content := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n# %s\n\n%s\n", name, summary, name, body)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func task26LoopSnapshot(t *testing.T, manager *skills.Manager, sessionID string) skills.CatalogSnapshot {
	t.Helper()
	snapshot, err := manager.Snapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("invalid catalog snapshot: %v", err)
	}
	return snapshot
}

func task26FindLoopSkill(t *testing.T, snapshot skills.CatalogSnapshot, name string, source skills.SkillSource) skills.EffectiveSkill {
	t.Helper()
	for _, row := range snapshot.Skills {
		if row.Name == name && row.Source == source {
			return row
		}
	}
	t.Fatalf("skill %q/%q not found in %#v", name, source, snapshot.Skills)
	return skills.EffectiveSkill{}
}

func task26RunLoopTurn(t *testing.T, query *loopapi.QueryLoop, recording *task26LoopProvider, user string) []types.Message {
	t.Helper()
	before := len(recording.Calls())
	if err := query.Run(context.Background(), user, func(loopapi.Event) {}); err != nil {
		t.Fatal(err)
	}
	calls := recording.Calls()
	if len(calls) != before+1 {
		t.Fatalf("provider calls advanced from %d to %d, want one", before, len(calls))
	}
	return calls[before].Messages
}

func task26LoopTailDelta(t *testing.T, messages []types.Message, user string) task26CatalogDeltaWire {
	t.Helper()
	if len(messages) < 2 || messages[len(messages)-1].Role != types.RoleUser || messages[len(messages)-1].GetText() != user {
		t.Fatalf("delta/user tail = %#v", messages)
	}
	message := messages[len(messages)-2]
	task26AssertLoopCatalog(t, message, types.DeveloperMessageKindSkillCatalogDelta)
	var delta task26CatalogDeltaWire
	if err := json.Unmarshal([]byte(message.GetText()), &delta); err != nil {
		t.Fatalf("decode catalog delta: %v\n%s", err, message.GetText())
	}
	if delta.Type != "skill_catalog_delta" || delta.FromRevision == 0 || delta.ToRevision <= delta.FromRevision {
		t.Fatalf("invalid catalog delta wire: %#v", delta)
	}
	return delta
}

func task26AssertOnlyLoopUpsert(t *testing.T, delta task26CatalogDeltaWire, id skills.SkillID, reason skills.CatalogUpsertReason) {
	t.Helper()
	if len(delta.Upserts) != 1 || len(delta.Revokes) != 0 || delta.Upserts[0].Skill.ID != id || delta.Upserts[0].Reason != reason {
		t.Fatalf("catalog upsert = %#v, want %s/%s", delta, id, reason)
	}
}

func task26AssertOnlyLoopRevoke(t *testing.T, delta task26CatalogDeltaWire, id skills.SkillID, reason skills.CatalogRevokeReason) {
	t.Helper()
	if len(delta.Upserts) != 0 || len(delta.Revokes) != 1 || delta.Revokes[0].ID != id || delta.Revokes[0].Reason != reason {
		t.Fatalf("catalog revoke = %#v, want %s/%s", delta, id, reason)
	}
}

func task26AssertLoopCatalog(t *testing.T, message types.Message, kind types.DeveloperMessageKind) {
	t.Helper()
	if message.Role != types.RoleDeveloper || !message.IsMeta || message.DeveloperMetadata == nil || message.DeveloperMetadata.Kind != kind {
		t.Fatalf("catalog message = %#v, want internal developer %q", message, kind)
	}
}

func task26CatalogCount(messages []types.Message) int {
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

func task26LoopMessagesJSON(t *testing.T, messages []types.Message) string {
	t.Helper()
	encoded, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func task26ToolsLedger(state loopapi.SkillLoadedLedgerState) tools.SkillLoadedLedgerState {
	return tools.SkillLoadedLedgerState{
		ContextEpoch:       state.ContextEpoch,
		LoadedContextEpoch: state.LoadedContextEpoch,
		ContentDigest:      state.ContentDigest,
		PayloadDigest:      state.PayloadDigest,
	}
}

type task26LoopEnvelopeWire struct {
	Kind skills.InvocationEnvelopeKind `json:"kind"`
	Body *string                       `json:"body"`
}

func task26DecodeLoopEnvelope(t *testing.T, result types.ToolResultBlock) task26LoopEnvelopeWire {
	t.Helper()
	if result.IsError || result.Outcome == types.ToolOutcomeFailed || result.Outcome == types.ToolOutcomeCancelled {
		t.Fatalf("Skill tool result failed: %#v", result)
	}
	var envelope task26LoopEnvelopeWire
	if err := json.Unmarshal([]byte(result.Content), &envelope); err != nil {
		t.Fatalf("decode Skill envelope: %v\n%s", err, result.Content)
	}
	return envelope
}

func task26FindLoopToolResult(t *testing.T, messages []types.Message, toolUseID string) types.ToolResultBlock {
	t.Helper()
	for _, message := range messages {
		for _, block := range message.Content {
			if result, ok := block.(types.ToolResultBlock); ok && result.ToolUseID == toolUseID {
				return result
			}
		}
	}
	t.Fatalf("tool result %q not found in %#v", toolUseID, messages)
	return types.ToolResultBlock{}
}

func task26LoopTextEvents(text string) []types.StreamEvent {
	stop := types.StopReasonEndTurn
	return []types.StreamEvent{
		{Type: types.EventMessageStart, Message: &types.APIMessage{Role: types.RoleAssistant}},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: text}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: &stop},
		{Type: types.EventMessageStop},
	}
}

func task26LoopToolEvents(id, name string, input map[string]any) []types.StreamEvent {
	encoded, _ := json.Marshal(input)
	stop := types.StopReasonToolUse
	return []types.StreamEvent{
		{Type: types.EventMessageStart, Message: &types.APIMessage{Role: types.RoleAssistant}},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeToolUse, ID: id, Name: name}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(encoded)}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: &stop},
		{Type: types.EventMessageStop},
	}
}
