package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/input"
	gtui "github.com/grindlemire/go-tui"
)

func newPromptHistoryTestRoot(t *testing.T) *RootComponent {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	state := NewAppState()
	state.SessionID.Set("session-a")
	root := NewRootComponent(state, func(string) {}, nil)
	root.input.Focus()
	return root
}

func submitPromptForHistoryTest(t *testing.T, root *RootComponent, value string) {
	t.Helper()
	root.input.SetText(value)
	if handled := root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyEnter}); !handled {
		t.Fatal("Enter was not handled")
	}
	if got := root.inputText.Get(); got != "" {
		t.Fatalf("input after submit = %q, want empty", got)
	}
}

func TestPromptHistoryEmptyInputUpRecallsLatestSubmission(t *testing.T) {
	root := newPromptHistoryTestRoot(t)
	submitPromptForHistoryTest(t, root, "latest prompt")

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})

	if got := root.inputText.Get(); got != "latest prompt" {
		t.Fatalf("input after Up = %q, want %q", got, "latest prompt")
	}
}

func TestPromptHistoryUpAndDownTraverseSubmissions(t *testing.T) {
	root := newPromptHistoryTestRoot(t)
	submitPromptForHistoryTest(t, root, "first prompt")
	submitPromptForHistoryTest(t, root, "second prompt")

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	if got := root.inputText.Get(); got != "second prompt" {
		t.Fatalf("input after first Up = %q, want second prompt", got)
	}
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	if got := root.inputText.Get(); got != "second prompt" {
		t.Fatalf("input after boundary Up = %q, want second prompt", got)
	}
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	if got := root.inputText.Get(); got != "first prompt" {
		t.Fatalf("input after history Up = %q, want first prompt", got)
	}
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyDown})
	if got := root.inputText.Get(); got != "second prompt" {
		t.Fatalf("input after first Down = %q, want second prompt", got)
	}
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyDown})
	if got := root.inputText.Get(); got != "second prompt" {
		t.Fatalf("input after boundary Down = %q, want second prompt", got)
	}
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyDown})
	if got := root.inputText.Get(); got != "" {
		t.Fatalf("input after history Down = %q, want empty draft", got)
	}
}

func TestPromptHistoryDownRestoresOriginalDraft(t *testing.T) {
	root := newPromptHistoryTestRoot(t)
	submitPromptForHistoryTest(t, root, "submitted prompt")
	root.input.SetText("unfinished draft")

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	if got := root.inputText.Get(); got != "submitted prompt" {
		t.Fatalf("input after Up = %q, want submitted prompt", got)
	}
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyDown})
	if got := root.inputText.Get(); got != "unfinished draft" {
		t.Fatalf("input after Down = %q, want unfinished draft", got)
	}
}

func TestPromptHistoryPreservesEditsWhileNavigating(t *testing.T) {
	root := newPromptHistoryTestRoot(t)
	submitPromptForHistoryTest(t, root, "older prompt")
	submitPromptForHistoryTest(t, root, "newer prompt")

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	root.input.SetText("edited newer prompt")
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyDown})

	if got := root.inputText.Get(); got != "edited newer prompt" {
		t.Fatalf("input after returning to edited entry = %q", got)
	}
}

func TestPromptHistoryWrappedInputMovesCursorBeforeRecall(t *testing.T) {
	root := newPromptHistoryTestRoot(t)
	submitPromptForHistoryTest(t, root, "submitted prompt")
	setTextAreaField(root.input.TextArea, "width", 4)
	root.input.SetText("abcdefgh")

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	if got := root.inputText.Get(); got != "abcdefgh" {
		t.Fatalf("first Up replaced wrapped input with %q", got)
	}
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	if got := root.inputText.Get(); got != "abcdefgh" {
		t.Fatalf("second Up replaced wrapped input with %q", got)
	}
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	if got := root.inputText.Get(); got != "submitted prompt" {
		t.Fatalf("third Up = %q, want submitted prompt", got)
	}
}

func TestPromptHistoryUpJumpsToStartFromFirstLineBeforeRecall(t *testing.T) {
	root := newPromptHistoryTestRoot(t)
	submitPromptForHistoryTest(t, root, "submitted prompt")
	root.input.SetText("first\nsecond\nthird")
	root.input.SetCursorPosition(3)

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	if got := root.inputText.Get(); got != "first\nsecond\nthird" {
		t.Fatalf("first Up changed input to %q", got)
	}
	root.input.InsertText("^")
	if got := root.inputText.Get(); got != "^first\nsecond\nthird" {
		t.Fatalf("insert after Up = %q, want insertion at absolute start", got)
	}

	root.input.SetText("first\nsecond\nthird")
	root.input.SetCursorPosition(3)
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	if got := root.inputText.Get(); got != "submitted prompt" {
		t.Fatalf("second Up = %q, want recalled history", got)
	}
}

func TestPromptHistoryArrowsKeepNativeMovementOnIntermediateLines(t *testing.T) {
	root := newPromptHistoryTestRoot(t)
	submitPromptForHistoryTest(t, root, "submitted prompt")
	root.input.SetText("first\nsecond\nthird")
	root.input.SetCursorPosition(8)

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	root.input.InsertText("^")
	if got := root.inputText.Get(); got != "fi^rst\nsecond\nthird" {
		t.Fatalf("insert after middle-line Up = %q, want native upward movement", got)
	}

	root.input.SetText("first\nsecond\nthird")
	root.input.SetCursorPosition(8)
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyDown})
	root.input.InsertText("^")
	if got := root.inputText.Get(); got != "first\nsecond\nth^ird" {
		t.Fatalf("insert after middle-line Down = %q, want native downward movement", got)
	}
}

func TestPromptHistoryDownJumpsToEndFromLastLineBeforeNextHistory(t *testing.T) {
	root := newPromptHistoryTestRoot(t)
	submitPromptForHistoryTest(t, root, "older\nentry")
	submitPromptForHistoryTest(t, root, "newer prompt")

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	if got := root.inputText.Get(); got != "older\nentry" {
		t.Fatalf("recalled input = %q, want multiline older entry", got)
	}
	root.input.SetCursorPosition(8)

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyDown})
	if got := root.inputText.Get(); got != "older\nentry" {
		t.Fatalf("first Down changed history to %q", got)
	}
	root.input.InsertText("!")
	if got := root.inputText.Get(); got != "older\nentry!" {
		t.Fatalf("insert after Down = %q, want insertion at absolute end", got)
	}
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyDown})
	if got := root.inputText.Get(); got != "newer prompt" {
		t.Fatalf("second Down = %q, want next history entry", got)
	}
}

func TestPromptHistoryModifiedUpDoesNotRecall(t *testing.T) {
	for _, mod := range []gtui.Modifier{gtui.ModShift, gtui.ModAlt, gtui.ModCtrl} {
		t.Run(mod.String(), func(t *testing.T) {
			root := newPromptHistoryTestRoot(t)
			submitPromptForHistoryTest(t, root, "submitted prompt")

			root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp, Mod: mod})

			if got := root.inputText.Get(); got != "" {
				t.Fatalf("modified Up recalled %q, want empty input", got)
			}
		})
	}
}

func TestPromptHistoryRecallCursorMatchesClaude(t *testing.T) {
	root := newPromptHistoryTestRoot(t)
	submitPromptForHistoryTest(t, root, "submitted prompt")
	root.input.SetText("draft")

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	root.input.InsertText("!")
	if got := root.inputText.Get(); got != "submitted prompt!" {
		t.Fatalf("text after editing recalled history = %q, want cursor at end", got)
	}

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyDown})
	root.input.InsertText("!")
	if got := root.inputText.Get(); got != "!draft" {
		t.Fatalf("text after restoring draft = %q, want cursor at start", got)
	}
}

func TestPromptHistoryInlineImageNavigationStaysInComposer(t *testing.T) {
	root := newPromptHistoryTestRoot(t)
	submitPromptForHistoryTest(t, root, "submitted prompt")
	root.state.Language.Set(i18n.LangEN)
	root.attachPastedImage("image", "image/png")

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyLeft})

	if got := root.input.CursorPosition(); got != 0 {
		t.Fatalf("cursor before image = %d, want 0", got)
	}
	if got := root.inputText.Get(); got != " [Image #1] " {
		t.Fatalf("input after image Left = %q, want inline placeholder", got)
	}
}

func TestPromptHistoryLoadsPersistedEntriesAndReloadsForSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	path := input.DefaultPromptHistoryPath()
	for _, entry := range []input.PromptHistoryEntry{
		{Display: "session-b prompt", Project: project, SessionID: "session-b"},
		{Display: "session-a prompt", Project: project, SessionID: "session-a"},
	} {
		if err := input.AppendPromptHistory(path, entry); err != nil {
			t.Fatal(err)
		}
	}

	state := NewAppState()
	state.SessionID.Set("session-a")
	root := NewRootComponent(state, func(string) {}, nil)
	root.inputHistoryPersistent = true
	root.input.Focus()
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	if got := root.inputText.Get(); got != "session-a prompt" {
		t.Fatalf("session-a recall = %q", got)
	}

	state.SessionID.Set("session-b")
	root.input.Clear()
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	if got := root.inputText.Get(); got != "session-b prompt" {
		t.Fatalf("session-b recall = %q", got)
	}
}

func TestPromptHistoryPreservesRepeatedSessionEntriesSeparatedOnDisk(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	path := input.DefaultPromptHistoryPath()
	for _, entry := range []input.PromptHistoryEntry{
		{Display: "repeated", Project: project, SessionID: "session-a"},
		{Display: "other session", Project: project, SessionID: "session-b"},
		{Display: "repeated", Project: project, SessionID: "session-a"},
	} {
		if err := input.AppendPromptHistory(path, entry); err != nil {
			t.Fatal(err)
		}
	}

	state := NewAppState()
	state.SessionID.Set("session-a")
	root := NewRootComponent(state, func(string) {}, nil)
	root.inputHistoryPersistent = true
	root.input.Focus()
	for i, want := range []string{"repeated", "repeated", "other session"} {
		if i > 0 {
			root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
		}
		root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
		if got := root.inputText.Get(); got != want {
			t.Fatalf("recall %d = %q, want %q", i, got, want)
		}
	}
}

func TestPromptHistoryUnboundRootDoesNotWritePersistentHistory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	state := NewAppState()
	state.SessionID.Set("session-a")
	root := NewRootComponent(state, func(string) {}, nil)
	root.input.Focus()

	submitPromptForHistoryTest(t, root, "unit-test prompt")

	if _, err := os.Stat(input.DefaultPromptHistoryPath()); !os.IsNotExist(err) {
		t.Fatalf("unbound root wrote persistent history: %v", err)
	}
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp})
	if got := root.inputText.Get(); got != "unit-test prompt" {
		t.Fatalf("unbound root lost in-memory history: %q", got)
	}
}

func TestPromptHistoryPersistenceFailureDoesNotBlockSubmit(t *testing.T) {
	root := newPromptHistoryTestRoot(t)
	root.inputHistoryPersistent = true
	root.inputHistoryPath = t.TempDir() // A directory cannot be opened as the JSONL file.
	var submitted string
	root.onSubmit = func(value string) { submitted = value }

	submitPromptForHistoryTest(t, root, "still submit this")

	if submitted != "still submit this" {
		t.Fatalf("submitted = %q, want prompt despite history failure", submitted)
	}
	if feedback := root.copyFeedback.Get(); !strings.Contains(feedback, "history not saved") {
		t.Fatalf("feedback = %q, want history persistence warning", feedback)
	}
}
