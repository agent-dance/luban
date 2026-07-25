package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/types"
	gtui "github.com/grindlemire/go-tui"
)

func checkpointTranscriptFixture() []types.Message {
	return []types.Message{
		types.UserMessage("Inspect the sources."),
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "I will inspect both files."},
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "read-a", Name: "Read", Input: map[string]any{"file_path": "/workspace/a.go"}},
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "read-b", Name: "Read", Input: map[string]any{"file_path": "/workspace/b.go"}},
		}},
		types.ToolResultMessage(types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "read-a", Content: "package a", Outcome: types.ToolOutcomeSucceeded}),
		types.ToolResultMessage(types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "read-b", Content: "package b", Outcome: types.ToolOutcomeSucceeded}),
		types.AssistantMessage("Both files use the same package."),
	}
}

func TestSessionViewTranscriptDigestTreatsNilAndEmptyAsEquivalent(t *testing.T) {
	nilDigest, err := sessionViewTranscriptDigest(nil)
	if err != nil {
		t.Fatal(err)
	}
	emptyDigest, err := sessionViewTranscriptDigest([]types.Message{})
	if err != nil {
		t.Fatal(err)
	}
	if nilDigest != emptyDigest {
		t.Fatalf("nil digest %q differs from empty transcript digest %q", nilDigest, emptyDigest)
	}
}

func TestSessionViewCheckpointReprojectsAcrossControlTrustDomainsAndGenerations(t *testing.T) {
	const controlText = "checkpoint control payload"
	lang := i18n.DetectOrLoadLanguage()
	base := types.UserMessage(controlText)
	base.IsMeta = true
	base.InternalKind = types.InternalMessageKindCompactReminder
	scope := func(generation uint64) messagecontrol.Scope {
		return messagecontrol.NewScope("scope-session", "/workspace", generation)
	}
	seal := func(generation uint64) types.Message {
		return base.WithInternalControlProvenance(messagecontrol.Runtime(), scope(generation))
	}
	currentIdentity := func(generation uint64) SessionIdentity {
		return (SessionIdentity{Namespace: "/workspace", SessionID: "scope-session", Epoch: 1}).
			WithInternalControlScope(messagecontrol.Runtime(), scope(generation))
	}

	cases := []struct {
		name           string
		save           types.Message
		saveGeneration uint64
		load           types.Message
		loadGeneration uint64
		wantVisible    bool
	}{
		{name: "trusted_to_stale", save: seal(7), saveGeneration: 7, load: seal(7), loadGeneration: 8, wantVisible: true},
		{name: "trusted_to_untrusted", save: seal(7), saveGeneration: 7, load: base, loadGeneration: 8, wantVisible: true},
		{name: "untrusted_to_trusted", save: base, saveGeneration: 7, load: seal(8), loadGeneration: 8, wantVisible: false},
		{name: "stale_to_trusted", save: seal(6), saveGeneration: 7, load: seal(8), loadGeneration: 8, wantVisible: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			saveTranscript := []types.Message{test.save}
			loadTranscript := []types.Message{test.load}
			saveJSON, err := json.Marshal(saveTranscript)
			if err != nil {
				t.Fatal(err)
			}
			loadJSON, err := json.Marshal(loadTranscript)
			if err != nil {
				t.Fatal(err)
			}
			if string(saveJSON) != string(loadJSON) {
				t.Fatalf("fixture JSON differs across trust transition:\n save=%s\n load=%s", saveJSON, loadJSON)
			}

			saveIdentity := currentIdentity(test.saveGeneration)
			saveProjection, err := ProjectPersistedMessagesInLanguage(lang, saveIdentity, saveTranscript, nil)
			if err != nil {
				t.Fatal(err)
			}
			state := NewAppState()
			state.Language.Set(lang)
			if err := state.ApplySessionSnapshot(SessionSnapshot{
				Identity: saveIdentity, Projection: saveProjection,
				ContextGeneration: test.saveGeneration, ContextGenerationPersisted: true,
			}); err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			if err := SaveSessionViewCheckpoint(root, state, saveTranscript); err != nil {
				t.Fatal(err)
			}

			loadIdentity := currentIdentity(test.loadGeneration)
			saveProjectionDigest, err := sessionViewProjectionDigest(lang, saveIdentity, saveTranscript)
			if err != nil {
				t.Fatal(err)
			}
			loadProjectionDigest, err := sessionViewProjectionDigest(lang, loadIdentity, loadTranscript)
			if err != nil {
				t.Fatal(err)
			}
			if saveProjectionDigest == loadProjectionDigest {
				t.Fatal("projection identity aliased different trust domains or generations")
			}

			loaded, restored, err := LoadSessionViewCheckpoint(root, loadTranscript, loadIdentity)
			if err != nil || !restored {
				t.Fatalf("load = restored %v err %v", restored, err)
			}
			visible := len(loaded.Projection.Messages) == 1 && loaded.Projection.Messages[0].Text == controlText
			if visible != test.wantVisible {
				t.Fatalf("visible=%v messages=%#v, want visible=%v", visible, loaded.Projection.Messages, test.wantVisible)
			}
		})
	}
}

func TestSessionViewCheckpointProjectionIdentityDoesNotSerializeControlAuthenticator(t *testing.T) {
	base := types.UserMessage("opaque checkpoint control")
	base.IsMeta = true
	base.InternalKind = types.InternalMessageKindCompactReminder
	scope := messagecontrol.NewScope("no-secret-session", "/workspace", 4)
	trusted := base.WithInternalControlProvenance(messagecontrol.Runtime(), scope)
	trustedJSON, err := json.Marshal(trusted)
	if err != nil {
		t.Fatal(err)
	}
	untrustedJSON, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	if string(trustedJSON) != string(untrustedJSON) {
		t.Fatalf("serialized message exposed provenance:\n trusted=%s\nuntrusted=%s", trustedJSON, untrustedJSON)
	}

	identity := (SessionIdentity{Namespace: "/workspace", SessionID: "no-secret-session", Epoch: 1}).
		WithInternalControlScope(messagecontrol.Runtime(), scope)
	projection, err := ProjectPersistedMessagesInLanguage(i18n.LangEN, identity, []types.Message{trusted}, nil)
	if err != nil {
		t.Fatal(err)
	}
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	if err := state.ApplySessionSnapshot(SessionSnapshot{
		Identity: identity, Projection: projection, ContextGeneration: 4, ContextGenerationPersisted: true,
	}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := SaveSessionViewCheckpoint(root, state, []types.Message{trusted}); err != nil {
		t.Fatal(err)
	}
	digest, err := sessionViewTranscriptDigest([]types.Message{trusted})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := os.ReadFile(sessionViewCheckpointPath(root, 1, digest))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"internalControlDigest", "internalControlScope", "authentication_prefix", "message-control-scope"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("checkpoint serialized private provenance field %q: %s", forbidden, encoded)
		}
	}
	if !strings.Contains(string(encoded), "projection_sha256") {
		t.Fatalf("checkpoint omitted opaque projection identity: %s", encoded)
	}
}

func TestForkSessionViewCheckpointReprojectsChangedSourceTrustDomainOrScope(t *testing.T) {
	const controlText = "fork checkpoint control"
	base := types.UserMessage(controlText)
	base.IsMeta = true
	base.InternalKind = types.InternalMessageKindCompactReminder
	scope := func(generation uint64) messagecontrol.Scope {
		return messagecontrol.NewScope("fork-scope-source", "/workspace", generation)
	}
	seal := func(generation uint64) types.Message {
		return base.WithInternalControlProvenance(messagecontrol.Runtime(), scope(generation))
	}
	identity := func(generation uint64) SessionIdentity {
		return (SessionIdentity{Namespace: "/workspace", SessionID: "fork-scope-source", Epoch: 1}).
			WithInternalControlScope(messagecontrol.Runtime(), scope(generation))
	}
	cases := []struct {
		name           string
		save           types.Message
		saveGeneration uint64
		fork           types.Message
		forkGeneration uint64
	}{
		{name: "trusted_to_untrusted", save: seal(7), saveGeneration: 7, fork: base, forkGeneration: 7},
		{name: "trusted_to_stale", save: seal(7), saveGeneration: 7, fork: seal(7), forkGeneration: 8},
		{name: "untrusted_to_trusted", save: base, saveGeneration: 7, fork: seal(7), forkGeneration: 7},
		{name: "stale_to_trusted", save: seal(6), saveGeneration: 7, fork: seal(7), forkGeneration: 7},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			saveTranscript := []types.Message{test.save, types.UserMessage("visible source")}
			forkTranscript := []types.Message{test.fork, types.UserMessage("visible source")}
			saveJSON, err := json.Marshal(saveTranscript)
			if err != nil {
				t.Fatal(err)
			}
			forkJSON, err := json.Marshal(forkTranscript)
			if err != nil {
				t.Fatal(err)
			}
			if string(saveJSON) != string(forkJSON) {
				t.Fatalf("fork trust fixture changed JSON:\n save=%s\n fork=%s", saveJSON, forkJSON)
			}

			saveIdentity := identity(test.saveGeneration)
			projection, err := ProjectPersistedMessagesInLanguage(i18n.LangEN, saveIdentity, saveTranscript, nil)
			if err != nil {
				t.Fatal(err)
			}
			state := NewAppState()
			state.Language.Set(i18n.LangEN)
			if err := state.ApplySessionSnapshot(SessionSnapshot{
				Identity: saveIdentity, Projection: projection,
				ContextGeneration: test.saveGeneration, ContextGenerationPersisted: true,
			}); err != nil {
				t.Fatal(err)
			}
			state.Messages.Set(append([]Message{{Kind: MsgInfo, Text: "STALE FORK PROJECTION"}}, state.Messages.Get()...))
			sourceRoot, targetRoot := t.TempDir(), t.TempDir()
			if err := SaveSessionViewCheckpoint(sourceRoot, state, saveTranscript); err != nil {
				t.Fatal(err)
			}
			targetScope := messagecontrol.NewScope("fork-scope-target", "/workspace", 1)
			targetIdentity := (SessionIdentity{Namespace: "/workspace", SessionID: "fork-scope-target", Epoch: 1}).
				WithInternalControlScope(messagecontrol.Runtime(), targetScope)
			targetControl := base
			if test.fork.IsInternalRuntimeMessageForScope(scope(test.forkGeneration)) {
				targetControl = base.WithInternalControlProvenance(messagecontrol.Runtime(), targetScope)
			}
			targetTranscript := []types.Message{targetControl, types.UserMessage("visible source")}
			err = ForkSessionViewCheckpoint(
				sourceRoot, targetRoot, identity(test.forkGeneration), targetIdentity,
				forkTranscript, targetTranscript,
			)
			if err != nil {
				t.Fatal(err)
			}
			loaded, restored, err := loadSessionViewCheckpointInLanguage(i18n.LangEN, targetRoot, targetTranscript, targetIdentity)
			if err != nil || !restored {
				t.Fatalf("load reprojected fork = restored %v err %v", restored, err)
			}
			want, err := ProjectPersistedMessagesInLanguage(i18n.LangEN, targetIdentity, targetTranscript, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !checkpointMessagesSemanticallyEqual(loaded.Projection.Messages, want.Messages) {
				t.Fatalf("fork reused stale source projection:\n got=%#v\nwant=%#v", loaded.Projection.Messages, want.Messages)
			}
			for _, message := range loaded.Projection.Messages {
				if message.Text == "STALE FORK PROJECTION" {
					t.Fatal("fork preserved an unprovable local row from the stale projection")
				}
			}
		})
	}
}

func TestForkSessionViewCheckpointExactScopesPreserveLocalView(t *testing.T) {
	sourceScope := messagecontrol.NewScope("fork-current-source", "/workspace", 9)
	targetScope := messagecontrol.NewScope("fork-current-target", "/workspace", 1)
	sourceIdentity := (SessionIdentity{Namespace: "/workspace", SessionID: "fork-current-source", Epoch: 1}).
		WithInternalControlScope(messagecontrol.Runtime(), sourceScope)
	targetIdentity := (SessionIdentity{Namespace: "/workspace", SessionID: "fork-current-target", Epoch: 1}).
		WithInternalControlScope(messagecontrol.Runtime(), targetScope)
	control := types.UserMessage("trusted fork-only control")
	control.IsMeta = true
	control.InternalKind = types.InternalMessageKindCompactReminder
	sourceTranscript := []types.Message{
		control.WithInternalControlProvenance(messagecontrol.Runtime(), sourceScope),
		types.UserMessage("visible fork prompt"), types.AssistantMessage("visible fork answer"),
	}
	targetTranscript := []types.Message{
		control.WithInternalControlProvenance(messagecontrol.Runtime(), targetScope),
		types.UserMessage("visible fork prompt"), types.AssistantMessage("visible fork answer"),
	}
	projection, err := ProjectPersistedMessagesInLanguage(i18n.LangEN, sourceIdentity, sourceTranscript, nil)
	if err != nil {
		t.Fatal(err)
	}
	source := NewAppState()
	source.Language.Set(i18n.LangEN)
	if err := source.ApplySessionSnapshot(SessionSnapshot{
		Identity: sourceIdentity, Projection: projection,
		ContextGeneration: 9, ContextGenerationPersisted: true,
	}); err != nil {
		t.Fatal(err)
	}
	source.Messages.Set(append([]Message{{Kind: MsgInfo, Text: "SCOPED FORK LOCAL RECEIPT"}}, source.Messages.Get()...))
	source.SetInteractionEditor("scoped fork draft", 6)

	sourceRoot, targetRoot := t.TempDir(), t.TempDir()
	if err := SaveSessionViewCheckpoint(sourceRoot, source, sourceTranscript); err != nil {
		t.Fatal(err)
	}
	if err := ForkSessionViewCheckpoint(
		sourceRoot, targetRoot, sourceIdentity, targetIdentity, sourceTranscript, targetTranscript,
	); err != nil {
		t.Fatal(err)
	}
	loaded, restored, err := loadSessionViewCheckpointInLanguage(i18n.LangEN, targetRoot, targetTranscript, targetIdentity)
	if err != nil || !restored {
		t.Fatalf("load scoped fork = restored %v err %v", restored, err)
	}
	if len(loaded.Projection.Messages) != 3 || loaded.Projection.Messages[0].Text != "SCOPED FORK LOCAL RECEIPT" {
		t.Fatalf("scoped fork lost local view or exposed trusted control: %#v", loaded.Projection.Messages)
	}
	if loaded.Interaction.InputDraft != "scoped fork draft" || loaded.Interaction.InputCursor != 6 {
		t.Fatalf("scoped fork lost interaction state: %+v", loaded.Interaction)
	}
}

func TestForkSessionViewCheckpointUsesTargetTrustSemantics(t *testing.T) {
	const controlText = "target trust control"
	base := types.UserMessage(controlText)
	base.IsMeta = true
	base.InternalKind = types.InternalMessageKindCompactReminder
	sourceScope := messagecontrol.NewScope("target-trust-source", "/workspace", 4)
	targetScope := messagecontrol.NewScope("target-trust-target", "/workspace", 1)
	sourceIdentity := (SessionIdentity{Namespace: "/workspace", SessionID: "target-trust-source", Epoch: 1}).
		WithInternalControlScope(messagecontrol.Runtime(), sourceScope)
	targetIdentity := (SessionIdentity{Namespace: "/workspace", SessionID: "target-trust-target", Epoch: 1}).
		WithInternalControlScope(messagecontrol.Runtime(), targetScope)
	cases := []struct {
		name   string
		source types.Message
		target types.Message
	}{
		{
			name:   "trusted_to_untrusted",
			source: base.WithInternalControlProvenance(messagecontrol.Runtime(), sourceScope), target: base,
		},
		{
			name:   "trusted_to_stale",
			source: base.WithInternalControlProvenance(messagecontrol.Runtime(), sourceScope),
			target: base.WithInternalControlProvenance(messagecontrol.Runtime(), sourceScope),
		},
		{
			name: "untrusted_to_trusted", source: base,
			target: base.WithInternalControlProvenance(messagecontrol.Runtime(), targetScope),
		},
		{
			name:   "stale_to_trusted",
			source: base.WithInternalControlProvenance(messagecontrol.Runtime(), messagecontrol.NewScope("target-trust-source", "/workspace", 3)),
			target: base.WithInternalControlProvenance(messagecontrol.Runtime(), targetScope),
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			sourceTranscript := []types.Message{test.source, types.UserMessage("target trust visible")}
			targetTranscript := []types.Message{test.target, types.UserMessage("target trust visible")}
			sourceProjection, err := ProjectPersistedMessagesInLanguage(i18n.LangEN, sourceIdentity, sourceTranscript, nil)
			if err != nil {
				t.Fatal(err)
			}
			state := NewAppState()
			state.Language.Set(i18n.LangEN)
			if err := state.ApplySessionSnapshot(SessionSnapshot{
				Identity: sourceIdentity, Projection: sourceProjection,
				ContextGeneration: 4, ContextGenerationPersisted: true,
			}); err != nil {
				t.Fatal(err)
			}
			state.Messages.Set(append([]Message{{Kind: MsgInfo, Text: "SOURCE-ONLY LOCAL PROJECTION"}}, state.Messages.Get()...))
			sourceRoot, targetRoot := t.TempDir(), t.TempDir()
			if err := SaveSessionViewCheckpoint(sourceRoot, state, sourceTranscript); err != nil {
				t.Fatal(err)
			}
			if err := ForkSessionViewCheckpoint(
				sourceRoot, targetRoot, sourceIdentity, targetIdentity, sourceTranscript, targetTranscript,
			); err != nil {
				t.Fatal(err)
			}
			loaded, restored, err := loadSessionViewCheckpointInLanguage(i18n.LangEN, targetRoot, targetTranscript, targetIdentity)
			if err != nil || !restored {
				t.Fatalf("load target-trust fork = restored %v err %v", restored, err)
			}
			want, err := ProjectPersistedMessagesInLanguage(i18n.LangEN, targetIdentity, targetTranscript, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !checkpointMessagesSemanticallyEqual(loaded.Projection.Messages, want.Messages) {
				t.Fatalf("target fork inherited source trust semantics:\n got=%#v\nwant=%#v", loaded.Projection.Messages, want.Messages)
			}
			if viewContainsText(loaded.Projection.Messages, "SOURCE-ONLY LOCAL PROJECTION") {
				t.Fatal("target trust transition retained an unprovable source-only row")
			}
		})
	}
}

func TestForkSessionViewCheckpointReprojectsNonForkTargetTranscript(t *testing.T) {
	sourceScope := messagecontrol.NewScope("nonfork-source", "/workspace", 2)
	targetScope := messagecontrol.NewScope("nonfork-target", "/workspace", 1)
	sourceIdentity := (SessionIdentity{Namespace: "/workspace", SessionID: "nonfork-source", Epoch: 1}).
		WithInternalControlScope(messagecontrol.Runtime(), sourceScope)
	targetIdentity := (SessionIdentity{Namespace: "/workspace", SessionID: "nonfork-target", Epoch: 1}).
		WithInternalControlScope(messagecontrol.Runtime(), targetScope)
	sourceTranscript := []types.Message{types.UserMessage("source-only transcript")}
	targetTranscript := []types.Message{types.UserMessage("different target transcript")}
	projection, err := ProjectPersistedMessagesInLanguage(i18n.LangEN, sourceIdentity, sourceTranscript, nil)
	if err != nil {
		t.Fatal(err)
	}
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	if err := state.ApplySessionSnapshot(SessionSnapshot{
		Identity: sourceIdentity, Projection: projection, ContextGeneration: 2, ContextGenerationPersisted: true,
	}); err != nil {
		t.Fatal(err)
	}
	state.Messages.Set(append([]Message{{Kind: MsgInfo, Text: "NONFORK SOURCE ROW"}}, state.Messages.Get()...))
	sourceRoot, targetRoot := t.TempDir(), t.TempDir()
	if err := SaveSessionViewCheckpoint(sourceRoot, state, sourceTranscript); err != nil {
		t.Fatal(err)
	}
	if err := ForkSessionViewCheckpoint(
		sourceRoot, targetRoot, sourceIdentity, targetIdentity, sourceTranscript, targetTranscript,
	); err != nil {
		t.Fatal(err)
	}
	loaded, restored, err := loadSessionViewCheckpointInLanguage(i18n.LangEN, targetRoot, targetTranscript, targetIdentity)
	if err != nil || !restored {
		t.Fatalf("load nonfork target = restored %v err %v", restored, err)
	}
	if len(loaded.Projection.Messages) != 1 || loaded.Projection.Messages[0].Text != "different target transcript" {
		t.Fatalf("nonfork target inherited source projection: %#v", loaded.Projection.Messages)
	}
}

func viewContainsText(messages []Message, text string) bool {
	for _, message := range messages {
		if message.Text == text {
			return true
		}
	}
	return false
}

func checkpointMessagesSemanticallyEqual(left, right []Message) bool {
	cloneMessages := func(messages []Message) []Message {
		cloned := make([]Message, len(messages))
		for index := range messages {
			cloned[index] = clonePresentationMessage(messages[index])
		}
		return cloned
	}
	leftCheckpoint := sessionViewCheckpoint{Messages: cloneMessages(left)}
	rightCheckpoint := sessionViewCheckpoint{Messages: cloneMessages(right)}
	normalizeCheckpointProjectionForComparison(&leftCheckpoint)
	normalizeCheckpointProjectionForComparison(&rightCheckpoint)
	return reflect.DeepEqual(leftCheckpoint.Messages, rightCheckpoint.Messages)
}

func TestDurableSessionViewHasOneSchemaOwner(t *testing.T) {
	durable := reflect.TypeOf(DurableSessionView{})
	for _, owner := range []reflect.Type{reflect.TypeOf(SessionSnapshot{}), reflect.TypeOf(sessionViewCheckpoint{})} {
		found := 0
		for index := 0; index < owner.NumField(); index++ {
			field := owner.Field(index)
			if field.Anonymous && field.Type == durable {
				found++
			}
		}
		if found != 1 {
			t.Fatalf("%s embeds DurableSessionView %d times, want exactly one", owner.Name(), found)
		}
	}
}

func TestAppSnapshotPublicationSynchronizesPreRunRootMirrors(t *testing.T) {
	persisted := checkpointTranscriptFixture()
	identity := SessionIdentity{Namespace: "/workspace", SessionID: "pre-run-root", Epoch: 1}
	projection, err := ProjectPersistedMessagesInLanguage(i18n.LangEN, identity, persisted, nil)
	if err != nil {
		t.Fatal(err)
	}
	anchor := projection.Messages[0].ObservationID
	state := NewAppState()
	root := NewRootComponent(state, nil, nil)
	state.Messages.Set([]Message{
		segmentTestTool("stale-a", "Read", "assistant", "foreground", OutcomeSucceeded),
		segmentTestTool("stale-b", "Grep", "assistant", "foreground", OutcomeSucceeded),
	})
	staleFrame := root.renderAtSize(nil, 80, 24)
	staleFrame.Render(gtui.NewBuffer(80, 24), 80, 24)
	if len(root.segmentRefs.All()) == 0 || root.contentRef.El() == nil {
		t.Fatal("test did not establish a stale pre-transition action map")
	}
	app := &App{state: state, root: root}
	if err := app.ApplySessionSnapshot(SessionSnapshot{
		Identity: identity, Projection: projection,
		DurableSessionView: DurableSessionView{Interaction: SessionInteraction{
			InputDraft: "pre-run draft", InputCursor: 3, InputCursorSet: true, ScrollAnchorID: anchor, ScrollOffset: 3,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if root.input.Text() != "pre-run draft" || root.input.CursorPosition() != 3 || root.scrollY.Get() != 3 || root.historyStart.Get() != 0 {
		t.Fatalf("pre-run root mirrors draft=%q cursor=%d scroll=%d historyStart=%d", root.input.Text(), root.input.CursorPosition(), root.scrollY.Get(), root.historyStart.Get())
	}
	if len(root.segmentRefs.All()) != 0 || root.contentRef.El() != nil {
		t.Fatal("snapshot publication retained stale action refs from the previous session")
	}
}

func checkpointStateFixture(t *testing.T, identity SessionIdentity, persisted []types.Message) *AppState {
	t.Helper()
	projection, err := ProjectPersistedMessagesInLanguage(i18n.LangEN, identity, persisted, nil)
	if err != nil {
		t.Fatal(err)
	}
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	if err := state.ApplySessionSnapshot(SessionSnapshot{Identity: identity, Projection: projection}); err != nil {
		t.Fatal(err)
	}
	return state
}

func renderCheckpointFrame(state *AppState, width, height int) *gtui.Buffer {
	root := NewRootComponent(state, nil, nil)
	root.syncSessionViewFromState()
	root.input.Focus()
	frame := root.renderAtSize(nil, width, height)
	buffer := gtui.NewBuffer(width, height)
	frame.Render(buffer, width, height)
	return buffer
}

func assertCheckpointFramesEqual(t *testing.T, source, target *AppState) {
	t.Helper()
	for _, lang := range i18n.AllLanguages() {
		assertCheckpointFramesEqualInLanguage(t, source, target, lang)
	}
}

func assertCheckpointFramesEqualInLanguage(t *testing.T, source, target *AppState, lang i18n.Language) {
	t.Helper()
	source.Language.Set(lang)
	target.Language.Set(lang)
	for _, size := range [][2]int{{40, 12}, {80, 24}, {120, 40}} {
		live := renderCheckpointFrame(source, size[0], size[1])
		restored := renderCheckpointFrame(target, size[0], size[1])
		for y := 0; y < size[1]; y++ {
			for x := 0; x < size[0]; x++ {
				if !live.Cell(x, y).Equal(restored.Cell(x, y)) {
					t.Fatalf("frame cell drift lang=%s size=%dx%d at %d,%d\n--- live ---\n%s\n--- restored ---\n%s", lang.Code(), size[0], size[1], x, y, live.String(), restored.String())
				}
			}
		}
	}
}

func TestSessionViewCheckpointSameLanguageMatrixUsesIndependentCheckpoints(t *testing.T) {
	persisted := checkpointTranscriptFixture()
	for _, lang := range i18n.AllLanguages() {
		lang := lang
		t.Run(lang.Code(), func(t *testing.T) {
			identity := SessionIdentity{Namespace: "/workspace", SessionID: "locale-" + lang.Code(), Epoch: 1}
			projection, err := ProjectPersistedMessagesInLanguage(lang, identity, persisted, nil)
			if err != nil {
				t.Fatal(err)
			}
			source := NewAppState()
			source.Language.Set(lang)
			if err := source.ApplySessionSnapshot(SessionSnapshot{Identity: identity, Projection: projection}); err != nil {
				t.Fatal(err)
			}
			source.SetInteractionEditor("多行🙂 draft\nsecond", 4)
			root := t.TempDir()
			if err := SaveSessionViewCheckpoint(root, source, persisted); err != nil {
				t.Fatal(err)
			}
			loaded, restored, err := loadSessionViewCheckpointInLanguage(lang, root, persisted, identity)
			if err != nil || !restored {
				t.Fatalf("load same-language checkpoint = restored %v err %v", restored, err)
			}
			target := NewAppState()
			target.Language.Set(lang)
			if err := target.ApplySessionSnapshot(loaded); err != nil {
				t.Fatal(err)
			}
			assertCheckpointFramesEqualInLanguage(t, source, target, lang)
		})
	}
}

func TestSessionViewCheckpointRoundTripsEveryDurableField(t *testing.T) {
	persisted := checkpointTranscriptFixture()
	identity := SessionIdentity{Namespace: "/workspace", SessionID: "all-durable-fields", Epoch: 4}
	projection, err := ProjectPersistedMessagesInLanguage(i18n.LangEN, identity, persisted, nil)
	if err != nil {
		t.Fatal(err)
	}
	view := DurableSessionView{
		Provider: "provider-a", Model: "model-a",
		Usage: SessionUsage{
			Known: true, RoundUsageKnown: true, InputTokens: 101, OutputTokens: 102, CacheReadTokens: 103, CacheCreateTokens: 104,
			HasCompacted: true, CompactionBaselineKnown: true, CompactionCount: 2, CompletedRoundInputTokens: 105, CompletedRoundOutputTokens: 106,
			InputTokensAtCompact: 107, CacheReadAtCompact: 108, LastInputTokens: 109, LastOutputTokens: 110,
			LastCacheReadTokens: 111, LastCacheCreateTokens: 112, WebSearchRequests: 3, CumulativeCost: 1.25,
			UsedTokens: 113, MaxTokens: 200000,
		},
		SessionCostKnown: false,
		Goal: &GoalViewState{Status: "active", Objective: "preserve everything", Revision: 2, Criteria: []GoalCriterionViewState{
			{ID: "AC-1", Text: "everything is preserved", Status: "met", Reason: "verified"},
			{ID: "AC-2", Text: "resume keeps acceptance state", Status: "pending"},
		}},
		Interaction: SessionInteraction{FocusedObservationID: "focus", ScrollAnchorID: "message:0", ScrollOffset: 2, InputDraft: "draft🙂 [Image #7] ", InputCursor: 2, InputCursorSet: true},
		DisclosureReturns: map[string]SessionInteraction{
			"return": {FocusedObservationID: "return", ScrollAnchorID: "message:0", ScrollOffset: 1, InputDraft: "return draft", InputCursor: 3, InputCursorSet: true},
		},
		PermissionMode: ModePlanEdit,
		Decisions: []DecisionRecord{{
			Prompt:     permissions.PromptRequest{DecisionID: "decision-1", SessionID: identity.SessionID, ToolName: "Write", Action: "write"},
			Response:   permissions.PromptResponse{DecisionID: "decision-1", Outcome: permissions.PromptOutcomeRejected},
			ResolvedAt: time.Unix(1234, 0).UTC(),
		}},
		Activities: []Activity{{
			ActivityEvent: ActivityEvent{ID: "activity-1", RunID: "run-1", Attempt: 1, SessionID: identity.SessionID, Epoch: identity.Epoch, WorkUnitID: "work-1", Actor: ActivityActor{ID: "agent-1", Type: "agent"}, Kind: ActivityAgent, Name: "Verifier", Phase: ActivityPhaseVerifying, Lifecycle: ActivityLifecycleBlocked, Attention: ActivityAttention{Kind: ActivityAttentionNeedsInput, Unread: true}, Outcome: OutcomeRunning, Sequence: 1},
			Actionability: ActivityActionDecision, Actions: []ActivityAction{ActivityCancel}, OccurrenceCount: 1, FirstSequence: 1, LastSequence: 1,
		}},
		ActivityFocus: "activity-1", ActivityViewOffset: 0,
		ToolSegmentExpansion: map[string]bool{"segment-a": true, "segment-b": false},
		ExpandedView:         "tasks", TaskViewItems: []TaskViewItem{{ID: "task-1", Subject: "Verify", Status: "in_progress", Owner: "agent-1", BlockedBy: []string{"task-0"}}},
		DecisionReceipt: "rejected", PendingImages: []ImageAttachment{{ID: 7, Base64: "aW1hZ2U=", MediaType: "image/png", Placeholder: "[Image #7]"}}, PendingImageSelected: 0,
	}
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	if err := state.ApplySessionSnapshot(SessionSnapshot{Identity: identity, Projection: projection, DurableSessionView: view}); err != nil {
		t.Fatal(err)
	}
	state.SetTranscriptShowAll(true)
	expected := state.DurableSessionViewSnapshot()
	if !state.TranscriptShowAll.Get() {
		t.Fatal("durable snapshot did not separate process-local evidence mode")
	}
	root := t.TempDir()
	if err := SaveSessionViewCheckpoint(root, state, persisted); err != nil {
		t.Fatal(err)
	}
	loaded, restored, err := loadSessionViewCheckpointInLanguage(i18n.LangEN, root, persisted, identity)
	if err != nil || !restored {
		t.Fatalf("load all-fields checkpoint = restored %v err %v", restored, err)
	}
	if !reflect.DeepEqual(loaded.DurableSessionView, expected) {
		t.Fatalf("durable view round-trip drift:\n got=%#v\nwant=%#v", loaded.DurableSessionView, expected)
	}
}

func TestSessionViewCheckpointRestoresSlashOverlaySelectionAndDismissal(t *testing.T) {
	persisted := checkpointTranscriptFixture()
	identity := SessionIdentity{Namespace: "/workspace", SessionID: "slash-overlay", Epoch: 1}
	source := checkpointStateFixture(t, identity, persisted)
	source.SetInteractionEditor("/", 1)
	commands := []SlashCommandEntry{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}
	sourceUI := NewRootComponent(source, nil, commands)
	sourceUI.syncSessionViewFromState()
	sourceUI.moveSlashSelection(1)
	sourceUI.moveSlashSelection(1)
	if interaction := source.ActiveSessionInteraction(); !interaction.SlashSelectedSet || interaction.SlashSelected != 2 {
		t.Fatalf("source slash selection was not durable: %+v", interaction)
	}

	root := t.TempDir()
	if err := SaveSessionViewCheckpoint(root, source, persisted); err != nil {
		t.Fatal(err)
	}
	loaded, restored, err := loadSessionViewCheckpointInLanguage(i18n.LangEN, root, persisted, identity)
	if err != nil || !restored {
		t.Fatalf("load selected slash overlay = restored %v err %v", restored, err)
	}
	target := NewAppState()
	target.Language.Set(i18n.LangEN)
	if err := target.ApplySessionSnapshot(loaded); err != nil {
		t.Fatal(err)
	}
	targetUI := NewRootComponent(target, nil, commands)
	targetUI.syncSessionViewFromState()
	if suggestions := targetUI.slash.Get(); suggestions == nil || suggestions.Selected != 2 {
		t.Fatalf("restored slash suggestions = %+v, want selected 2", suggestions)
	}
	for _, pair := range [][2]*RootComponent{{sourceUI, targetUI}} {
		left, right := gtui.NewBuffer(80, 24), gtui.NewBuffer(80, 24)
		pair[0].renderAtSize(nil, 80, 24).Render(left, 80, 24)
		pair[1].renderAtSize(nil, 80, 24).Render(right, 80, 24)
		for y := 0; y < 24; y++ {
			for x := 0; x < 80; x++ {
				if !left.Cell(x, y).Equal(right.Cell(x, y)) {
					t.Fatalf("slash overlay frame drift at %d,%d", x, y)
				}
			}
		}
	}

	sourceUI.dismissSlashSuggestions()
	if err := SaveSessionViewCheckpoint(root, source, persisted); err != nil {
		t.Fatal(err)
	}
	loaded, restored, err = loadSessionViewCheckpointInLanguage(i18n.LangEN, root, persisted, identity)
	if err != nil || !restored {
		t.Fatalf("load dismissed slash overlay = restored %v err %v", restored, err)
	}
	dismissed := NewAppState()
	if err := dismissed.ApplySessionSnapshot(loaded); err != nil {
		t.Fatal(err)
	}
	dismissedUI := NewRootComponent(dismissed, nil, commands)
	dismissedUI.syncSessionViewFromState()
	if dismissedUI.slash.Get() != nil || dismissed.ActiveSessionInteraction().SlashDismissedInput != "/" {
		t.Fatalf("dismissed slash overlay reopened: root=%+v interaction=%+v", dismissedUI.slash.Get(), dismissed.ActiveSessionInteraction())
	}
}

func TestWriteSessionViewFidelityVisualFixtures(t *testing.T) {
	dir := os.Getenv("SESSION_VIEW_VISUAL_DIR")
	if dir == "" {
		t.Skip("set SESSION_VIEW_VISUAL_DIR to write visual-verdict fixtures")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	persisted := checkpointTranscriptFixture()
	sourceIdentity := SessionIdentity{Namespace: "/workspace", SessionID: "visual-source", Epoch: 1}
	source := checkpointStateFixture(t, sourceIdentity, persisted)
	source.Provider.Set("deepseek")
	source.Model.Set("deepseek-v4-flash")
	source.SetInteractionEditor("继续验证 resume / fork", 6)
	source.SetInteractionScroll(0)
	source.SetToolSegmentExpanded(firstCheckpointSegment(t, source.Messages.Get()).ID, false)

	sourceRoot, forkRoot := filepath.Join(dir, "source-artifacts"), filepath.Join(dir, "fork-artifacts")
	if err := SaveSessionViewCheckpoint(sourceRoot, source, persisted); err != nil {
		t.Fatal(err)
	}
	resumeSnapshot, restored, err := loadSessionViewCheckpointInLanguage(i18n.LangEN, sourceRoot, persisted, sourceIdentity)
	if err != nil || !restored {
		t.Fatalf("load visual resume = restored %v err %v", restored, err)
	}
	resume := NewAppState()
	resume.Language.Set(i18n.LangEN)
	if err := resume.ApplySessionSnapshot(resumeSnapshot); err != nil {
		t.Fatal(err)
	}
	forkIdentity := SessionIdentity{Namespace: sourceIdentity.Namespace, SessionID: "visual-fork", Epoch: 1}
	if err := ForkSessionViewCheckpoint(sourceRoot, forkRoot, sourceIdentity, forkIdentity, persisted, persisted); err != nil {
		t.Fatal(err)
	}
	forkSnapshot, restored, err := loadSessionViewCheckpointInLanguage(i18n.LangEN, forkRoot, persisted, forkIdentity)
	if err != nil || !restored {
		t.Fatalf("load visual fork = restored %v err %v", restored, err)
	}
	forked := NewAppState()
	forked.Language.Set(i18n.LangEN)
	if err := forked.ApplySessionSnapshot(forkSnapshot); err != nil {
		t.Fatal(err)
	}

	frames := map[string]*gtui.Buffer{
		"live":   renderCheckpointFrame(source, 100, 34),
		"resume": renderCheckpointFrame(resume, 100, 34),
		"fork":   renderCheckpointFrame(forked, 100, 34),
	}
	for name, frame := range frames {
		if err := os.WriteFile(filepath.Join(dir, name+".txt"), []byte(frame.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if frames["live"].String() != frames["resume"].String() || frames["live"].String() != frames["fork"].String() {
		t.Fatal("visual fixture frames differ before image generation")
	}
}

func TestSessionViewCheckpointResumeRestoresExactSemanticFrame(t *testing.T) {
	persisted := checkpointTranscriptFixture()
	identity := SessionIdentity{Namespace: "/workspace", SessionID: "resume-source", Epoch: 1}
	source := checkpointStateFixture(t, identity, persisted)

	// This local presentation row is intentionally absent from the model
	// transcript. Re-projecting only persisted model messages cannot recover it.
	messages := append([]Message(nil), source.Messages.Get()...)
	messages = append(messages[:1], append([]Message{{Kind: MsgInfo, Text: "LOCAL PRESENTATION RECEIPT"}}, messages[1:]...)...)
	source.Messages.Set(messages)
	segment := firstCheckpointSegment(t, source.Messages.Get())
	source.SetToolSegmentExpanded(segment.ID, true)
	source.SetInteractionDraft("unfinished draft")
	source.SetInteractionCursor(5)
	source.SetInteractionAnchor(segment.Messages[0].ObservationID)
	source.SetInteractionScroll(2)
	source.Provider.Set("deepseek")
	source.Model.Set("deepseek-v4-flash")
	source.Mode.Set(ModePlanEdit)
	source.SetGoalStatus("active", "preserve the exact session view")
	source.SessionUsageKnown.Set(true)
	source.SessionTotalInputTokens.Set(1234)
	source.SessionTotalOutputTokens.Set(321)
	source.SessionCacheReadTokens.Set(600)
	source.UsedTokens.Set(777)
	source.MaxTokens.Set(200000)
	source.CumulativeCost.Set(0.42)
	source.SessionCostKnown.Set(false)
	source.AddPendingImage("cGVuZGluZy1pbWFnZQ==", "image/png")
	source.MovePendingImageSelection(-1)
	if err := source.ApplyActivity(ActivityEvent{
		ID: "background:running", SessionID: identity.SessionID, Epoch: identity.Epoch,
		Actor: ActivityActor{ID: "worker", Type: "agent"}, Kind: ActivityAgent, Name: "Verify resume view",
		Phase: ActivityPhaseVerifying, Lifecycle: ActivityLifecycleRunning, Outcome: OutcomeRunning,
	}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := SaveSessionViewCheckpoint(root, source, persisted); err != nil {
		t.Fatal(err)
	}

	snapshot, restored, err := LoadSessionViewCheckpoint(root, persisted, SessionIdentity{
		Namespace: identity.Namespace, SessionID: identity.SessionID, Epoch: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !restored {
		t.Fatal("exact transcript checkpoint was not restored")
	}
	target := NewAppState()
	target.Language.Set(i18n.LangEN)
	if err := target.ApplySessionSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}

	assertCheckpointFramesEqual(t, source, target)
	if got := target.ActiveSessionInteraction(); got.InputDraft != "unfinished draft [Image #1] " || got.InputCursor != len([]rune(got.InputDraft)) || !got.InputCursorSet || got.ScrollAnchorID == "" || got.ScrollOffset != 2 {
		t.Fatalf("resume interaction = %+v", got)
	}
	if target.Provider.Get() != "deepseek" || target.Model.Get() != "deepseek-v4-flash" || target.Mode.Get() != ModePlanEdit || target.Goal.Get() == nil || target.SessionCostKnown.Get() {
		t.Fatalf("resume lost durable chrome inputs: provider=%q model=%q mode=%v goal=%+v costKnown=%v", target.Provider.Get(), target.Model.Get(), target.Mode.Get(), target.Goal.Get(), target.SessionCostKnown.Get())
	}
	if images := target.PendingImages.Get(); len(images) != 1 || images[0].Base64 != "cGVuZGluZy1pbWFnZQ==" || target.PendingImageSelected.Get() != 0 {
		t.Fatalf("resume pending image draft = %+v selected=%d", images, target.PendingImageSelected.Get())
	}
	targetSegment := firstCheckpointSegment(t, target.Messages.Get())
	if !target.ToolSegmentExpanded(targetSegment.ID) {
		t.Fatalf("resume lost explicit group expansion: source=%q target=%q", segment.ID, targetSegment.ID)
	}
}

func TestSessionViewCheckpointReprojectsTranscriptWhenLanguageChanges(t *testing.T) {
	persisted := checkpointTranscriptFixture()
	identity := SessionIdentity{Namespace: "/workspace", SessionID: "language-change", Epoch: 1}
	source := checkpointStateFixture(t, identity, persisted)
	messages := append([]Message(nil), source.Messages.Get()...)
	messages = append(messages, Message{Kind: MsgInfo, Text: "FROZEN ENGLISH CHECKPOINT COPY"})
	source.Messages.Set(messages)

	root := t.TempDir()
	if err := SaveSessionViewCheckpoint(root, source, persisted); err != nil {
		t.Fatal(err)
	}
	targetIdentity := SessionIdentity{Namespace: identity.Namespace, SessionID: identity.SessionID, Epoch: 2}
	loaded, restored, err := loadSessionViewCheckpointInLanguage(i18n.LangZH, root, persisted, targetIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if !restored {
		t.Fatal("checkpoint was not restored")
	}
	want, err := ProjectPersistedMessagesInLanguage(i18n.LangZH, targetIdentity, persisted, loaded.Projection.Details)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Projection.Messages, want.Messages) {
		t.Fatalf("language-change projection retained frozen checkpoint copy:\n got=%#v\nwant=%#v", loaded.Projection.Messages, want.Messages)
	}
}

func TestSessionViewCheckpointForkRestoresSelectedPrefixFrame(t *testing.T) {
	persisted := checkpointTranscriptFixture()
	sourceIdentity := SessionIdentity{Namespace: "/workspace", SessionID: "fork-source", Epoch: 1}
	source := checkpointStateFixture(t, sourceIdentity, persisted)
	messages := append([]Message(nil), source.Messages.Get()...)
	messages = append(messages[:1], append([]Message{{Kind: MsgInfo, Text: "FORK-LOCAL RECEIPT"}}, messages[1:]...)...)
	source.Messages.Set(messages)
	segment := firstCheckpointSegment(t, source.Messages.Get())
	source.SetToolSegmentExpanded(segment.ID, true)

	sourceRoot, targetRoot := t.TempDir(), t.TempDir()
	if err := SaveSessionViewCheckpoint(sourceRoot, source, persisted); err != nil {
		t.Fatal(err)
	}
	targetIdentity := SessionIdentity{Namespace: sourceIdentity.Namespace, SessionID: "fork-target", Epoch: 1}
	if err := ForkSessionViewCheckpoint(sourceRoot, targetRoot, sourceIdentity, targetIdentity, persisted, persisted); err != nil {
		t.Fatal(err)
	}
	snapshot, restored, err := LoadSessionViewCheckpoint(targetRoot, persisted, targetIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if !restored {
		t.Fatal("fork checkpoint was not restored")
	}
	target := NewAppState()
	target.Language.Set(i18n.LangEN)
	target.DecisionReq.Set(&DecisionRequest{DecisionID: "stale", ToolName: "Bash"})
	target.SessionPicker.Set(&SessionPickerState{Visible: true})
	target.ForkPicker.Set(&ForkPickerState{Visible: true})
	target.ModelPicker.Set(&ModelPickerState{Visible: true})
	if err := target.ApplySessionSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	if target.DecisionReq.Get() != nil || target.SessionPicker.Get() != nil || target.ForkPicker.Get() != nil || target.ModelPicker.Get() != nil {
		t.Fatal("fork publication retained a stale modal that can intercept group controls")
	}
	assertCheckpointFramesEqual(t, source, target)
	if !strings.Contains(renderCheckpointFrame(target, 100, 40).String(), "FORK-LOCAL RECEIPT") {
		t.Fatal("fork lost local presentation history")
	}
	if !target.ToolSegmentExpanded(firstCheckpointSegment(t, target.Messages.Get()).ID) {
		t.Fatal("fork lost explicit group expansion")
	}

	const width, height = 80, 24
	sourceUI := NewRootComponent(source, nil, nil)
	sourceUI.syncSessionViewFromState()
	targetUI := NewRootComponent(target, nil, nil)
	targetUI.syncSessionViewFromState()
	for _, root := range []*RootComponent{sourceUI, targetUI} {
		frame := root.renderAtSize(nil, width, height)
		frame.Render(gtui.NewBuffer(width, height), width, height)
	}
	sourceHeader := sourceUI.segmentRefs.Get(firstCheckpointSegment(t, source.Messages.Get()).ID)
	targetSegment := firstCheckpointSegment(t, target.Messages.Get())
	targetHeader := targetUI.segmentRefs.Get(targetSegment.ID)
	if sourceHeader == nil || targetHeader == nil || sourceHeader.Rect() != targetHeader.Rect() {
		t.Fatalf("fork group action rect drift: source=%v target=%v", sourceHeader, targetHeader)
	}
	viewport := targetUI.contentRef.El()
	sx, sy := viewport.ScrollOffset()
	visible := targetHeader.Rect().Translate(viewport.ContentRect().X-sx, viewport.ContentRect().Y-sy).Intersect(viewport.ContentRect())
	if !targetUI.HandleMouse(gtui.MouseEvent{Button: gtui.MouseLeft, Action: gtui.MousePress, X: visible.Right() - 2, Y: visible.Y}) {
		t.Fatal("fork-restored group header click was not consumed")
	}
	if target.ToolSegmentExpanded(targetSegment.ID) {
		t.Fatal("fork-restored group header click did not collapse the group")
	}
	altG := gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'g', Mod: gtui.ModAlt}
	binding := toolSegmentShortcutBinding(t, targetUI, altG)
	binding.Handler(altG)
	if !target.ToolSegmentExpanded(targetSegment.ID) {
		t.Fatal("fork-restored Alt+G did not re-expand the group")
	}
}

func TestForkSessionViewCheckpointDoesNotPersistExpandedEvidenceMode(t *testing.T) {
	persisted := checkpointTranscriptFixture()
	sourceIdentity := SessionIdentity{Namespace: "/workspace", SessionID: "evidence-present-source", Epoch: 1}
	source := checkpointStateFixture(t, sourceIdentity, persisted)
	segment := firstCheckpointSegment(t, source.Messages.Get())
	source.SetToolSegmentExpanded(segment.ID, true)

	source.Observations.mu.Lock()
	representativeID := ""
	for index := range source.Observations.observations {
		observation := &source.Observations.observations[index]
		if !observation.Aggregation.Representative {
			continue
		}
		observation.Disclosure.Level = DisclosureEvidence
		representativeID = observation.ID
		break
	}
	source.Observations.mu.Unlock()
	if representativeID == "" || len(source.ObservationAggregateSnapshot()) == 0 {
		t.Fatal("fixture did not produce an aggregate representative")
	}
	messages := append([]Message(nil), source.Messages.Get()...)
	for index := range messages {
		if messages[index].ObservationID != representativeID {
			continue
		}
		observation, _ := source.GetObservation(representativeID)
		replacement := messageFromObservation(observation, messages[index].Kind)
		replacement.Timestamp = messages[index].Timestamp
		messages[index] = replacement
	}
	source.Messages.Set(messages)
	source.ObservationRevision.Set(source.ObservationRevision.Get() + 1)

	sourceRoot, targetRoot := t.TempDir(), t.TempDir()
	if err := SaveSessionViewCheckpoint(sourceRoot, source, persisted); err != nil {
		t.Fatal(err)
	}
	targetIdentity := SessionIdentity{Namespace: sourceIdentity.Namespace, SessionID: "evidence-present-target", Epoch: 1}
	if err := ForkSessionViewCheckpoint(sourceRoot, targetRoot, sourceIdentity, targetIdentity, persisted, persisted); err != nil {
		t.Fatal(err)
	}
	loaded, restored, err := LoadSessionViewCheckpoint(targetRoot, persisted, targetIdentity)
	if err != nil || !restored {
		t.Fatalf("load evidence fork = restored %v err %v", restored, err)
	}
	target := NewAppState()
	if err := target.ApplySessionSnapshot(loaded); err != nil {
		t.Fatal(err)
	}
	remapped, ok := target.GetObservation(strings.Replace(representativeID, sourceIdentity.SessionID, targetIdentity.SessionID, 1))
	if !ok {
		t.Fatalf("target operational observation was not remapped from %q", representativeID)
	}
	if remapped.Disclosure.Level != DisclosureSummary || remapped.Disclosure.UserPinned {
		t.Fatalf("fork restored process-local evidence disclosure: %+v", remapped.Disclosure)
	}
	if remapped.PresentationID != representativeID || remapped.ID == remapped.PresentationID {
		t.Fatalf("fork presentation/operational identity did not split: %+v", remapped)
	}
}

func TestSessionViewCheckpointEvidenceDisclosureIsResetOnLoad(t *testing.T) {
	checkpoint := sessionViewCheckpoint{
		Messages:     []Message{{ObservationID: "obs", Disclosure: DisclosureState{Level: DisclosureEvidence, UserPinned: true}}},
		Observations: []Observation{{ID: "obs", Disclosure: DisclosureState{Level: DisclosureEvidence, UserPinned: true}}},
	}
	snapshot := SessionSnapshot{}
	applyCheckpointToSnapshot(checkpoint, NewMemoryDetailStore(), &snapshot)
	if got := snapshot.Projection.Observations[0].Disclosure; got.Level != DisclosureSummary || got.UserPinned {
		t.Fatalf("observation disclosure restored as %+v", got)
	}
	if got := snapshot.Projection.Messages[0].Disclosure; got.Level != DisclosureSummary || got.UserPinned {
		t.Fatalf("message disclosure restored as %+v", got)
	}
}

func TestForkSessionViewCheckpointPreservesExpandedActivityFrameAndControls(t *testing.T) {
	persisted := checkpointTranscriptFixture()
	sourceIdentity := SessionIdentity{Namespace: "/workspace", SessionID: "activity-present-source", Epoch: 1}
	source := checkpointStateFixture(t, sourceIdentity, persisted)
	event := ActivityEvent{
		ID: "agent:reviewer", RunID: "run-1", Attempt: 1,
		SessionID: sourceIdentity.SessionID, Epoch: sourceIdentity.Epoch,
		TurnID: sourceIdentity.SessionID + ":turn-1", WorkUnitID: sourceIdentity.SessionID + ":task-1",
		Actor: ActivityActor{ID: "reviewer", Type: "agent"}, Kind: ActivityAgent, Name: "Review exact view",
		Phase: ActivityPhaseVerifying, Lifecycle: ActivityLifecycleBlocked, Outcome: OutcomeRunning,
		Attention: ActivityAttention{Kind: ActivityAttentionNeedsInput, Severity: ActivityAttentionSeverityWarning, Unread: true},
		Control:   ActivityControl{Cancelable: true},
	}
	if err := source.ApplyActivity(event); err != nil {
		t.Fatal(err)
	}
	source.SetExpandedView("activities")
	source.ActivityFocus.Set(event.ID)
	source.ActivityViewOffset.Set(0)

	sourceRoot, targetRoot := t.TempDir(), t.TempDir()
	if err := SaveSessionViewCheckpoint(sourceRoot, source, persisted); err != nil {
		t.Fatal(err)
	}
	targetIdentity := SessionIdentity{Namespace: sourceIdentity.Namespace, SessionID: "activity-present-target", Epoch: 1}
	if err := ForkSessionViewCheckpoint(sourceRoot, targetRoot, sourceIdentity, targetIdentity, persisted, persisted); err != nil {
		t.Fatal(err)
	}
	loaded, restored, err := LoadSessionViewCheckpoint(targetRoot, persisted, targetIdentity)
	if err != nil || !restored {
		t.Fatalf("load activity fork = restored %v err %v", restored, err)
	}
	target := NewAppState()
	if err := target.ApplySessionSnapshot(loaded); err != nil {
		t.Fatal(err)
	}
	assertCheckpointFramesEqual(t, source, target)
	activity, ok := target.GetActivity(event.ID)
	if !ok {
		t.Fatalf("target activity %q missing", event.ID)
	}
	if activity.WorkUnitID == event.WorkUnitID || activity.PresentationWorkUnitID != event.WorkUnitID {
		t.Fatalf("activity identity/control surface drift: %+v", activity)
	}
}

func TestSessionViewCheckpointCapableSessionFailsClosedForDifferentTranscript(t *testing.T) {
	persisted := checkpointTranscriptFixture()
	identity := SessionIdentity{Namespace: "/workspace", SessionID: "digest-source", Epoch: 1}
	state := checkpointStateFixture(t, identity, persisted)
	root := t.TempDir()
	if err := SaveSessionViewCheckpoint(root, state, persisted); err != nil {
		t.Fatal(err)
	}
	different := append([]types.Message(nil), persisted...)
	different = append(different, types.UserMessage("newer turn"))
	_, restored, err := LoadSessionViewCheckpoint(root, different, identity)
	if restored {
		t.Fatal("checkpoint for a different transcript was accepted")
	}
	info, ok := i18n.DescribeSemanticError(err)
	if !ok || info.Key != i18n.KeyTUISessionViewMissingCheckpoint {
		t.Fatalf("different transcript error = %v (%+v, %v)", err, info, ok)
	}
}

func TestSessionViewCheckpointRejectsOutOfOrderCapturePublication(t *testing.T) {
	persisted := checkpointTranscriptFixture()
	identity := SessionIdentity{Namespace: "/workspace", SessionID: "capture-order", Epoch: 1}
	state := checkpointStateFixture(t, identity, persisted)
	state.SetInteractionDraft("older view")
	older, err := CaptureSessionViewCheckpoint(state, persisted)
	if err != nil {
		t.Fatal(err)
	}
	state.SetInteractionDraft("newer view")
	newer, err := CaptureSessionViewCheckpoint(state, persisted)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := SaveSessionViewCapture(root, newer); err != nil {
		t.Fatal(err)
	}
	err = SaveSessionViewCapture(root, older)
	info, ok := i18n.DescribeSemanticError(err)
	if !ok || info.Key != i18n.KeyTUISessionViewStaleCapture {
		t.Fatalf("out-of-order capture error = %v (%+v, %v)", err, info, ok)
	}
	loaded, restored, err := LoadSessionViewCheckpoint(root, persisted, identity)
	if err != nil || !restored {
		t.Fatalf("load newest capture = restored %v err %v", restored, err)
	}
	if got := loaded.Interaction.InputDraft; got != "newer view" {
		t.Fatalf("stale publication replaced newer view: %q", got)
	}
}

func TestSessionViewCheckpointCapableSessionRejectsMissingCurrentPayload(t *testing.T) {
	persisted := checkpointTranscriptFixture()
	identity := SessionIdentity{Namespace: "/workspace", SessionID: "missing-payload", Epoch: 1}
	state := checkpointStateFixture(t, identity, persisted)
	root := t.TempDir()
	if err := SaveSessionViewCheckpoint(root, state, persisted); err != nil {
		t.Fatal(err)
	}
	digest, err := sessionViewTranscriptDigest(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sessionViewCheckpointPath(root, len(persisted), digest)); err != nil {
		t.Fatal(err)
	}
	_, restored, err := LoadSessionViewCheckpoint(root, persisted, identity)
	if restored {
		t.Fatal("missing current payload was restored")
	}
	info, ok := i18n.DescribeSemanticError(err)
	if !ok || info.Key != i18n.KeyTUISessionViewMissingCheckpoint {
		t.Fatalf("missing current payload error = %v (%+v, %v)", err, info, ok)
	}
}

func TestSessionViewCheckpointRejectsUnsupportedVersionsAndIdentityMismatch(t *testing.T) {
	persisted := checkpointTranscriptFixture()
	identity := SessionIdentity{Namespace: "/workspace", SessionID: "version-source", Epoch: 1}
	state := checkpointStateFixture(t, identity, persisted)
	root := t.TempDir()
	if err := SaveSessionViewCheckpoint(root, state, persisted); err != nil {
		t.Fatal(err)
	}
	if _, restored, err := LoadSessionViewCheckpoint(root, persisted, SessionIdentity{Namespace: identity.Namespace, SessionID: "other", Epoch: 1}); restored || err == nil {
		t.Fatalf("identity mismatch restored=%v err=%v", restored, err)
	} else if info, ok := i18n.DescribeSemanticError(err); !ok || info.Key != i18n.KeyTUISessionViewIdentityMismatch {
		t.Fatalf("identity mismatch error = %v (%+v, %v)", err, info, ok)
	}
	digest, err := sessionViewTranscriptDigest(persisted)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, ok, err := readSessionViewCheckpoint(root, len(persisted), digest)
	if err != nil || !ok {
		t.Fatalf("read current checkpoint = ok %v err %v", ok, err)
	}
	for _, version := range []int{sessionViewCheckpointVersion - 1, 99} {
		checkpoint.Version = version
		if err := writeSessionViewCheckpoint(root, checkpoint); err != nil {
			t.Fatal(err)
		}
		if _, restored, err := LoadSessionViewCheckpoint(root, persisted, identity); restored || err == nil {
			t.Fatalf("checkpoint version %d restored=%v err=%v", version, restored, err)
		} else if info, ok := i18n.DescribeSemanticError(err); !ok || info.Key != i18n.KeyTUISessionViewUnsupportedVersion {
			t.Fatalf("checkpoint version %d error = %v (%+v, %v)", version, err, info, ok)
		}
	}
}

func TestSessionViewCheckpointRejectsRemovedSchemaFields(t *testing.T) {
	persisted := checkpointTranscriptFixture()
	identity := SessionIdentity{Namespace: "/workspace", SessionID: "strict-schema", Epoch: 1}
	state := checkpointStateFixture(t, identity, persisted)
	root := t.TempDir()
	if err := SaveSessionViewCheckpoint(root, state, persisted); err != nil {
		t.Fatal(err)
	}
	digest, err := sessionViewTranscriptDigest(persisted)
	if err != nil {
		t.Fatal(err)
	}
	path := sessionViewCheckpointPath(root, len(persisted), digest)
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var envelope sessionViewCheckpointEnvelope
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		mutate  func(map[string]any)
		wantKey i18n.Key
	}{
		{name: "removed field", mutate: func(payload map[string]any) { payload["transcript_show_all"] = true }, wantKey: i18n.KeyTUISessionViewDecodeCheckpointFile},
		{name: "missing required field", mutate: func(payload map[string]any) { delete(payload, "session_cost_known") }, wantKey: i18n.KeyTUISessionViewInvalidCheckpoint},
	} {
		t.Run(test.name, func(t *testing.T) {
			var payload map[string]any
			if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			test.mutate(payload)
			mutatedPayload, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			mutatedEnvelope := envelope
			mutatedEnvelope.Payload = mutatedPayload
			payloadDigest := sha256.Sum256(mutatedPayload)
			mutatedEnvelope.PayloadSHA256 = hex.EncodeToString(payloadDigest[:])
			mutatedEncoded, err := json.Marshal(mutatedEnvelope)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, mutatedEncoded, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, ok, err := readSessionViewCheckpoint(root, len(persisted), digest); ok || err == nil {
				t.Fatalf("invalid schema accepted: ok=%v err=%v", ok, err)
			} else if info, described := i18n.DescribeSemanticError(err); !described || info.Key != test.wantKey {
				t.Fatalf("invalid schema error = %v (%+v, %v), want %q", err, info, described, test.wantKey)
			}
		})
	}
}

func TestApplySessionSnapshotRejectsIncompleteToolPresentation(t *testing.T) {
	identity := SessionIdentity{Namespace: "/workspace", SessionID: "strict-presentation", Epoch: 1}
	projection, err := ProjectPersistedMessages(identity, checkpointTranscriptFixture(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for index := range projection.Observations {
		if projection.Observations[index].ToolName == "" {
			continue
		}
		projection.Observations[index].Presentation.Summary = ""
		break
	}
	state := NewAppState()
	if err := state.ApplySessionSnapshot(SessionSnapshot{Identity: identity, Projection: projection}); err == nil {
		t.Fatal("incomplete tool presentation was accepted")
	}
}

func TestSessionViewCheckpointMaterializesEveryReachableEvidenceSurface(t *testing.T) {
	persisted := checkpointTranscriptFixture()
	identity := SessionIdentity{Namespace: "/workspace", SessionID: "evidence-resume", Epoch: 4}
	source := checkpointStateFixture(t, identity, persisted)
	appendCheckpointLocalEvidence(t, source, identity)

	root := t.TempDir()
	if err := SaveSessionViewCheckpoint(root, source, persisted); err != nil {
		t.Fatal(err)
	}
	loaded, restored, err := LoadSessionViewCheckpoint(root, persisted, SessionIdentity{Namespace: identity.Namespace, SessionID: identity.SessionID, Epoch: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !restored {
		t.Fatal("materialized checkpoint was not restored")
	}
	assertCheckpointEvidenceReadable(t, loaded)
}

func TestForkSessionViewCheckpointOwnsEvidenceAfterSourceDeletion(t *testing.T) {
	persisted := checkpointTranscriptFixture()
	sourceIdentity := SessionIdentity{Namespace: "/workspace", SessionID: "evidence-fork-source", Epoch: 1}
	source := checkpointStateFixture(t, sourceIdentity, persisted)
	appendCheckpointLocalEvidence(t, source, sourceIdentity)
	sourceRoot, targetRoot := t.TempDir(), t.TempDir()
	if err := SaveSessionViewCheckpoint(sourceRoot, source, persisted); err != nil {
		t.Fatal(err)
	}
	targetIdentity := SessionIdentity{Namespace: sourceIdentity.Namespace, SessionID: "evidence-fork-target", Epoch: 1}
	if err := ForkSessionViewCheckpoint(sourceRoot, targetRoot, sourceIdentity, targetIdentity, persisted, persisted); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(sourceRoot); err != nil {
		t.Fatal(err)
	}
	loaded, restored, err := LoadSessionViewCheckpoint(targetRoot, persisted, targetIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if !restored {
		t.Fatal("fork checkpoint was not restored without its source")
	}
	assertCheckpointEvidenceReadable(t, loaded)
	for _, observation := range loaded.Projection.Observations {
		if strings.Contains(observation.ID, sourceIdentity.SessionID) || observation.SessionID != targetIdentity.SessionID {
			t.Fatalf("fork observation retained source identity: %+v", observation)
		}
	}
}

func TestSessionViewCheckpointFailsClosedBeforePublishingDanglingEvidence(t *testing.T) {
	persisted := checkpointTranscriptFixture()
	identity := SessionIdentity{Namespace: "/workspace", SessionID: "evidence-corrupt", Epoch: 1}
	source := checkpointStateFixture(t, identity, persisted)
	appendCheckpointLocalEvidence(t, source, identity)
	root := t.TempDir()
	if err := SaveSessionViewCheckpoint(root, source, persisted); err != nil {
		t.Fatal(err)
	}
	digest, err := sessionViewTranscriptDigest(persisted)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, ok, err := readSessionViewCheckpoint(root, len(persisted), digest)
	if err != nil || !ok || len(checkpoint.EvidenceManifest) == 0 {
		t.Fatalf("read checkpoint = ok %v err %v manifest %+v", ok, err, checkpoint.EvidenceManifest)
	}
	store, err := NewFileDetailStore(filepath.Join(root, "tui-details"))
	if err != nil {
		t.Fatal(err)
	}
	path, err := store.pathForRef(checkpoint.EvidenceManifest[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	_, restored, err := LoadSessionViewCheckpoint(root, persisted, identity)
	if restored || err == nil {
		t.Fatalf("dangling evidence restored=%v err=%v", restored, err)
	}
	info, ok := i18n.DescribeSemanticError(err)
	if !ok || info.Key != i18n.KeyTUISessionViewValidateEvidence {
		t.Fatalf("dangling evidence error = %v (%+v, %v)", err, info, ok)
	}
}

func appendCheckpointLocalEvidence(t *testing.T, state *AppState, identity SessionIdentity) {
	t.Helper()
	messageRef, err := state.Details.Put("local-message", []byte("message-only exact bytes"))
	if err != nil {
		t.Fatal(err)
	}
	state.AppendMessage(Message{Kind: MsgInfo, Text: "LOCAL EVIDENCE", DetailRefs: []DetailRef{messageRef}})
	activityRef, observationID, err := state.RecordActivityEvidenceForEpoch(
		identity.SessionID, identity.Epoch, "activity:local", "LocalActivity", "work-local", "actor-local", "local-observation", OutcomeSucceeded, []byte("observation exact bytes"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyActivity(ActivityEvent{
		ID: "activity:local", SessionID: identity.SessionID, Epoch: identity.Epoch,
		Lifecycle: ActivityLifecycleCompleted, Outcome: OutcomeSucceeded,
		Control: ActivityControl{JumpTarget: observationID, DetailRefs: []DetailRef{activityRef}},
	}); err != nil {
		t.Fatal(err)
	}
}

func assertCheckpointEvidenceReadable(t *testing.T, snapshot SessionSnapshot) {
	t.Helper()
	checkpoint := sessionViewCheckpoint{
		Messages: snapshot.Projection.Messages, Observations: snapshot.Projection.Observations,
		DurableSessionView: snapshot.DurableSessionView,
	}
	refs := collectCheckpointDetailRefs(checkpoint)
	if len(refs) < 3 {
		t.Fatalf("reachable evidence refs = %d, want message/observation/activity coverage", len(refs))
	}
	for _, ref := range refs {
		data, err := snapshot.Projection.Details.Get(ref)
		if err != nil {
			t.Fatalf("read evidence %q: %v", ref.Key, err)
		}
		if len(data) != ref.Size || digestBytes(data) != ref.Digest {
			t.Fatalf("evidence %q failed content validation", ref.Key)
		}
	}
}

func firstCheckpointSegment(t *testing.T, messages []Message) *TranscriptToolSegment {
	t.Helper()
	for _, item := range BuildTranscriptToolSegments(messages) {
		if item.Segment != nil {
			return item.Segment
		}
	}
	t.Fatal("fixture has no tool segment")
	return nil
}
