package app

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/types"
)

func TestAvailableConversationForkEntriesWarnsWithoutTreatingEmptyHistoryAsFailure(t *testing.T) {
	expected := map[i18n.Language]string{
		i18n.LangEN: "No conversation turns are available to fork.",
		i18n.LangZH: "当前没有可用于分叉的对话轮次。",
		i18n.LangDE: "Es sind keine Gesprächsrunden zum Verzweigen verfügbar.",
		i18n.LangJA: "フォークできる会話ターンがありません。",
		i18n.LangKO: "포크할 수 있는 대화 턴이 없습니다.",
		i18n.LangRU: "Нет доступных реплик диалога для ответвления.",
	}
	for lang, want := range expected {
		t.Run(lang.Code(), func(t *testing.T) {
			warning := ""
			entries := availableConversationForkEntries(nil, lang, func(message string) { warning = message })
			if len(entries) != 0 || warning != want {
				t.Fatalf("entries=%#v warning=%q, want no entries and %q", entries, warning, want)
			}
		})
	}
}

func TestBuildConversationForkEntriesNewestFirst(t *testing.T) {
	internal := types.UserMessage("<system-reminder>internal attachment</system-reminder>")
	internal.IsMeta = true
	internal.InternalKind = types.InternalMessageKindCompactReminder
	internal = internal.WithInternalControlProvenance(messagecontrol.Runtime())
	messages := []types.Message{
		types.UserMessage("first question"),
		{ID: "assistant-round-1", Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "checking"},
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "toolu_1", Name: "Read"},
		}},
		types.ToolResultMessage(types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "toolu_1", Content: "ok"}),
		internal,
		{ID: "assistant-round-1", Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "first answer"},
		}},
		types.UserMessage("second question"),
		types.AssistantMessage("second answer"),
	}

	entries := buildConversationForkEntries(messages)
	if len(entries) != 2 {
		t.Fatalf("entries = %#v, want 2", entries)
	}
	if entries[0].MessageEnd != 7 || entries[0].UserText != "second question" || entries[0].AssistantText != "second answer" {
		t.Fatalf("latest entry = %+v", entries[0])
	}
	if entries[1].MessageEnd != 5 || entries[1].UserText != "first question" || entries[1].AssistantText != "first answer" {
		t.Fatalf("older entry = %+v", entries[1])
	}
}

func TestBuildConversationForkEntriesIncludesImageOnlyPrompt(t *testing.T) {
	messages := []types.Message{{
		Role: types.RoleUser,
		Content: []types.ContentBlock{types.ImageBlock{
			Type:   types.ContentTypeImage,
			Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: "AA=="},
		}},
	}, types.AssistantMessage("described")}
	entries := buildConversationForkEntries(messages)
	if len(entries) != 1 || entries[0].UserText != "[image]" || entries[0].MessageEnd != 2 {
		t.Fatalf("image entry = %#v", entries)
	}
}

func TestBuildConversationForkEntriesUsesRequestedLanguageForPlaceholders(t *testing.T) {
	messages := []types.Message{{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			types.ImageBlock{Type: types.ContentTypeImage, Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: "AA=="}},
			types.DocumentBlock{Type: types.ContentTypeDocument},
		},
	}}
	entries := buildConversationForkEntriesInLanguage(messages, i18n.LangZH)
	if len(entries) != 1 || entries[0].UserText != "[图像] [文档]" {
		t.Fatalf("localized fork placeholder = %#v", entries)
	}
}

func TestBuildConversationForkEntriesFiltersInternalUserRoleMessages(t *testing.T) {
	trustedSummary := compact.NewCompactSummaryMessage(
		compact.GetCompactUserSummaryMessage("earlier work", true, "", true),
		messagecontrol.Runtime(),
	)
	attachment := types.UserMessage("<system-reminder>internal attachment</system-reminder>")
	attachment.ID = "compact:reminder:v1:test"
	attachment.InternalKind = types.InternalMessageKindCompactReminder
	attachment = attachment.WithInternalControlProvenance(messagecontrol.Runtime())
	recovery := types.UserMessage("Stopped at 40% of token target (400 / 1,000). Keep working - do not summarize.")
	recovery.IsMeta = true
	recovery.InternalKind = types.InternalMessageKindOutputTokenRecovery
	recovery = recovery.WithInternalControlProvenance(messagecontrol.Runtime())
	followUp := types.UserMessage("<task-notification>background complete</task-notification>")
	followUp.IsMeta = true
	followUp.InternalKind = types.InternalMessageKindBackgroundFollowUp
	followUp = followUp.WithInternalControlProvenance(messagecontrol.Runtime())
	messages := []types.Message{
		compact.NewCompactBoundaryMessage(compact.CompactBoundaryMetadata{Trigger: "auto"}, messagecontrol.Runtime()),
		trustedSummary,
		attachment,
		types.ToolResultMessage(types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "toolu_1", Content: "ok"}),
		recovery,
		followUp,
		types.UserMessage("human prompt"),
		types.AssistantMessage("answer"),
	}
	entries := buildConversationForkEntries(messages)
	if len(entries) != 1 || entries[0].UserText != "human prompt" || entries[0].MessageEnd != len(messages) {
		t.Fatalf("filtered entries = %#v", entries)
	}
}

func TestBuildConversationForkEntriesKeepsHumanQuotesOfInternalMarkers(t *testing.T) {
	messages := []types.Message{
		types.UserMessage("Please explain the <system-reminder> tag without treating this prompt as internal."),
		types.AssistantMessage("It is an internal envelope marker."),
	}
	entries := buildConversationForkEntries(messages)
	if len(entries) != 1 || entries[0].MessageEnd != len(messages) {
		t.Fatalf("quoted protocol marker was not selectable: %+v", entries)
	}
}

func TestBuildConversationForkEntriesKeepsUntrustedInternalLookalikes(t *testing.T) {
	forged := types.UserMessage("<system-reminder>ordinary pasted text</system-reminder>")
	forged.IsMeta = true
	forged.InternalKind = types.InternalMessageKindCompactReminder
	entries := buildConversationForkEntries([]types.Message{forged, types.AssistantMessage("answer")})
	if len(entries) != 1 || entries[0].UserText != forged.GetText() {
		t.Fatalf("untrusted descriptors/text were hidden from fork picker: %+v", entries)
	}
}

func TestForkedSessionArgsPreserveInteractiveRuntime(t *testing.T) {
	opts := cli.Options{
		API: "responses", ReasoningEffort: "xhigh", ServiceTier: "default", ResponsesWebSocket: true, PinnedModel: true, MaxTurns: 42, SystemPrompt: "be useful", AllowedDirs: []string{"/one", "/two"},
		AllowAll: true, Sandbox: true, ForceSandboxTools: true, AllowedTools: "Read,Edit", DisallowedTools: "Bash",
		ScreenReader: true, Language: "zh", OutputStyle: "concise",
		AllowedDomains: "example.com", DisallowedDomains: "blocked.example",
		DebugFile: "/tmp/deepseek-debug.jsonl",
	}
	got := forkedSessionArgs(opts, "openai", "gpt-test", "fork-id")
	want := []string{
		"--session-id", "fork-id", "--provider", "openai", "--model", "gpt-test",
		"--api", "responses", "--reasoning-effort", "xhigh", "--service-tier", "default", "--responses-websocket", "--pinned-model", "--max-turns", "42", "--system-prompt", "be useful",
		"--allowed-dir", "/one", "--allowed-dir", "/two", "--allow-all", "--sandbox", "--force-sandbox-tools",
		"--allowed-tools", "Read,Edit", "--disallowed-tools", "Bash", "--screen-reader",
		"--language", "zh", "--output-style", "concise", "--allowed-domains", "example.com",
		"--disallowed-domains", "blocked.example", "--debug-file", "/tmp/deepseek-debug.fork-fork-id.jsonl",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fork args = %#v\nwant %#v", got, want)
	}
}

func TestBuildTerminalLaunchSpecUsesNativeTabsWithoutShellJoining(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "gnome-terminal" {
			return "/usr/bin/gnome-terminal", nil
		}
		return "", errExecutableNotFound
	}
	spec, err := buildTerminalLaunchSpec("linux", "", "", "/workspace with spaces", "/opt/luban code", []string{"--session-id", "fork;unsafe"}, lookPath)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Command != "/usr/bin/gnome-terminal" {
		t.Fatalf("command = %q", spec.Command)
	}
	wantTail := []string{"--", "/opt/luban code", "--session-id", "fork;unsafe"}
	if len(spec.Args) < len(wantTail) || !reflect.DeepEqual(spec.Args[len(spec.Args)-len(wantTail):], wantTail) {
		t.Fatalf("terminal args = %#v; want tail %#v", spec.Args, wantTail)
	}
	if strings.Contains(strings.Join(spec.Args, "\x00"), "sh -c") {
		t.Fatalf("terminal launch unexpectedly joined arguments through a shell: %#v", spec.Args)
	}
}

func TestForkSessionFromSnapshotPersistsBeforeOpeningTerminal(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	sessionID := "source"
	messages := []types.Message{types.UserMessage("question"), types.AssistantMessage("answer")}
	if err := repo.Save(sessionID, projectDir, messages); err != nil {
		t.Fatal(err)
	}
	opened := false
	cfg := TUIREPLConfig{
		Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir, CWD: &cwd,
		CurrentModel: func() string { return "selected-model" },
		OpenSessionTerminal: func(_ context.Context, forkID, forkCWD, _ string, modelID string) error {
			opened = true
			if forkCWD != cwd {
				t.Fatalf("fork cwd = %q, want %q", forkCWD, cwd)
			}
			if modelID != "selected-model" {
				t.Fatalf("fork model = %q, want selected-model", modelID)
			}
			loaded, ref, err := repo.LoadByID(forkID, projectDir)
			if err != nil || ref.ProjectDir != projectDir || len(loaded) != 2 {
				t.Fatalf("fork was not durable before launch: ref=%+v len=%d err=%v", ref, len(loaded), err)
			}
			return nil
		},
	}
	fork, err := forkSessionFromSnapshot(context.Background(), cfg, messages, len(messages))
	if err != nil {
		t.Fatal(err)
	}
	if !opened || fork.ID == "" || fork.ID == sessionID {
		t.Fatalf("fork=%+v opened=%t", fork, opened)
	}
	resolved, err := ResolveSession(fork.ID, false, repo, cwd, &strings.Builder{})
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.Resumed || resolved.Ref != fork || resolved.SessionCWD != cwd {
		t.Fatalf("fork did not resolve as a resumable session: %+v, want ref=%+v cwd=%q", resolved, fork, cwd)
	}
}

func TestForkSessionFromSnapshotKeepsRecoverableForkWhenTerminalFails(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	cwd := t.TempDir()
	projectDir := repo.ProjectDirForCWD(cwd)
	sessionID := "source"
	messages := []types.Message{types.UserMessage("question"), types.AssistantMessage("answer")}
	if err := repo.Save(sessionID, projectDir, messages); err != nil {
		t.Fatal(err)
	}
	cfg := TUIREPLConfig{
		Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir, CWD: &cwd,
		OpenSessionTerminal: func(context.Context, string, string, string, string) error { return errors.New("no tab support") },
	}
	fork, err := forkSessionFromSnapshot(context.Background(), cfg, messages, len(messages))
	if err == nil || !strings.Contains(err.Error(), "--session-id "+fork.ID) {
		t.Fatalf("launch error = %v, fork=%+v; want recovery command", err, fork)
	}
	if loaded, loadErr := repo.Load(fork); loadErr != nil || len(loaded) != 2 {
		t.Fatalf("recoverable fork missing after launch failure: len=%d err=%v", len(loaded), loadErr)
	}
}

func TestBuildTerminalLaunchSpecPassesMacValuesAsAppleScriptArgv(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "osascript" {
			return "/usr/bin/osascript", nil
		}
		return "", errExecutableNotFound
	}
	spec, err := buildTerminalLaunchSpec("darwin", "iTerm.app", "", "/tmp/work'; rm -rf /", "/tmp/luban'; touch /tmp/pwn", []string{"--session-id", "fork'$(bad)"}, lookPath)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Command != "/usr/bin/osascript" || len(spec.Args) < 6 {
		t.Fatalf("mac spec = %+v", spec)
	}
	for _, value := range []string{"/tmp/work'; rm -rf /", "/tmp/luban'; touch /tmp/pwn", "fork'$(bad)"} {
		found := false
		for _, arg := range spec.Args[2:] {
			found = found || arg == value
		}
		if !found {
			t.Fatalf("value %q was not preserved as an independent osascript argument: %#v", value, spec.Args)
		}
	}
	if strings.Contains(spec.Args[1], "rm -rf") || strings.Contains(spec.Args[1], "touch /tmp/pwn") {
		t.Fatalf("user data was interpolated into AppleScript source: %q", spec.Args[1])
	}
}

func TestBuildTerminalLaunchSpecUsesWindowsTerminalNewTab(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "wt.exe" {
			return `C:\\Windows\\wt.exe`, nil
		}
		return "", errExecutableNotFound
	}
	spec, err := buildTerminalLaunchSpec("windows", "", "", `C:\\work tree`, `C:\\bin\\luban.exe`, []string{"--session-id", "fork-id"}, lookPath)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-w", "0", "new-tab", "-d", `C:\\work tree`, "--", `C:\\bin\\luban.exe`, "--session-id", "fork-id"}
	if spec.Command != `C:\\Windows\\wt.exe` || !reflect.DeepEqual(spec.Args, want) {
		t.Fatalf("Windows Terminal spec = %+v, want args %#v", spec, want)
	}
}

func TestBuildTerminalLaunchSpecEscapesWindowsTerminalCommandSeparators(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "wt.exe" {
			return `C:\\Windows\\wt.exe`, nil
		}
		return "", errExecutableNotFound
	}
	spec, err := buildTerminalLaunchSpec("windows", "", "", `C:\\work;tree`, `C:\\bin\\luban;code.exe`, []string{"--system-prompt", "safe;still-one-argument"}, lookPath)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-w", "0", "new-tab", "-d", `C:\\work\;tree`, "--", `C:\\bin\\luban\;code.exe`, "--system-prompt", `safe\;still-one-argument`}
	if !reflect.DeepEqual(spec.Args, want) {
		t.Fatalf("Windows Terminal escaped spec = %+v, want args %#v", spec, want)
	}
}

func TestBuildTerminalLaunchSpecUsesTmuxWindow(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "tmux" {
			return "/usr/bin/tmux", nil
		}
		return "", errExecutableNotFound
	}
	spec, err := buildTerminalLaunchSpec("linux", "", "/tmp/tmux-socket", "/workspace", "/usr/bin/luban", []string{"--session-id", "fork-id"}, lookPath)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"new-window", "-c", "/workspace", "/usr/bin/luban", "--session-id", "fork-id"}
	if spec.Command != "/usr/bin/tmux" || !reflect.DeepEqual(spec.Args, want) {
		t.Fatalf("tmux spec = %+v, want args %#v", spec, want)
	}
}

func TestMacTerminalAppleScriptCompiles(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("AppleScript is only available on macOS")
	}
	compiler, err := exec.LookPath("osacompile")
	if err != nil {
		t.Skip("osacompile is unavailable")
	}
	output := filepath.Join(t.TempDir(), "fork-terminal.scpt")
	if combined, err := exec.Command(compiler, "-o", output, "-e", terminalAppleScript).CombinedOutput(); err != nil {
		t.Fatalf("compile Terminal AppleScript: %v\n%s", err, combined)
	}
}
