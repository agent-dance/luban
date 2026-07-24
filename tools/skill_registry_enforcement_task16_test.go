package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

type task16Envelope struct {
	Kind           skills.InvocationEnvelopeKind  `json:"kind"`
	PayloadDigest  skills.InvocationPayloadDigest `json:"payload_digest"`
	PreviousDigest skills.SkillDigest             `json:"previous_digest"`
	Body           *string                        `json:"body"`
}

func TestSkillRegistryStableIDRevisionAndPendingReceipt(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "review", "first body")
	manager := newTestSkillManager(dir)
	row := task16OnlySkill(t, manager, "session")
	ledger := SkillLoadedLedgerState{ContextEpoch: 7}
	tool := &SkillTool{
		Manager:           manager,
		FallbackSessionID: "session",
		LoadedLedgerResolver: func(context.Context, string, skills.SkillID) SkillLoadedLedgerState {
			return ledger
		},
		LanguageResolver: func(context.Context) i18n.Language { return i18n.LangEN },
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"skill": string(row.ID), "revision": uint64(row.Revision),
	})
	if err != nil || result.IsError {
		t.Fatalf("stable-ID execution = %#v, %v", result, err)
	}
	envelope := task16DecodeEnvelope(t, result.Content)
	if envelope.Kind != skills.InvocationEnvelopeFull || envelope.Body == nil || !strings.Contains(*envelope.Body, "first body") {
		t.Fatalf("first envelope = %#v", envelope)
	}
	receipt, found, err := skills.DecodeSkillExecutionReceiptMetadata(result.Metadata)
	if err != nil || !found {
		t.Fatalf("receipt = %#v, found %t, err %v", receipt, found, err)
	}
	if receipt.ContextEpoch != 7 || receipt.SkillID != row.ID || receipt.ContentDigest != row.Digest ||
		receipt.InvocationPayloadDigest != envelope.PayloadDigest || receipt.InvocationEnvelopeKind != skills.InvocationEnvelopeFull {
		t.Fatalf("receipt does not describe exact envelope: %#v / %#v", receipt, envelope)
	}
	if ledger.LoadedContextEpoch != 0 || ledger.ContentDigest != "" || ledger.PayloadDigest != "" {
		t.Fatalf("SkillTool prematurely committed its receipt: %#v", ledger)
	}

	writeMDSkill(t, dir, "review", "second body")
	updated, err := manager.RefreshSnapshot("session")
	if err != nil {
		t.Fatal(err)
	}
	updatedRow, ok := updated.Find(row.ID)
	if !ok || updatedRow.Revision == row.Revision {
		t.Fatalf("updated row = %#v", updatedRow)
	}
	stale, err := tool.Execute(context.Background(), map[string]any{
		"skill": string(row.ID), "revision": uint64(row.Revision),
	})
	if err != nil || !stale.IsError || stale.Metadata["registryOutcome"] != string(skills.SkillResolveStale) {
		t.Fatalf("stale execution = %#v, %v", stale, err)
	}
	if stale.Metadata["skillRevision"] != task16Uint(uint64(updatedRow.Revision)) {
		t.Fatalf("stale result lost current revision: %#v", stale.Metadata)
	}
}

func TestSkillDigestLedgerSelectsEnvelopeOnlyInCurrentEpoch(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "review", "body $ARGUMENTS")
	manager := newTestSkillManager(dir)
	row := task16OnlySkill(t, manager, "session")
	ledger := SkillLoadedLedgerState{ContextEpoch: 11}
	var ledgerMu sync.RWMutex
	tool := &SkillTool{
		Manager: manager,
		LoadedLedgerResolver: func(context.Context, string, skills.SkillID) SkillLoadedLedgerState {
			ledgerMu.RLock()
			defer ledgerMu.RUnlock()
			return ledger
		},
	}

	first, _ := tool.Execute(context.Background(), map[string]any{"skill": string(row.ID), "args": "same"})
	if first.IsError {
		t.Fatalf("first execution = %#v", first)
	}
	firstEnvelope := task16DecodeEnvelope(t, first.Content)
	firstReceipt := task16Receipt(t, first)
	if firstEnvelope.Kind != skills.InvocationEnvelopeFull {
		t.Fatalf("first kind = %q", firstEnvelope.Kind)
	}

	// Simulate task_19 committing only after the exact result became visible.
	ledgerMu.Lock()
	ledger = SkillLoadedLedgerState{
		ContextEpoch: 11, LoadedContextEpoch: 11,
		ContentDigest: firstReceipt.ContentDigest, PayloadDigest: firstReceipt.InvocationPayloadDigest,
	}
	ledgerMu.Unlock()
	second, _ := tool.Execute(context.Background(), map[string]any{"skill": string(row.ID), "args": "same"})
	secondEnvelope := task16DecodeEnvelope(t, second.Content)
	if second.IsError || secondEnvelope.Kind != skills.InvocationEnvelopeAlreadyLoaded || secondEnvelope.Body != nil {
		t.Fatalf("same-payload envelope = %#v, result=%#v", secondEnvelope, second)
	}

	differentArgs, _ := tool.Execute(context.Background(), map[string]any{"skill": string(row.ID), "args": "different"})
	differentEnvelope := task16DecodeEnvelope(t, differentArgs.Content)
	if differentArgs.IsError || differentEnvelope.Kind != skills.InvocationEnvelopeFull || differentEnvelope.Body == nil {
		t.Fatalf("different-args envelope = %#v, result=%#v", differentEnvelope, differentArgs)
	}

	ledgerMu.Lock()
	ledger.ContextEpoch = 12
	ledger.LoadedContextEpoch = 11
	ledgerMu.Unlock()
	newEpoch, _ := tool.Execute(context.Background(), map[string]any{"skill": string(row.ID), "args": "same"})
	newEpochEnvelope := task16DecodeEnvelope(t, newEpoch.Content)
	newEpochReceipt := task16Receipt(t, newEpoch)
	if newEpochEnvelope.Kind != skills.InvocationEnvelopeFull || newEpochReceipt.ContextEpoch != 12 {
		t.Fatalf("new-epoch execution reused old ledger: %#v / %#v", newEpochEnvelope, newEpochReceipt)
	}

	for name, invalidLedger := range map[string]SkillLoadedLedgerState{
		"invalid content digest": {
			ContextEpoch: 12, LoadedContextEpoch: 12,
			ContentDigest: "not-a-digest", PayloadDigest: firstReceipt.InvocationPayloadDigest,
		},
		"invalid payload digest": {
			ContextEpoch: 12, LoadedContextEpoch: 12,
			ContentDigest: firstReceipt.ContentDigest, PayloadDigest: "not-a-digest",
		},
	} {
		ledgerMu.Lock()
		ledger = invalidLedger
		ledgerMu.Unlock()
		result, _ := tool.Execute(context.Background(), map[string]any{"skill": string(row.ID), "args": "same"})
		envelope := task16DecodeEnvelope(t, result.Content)
		if result.IsError || envelope.Kind != skills.InvocationEnvelopeFull || envelope.Body == nil {
			t.Fatalf("%s reused invalid ledger: %#v / %#v", name, envelope, result)
		}
	}

	ledgerMu.Lock()
	ledger = SkillLoadedLedgerState{
		ContextEpoch: 12, LoadedContextEpoch: 12,
		ContentDigest: firstReceipt.ContentDigest, PayloadDigest: firstReceipt.InvocationPayloadDigest,
	}
	ledgerMu.Unlock()
	writeMDSkill(t, dir, "review", "updated body $ARGUMENTS")
	updated := task16RefreshOnlySkill(t, manager, "session")
	superseding, _ := tool.Execute(context.Background(), map[string]any{
		"skill": string(updated.ID), "revision": uint64(updated.Revision), "args": "same",
	})
	supersedingEnvelope := task16DecodeEnvelope(t, superseding.Content)
	if superseding.IsError || supersedingEnvelope.Kind != skills.InvocationEnvelopeSuperseding ||
		supersedingEnvelope.PreviousDigest != firstReceipt.ContentDigest || supersedingEnvelope.Body == nil {
		t.Fatalf("updated-body envelope = %#v, result=%#v", supersedingEnvelope, superseding)
	}
}

func TestSkillVisibilityUsesExplicitInvocationOrigin(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "manual", "---\ndisable-model-invocation: true\ndescription: manual\n---\nmanual body")
	manager := newTestSkillManager(dir)
	row := task16OnlySkill(t, manager, "session")
	tool := &SkillTool{
		Manager: manager,
		LoadedLedgerResolver: func(context.Context, string, skills.SkillID) SkillLoadedLedgerState {
			return SkillLoadedLedgerState{ContextEpoch: 3}
		},
	}

	model, _ := tool.Execute(context.Background(), map[string]any{
		"skill": string(row.ID), "revision": uint64(row.Revision),
		// Unknown transport fields must not let a model-origin tool call
		// upgrade itself to the explicit-user authority path.
		"origin": string(skills.InvocationOriginUser),
	})
	if !model.IsError || model.Metadata["registryOutcome"] != string(skills.SkillResolvePolicyDenied) ||
		model.Metadata["invocationOrigin"] != string(skills.InvocationOriginModel) {
		t.Fatalf("manual-only model invocation = %#v", model)
	}
	user, err := tool.Invoke(context.Background(), SkillInvocationRequest{
		SessionID: "session", Selector: string(row.ID), ExpectedRevision: row.Revision,
		Origin: skills.InvocationOriginUser,
	})
	if err != nil || user.IsError || user.Metadata["invocationOrigin"] != string(skills.InvocationOriginUser) {
		t.Fatalf("manual-only user invocation = %#v, %v", user, err)
	}
	if task16DecodeEnvelope(t, user.Content).Kind != skills.InvocationEnvelopeFull {
		t.Fatalf("user invocation did not carry full body: %s", user.Content)
	}

	for _, origin := range []skills.InvocationOrigin{skills.InvocationOriginModel, skills.InvocationOrigin("unknown"), ""} {
		rejected, _ := tool.Invoke(context.Background(), SkillInvocationRequest{
			SessionID: "session", Selector: string(row.ID), Origin: origin,
		})
		if !rejected.IsError || rejected.Metadata["errorCode"] != task16Uint(uint64(SkillErrInvalidFormat)) ||
			rejected.Metadata["invocationOrigin"] != string(origin) {
			t.Fatalf("explicit origin %q did not fail closed: %#v", origin, rejected)
		}
	}
}

func TestSkillRegistryManagedDenyBlocksBeforeExecutionSideEffects(t *testing.T) {
	root, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "managed", "---\nallowed-tools: Bash\n---\nmanaged body")
	manager := newTestSkillManager(dir)
	row := task16OnlySkill(t, manager, "session")
	store, err := skills.NewFileOverrideStoreAt(skills.OverrideStorePaths{
		UserSettings:    filepath.Join(root, "user-settings.json"),
		ProjectSettings: filepath.Join(root, "project-settings.json"),
	}, map[skills.SkillID]skills.VisibilityOverride{
		row.ID: {
			SkillID: row.ID, Scope: skills.SkillScopeManaged, Visibility: skills.VisibilityOff,
		},
	}, skills.NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	manager.SetOverrideStore(store)

	applied := false
	tool := &SkillTool{
		Manager: manager,
		AllowedToolsApplier: func(context.Context, string, string, []string) func() {
			applied = true
			return nil
		},
	}
	denied, err := tool.Invoke(context.Background(), SkillInvocationRequest{
		SessionID: "session", Selector: string(row.ID), Origin: skills.InvocationOriginUser,
	})
	if err != nil || !denied.IsError || denied.Metadata["registryOutcome"] != string(skills.SkillResolvePolicyDenied) {
		t.Fatalf("managed deny = %#v, %v", denied, err)
	}
	if applied || len(tool.InvokedSkills("session")) != 0 {
		t.Fatalf("managed denial crossed execution side-effect boundary: applied=%t invoked=%#v", applied, tool.InvokedSkills("session"))
	}
}

func TestSkillRegistryRejectsShadowedAndRevokedStableIDs(t *testing.T) {
	high := t.TempDir()
	low := t.TempDir()
	task16WriteNamedSkill(t, high, "high", "shared", "high body")
	task16WriteNamedSkill(t, low, "low", "shared", "low body")
	manager := skills.NewManager(
		skills.DirSource{Dir: high, Source: skills.SourceProject},
		skills.DirSource{Dir: low, Source: skills.SourceUser},
	)
	snapshot, err := manager.Snapshot("session")
	if err != nil {
		t.Fatal(err)
	}
	var winner, shadow skills.EffectiveSkill
	for _, row := range snapshot.Skills {
		if row.ShadowedBy == "" {
			winner = row
		} else {
			shadow = row
		}
	}
	tool := &SkillTool{Manager: manager}
	shadowed, _ := tool.Invoke(context.Background(), SkillInvocationRequest{
		SessionID: "session", Selector: string(shadow.ID), Origin: skills.InvocationOriginUser,
	})
	if !shadowed.IsError || shadowed.Metadata["registryOutcome"] != string(skills.SkillResolveShadowed) {
		t.Fatalf("shadowed stable ID = %#v", shadowed)
	}
	byName, _ := tool.Execute(context.Background(), map[string]any{"skill": "shared"})
	if byName.IsError || byName.Metadata["skillID"] != string(winner.ID) {
		t.Fatalf("winner name resolution = %#v", byName)
	}

	if err := os.RemoveAll(filepath.Join(high, "high")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RefreshSnapshot("session"); err != nil {
		t.Fatal(err)
	}
	revoked, _ := tool.Execute(context.Background(), map[string]any{
		"skill": string(winner.ID), "revision": uint64(winner.Revision),
	})
	if !revoked.IsError || revoked.Metadata["registryOutcome"] != string(skills.SkillResolveNotFound) {
		t.Fatalf("revoked stable ID = %#v", revoked)
	}
}

func TestSkillDisableRaceLinearizesBeforeFutureExecution(t *testing.T) {
	_, dir := makeTempSkillDir(t)
	writeMDSkill(t, dir, "review", "race body")
	manager := newTestSkillManager(dir)
	row := task16OnlySkill(t, manager, "session")
	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce sync.Once
	tool := &SkillTool{
		Manager:           manager,
		FallbackSessionID: "session",
		LoadedLedgerResolver: func(context.Context, string, skills.SkillID) SkillLoadedLedgerState {
			enteredOnce.Do(func() { close(entered) })
			<-release
			return SkillLoadedLedgerState{ContextEpoch: 5}
		},
	}

	executed := make(chan typesResult, 1)
	go func() {
		result, err := tool.Execute(context.Background(), map[string]any{
			"skill": string(row.ID), "revision": uint64(row.Revision),
		})
		executed <- typesResult{result: result, err: err}
	}()
	<-entered
	disabled := make(chan struct {
		changed bool
		found   bool
	}, 1)
	go func() {
		changed, found := manager.SetEnabled("session", "review", false)
		disabled <- struct {
			changed bool
			found   bool
		}{changed: changed, found: found}
	}()
	select {
	case state := <-disabled:
		t.Fatalf("disable crossed ResolveLatest consume boundary: %#v", state)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	first := <-executed
	if first.err != nil || first.result.IsError {
		t.Fatalf("linearized execution = %#v, %v", first.result, first.err)
	}
	state := <-disabled
	if !state.changed || !state.found {
		t.Fatalf("disable = %#v", state)
	}
	future, _ := tool.Execute(context.Background(), map[string]any{"skill": string(row.ID)})
	if !future.IsError || future.Metadata["registryOutcome"] != string(skills.SkillResolvePolicyDenied) {
		t.Fatalf("post-disable execution was not rejected: %#v", future)
	}
}

func TestSkillRegistryRejectionUsesActiveLanguage(t *testing.T) {
	tool := &SkillTool{
		Manager:          newTestSkillManager(),
		LanguageResolver: func(context.Context) i18n.Language { return i18n.LangZH },
	}
	result, _ := tool.Execute(context.Background(), map[string]any{"skill": "missing"})
	if !result.IsError || !strings.Contains(result.Content, "未找到技能") || strings.Contains(result.Content, "not found") {
		t.Fatalf("localized registry rejection = %#v", result)
	}
}

func TestSkillRegistryDeterministicFailuresUseActiveLanguage(t *testing.T) {
	ctx := context.Background()
	language := func(context.Context) i18n.Language { return i18n.LangZH }

	unavailable, _ := (&SkillTool{LanguageResolver: language}).Execute(ctx, map[string]any{"skill": "review"})
	if !unavailable.IsError || !strings.Contains(unavailable.Content, "技能服务不可用") ||
		strings.Contains(unavailable.Content, "manager is not configured") {
		t.Fatalf("localized unavailable result = %#v", unavailable)
	}

	invalidSelector, _ := (&SkillTool{
		Manager: newTestSkillManager(), LanguageResolver: language,
	}).Execute(ctx, map[string]any{"skill": "../review"})
	if !invalidSelector.IsError || !strings.Contains(invalidSelector.Content, "技能选择器无效") ||
		strings.Contains(invalidSelector.Content, "path") {
		t.Fatalf("localized invalid selector = %#v", invalidSelector)
	}

	explicitModel, _ := (&SkillTool{
		Manager: newTestSkillManager(), LanguageResolver: language,
	}).Invoke(ctx, SkillInvocationRequest{SessionID: "session", Selector: "review", Origin: skills.InvocationOriginModel})
	if !explicitModel.IsError || !strings.Contains(explicitModel.Content, "必须使用用户来源") ||
		strings.Contains(explicitModel.Content, "requires user origin") {
		t.Fatalf("localized explicit-origin rejection = %#v", explicitModel)
	}
}

type typesResult struct {
	result types.ToolResult
	err    error
}

func task16OnlySkill(t *testing.T, manager *skills.Manager, sessionID string) skills.EffectiveSkill {
	t.Helper()
	snapshot, err := manager.Snapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Skills) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	return snapshot.Skills[0]
}

func task16RefreshOnlySkill(t *testing.T, manager *skills.Manager, sessionID string) skills.EffectiveSkill {
	t.Helper()
	snapshot, err := manager.RefreshSnapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Skills) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	return snapshot.Skills[0]
}

func task16DecodeEnvelope(t *testing.T, content string) task16Envelope {
	t.Helper()
	var envelope task16Envelope
	if err := json.Unmarshal([]byte(content), &envelope); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, content)
	}
	return envelope
}

func task16Receipt(t *testing.T, result types.ToolResult) skills.SkillExecutionReceipt {
	t.Helper()
	receipt, found, err := skills.DecodeSkillExecutionReceiptMetadata(result.Metadata)
	if err != nil || !found {
		t.Fatalf("receipt = %#v, found %t, err %v", receipt, found, err)
	}
	return receipt
}

func task16WriteNamedSkill(t *testing.T, root, dirName, name, body string) {
	t.Helper()
	dir := filepath.Join(root, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: shared\n---\n" + body
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func task16Uint(value uint64) string {
	return strconv.FormatUint(value, 10)
}
