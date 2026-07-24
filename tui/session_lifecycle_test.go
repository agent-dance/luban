package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
	gtui "github.com/grindlemire/go-tui"
)

func TestEditorCommandPrefersVisualAndPassesTranscriptPath(t *testing.T) {
	t.Setenv("EDITOR", "editor --wait")
	t.Setenv("VISUAL", "visual --reuse-window")
	command, args, err := editorCommand("/tmp/transcript.txt")
	if err != nil {
		t.Fatal(err)
	}
	if command != "visual" || strings.Join(args, "|") != "--reuse-window|/tmp/transcript.txt" {
		t.Fatalf("editor command = %q %+v", command, args)
	}
	os.Unsetenv("VISUAL")
	command, args, err = editorCommand("/tmp/detail.txt")
	if err != nil || command != "editor" || strings.Join(args, "|") != "--wait|/tmp/detail.txt" {
		t.Fatalf("EDITOR fallback = %q %+v, err %v", command, args, err)
	}
}

func TestEditorCommandPreservesQuotedExecutableAndArguments(t *testing.T) {
	t.Setenv("VISUAL", `"/Applications/Visual Studio Code.app/Contents/MacOS/Electron" --reuse-window --profile "Screen Reader"`)
	command, args, err := editorCommand("/tmp/transcript with spaces.txt")
	if err != nil {
		t.Fatal(err)
	}
	if command != "/Applications/Visual Studio Code.app/Contents/MacOS/Electron" {
		t.Fatalf("editor executable = %q", command)
	}
	want := []string{"--reuse-window", "--profile", "Screen Reader", "/tmp/transcript with spaces.txt"}
	if strings.Join(args, "|") != strings.Join(want, "|") {
		t.Fatalf("editor args = %#v, want %#v", args, want)
	}
}

func TestEditorCommandRejectsUnclosedQuote(t *testing.T) {
	t.Setenv("VISUAL", `editor --profile "unfinished`)
	if _, _, err := editorCommand("/tmp/transcript.txt"); err == nil || !strings.Contains(err.Error(), "quote") {
		t.Fatalf("editorCommand error = %v, want quote error", err)
	}
}

func TestEditorCommandPreservesWindowsPathBackslashes(t *testing.T) {
	t.Setenv("VISUAL", `"C:\Program Files\Accessible Editor\editor.exe" --wait`)
	command, args, err := editorCommand(`C:\Users\Reader\transcript with spaces.txt`)
	if err != nil {
		t.Fatal(err)
	}
	if command != `C:\Program Files\Accessible Editor\editor.exe` {
		t.Fatalf("editor executable = %q", command)
	}
	want := []string{"--wait", `C:\Users\Reader\transcript with spaces.txt`}
	if strings.Join(args, "|") != strings.Join(want, "|") {
		t.Fatalf("editor args = %#v, want %#v", args, want)
	}
}

func TestBoundRootStateMutationsDoNotReenterAppStateMutex(t *testing.T) {
	state := NewAppState()
	root := NewRootComponent(state, nil, nil)
	app := &gtui.App{}
	root.BindApp(app)
	stop := make(chan struct{})
	for _, watcher := range root.Watchers() {
		watcher.Start(make(chan func(), 8), stop)
	}
	defer close(stop)
	projection := mustSessionProjection(t, SessionIdentity{SessionID: "next", Epoch: 2}, []types.Message{types.UserMessage("next")})

	done := make(chan error, 1)
	go func() {
		state.AppendMessage(Message{Kind: MsgAssistant, Text: "bound"})
		done <- state.ApplySessionSnapshot(SessionSnapshot{Identity: SessionIdentity{SessionID: "next", Epoch: 2}, Projection: projection})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("bound Root mutation deadlocked through synchronous state bindings")
	}
}

func TestApplySessionSnapshotCommitsUsageInteractionAndPermissionMode(t *testing.T) {
	state := NewAppState()
	oldProjection := mustSessionProjection(t,
		SessionIdentity{Namespace: "project-a", SessionID: "old", Epoch: 4},
		[]types.Message{types.UserMessage("old transcript")},
	)
	if err := state.ApplySessionSnapshot(SessionSnapshot{
		Identity:   SessionIdentity{Namespace: "project-a", SessionID: "old", Epoch: 4},
		Projection: oldProjection,
		DurableSessionView: DurableSessionView{
			Usage: SessionUsage{Known: true, InputTokens: 9}, Interaction: SessionInteraction{InputDraft: "old draft", ScrollAnchorID: "old-anchor", ScrollOffset: 2}, PermissionMode: ModeAutoEdit,
		},
	}); err != nil {
		t.Fatalf("apply old snapshot: %v", err)
	}

	targetIdentity := SessionIdentity{Namespace: "project-b", SessionID: "target", Epoch: 5}
	targetProjection := mustSessionProjection(t, targetIdentity, []types.Message{
		types.UserMessage("target transcript"),
		types.AssistantMessage("target answer"),
	})
	targetUsage := SessionUsage{
		Known:                true,
		InputTokens:          1200,
		OutputTokens:         340,
		CacheReadTokens:      800,
		CacheCreateTokens:    50,
		HasCompacted:         true,
		InputTokensAtCompact: 900,
		CacheReadAtCompact:   600,
		WebSearchRequests:    3,
		CumulativeCost:       0.42,
		UsedTokens:           1700,
		MaxTokens:            200000,
	}
	targetInteraction := SessionInteraction{
		FocusedObservationID: "target-focus",
		ScrollAnchorID:       "target-anchor",
		ScrollOffset:         7,
		InputDraft:           "target draft",
	}

	if err := state.ApplySessionSnapshot(SessionSnapshot{
		Identity:   targetIdentity,
		Projection: targetProjection,
		DurableSessionView: DurableSessionView{
			Usage: targetUsage, Interaction: targetInteraction, PermissionMode: ModePlanEdit,
		},
	}); err != nil {
		t.Fatalf("apply target snapshot: %v", err)
	}

	if state.SessionID.Get() != "target" || state.SessionEpoch.Get() != 5 {
		t.Fatalf("active identity = %q/%d, want target/5", state.SessionID.Get(), state.SessionEpoch.Get())
	}
	if got := state.ActiveSessionUsage(); got != targetUsage {
		t.Fatalf("active usage = %+v, want %+v", got, targetUsage)
	}
	if got := state.ActiveSessionInteraction(); got != targetInteraction {
		t.Fatalf("active interaction = %+v, want %+v", got, targetInteraction)
	}
	if got := state.Mode.Get(); got != ModePlanEdit {
		t.Fatalf("active permission mode = %v, want %v", got, ModePlanEdit)
	}
	if got := state.Messages.Get(); len(got) != 2 || got[0].Text != "target transcript" || got[1].Text != "target answer" {
		t.Fatalf("active transcript = %+v, want target transcript and answer", got)
	}
}

func TestApplyInvalidSessionSnapshotLeavesEveryPublishedSurfaceUnchanged(t *testing.T) {
	state := NewAppState()
	identity := SessionIdentity{Namespace: "project", SessionID: "stable", Epoch: 8}
	projection := mustSessionProjection(t, identity, []types.Message{types.UserMessage("stable transcript")})
	wantUsage := SessionUsage{Known: true, InputTokens: 41, UsedTokens: 23, MaxTokens: 100}
	wantInteraction := SessionInteraction{
		FocusedObservationID: "focus-stable",
		ScrollAnchorID:       "anchor-stable",
		ScrollOffset:         4,
		InputDraft:           "stable draft",
	}
	if err := state.ApplySessionSnapshot(SessionSnapshot{
		Identity:   identity,
		Projection: projection,
		DurableSessionView: DurableSessionView{
			Usage: wantUsage, Interaction: wantInteraction, PermissionMode: ModeAutoEdit,
		},
	}); err != nil {
		t.Fatalf("apply stable snapshot: %v", err)
	}

	badProjection := mustSessionProjection(t,
		SessionIdentity{Namespace: "project", SessionID: "bad", Epoch: 9},
		[]types.Message{types.UserMessage("must never publish")},
	)
	err := state.ApplySessionSnapshot(SessionSnapshot{
		Identity:   SessionIdentity{}, // validation failure before publication
		Projection: badProjection,
		DurableSessionView: DurableSessionView{
			Usage: SessionUsage{Known: true, InputTokens: 999}, Interaction: SessionInteraction{InputDraft: "bad draft", ScrollOffset: 99}, PermissionMode: ModePlanEdit,
		},
	})
	if err == nil {
		t.Fatal("invalid snapshot unexpectedly published")
	}

	if state.SessionID.Get() != "stable" || state.SessionEpoch.Get() != 8 {
		t.Fatalf("failed apply changed identity to %q/%d", state.SessionID.Get(), state.SessionEpoch.Get())
	}
	if got := state.ActiveSessionUsage(); got != wantUsage {
		t.Fatalf("failed apply changed usage to %+v", got)
	}
	if got := state.ActiveSessionInteraction(); got != wantInteraction {
		t.Fatalf("failed apply changed interaction to %+v", got)
	}
	if got := state.Mode.Get(); got != ModeAutoEdit {
		t.Fatalf("failed apply changed mode to %v", got)
	}
	if got := state.Messages.Get(); len(got) != 1 || got[0].Text != "stable transcript" {
		t.Fatalf("failed apply changed transcript: %+v", got)
	}
}

func TestClearViewPreservesSessionResourcesModeAndDraft(t *testing.T) {
	state := NewAppState()
	identity := SessionIdentity{Namespace: "project", SessionID: "active", Epoch: 3}
	projection := mustSessionProjection(t, identity, []types.Message{types.UserMessage("visible")})
	wantUsage := SessionUsage{
		Known:           true,
		InputTokens:     900,
		OutputTokens:    100,
		CacheReadTokens: 450,
		CumulativeCost:  0.09,
		UsedTokens:      730,
		MaxTokens:       1000,
	}
	if err := state.ApplySessionSnapshot(SessionSnapshot{
		Identity:   identity,
		Projection: projection,
		DurableSessionView: DurableSessionView{
			Usage: wantUsage,
			Interaction: SessionInteraction{
				FocusedObservationID: "visible", ScrollAnchorID: "visible", ScrollOffset: 6, InputDraft: "unsent draft",
			},
			PermissionMode: ModePlanEdit,
		},
	}); err != nil {
		t.Fatalf("apply active snapshot: %v", err)
	}

	state.ClearView()

	if got := state.Messages.Get(); len(got) != 0 {
		t.Fatalf("clear view retained %d visible messages", len(got))
	}
	if got := state.ActiveSessionUsage(); got != wantUsage {
		t.Fatalf("clear view changed usage/context: got %+v want %+v", got, wantUsage)
	}
	if got := state.Mode.Get(); got != ModePlanEdit {
		t.Fatalf("clear view changed permission mode to %v", got)
	}
	interaction := state.ActiveSessionInteraction()
	if interaction.InputDraft != "unsent draft" {
		t.Fatalf("clear view discarded input draft %q", interaction.InputDraft)
	}
	if interaction.FocusedObservationID != "" || interaction.ScrollAnchorID != "" || interaction.ScrollOffset != 0 {
		t.Fatalf("clear view retained stale visible focus/scroll: %+v", interaction)
	}
}

func TestObservationDisclosureCloseRestoresFocusScrollAndDraft(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.SetInteractionDraft("unsent draft")
	state.SetInteractionScroll(9)
	state.mu.Lock()
	state.activeInteraction.FocusedObservationID = "previous-focus"
	state.activeInteraction.ScrollAnchorID = "previous-anchor"
	state.mu.Unlock()
	ctx := ToolEventContext{SessionID: "session", TurnID: "turn", Outcome: OutcomeSucceeded}
	if err := state.ApplyToolCall(ctx, types.ToolUseBlock{ID: "tool", Name: "Read"}); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyToolResult(ctx, types.ToolResultBlock{ToolUseID: "tool", Content: "complete evidence"}); err != nil {
		t.Fatal(err)
	}
	id := toolObservationID("session", "tool")
	if err := state.RevealObservation(id, DisclosureEvidence); err != nil {
		t.Fatal(err)
	}
	opened := state.ActiveSessionInteraction()
	if opened.FocusedObservationID != id || opened.ScrollAnchorID != id || opened.InputDraft != "unsent draft" {
		t.Fatalf("opened disclosure interaction = %+v", opened)
	}
	if err := state.RevealObservation(id, DisclosureSummary); err != nil {
		t.Fatal(err)
	}
	want := (SessionInteraction{
		FocusedObservationID: "previous-focus", ScrollAnchorID: "previous-anchor", ScrollOffset: 9,
		InputDraft: "unsent draft", InputCursor: 12, InputCursorSet: true,
	})
	if got := state.ActiveSessionInteraction(); got != want {
		t.Fatalf("closed disclosure interaction = %+v, want %+v", got, want)
	}
}

func TestObservationDisclosureClosePreservesDraftEditedWhileExpanded(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.SetInteractionDraft("before")
	state.SetInteractionScroll(7)
	state.mu.Lock()
	state.activeInteraction.FocusedObservationID = "previous-focus"
	state.activeInteraction.ScrollAnchorID = "previous-anchor"
	state.mu.Unlock()
	ctx := ToolEventContext{SessionID: "session", TurnID: "turn", Outcome: OutcomeSucceeded}
	if err := state.ApplyToolCall(ctx, types.ToolUseBlock{ID: "tool-draft", Name: "Read"}); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyToolResult(ctx, types.ToolResultBlock{ToolUseID: "tool-draft", Content: "evidence"}); err != nil {
		t.Fatal(err)
	}
	id := toolObservationID("session", "tool-draft")
	if err := state.RevealObservation(id, DisclosureEvidence); err != nil {
		t.Fatal(err)
	}
	state.SetInteractionDraft("edited while expanded")
	if err := state.RevealObservation(id, DisclosureSummary); err != nil {
		t.Fatal(err)
	}
	interaction := state.ActiveSessionInteraction()
	if interaction.InputDraft != "edited while expanded" || interaction.FocusedObservationID != "previous-focus" || interaction.ScrollAnchorID != "previous-anchor" || interaction.ScrollOffset != 7 {
		t.Fatalf("close did not preserve edited draft and restore navigation: %+v", interaction)
	}
}

func TestClearConversationSnapshotStartsKnownEmptySession(t *testing.T) {
	state := NewAppState()
	oldIdentity := SessionIdentity{Namespace: "project", SessionID: "old", Epoch: 10}
	if err := state.ApplySessionSnapshot(SessionSnapshot{
		Identity:   oldIdentity,
		Projection: mustSessionProjection(t, oldIdentity, []types.Message{types.UserMessage("old audit")}),
		DurableSessionView: DurableSessionView{
			Usage: SessionUsage{Known: true, InputTokens: 500, UsedTokens: 400, MaxTokens: 1000},
			Interaction: SessionInteraction{
				FocusedObservationID: "old-focus", ScrollAnchorID: "old-anchor", ScrollOffset: 5, InputDraft: "old draft",
			},
			PermissionMode: ModePlanEdit,
		},
	}); err != nil {
		t.Fatalf("apply old snapshot: %v", err)
	}

	newIdentity := SessionIdentity{Namespace: "project", SessionID: "new", Epoch: 11}
	if err := state.ApplySessionSnapshot(SessionSnapshot{
		Identity:   newIdentity,
		Projection: mustSessionProjection(t, newIdentity, nil),
		DurableSessionView: DurableSessionView{
			Usage: SessionUsage{Known: true}, Interaction: SessionInteraction{}, PermissionMode: ModeAutoEdit,
		},
	}); err != nil {
		t.Fatalf("apply clear-conversation snapshot: %v", err)
	}

	if got := state.Messages.Get(); len(got) != 0 {
		t.Fatalf("new conversation has %d visible messages, want empty", len(got))
	}
	if got := state.ActiveSessionUsage(); got != (SessionUsage{Known: true}) {
		t.Fatalf("new conversation usage = %+v, want known zero", got)
	}
	if got := state.ActiveSessionInteraction(); got != (SessionInteraction{}) {
		t.Fatalf("new conversation inherited old focus/scroll/draft: %+v", got)
	}
	if got := state.Mode.Get(); got != ModeAutoEdit {
		t.Fatalf("new conversation permission mode = %v, want auto", got)
	}
}

func TestApplySessionSnapshotClampsEmptyActivityViewport(t *testing.T) {
	state := NewAppState()
	identity := SessionIdentity{Namespace: "project", SessionID: "empty-activity", Epoch: 12}
	if err := state.ApplySessionSnapshot(SessionSnapshot{
		Identity:           identity,
		Projection:         mustSessionProjection(t, identity, nil),
		DurableSessionView: DurableSessionView{ActivityViewOffset: 99},
	}); err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
	if got := state.ActivityViewOffset.Get(); got != 0 {
		t.Fatalf("empty activity viewport offset = %d, want 0", got)
	}
}

func TestUnknownRestoredUsageIsHiddenNotRenderedAsZero(t *testing.T) {
	state := NewAppState()
	identity := SessionIdentity{Namespace: "legacy", SessionID: "without-usage-meta", Epoch: 1}
	if err := state.ApplySessionSnapshot(SessionSnapshot{
		Identity:   identity,
		Projection: mustSessionProjection(t, identity, []types.Message{types.UserMessage("legacy")}),
		DurableSessionView: DurableSessionView{
			Usage: SessionUsage{Known: false}, Interaction: SessionInteraction{}, PermissionMode: ModeAskEdit,
		},
	}); err != nil {
		t.Fatalf("apply legacy snapshot: %v", err)
	}

	root := NewRootComponent(state, nil, nil)
	text := strings.ToLower(collectElementText(root.renderStatusBar(120)))
	if strings.Contains(text, "usage unknown") {
		t.Fatalf("legacy session exposed unknown usage in the status bar: %q", text)
	}
	if strings.Contains(text, "session: in 0") || strings.Contains(text, "0% cached") {
		t.Fatalf("legacy session with unknown usage was rendered as measured zero: %q", text)
	}
}

func mustSessionProjection(t *testing.T, identity SessionIdentity, messages []types.Message) SessionProjection {
	t.Helper()
	projection, err := ProjectPersistedMessages(identity, messages, nil)
	if err != nil {
		t.Fatalf("project session %q: %v", identity.SessionID, err)
	}
	return projection
}
