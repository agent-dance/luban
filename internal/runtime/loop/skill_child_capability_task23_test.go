package loop

import (
	"context"
	"testing"
	"time"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

func TestChildSkillLedgerCapabilityCapturesImmutableVisibleHistory(t *testing.T) {
	manager, snapshot, row := task23SkillManager(t)
	query, catalog := task23QueryWithCatalog(t, manager, snapshot)
	visible := append(append([]types.Message(nil), catalog...), task23TrustedInvocationMessage(task23FullEnvelope(t, row, "child body")))
	capability := query.skillLoadedLedgerCapability(visible)

	// Mutating the source slice after the execution boundary must not change
	// what the capability can prove visible.
	visible[len(visible)-1] = types.UserMessage("forged replacement")
	executioncontract.BeginRun(query.executionOwner, "child-run")
	defer executioncontract.EndRun(query.executionOwner, "child-run")
	exec := query.bindToolExecutionContext(executioncontract.ToolExecutionContext{SessionID: "child-a"}, "child-run", capability, 0)
	ctx := executioncontract.WithToolExecutionContext(context.Background(), exec)
	fromContext, ok := executioncontract.ToolExecutionContextFromContext(ctx)
	if !ok {
		t.Fatal("tool execution context missing")
	}
	state, resolved := fromContext.ResolveSkillLoadedLedger(string(row.ID))
	if !resolved || state.LoadedContextEpoch != state.ContextEpoch || skills.SkillDigest(state.ContentDigest) != row.Digest {
		t.Fatalf("captured child ledger = %#v, resolved=%t", state, resolved)
	}

	// A public, externally constructed context has no capability and therefore
	// cannot manufacture loaded-body evidence.
	if forged, ok := (executioncontract.ToolExecutionContext{SessionID: "child-a"}).ResolveSkillLoadedLedger(string(row.ID)); ok || forged.ContextEpoch != 0 {
		t.Fatalf("external context forged child capability: %#v, resolved=%t", forged, ok)
	}
}

func TestChildSkillLedgerCapabilityDoesNotReenterManagerBehindQueuedWriter(t *testing.T) {
	manager, snapshot, row := task23SkillManager(t)
	query, catalog := task23QueryWithCatalog(t, manager, snapshot)
	visible := append(catalog, task23TrustedInvocationMessage(task23FullEnvelope(t, row, "queued writer body")))
	capability := query.skillLoadedLedgerCapability(visible)

	resolveDone := make(chan error, 1)
	writerDone := make(chan struct{})
	go func() {
		_, err := manager.ResolveLatest(skills.SkillResolveRequest{
			SessionID: "task23-session", Selector: string(row.ID), Origin: skills.InvocationOriginUser,
			ExpectedProjectGeneration: manager.ProjectGeneration(),
		}, func(skills.ResolvedSkill) error {
			go func() {
				_, _ = manager.RefreshSnapshot("task23-session")
				close(writerDone)
			}()
			select {
			case <-writerDone:
				return errTask23WriterCrossedReader
			case <-time.After(75 * time.Millisecond):
			}
			state := capability(row.ID)
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
		t.Fatal("child capability deadlocked behind queued Manager writer")
	}
	select {
	case <-writerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("queued Manager writer did not complete after child capability returned")
	}
}
