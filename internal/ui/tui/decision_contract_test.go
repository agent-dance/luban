package tui

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/permissions"
	gtui "github.com/grindlemire/go-tui"
)

func TestDecisionRequestsAreSerializedWithoutCrossedResponses(t *testing.T) {
	state := NewAppState()
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	requests := []permissions.PromptRequest{{DecisionID: "first", Choices: []string{"allow_once", "reject"}}, {DecisionID: "second", Choices: []string{"allow_once", "reject"}}}
	responses := make(chan permissions.PromptResponse, 2)
	for _, request := range requests {
		request := request
		go func() { responses <- renderer.DecisionRequest(context.Background(), request) }()
	}
	waitForDecisionID(t, state, "first", "second")
	firstID := state.DecisionReq.Get().DecisionID
	state.DecisionResp <- decisionResponse(firstID, permissions.PromptOutcomeApproved, "allow_once")
	first := <-responses
	if first.DecisionID != firstID {
		t.Fatalf("first response crossed requests: %+v", first)
	}
	wantSecond := "first"
	if firstID == "first" {
		wantSecond = "second"
	}
	waitForDecisionID(t, state, wantSecond)
	state.DecisionResp <- decisionResponse(wantSecond, permissions.PromptOutcomeRejected, "reject")
	second := <-responses
	if second.DecisionID != wantSecond {
		t.Fatalf("second response crossed requests: %+v", second)
	}
}

func TestDuplicateStableDecisionRegistrationSharesOneOverlayAndTerminalAudit(t *testing.T) {
	state := newDecisionBrokerTestState()
	t.Cleanup(state.SignalStop)
	var updates atomic.Int32
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool {
		updates.Add(1)
		fn()
		return true
	}}
	request := permissions.PromptRequest{
		DecisionID: "decision:session:tool-one", SessionID: "session", TurnID: "turn-one",
		ToolUseID: "tool-one", ToolName: "Write", Input: map[string]any{"file_path": "/workspace/one"},
		Kind: permissions.PromptKindPermission, Choices: []string{"allow_once", "reject"},
	}
	responses := make(chan permissions.PromptResponse, 2)
	go func() { responses <- renderer.DecisionRequest(context.Background(), request) }()
	waitForDecisionID(t, state, request.DecisionID)
	go func() { responses <- renderer.DecisionRequest(context.Background(), request) }()
	waitForAtomicAtLeast(t, &updates, 2)

	broker := renderer.decisionBroker()
	broker.mu.Lock()
	waiters, pending := len(broker.waiters), len(broker.pendingByID)
	broker.mu.Unlock()
	if waiters != 1 || pending != 1 || state.ActivitySnapshot().Counts.NeedsInput != 1 {
		t.Fatalf("duplicate decision was not coalesced: waiters=%d pending=%d snapshot=%+v", waiters, pending, state.ActivitySnapshot())
	}
	state.DecisionResp <- decisionResponse(request.DecisionID, permissions.PromptOutcomeApproved, "allow_once")
	for range 2 {
		select {
		case response := <-responses:
			if response.DecisionID != request.DecisionID || response.Outcome != permissions.PromptOutcomeApproved || response.Decision != permissions.DecisionAllowOnce {
				t.Fatalf("coalesced response = %+v", response)
			}
		case <-time.After(time.Second):
			t.Fatal("coalesced decision registration did not receive the committed terminal response")
		}
	}
	if history := state.DecisionHistory.Get(); len(history) != 1 || state.DecisionReq.Get() != nil {
		t.Fatalf("coalesced decision terminal audit = %+v; active=%+v", history, state.DecisionReq.Get())
	}
}

func TestDecisionIDCollisionDoesNotShareAllowOnceAcrossToolUseIDs(t *testing.T) {
	state := newDecisionBrokerTestState()
	t.Cleanup(state.SignalStop)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	firstRequest := permissions.PromptRequest{
		DecisionID: "decision:session:reused", SessionID: "session", ToolUseID: "tool-one", ToolName: "Write",
		Input: map[string]any{"file_path": "/workspace/same"}, Kind: permissions.PromptKindPermission,
		Choices: []string{"allow_once", "reject"},
	}
	secondRequest := clonePromptRequest(firstRequest)
	secondRequest.ToolUseID = "tool-two"
	firstReturned := make(chan permissions.PromptResponse, 1)
	secondReturned := make(chan permissions.PromptResponse, 1)
	go func() { firstReturned <- renderer.DecisionRequest(context.Background(), firstRequest) }()
	waitForDecisionID(t, state, firstRequest.DecisionID)
	go func() { secondReturned <- renderer.DecisionRequest(context.Background(), secondRequest) }()

	select {
	case response := <-secondReturned:
		if response.Outcome != permissions.PromptOutcomeCancelled || response.Decision != permissions.DecisionDeny {
			t.Fatalf("colliding tool invocation inherited authority: %+v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("colliding decision identity was not rejected deterministically")
	}
	if active := state.DecisionReq.Get(); active == nil || active.ToolUseID != "tool-one" {
		t.Fatalf("decision collision replaced the canonical overlay: %+v", active)
	}
	state.DecisionResp <- decisionResponse(firstRequest.DecisionID, permissions.PromptOutcomeApproved, "allow_once")
	select {
	case response := <-firstReturned:
		if response.Decision != permissions.DecisionAllowOnce || response.Outcome != permissions.PromptOutcomeApproved {
			t.Fatalf("canonical decision response = %+v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("canonical decision did not resolve after a colliding registration")
	}
	if history := state.DecisionHistory.Get(); len(history) != 1 || history[0].Prompt.ToolUseID != "tool-one" {
		t.Fatalf("decision collision polluted terminal audit: %+v", history)
	}
}

func TestMissingDecisionIDCannotCreateParallelDuplicateOverlay(t *testing.T) {
	state := newDecisionBrokerTestState()
	t.Cleanup(state.SignalStop)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	request := permissions.PromptRequest{
		SessionID: "session", ToolUseID: "tool-without-decision-id", ToolName: "Write",
		Input: map[string]any{"file_path": "/workspace/same"}, Kind: permissions.PromptKindPermission,
		Choices: []string{"allow_once", "reject"},
	}
	firstReturned := make(chan permissions.PromptResponse, 1)
	secondReturned := make(chan permissions.PromptResponse, 1)
	go func() { firstReturned <- renderer.DecisionRequest(context.Background(), request) }()
	waitForDecisionID(t, state, "")
	go func() { secondReturned <- renderer.DecisionRequest(context.Background(), request) }()
	select {
	case response := <-secondReturned:
		if response.Outcome != permissions.PromptOutcomeCancelled || response.Decision != permissions.DecisionDeny {
			t.Fatalf("missing decision identity shared authority: %+v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("missing decision identity created a second unresolved registration")
	}
	broker := renderer.decisionBroker()
	broker.mu.Lock()
	waiters := len(broker.waiters)
	broker.mu.Unlock()
	if waiters != 1 {
		t.Fatalf("missing decision identity created %d parallel waiters", waiters)
	}
	state.DecisionResp <- decisionResponse("", permissions.PromptOutcomeRejected, "reject")
	select {
	case <-firstReturned:
	case <-time.After(time.Second):
		t.Fatal("canonical request without a decision identity did not resolve")
	}
}

func TestMatchingDecisionArgumentsRemainIndependentForDistinctInvocationIDs(t *testing.T) {
	state := newDecisionBrokerTestState()
	t.Cleanup(state.SignalStop)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	first := permissions.PromptRequest{
		DecisionID: "decision:session:tool-one", SessionID: "session", ToolUseID: "tool-one", ToolName: "Write",
		Input: map[string]any{"file_path": "/workspace/same"}, Kind: permissions.PromptKindPermission,
		Choices: []string{"allow_once", "reject"},
	}
	second := clonePromptRequest(first)
	second.DecisionID = "decision:session:tool-two"
	second.ToolUseID = "tool-two"
	responses := make(chan permissions.PromptResponse, 2)
	go func() { responses <- renderer.DecisionRequest(context.Background(), first) }()
	waitForDecisionID(t, state, first.DecisionID)
	go func() { responses <- renderer.DecisionRequest(context.Background(), second) }()
	waitForDecisionAttentionCount(t, state, 2)

	for _, id := range []string{first.DecisionID, second.DecisionID} {
		waitForDecisionID(t, state, id)
		state.DecisionResp <- decisionResponse(id, permissions.PromptOutcomeApproved, "allow_once")
		select {
		case response := <-responses:
			if response.DecisionID != id || response.Decision != permissions.DecisionAllowOnce {
				t.Fatalf("independent invocation response crossed decisions: got=%+v want=%q", response, id)
			}
		case <-time.After(time.Second):
			t.Fatalf("independent invocation %q did not resolve", id)
		}
	}
	if history := state.DecisionHistory.Get(); len(history) != 2 {
		t.Fatalf("independent invocations shared one terminal audit: %+v", history)
	}
}

func TestConcurrentDecisionRequestsRegisterAllPendingAttentionBeforeResolution(t *testing.T) {
	state := newDecisionBrokerTestState()
	t.Cleanup(state.SignalStop)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	responses := make(chan permissions.PromptResponse, 3)
	for _, id := range []string{"alpha", "beta", "gamma"} {
		id := id
		go func() {
			responses <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{
				DecisionID: id, SessionID: "session", ActorID: "agent-" + id, Action: "Review " + id,
			})
		}()
	}

	waitForDecisionAttentionCount(t, state, 3)
	for _, id := range []string{"alpha", "beta", "gamma"} {
		activity, ok := state.GetActivity("decision:" + id)
		if !ok || activity.Control.JumpTarget == "" || len(activity.Control.DetailRefs) != 1 || !activityTestHasAction(activity.Actions, ActivityDetails) {
			t.Fatalf("decision %s lacks searchable detail evidence: %+v ok=%v", id, activity, ok)
		}
	}
	active := state.DecisionReq.Get()
	if active == nil {
		t.Fatal("concurrent decisions registered attention without one serialized overlay")
	}
	select {
	case response := <-responses:
		t.Fatalf("decision returned before its own response: %+v", response)
	default:
	}

	seen := make(map[string]bool, 3)
	for len(seen) < 3 {
		request := waitForAnyDecisionID(t, state)
		state.DecisionResp <- decisionResponse(request.DecisionID, permissions.PromptOutcomeApproved, "allow_once")
		select {
		case response := <-responses:
			seen[response.DecisionID] = true
		case <-time.After(time.Second):
			t.Fatalf("decision %q did not receive its response", request.DecisionID)
		}
	}
	if len(state.DecisionHistory.Get()) != 3 || state.DecisionReq.Get() != nil {
		t.Fatalf("concurrent decision terminal state: history=%+v active=%+v", state.DecisionHistory.Get(), state.DecisionReq.Get())
	}
}

func TestDecisionResponsesRouteByIDWithoutCrossingSerializedOverlay(t *testing.T) {
	state := newDecisionBrokerTestState()
	t.Cleanup(state.SignalStop)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	responses := make(chan permissions.PromptResponse, 2)

	go func() {
		responses <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{
			DecisionID: "first", SessionID: "session", Action: "Review first",
		})
	}()
	waitForDecisionID(t, state, "first")
	go func() {
		responses <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{
			DecisionID: "second", SessionID: "session", Action: "Review second",
		})
	}()
	waitForDecisionAttentionCount(t, state, 2)

	// A keyed producer may resolve a queued decision, but that must not steal
	// the first decision's waiter or replace its serialized overlay.
	state.DecisionResp <- decisionResponse("second", permissions.PromptOutcomeRejected, "reject")
	select {
	case response := <-responses:
		if response.DecisionID != "second" || response.Outcome != permissions.PromptOutcomeRejected {
			t.Fatalf("out-of-order keyed response crossed waiters: %+v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("queued keyed decision did not receive its independent response")
	}
	if active := state.DecisionReq.Get(); active == nil || active.DecisionID != "first" {
		t.Fatalf("queued resolution replaced active overlay: %+v", active)
	}

	state.DecisionResp <- decisionResponse("first", permissions.PromptOutcomeApproved, "allow_once")
	select {
	case response := <-responses:
		if response.DecisionID != "first" || response.Outcome != permissions.PromptOutcomeApproved {
			t.Fatalf("active response crossed waiters: %+v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("active keyed decision did not resolve")
	}
}

func TestQueuedDecisionCancellationDoesNotDismissActiveOverlay(t *testing.T) {
	state := newDecisionBrokerTestState()
	t.Cleanup(state.SignalStop)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	responses := make(chan permissions.PromptResponse, 2)

	go func() {
		responses <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{
			DecisionID: "active", SessionID: "session", Action: "Review active",
		})
	}()
	waitForDecisionID(t, state, "active")
	queuedContext, cancelQueued := context.WithCancel(context.Background())
	go func() {
		responses <- renderer.DecisionRequest(queuedContext, permissions.PromptRequest{
			DecisionID: "queued", SessionID: "session", Action: "Review queued",
		})
	}()
	waitForDecisionAttentionCount(t, state, 2)
	cancelQueued()

	select {
	case response := <-responses:
		if response.DecisionID != "queued" || response.Outcome != permissions.PromptOutcomeCancelled {
			t.Fatalf("queued cancellation response = %+v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("queued decision cancellation leaked its waiter")
	}
	if active := state.DecisionReq.Get(); active == nil || active.DecisionID != "active" {
		t.Fatalf("queued cancellation dismissed active overlay: %+v", active)
	}

	state.DecisionResp <- decisionResponse("active", permissions.PromptOutcomeRejected, "reject")
	select {
	case response := <-responses:
		if response.DecisionID != "active" {
			t.Fatalf("active response crossed cancelled waiter: %+v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("active decision did not resolve after queued cancellation")
	}
	if state.DecisionReq.Get() != nil {
		t.Fatalf("decision overlay remained after all waiters resolved: %+v", state.DecisionReq.Get())
	}
}

func TestConcurrentPendingDecisionShutdownResolvesEveryWaiter(t *testing.T) {
	state := newDecisionBrokerTestState()
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool {
		select {
		case <-state.stopCh:
			return false
		default:
			fn()
			return true
		}
	}}
	responses := make(chan permissions.PromptResponse, 3)
	for _, id := range []string{"one", "two", "three"} {
		id := id
		go func() {
			responses <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{
				DecisionID: id, SessionID: "session", Action: "Review " + id,
			})
		}()
	}
	waitForDecisionAttentionCount(t, state, 3)
	state.SignalStop()

	seen := make(map[string]bool, 3)
	for range 3 {
		select {
		case response := <-responses:
			if response.Outcome != permissions.PromptOutcomeShutdown {
				t.Fatalf("shutdown response = %+v", response)
			}
			seen[response.DecisionID] = true
		case <-time.After(time.Second):
			t.Fatalf("shutdown leaked decision waiters; resolved=%v", seen)
		}
	}
	if len(seen) != 3 || len(state.DecisionHistory.Get()) != 3 || state.DecisionReq.Get() != nil {
		t.Fatalf("shutdown did not terminalize all decisions: seen=%v history=%+v active=%+v", seen, state.DecisionHistory.Get(), state.DecisionReq.Get())
	}
}

func TestDecisionEpochChangeCancelsQueuedWaitersWithoutInvisibleLeak(t *testing.T) {
	state := newDecisionBrokerTestState()
	t.Cleanup(state.SignalStop)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	responses := make(chan permissions.PromptResponse, 2)
	for _, id := range []string{"active-old-epoch", "queued-old-epoch"} {
		id := id
		go func() {
			responses <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{
				DecisionID: id, SessionID: "session", Action: "Review " + id,
			})
		}()
		if id == "active-old-epoch" {
			waitForDecisionID(t, state, id)
		}
	}
	waitForDecisionAttentionCount(t, state, 2)
	state.SessionEpoch.Set(2)
	state.DecisionResp <- decisionResponse("active-old-epoch", permissions.PromptOutcomeApproved, "allow_once")

	seen := make(map[string]permissions.PromptResponse, 2)
	for range 2 {
		select {
		case response := <-responses:
			seen[response.DecisionID] = response
		case <-time.After(time.Second):
			t.Fatalf("epoch transition leaked an invisible queued waiter: resolved=%+v active=%+v", seen, state.DecisionReq.Get())
		}
	}
	for id, response := range seen {
		if response.Outcome != permissions.PromptOutcomeCancelled || response.Decision != permissions.DecisionDeny {
			t.Fatalf("old-epoch decision %q response = %+v", id, response)
		}
	}
	if state.DecisionReq.Get() != nil {
		t.Fatalf("old-epoch overlay remained visible: %+v", state.DecisionReq.Get())
	}
}

func TestDecisionResolutionQueueFailureFailsClosedAndDrainsBroker(t *testing.T) {
	state := newDecisionBrokerTestState()
	t.Cleanup(state.SignalStop)
	var enqueueCalls atomic.Int32
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool {
		if enqueueCalls.Add(1) != 1 {
			return false
		}
		fn()
		return true
	}}
	returned := make(chan permissions.PromptResponse, 1)
	go func() {
		returned <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{
			DecisionID: "queue-failed", SessionID: "session", Action: "Review queue failure",
		})
	}()
	waitForDecisionID(t, state, "queue-failed")
	state.DecisionResp <- decisionResponse("queue-failed", permissions.PromptOutcomeApproved, "allow_once")

	select {
	case response := <-returned:
		if response.Outcome != permissions.PromptOutcomeShutdown || response.Decision != permissions.DecisionDeny {
			t.Fatalf("failed terminal queue did not fail closed: %+v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("failed terminal queue leaked decision waiter")
	}
	history := state.DecisionHistory.Get()
	if len(history) != 1 || history[0].Response.Outcome != permissions.PromptOutcomeShutdown {
		t.Fatalf("failed terminal queue audit = %+v", history)
	}
	if state.DecisionReq.Get() != nil {
		t.Fatalf("failed terminal queue left overlay active: %+v", state.DecisionReq.Get())
	}
}

func newDecisionBrokerTestState() *AppState {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	return state
}

func waitForDecisionAttentionCount(t *testing.T, state *AppState, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := state.ActivitySnapshot().Counts.NeedsInput; got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("pending decision attention count = %d, want %d; snapshot=%+v", state.ActivitySnapshot().Counts.NeedsInput, want, state.ActivitySnapshot())
}

func waitForAtomicAtLeast(t *testing.T, value *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if value.Load() >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("atomic value = %d, want at least %d", value.Load(), want)
}

func waitForAnyDecisionID(t *testing.T, state *AppState) *DecisionRequest {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if request := state.DecisionReq.Get(); request != nil {
			return request
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no serialized decision overlay became active")
	return nil
}

func TestDecisionRequestWaitsForResolutionCommit(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(4)
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 4})
	updates := make(chan func(), 2)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { updates <- fn; return true }}
	returned := make(chan permissions.PromptResponse, 1)
	go func() {
		returned <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{DecisionID: "decision", SessionID: "session"})
	}()
	(<-updates)() // publish request
	state.DecisionResp <- decisionResponse("decision", permissions.PromptOutcomeRejected, "reject")
	resolution := <-updates
	select {
	case <-returned:
		t.Fatal("DecisionRequest returned before its terminal audit was committed")
	case <-time.After(20 * time.Millisecond):
	}
	resolution()
	select {
	case response := <-returned:
		if response.Outcome != permissions.PromptOutcomeRejected {
			t.Fatalf("response = %+v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("DecisionRequest did not return after resolution commit")
	}
	if history := state.DecisionHistory.Get(); len(history) != 1 || history[0].Prompt.DecisionID != "decision" {
		t.Fatalf("decision history = %+v", history)
	}
	activity, ok := state.GetActivity("decision:decision")
	if !ok || activity.State != ActivityFailed || activity.Outcome != OutcomeDenied {
		t.Fatalf("rejected decision activity = %+v, ok=%v", activity, ok)
	}
}

func TestPendingDecisionShutdownCommitsTerminalAuditAfterUpdateQueueStops(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(2)
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 2})
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool {
		select {
		case <-state.stopCh:
			return false
		default:
			fn()
			return true
		}
	}}
	returned := make(chan permissions.PromptResponse, 1)
	go func() {
		returned <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{
			DecisionID: "shutdown", SessionID: "session", Action: "Review shutdown",
		})
	}()
	waitForDecisionID(t, state, "shutdown")
	state.SignalStop()

	select {
	case response := <-returned:
		if response.Outcome != permissions.PromptOutcomeShutdown {
			t.Fatalf("shutdown response = %+v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("pending Decision did not resolve during shutdown")
	}
	history := state.DecisionHistory.Get()
	if len(history) != 1 || history[0].Prompt.DecisionID != "shutdown" || history[0].Response.Outcome != permissions.PromptOutcomeShutdown {
		t.Fatalf("shutdown Decision audit = %+v", history)
	}
	if state.DecisionReq.Get() != nil || !strings.Contains(state.DecisionReceipt.Get(), "shutdown") {
		t.Fatalf("shutdown Decision projection not terminal: request=%+v receipt=%q", state.DecisionReq.Get(), state.DecisionReceipt.Get())
	}
	activity, ok := state.GetActivity("decision:shutdown")
	if !ok || activity.State != ActivityCancelled || activity.Outcome != OutcomeShutdown {
		t.Fatalf("shutdown Decision Activity = %+v, ok=%v", activity, ok)
	}
}

func TestDecisionAdmissionAndCommitRejectSessionEpochMismatch(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(3)
	updates := make(chan func(), 2)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { updates <- fn; return true }}
	returned := make(chan permissions.PromptResponse, 1)
	go func() {
		returned <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{DecisionID: "decision", SessionID: "session"})
	}()
	publish := <-updates
	state.SessionEpoch.Set(4)
	publish()
	select {
	case response := <-returned:
		if response.Outcome != permissions.PromptOutcomeCancelled || response.Decision != permissions.DecisionDeny {
			t.Fatalf("mismatched admission response = %+v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("mismatched Decision admission waited without a visible prompt")
	}
	if state.DecisionReq.Get() != nil {
		t.Fatal("mismatched Decision was published")
	}
}

func TestApprovedDecisionIsDeniedWhenTerminalAuditCannotCommit(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(3)
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 3})
	updates := make(chan func(), 2)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { updates <- fn; return true }}
	returned := make(chan permissions.PromptResponse, 1)
	go func() {
		returned <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{DecisionID: "decision", SessionID: "session"})
	}()
	(<-updates)()
	state.DecisionResp <- decisionResponse("decision", permissions.PromptOutcomeApproved, "allow_once")
	resolution := <-updates
	state.SessionEpoch.Set(4)
	resolution()
	response := <-returned
	if response.Outcome != permissions.PromptOutcomeCancelled || response.Decision != permissions.DecisionDeny {
		t.Fatalf("uncommitted approval response = %+v", response)
	}
	if len(state.DecisionHistory.Get()) != 0 {
		t.Fatal("uncommitted approval entered Decision history")
	}
}

func TestDecisionAuditDeepCopiesMutablePromptInput(t *testing.T) {
	state := NewAppState()
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	nested := map[string]any{"path": "/before"}
	typedSlice := []map[string]any{{"command": "before"}}
	typedMap := map[string]string{"mode": "before"}
	pointer := &[]string{"before"}
	request := permissions.PromptRequest{DecisionID: "copy", Input: map[string]any{"nested": nested, "typed": typedSlice, "typed_map": typedMap, "pointer": pointer}, Choices: []string{"allow_once", "reject"}, ReviewDetails: []string{"before"}}
	returned := make(chan permissions.PromptResponse, 1)
	go func() { returned <- renderer.DecisionRequest(context.Background(), request) }()
	waitForDecisionID(t, state, "copy")
	nested["path"] = "/after"
	typedSlice[0]["command"] = "after"
	typedMap["mode"] = "after"
	(*pointer)[0] = "after"
	request.Choices[0] = "mutated"
	request.ReviewDetails[0] = "mutated"
	state.DecisionResp <- decisionResponse("copy", permissions.PromptOutcomeRejected, "reject")
	<-returned
	record := state.DecisionHistory.Get()[0]
	if got := record.Prompt.Input["nested"].(map[string]any)["path"]; got != "/before" || record.Prompt.Input["typed"].([]map[string]any)[0]["command"] != "before" || record.Prompt.Input["typed_map"].(map[string]string)["mode"] != "before" || (*record.Prompt.Input["pointer"].(*[]string))[0] != "before" || record.Prompt.Choices[0] != "allow_once" || record.Prompt.ReviewDetails[0] != "before" {
		t.Fatalf("Decision audit was mutated after approval: %+v", record.Prompt)
	}
	if receipt := state.DecisionReceipt.Get(); !strings.Contains(receipt, "rejected") {
		t.Fatalf("fullscreen decision receipt = %q", receipt)
	}
}

func waitForDecisionID(t *testing.T, state *AppState, ids ...string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if request := state.DecisionReq.Get(); request != nil {
			for _, id := range ids {
				if request.DecisionID == id {
					return
				}
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("decision request did not become one of %v", ids)
}

type structuredDecisionRequester interface {
	DecisionRequest(context.Context, permissions.PromptRequest) permissions.PromptResponse
}

var _ structuredDecisionRequester = (*TuiRenderer)(nil)

func fireDecisionBinding(t *testing.T, root *RootComponent, key gtui.Key, r rune) {
	t.Helper()
	for _, binding := range root.KeyMap() {
		if r != 0 && binding.Pattern.Rune == r {
			binding.Handler(gtui.KeyEvent{Key: gtui.KeyRune, Rune: r})
			return
		}
		if r == 0 && binding.Pattern.Key == key && !binding.Pattern.AnyKey {
			binding.Handler(gtui.KeyEvent{Key: key})
			return
		}
	}
	t.Fatalf("decision binding not found for key=%v rune=%q", key, r)
}

func readDecisionResponse(t *testing.T, state *AppState) permissions.PromptResponse {
	t.Helper()
	select {
	case response := <-state.DecisionResp:
		return response
	default:
		t.Fatal("expected a structured decision response")
		return permissions.PromptResponse{}
	}
}

func TestPlanDecisionDialogPreservesCompletePlanAndPlanChoices(t *testing.T) {
	state := NewAppState()
	root := NewRootComponent(state, nil, nil)
	root.termWidth = 100
	plan := "# Complete plan\n\n1. Inspect\n2. Implement\n3. Verify\n\nEND OF PLAN"
	request := permissions.PromptRequest{
		DecisionID:    "decision:session-plan:toolu-plan",
		SessionID:     "session-plan",
		ToolUseID:     "toolu-plan",
		ActorID:       "assistant",
		ActorType:     "planner",
		WorkUnitID:    "work-plan",
		Kind:          permissions.PromptKindPlan,
		Action:        "Execute the approved plan",
		Target:        "/workspace/PLAN.md",
		Impact:        "Leave plan mode and begin implementation",
		RiskReason:    "Implementation can modify the workspace and run commands",
		RuleSource:    "plan mode gate",
		ApprovalScope: "this plan transition",
		Choices:       []string{"execute", "stay_in_plan"},
		Body:          plan,
	}

	dialog := root.renderDecisionDialog(&request)
	text := collectElementText(dialog)
	for _, want := range []string{
		"assistant", "planner", "Execute the approved plan", "/workspace/PLAN.md",
		"Leave plan mode", "Implementation can modify", "plan mode gate",
		"this plan transition", "Execute", "Stay in Plan", "END OF PLAN",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("plan decision dialog omitted %q:\n%s", want, text)
		}
	}
	if !strings.Contains(text, plan) {
		t.Fatalf("plan decision dialog did not retain the complete plan body:\n%s", text)
	}
}

func TestDecisionResultsDistinguishRejectEscapeCancelTimeoutAndShutdown(t *testing.T) {
	cases := []struct {
		name    string
		outcome permissions.PromptOutcome
	}{
		{name: "approved", outcome: permissions.PromptOutcomeApproved},
		{name: "rejected", outcome: permissions.PromptOutcomeRejected},
		{name: "escaped", outcome: permissions.PromptOutcomeEscaped},
		{name: "cancelled", outcome: permissions.PromptOutcomeCancelled},
		{name: "timed out", outcome: permissions.PromptOutcomeTimedOut},
		{name: "shutdown", outcome: permissions.PromptOutcomeShutdown},
	}
	seen := map[permissions.PromptOutcome]bool{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := decisionResponse("decision-1", tc.outcome, "")
			if response.DecisionID != "decision-1" {
				t.Fatalf("decision response ID = %q, want decision-1", response.DecisionID)
			}
			if response.Outcome != tc.outcome {
				t.Fatalf("decision response outcome = %q, want %q", response.Outcome, tc.outcome)
			}
			if response.Outcome == permissions.PromptOutcomeRejected && tc.name != "rejected" {
				t.Fatalf("%s was collapsed into explicit rejection", tc.name)
			}
			if seen[response.Outcome] {
				t.Fatalf("outcome %q aliases a prior decision result", response.Outcome)
			}
			seen[response.Outcome] = true
		})
	}
}

func TestDecisionActivityRetainsExactTerminalOutcome(t *testing.T) {
	tests := []struct {
		outcome permissions.PromptOutcome
		want    ObservationOutcome
	}{
		{permissions.PromptOutcomeApproved, OutcomeSucceeded},
		{permissions.PromptOutcomeRejected, OutcomeDenied},
		{permissions.PromptOutcomeEscaped, OutcomeEscaped},
		{permissions.PromptOutcomeCancelled, OutcomeCancelled},
		{permissions.PromptOutcomeTimedOut, OutcomeTimedOut},
		{permissions.PromptOutcomeShutdown, OutcomeShutdown},
	}
	for _, tc := range tests {
		t.Run(string(tc.outcome), func(t *testing.T) {
			state := NewAppState()
			state.SessionID.Set("session")
			state.SessionEpoch.Set(1)
			state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
			renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
			returned := make(chan permissions.PromptResponse, 1)
			go func() {
				returned <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{
					DecisionID: "exact", SessionID: "session", TurnID: "turn-3", Choices: []string{"allow_once", "reject"},
				})
			}()
			waitForDecisionID(t, state, "exact")
			state.DecisionResp <- decisionResponse("exact", tc.outcome, "")
			<-returned
			activity, ok := state.GetActivity("decision:exact")
			if !ok || activity.Outcome != tc.want || activity.TurnID != "turn-3" {
				t.Fatalf("decision activity = %+v, ok=%v; want outcome=%s turn=turn-3", activity, ok, tc.want.String())
			}
		})
	}
}

func TestDecisionProjectsRequestingAgentAsNeedsInputThenRestoresRun(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	if err := state.ApplyActivity(ActivityEvent{
		ID: "background:agent-1", RunID: "run-1", Attempt: 1,
		SessionID: "session", Epoch: 1, WorkUnitID: "work-1",
		Actor: ActivityActor{ID: "agent-1", Type: "agent"}, Kind: ActivityAgent,
		Name: "verifier", Lifecycle: ActivityLifecycleRunning, Outcome: OutcomeRunning,
	}); err != nil {
		t.Fatal(err)
	}
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	returned := make(chan permissions.PromptResponse, 1)
	go func() {
		returned <- renderer.DecisionRequest(context.Background(), permissions.PromptRequest{
			DecisionID: "agent-input", SessionID: "session", ActorID: "agent-1", ActorType: "agent", WorkUnitID: "work-1",
			Action: "choose scope", Message: "Which files?", Choices: []string{"allow_once", "reject"},
		})
	}()
	waitForDecisionID(t, state, "agent-input")
	agent, ok := state.GetActivity("background:agent-1")
	if !ok || agent.Lifecycle != ActivityLifecycleBlocked || agent.Attention.Kind != ActivityAttentionNeedsInput || !agent.Attention.Unread {
		t.Fatalf("requesting agent did not enter needs-input: %+v ok=%t", agent, ok)
	}
	state.DecisionResp <- decisionResponse("agent-input", permissions.PromptOutcomeApproved, "allow_once")
	<-returned
	agent, ok = state.GetActivity("background:agent-1")
	if !ok || agent.Lifecycle != ActivityLifecycleRunning || agent.Attention.Kind != ActivityAttentionNone || agent.Attention.Unread {
		t.Fatalf("resolved decision did not restore agent run: %+v ok=%t", agent, ok)
	}
}

func TestAgentRemainsBlockedUntilConcurrentDecisionsResolveInReverseOrder(t *testing.T) {
	state := newDecisionBrokerTestState()
	t.Cleanup(state.SignalStop)
	if err := state.ApplyActivity(ActivityEvent{
		ID: "background:agent-1", RunID: "run-1", Attempt: 1,
		SessionID: "session", Epoch: 1, WorkUnitID: "work-1",
		Actor: ActivityActor{ID: "agent-1", Type: "agent"}, Kind: ActivityAgent,
		Name: "verifier", Lifecycle: ActivityLifecycleRunning, Outcome: OutcomeRunning,
	}); err != nil {
		t.Fatal(err)
	}
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	firstReturned := make(chan permissions.PromptResponse, 1)
	secondReturned := make(chan permissions.PromptResponse, 1)
	request := func(id, message string) permissions.PromptRequest {
		return permissions.PromptRequest{
			DecisionID: id, SessionID: "session", ActorID: "agent-1", ActorType: "agent", WorkUnitID: "work-1",
			Action: "choose scope", Message: message, Choices: []string{"allow_once", "reject"},
		}
	}
	go func() {
		firstReturned <- renderer.DecisionRequest(context.Background(), request("first-input", "First choice"))
	}()
	waitForDecisionID(t, state, "first-input")
	go func() {
		secondReturned <- renderer.DecisionRequest(context.Background(), request("second-input", "Second choice"))
	}()
	waitForDecisionAttentionCount(t, state, 3) // two decisions plus their shared blocked agent

	state.DecisionResp <- decisionResponse("second-input", permissions.PromptOutcomeApproved, "allow_once")
	select {
	case response := <-secondReturned:
		if response.DecisionID != "second-input" {
			t.Fatalf("queued keyed response crossed decisions: %+v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("queued decision did not resolve by ID")
	}
	agent, ok := state.GetActivity("background:agent-1")
	if !ok || agent.Lifecycle != ActivityLifecycleBlocked || agent.Attention.Kind != ActivityAttentionNeedsInput || agent.Attention.DecisionID != "first-input" {
		t.Fatalf("agent restored while another decision remained pending: %+v ok=%t", agent, ok)
	}

	state.DecisionResp <- decisionResponse("first-input", permissions.PromptOutcomeApproved, "allow_once")
	select {
	case <-firstReturned:
	case <-time.After(time.Second):
		t.Fatal("remaining decision did not resolve")
	}
	agent, ok = state.GetActivity("background:agent-1")
	if !ok || agent.Lifecycle != ActivityLifecycleRunning || agent.Attention.Kind != ActivityAttentionNone || agent.Attention.Unread {
		t.Fatalf("agent did not restore after all decisions resolved: %+v ok=%t", agent, ok)
	}
}

func TestDecisionEscapeIsNotExplicitRejection(t *testing.T) {
	request := permissions.PromptRequest{
		DecisionID: "decision-escape",
		Kind:       permissions.PromptKindPermission,
		ToolName:   "Write",
		Choices:    []string{"allow_once", "reject", "always_allow"},
	}
	state := NewAppState()
	state.DecisionReq.Set(&request)
	root := NewRootComponent(state, nil, nil)

	fireDecisionBinding(t, root, gtui.KeyEscape, 0)
	response := readDecisionResponse(t, state)
	if response.DecisionID != request.DecisionID || response.Outcome != permissions.PromptOutcomeEscaped {
		t.Fatalf("escape response = %+v, want decision ID %q and escaped", response, request.DecisionID)
	}
	if response.Decision == permissions.DecisionAllow || response.Decision == permissions.DecisionAllowOnce {
		t.Fatalf("escape unexpectedly approved execution: %+v", response)
	}
}

func TestDecisionRejectShortcutIsExplicitRejection(t *testing.T) {
	request := permissions.PromptRequest{
		DecisionID: "decision-reject",
		Kind:       permissions.PromptKindPermission,
		ToolName:   "Write",
		Choices:    []string{"allow_once", "reject", "always_allow"},
	}
	state := NewAppState()
	state.DecisionReq.Set(&request)
	root := NewRootComponent(state, nil, nil)

	fireDecisionBinding(t, root, 0, 'n')
	response := readDecisionResponse(t, state)
	if response.DecisionID != request.DecisionID || response.Outcome != permissions.PromptOutcomeRejected || response.Choice != "reject" {
		t.Fatalf("reject response = %+v, want explicit reject for %q", response, request.DecisionID)
	}
}
