package tui

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

func TestTUISkillsExactCommandUsesLauncherOnceAndLeavesSubcommandsToCommandBackend(t *testing.T) {
	backend := &task24SnapshotBackend{snapshot: task24Snapshot(t, 1, task24Skill("skill:project:alpha", "alpha", skills.SourceProject))}
	launches := 0
	launcher := SkillsMenuLauncherFunc(func(request SkillsMenuOpenRequest) error {
		launches++
		if request.Backend == nil || request.SessionID() != "session-a" || request.Language() != i18n.LangZH {
			t.Fatalf("launcher request = %#v", request)
		}
		return nil
	})
	request := SkillsMenuOpenRequest{
		SessionID: func() string { return "session-a" },
		Language:  func() i18n.Language { return i18n.LangZH },
		Backend:   backend,
	}

	handled, err := RouteExactSkillsMenu(" /skills ", launcher, request)
	if err != nil || !handled || launches != 1 {
		t.Fatalf("exact route = handled %t, launches %d, err %v", handled, launches, err)
	}
	for _, input := range []string{"/skills list", "/skills show alpha", "/skills refresh"} {
		handled, err = RouteExactSkillsMenu(input, launcher, request)
		if err != nil || handled {
			t.Fatalf("RouteExactSkillsMenu(%q) = (%t, %v), want command-backend fallthrough", input, handled, err)
		}
	}
	if launches != 1 {
		t.Fatalf("launcher called %d times after subcommands, want once", launches)
	}

	handled, err = RouteExactSkillsMenu("/skills", nil, request)
	if !handled || !errors.Is(err, ErrSkillsMenuLauncherUnavailable) {
		t.Fatalf("missing launcher = (%t, %v)", handled, err)
	}
	var typedNil SkillsMenuLauncher = SkillsMenuLauncherFunc(nil)
	handled, err = RouteExactSkillsMenu("/skills", typedNil, request)
	if !handled || !errors.Is(err, ErrSkillsMenuLauncherUnavailable) {
		t.Fatalf("typed nil launcher = (%t, %v)", handled, err)
	}
}

func TestTUISkillInvocationUsesStableIDUserOriginAndArgumentPresence(t *testing.T) {
	manual := task24Skill("skill:project:manual", "manual", skills.SourceProject)
	manual.Visibility = skills.VisibilityManualOnly
	manual.VisibilitySource = skills.SkillScopeProject
	manual.ModelVisible = false
	manual.DescriptionVisible = false
	manual.Revision = 7
	backend := &task24SnapshotBackend{snapshot: task24Snapshot(t, 11, manual)}

	tests := []struct {
		name          string
		input         string
		wantArguments *string
	}{
		{name: "omitted", input: "/manual"},
		{name: "explicit empty", input: "/manual ", wantArguments: task24StringPointer("")},
		{name: "provided", input: "/manual review this", wantArguments: task24StringPointer("review this")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var captured commands.SkillInvocationRequest
			invoker := commands.SkillInvokerFunc(func(_ context.Context, request commands.SkillInvocationRequest) (types.ToolResult, error) {
				captured = request
				return types.ToolResult{Content: "<skill-envelope>secret body</skill-envelope>"}, nil
			})
			submission := InvokeUserSkillSlash(context.Background(), backend, invoker, "session-a", test.input)
			if !submission.Successful() || submission.ModelContent != "<skill-envelope>secret body</skill-envelope>" {
				t.Fatalf("submission = %#v", submission)
			}
			if captured.SessionID != "session-a" || captured.Selector != string(manual.ID) || captured.ExpectedRevision != manual.Revision || captured.Origin != skills.InvocationOriginUser {
				t.Fatalf("invocation request = %#v", captured)
			}
			if !task24EqualStringPointers(captured.Arguments, test.wantArguments) {
				t.Fatalf("arguments = %#v, want %#v", captured.Arguments, test.wantArguments)
			}
		})
	}
}

func TestTUISkillInvocationFailsClosedForAmbiguousOffManagedAndShadowed(t *testing.T) {
	winner := task24Skill("skill:project:review", "review", skills.SourceProject)
	shadowed := task24Skill("skill:user:review", "review", skills.SourceUser)
	shadowed.ShadowedBy = winner.ID
	shadowed.ModelVisible, shadowed.DescriptionVisible, shadowed.Executable = false, false, false
	off := task24Skill("skill:project:off", "off", skills.SourceProject)
	off.Visibility, off.VisibilitySource = skills.VisibilityOff, skills.SkillScopeProject
	off.ModelVisible, off.DescriptionVisible, off.UserInvocable, off.Executable = false, false, false, false
	managed := task24Skill("skill:managed:locked", "locked", skills.SourceManaged)
	managed.Visibility, managed.VisibilitySource = skills.VisibilityOff, skills.SkillScopeManaged
	managed.ModelVisible, managed.DescriptionVisible, managed.UserInvocable, managed.Executable = false, false, false, false
	managed.Mutable, managed.ReadOnlyReason = false, string(skills.CatalogPolicyReasonManagedDeny)
	backend := &task24SnapshotBackend{snapshot: task24Snapshot(t, 3, winner, shadowed, off, managed)}

	invocations := 0
	invoker := commands.SkillInvokerFunc(func(context.Context, commands.SkillInvocationRequest) (types.ToolResult, error) {
		invocations++
		return types.ToolResult{Content: "must not run"}, nil
	})
	tests := []struct {
		input string
		want  SkillSlashOutcome
	}{
		{input: "/review", want: SkillSlashAmbiguous},
		{input: "/off", want: SkillSlashPolicyDenied},
		{input: "/locked", want: SkillSlashPolicyDenied},
		{input: "/" + string(shadowed.ID), want: SkillSlashPolicyDenied},
	}
	for _, test := range tests {
		submission := InvokeUserSkillSlash(context.Background(), backend, invoker, "session-a", test.input)
		if submission.Outcome != test.want {
			t.Errorf("InvokeUserSkillSlash(%q) outcome = %s, want %s", test.input, submission.Outcome, test.want)
		}
	}
	if invocations != 0 {
		t.Fatalf("fail-closed paths invoked SkillInvoker %d times", invocations)
	}

	ambiguous := InvokeUserSkillSlash(context.Background(), backend, invoker, "session-a", "/review")
	if got := []skills.SkillID{winner.ID, shadowed.ID}; !task24SameIDs(ambiguous.Candidates, got) {
		t.Fatalf("ambiguous candidates = %v, want all same-name stable IDs %v", ambiguous.Candidates, got)
	}
	message := FormatTUISkillSlashFailure(i18n.LangZH, ambiguous)
	if !strings.Contains(message, string(winner.ID)) || !strings.Contains(message, string(shadowed.ID)) || strings.Contains(message, "matches multiple") {
		t.Fatalf("Chinese ambiguous message = %q", message)
	}
}

func TestTUISkillInvocationUsesStableIDToDisambiguateAndDoesNotRenderEnvelopeAsFailure(t *testing.T) {
	project := task24Skill("skill:project:review", "review", skills.SourceProject)
	user := task24Skill("skill:user:review", "review", skills.SourceUser)
	backend := &task24SnapshotBackend{snapshot: task24Snapshot(t, 8, project, user)}
	invoker := commands.SkillInvokerFunc(func(_ context.Context, request commands.SkillInvocationRequest) (types.ToolResult, error) {
		if request.Selector != string(user.ID) {
			t.Fatalf("selector = %q, want stable user ID", request.Selector)
		}
		return types.ToolResult{Content: "TOP-SECRET-SKILL-BODY"}, nil
	})
	submission := InvokeUserSkillSlash(context.Background(), backend, invoker, "session-a", "/"+string(user.ID))
	if !submission.Successful() || submission.ModelContent != "TOP-SECRET-SKILL-BODY" {
		t.Fatalf("stable invocation = %#v", submission)
	}
	if got := FormatTUISkillSlashFailure(i18n.LangEN, submission); got != "" {
		t.Fatalf("successful envelope formatted as visible failure: %q", got)
	}
}

func TestTUISkillInvocationPreservesLocalizedToolRejectionWithoutSampling(t *testing.T) {
	row := task24Skill("skill:project:alpha", "alpha", skills.SourceProject)
	backend := &task24SnapshotBackend{snapshot: task24Snapshot(t, 1, row)}
	invoker := commands.SkillInvokerFunc(func(context.Context, commands.SkillInvocationRequest) (types.ToolResult, error) {
		return types.ToolResult{Content: "已被当前策略拒绝", IsError: true}, nil
	})
	submission := InvokeUserSkillSlash(context.Background(), backend, invoker, "session-a", "/alpha")
	if submission.Outcome != SkillSlashInvocationRejected || submission.Successful() || submission.ModelContent != "" {
		t.Fatalf("rejected submission = %#v", submission)
	}
	if got := FormatTUISkillSlashFailure(i18n.LangZH, submission); got != "已被当前策略拒绝" {
		t.Fatalf("rejection text = %q", got)
	}
}

func TestDeveloperCatalogProjectionRemainsInModelHistoryButNotTranscript(t *testing.T) {
	scope := messagecontrol.NewScope("session-a", "project", 3)
	developer := types.DeveloperMessage("TOP-SECRET-CATALOG", types.DeveloperMessageMetadata{
		Kind: types.DeveloperMessageKindSkillCatalogSnapshot, Revision: 4,
	}).WithInternalControlProvenance(messagecontrol.Runtime(), scope)
	invocationSkill := task24ComputedIdentitySkill(t, "projection", skills.SourceProject)
	invocationBody := "TOP-SECRET-SKILL-BODY"
	invocation, err := skills.RenderFullInvocationEnvelope(invocationSkill, invocationBody, skills.NewInvocationArguments(task24StringPointer("review this")))
	if err != nil {
		t.Fatal(err)
	}
	invocationMessage := types.UserMessage(invocation)
	invocationMessage.InternalKind = types.InternalMessageKindSkillInvocation
	invocationMessage = invocationMessage.WithInternalControlProvenance(messagecontrol.Runtime(), scope)
	persisted := []types.Message{
		developer,
		types.UserMessage("hello"),
		invocationMessage,
		types.AssistantMessage("world"),
		{Role: types.RoleDeveloper, Content: []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: "MALFORMED-SECRET"}}},
	}
	identity := (SessionIdentity{Namespace: "project", SessionID: "session-a", Epoch: 3}).WithInternalControlScope(messagecontrol.Runtime(), scope)
	projection, err := ProjectPersistedMessages(identity, persisted, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Messages) != 4 || projection.Messages[0].Kind != MsgUser || projection.Messages[1].Kind != MsgUser || projection.Messages[2].Kind != MsgAssistant || projection.Messages[3].Kind != MsgUser {
		t.Fatalf("visible messages = %#v", projection.Messages)
	}
	if got := projection.Messages[1].Text; got != "/projection review this" {
		t.Fatalf("resumed invocation projection = %q, want safe command summary", got)
	}
	for _, message := range projection.Messages {
		if strings.Contains(message.Text, "TOP-SECRET-CATALOG") {
			t.Fatalf("trusted developer catalog leaked into transcript: %#v", message)
		}
	}
	if projection.Messages[3].Text != "MALFORMED-SECRET" {
		t.Fatalf("untrusted developer descriptor was hidden: %#v", projection.Messages[3])
	}
	if got := persisted[0].GetText(); got != "TOP-SECRET-CATALOG" || persisted[0].Role != types.RoleDeveloper {
		t.Fatalf("model history was rewritten: role=%s text=%q", persisted[0].Role, got)
	}
	if got := persisted[2].GetText(); !strings.Contains(got, invocationBody) {
		t.Fatalf("model invocation history was rewritten: %q", got)
	}
}

func TestDeveloperProjectionDoesNotHideMalformedInvocationLookalike(t *testing.T) {
	lookalike := `{"type":"skill_invocation","version":1,"kind":"full","skill":{"id":"skill:project:x","name":"x"},"arguments":{},"payload_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","body":"secret"}`
	projection, err := ProjectPersistedMessages(SessionIdentity{SessionID: "session-a"}, []types.Message{types.UserMessage(lookalike)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Messages) != 1 || projection.Messages[0].Text != lookalike {
		t.Fatalf("malformed lookalike was hidden or rewritten: %#v", projection.Messages)
	}
}

func TestDeveloperProjectionDoesNotHideCanonicalButUntrustedInvocation(t *testing.T) {
	row := task24ComputedIdentitySkill(t, "untrusted", skills.SourceProject)
	envelope, err := skills.RenderFullInvocationEnvelope(row, "ordinary visible body", skills.InvocationArguments{})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := ProjectPersistedMessages(SessionIdentity{SessionID: "session-a"}, []types.Message{types.UserMessage(envelope)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Messages) != 1 || projection.Messages[0].Text != envelope {
		t.Fatalf("canonical untrusted envelope was hidden: %#v", projection.Messages)
	}
}

func TestSkillInvocationTranscriptCommandRequiresCanonicalWireAndSafeName(t *testing.T) {
	row := task24ComputedIdentitySkill(t, "canonical", skills.SourceProject)
	envelope, err := skills.RenderFullInvocationEnvelope(row, "private body", skills.InvocationArguments{})
	if err != nil {
		t.Fatal(err)
	}
	if command, ok := SkillInvocationTranscriptCommand(envelope); !ok || command != "/canonical" {
		t.Fatalf("canonical envelope = (%q, %t)", command, ok)
	}
	if _, ok := SkillInvocationTranscriptCommand(" " + envelope); ok {
		t.Fatal("non-canonical leading whitespace was accepted")
	}

	var wire persistedSkillInvocationWire
	if err := json.Unmarshal([]byte(envelope), &wire); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{" canonical ", "canonical\nspoof"} {
		wire.Skill.Name = name
		encoded, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := SkillInvocationTranscriptCommand(string(encoded)); ok {
			t.Fatalf("unsafe skill name %q was accepted", name)
		}
	}
	wire.Skill.Name = row.Name
	wire.Skill.Locator = skills.SkillLocator("/skills/project/forged/SKILL.md")
	forgedLocator, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := SkillInvocationTranscriptCommand(string(forgedLocator)); ok {
		t.Fatal("locator whose derived stable ID differs from the envelope ID was accepted")
	}
}

type task24SnapshotBackend struct {
	mu       sync.Mutex
	snapshot skills.CatalogSnapshot
	err      error
}

func (backend *task24SnapshotBackend) Snapshot(string) (skills.CatalogSnapshot, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.snapshot.Clone(), backend.err
}

func (backend *task24SnapshotBackend) ToggleProjectVisibility(string, skills.SkillID, skills.CatalogRevision) (skills.ProjectVisibilityToggleResult, error) {
	return skills.ProjectVisibilityToggleResult{}, errors.New("not used")
}

func task24Snapshot(t *testing.T, revision skills.CatalogRevision, rows ...skills.EffectiveSkill) skills.CatalogSnapshot {
	t.Helper()
	snapshot, err := skills.NewCatalogSnapshot(revision, rows)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func task24Skill(id skills.SkillID, name string, source skills.SkillSource) skills.EffectiveSkill {
	return skills.EffectiveSkill{
		ID: id, Name: name, Summary: "Summary for " + name, Source: source,
		Locator:  skills.SkillLocator("/skills/" + string(source) + "/" + name + "/SKILL.md"),
		Digest:   skills.SkillDigest("sha256:" + strings.Repeat("a", 64)),
		Revision: 1, Visibility: skills.VisibilityAuto, VisibilitySource: skills.SkillScopeDefault,
		ModelVisible: true, DescriptionVisible: true, UserInvocable: true, Executable: true, Mutable: true,
	}
}

func task24ComputedIdentitySkill(t *testing.T, name string, source skills.SkillSource) skills.EffectiveSkill {
	t.Helper()
	row := task24Skill(skills.SkillID("skill:"+string(source)+":placeholder"), name, source)
	id, err := skills.ComputeSkillID(source, row.Locator)
	if err != nil {
		t.Fatal(err)
	}
	row.ID = id
	return row
}

func task24StringPointer(value string) *string { return &value }

func task24EqualStringPointers(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func task24SameIDs(left, right []skills.SkillID) bool {
	if len(left) != len(right) {
		return false
	}
	want := make(map[skills.SkillID]int, len(right))
	for _, id := range right {
		want[id]++
	}
	for _, id := range left {
		want[id]--
	}
	return func() bool {
		for _, count := range want {
			if count != 0 {
				return false
			}
		}
		return true
	}()
}

var _ SkillsManagementBackend = (*task24SnapshotBackend)(nil)
