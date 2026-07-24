package tui

import (
	"testing"

	gtui "github.com/grindlemire/go-tui"
)

func TestMultilinePasteUsesPlaceholderUntilSubmit(t *testing.T) {
	var submissions []string
	root := NewRootComponent(NewAppState(), func(input string) {
		submissions = append(submissions, input)
	}, nil)
	root.input.Focus()

	pasted := "first\r\nsecond\rthird"
	if handled := root.input.HandlePaste(gtui.PasteEvent{Text: pasted}); !handled {
		t.Fatal("expected paste event to be handled")
	}
	if len(submissions) != 0 {
		t.Fatalf("submissions before Enter = %d, want 0", len(submissions))
	}
	if got := root.inputText.Get(); got != "[Pasted text #1 +3 lines]" {
		t.Fatalf("input text = %q, want multiline placeholder", got)
	}

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyEnter})
	if len(submissions) != 1 {
		t.Fatalf("submissions after Enter = %d, want 1", len(submissions))
	}
	if got := submissions[0]; got != "first\nsecond\nthird" {
		t.Fatalf("submitted text = %q", got)
	}
	if got := root.inputText.Get(); got != "" {
		t.Fatalf("input after submit = %q, want empty", got)
	}
	if len(root.pastes) != 0 || root.nextPaste != 0 {
		t.Fatalf("paste state after submit = (%d, %d), want cleared", len(root.pastes), root.nextPaste)
	}
}

func TestMultilinePastePlaceholderEditsAsAtomicToken(t *testing.T) {
	root := NewRootComponent(NewAppState(), func(string) {}, nil)
	root.input.SetText("before ")
	root.input.HandleEvent(gtui.PasteEvent{Text: "first\nsecond"})

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyLeft})
	if got, want := root.input.CursorPosition(), len([]rune("before ")); got != want {
		t.Fatalf("cursor after Left = %d, want placeholder start %d", got, want)
	}
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyDelete})
	if got := root.input.Text(); got != "before " {
		t.Fatalf("input after Delete = %q, want whole placeholder removed", got)
	}
	if len(root.pastes) != 0 {
		t.Fatalf("stored pastes after Delete = %d, want 0", len(root.pastes))
	}
}

func TestSingleLinePasteRemainsInline(t *testing.T) {
	root := NewRootComponent(NewAppState(), func(string) {}, nil)
	root.input.SetText("before ")
	root.input.HandleEvent(gtui.PasteEvent{Text: "inline text"})
	if got := root.inputText.Get(); got != "before inline text" {
		t.Fatalf("input text = %q", got)
	}
	if len(root.pastes) != 0 {
		t.Fatalf("stored pastes = %d, want 0", len(root.pastes))
	}
}

func TestMultiplePastesExpandInOrderWithTypedText(t *testing.T) {
	var submitted string
	root := NewRootComponent(NewAppState(), func(input string) {
		submitted = input
	}, nil)
	root.input.SetText("prefix ")
	root.input.HandleEvent(gtui.PasteEvent{Text: "one\ntwo"})
	root.input.InsertText(" between ")
	root.input.HandleEvent(gtui.PasteEvent{Text: "three\nfour"})
	root.input.InsertText(" suffix")

	wantDisplay := "prefix [Pasted text #1 +2 lines] between [Pasted text #2 +2 lines] suffix"
	if got := root.inputText.Get(); got != wantDisplay {
		t.Fatalf("display text = %q, want %q", got, wantDisplay)
	}
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyEnter})
	if want := "prefix one\ntwo between three\nfour suffix"; submitted != want {
		t.Fatalf("submitted = %q, want %q", submitted, want)
	}
}

func TestRemovedPastePlaceholderDoesNotSubmitHiddenText(t *testing.T) {
	var submitted string
	root := NewRootComponent(NewAppState(), func(input string) {
		submitted = input
	}, nil)
	root.input.HandleEvent(gtui.PasteEvent{Text: "secret\ntext"})
	root.input.SetText("replacement")
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyEnter})
	if submitted != "replacement" {
		t.Fatalf("submitted = %q, hidden paste should be excluded", submitted)
	}
}

func TestPasteDoesNotLeakThroughPickerOverlay(t *testing.T) {
	state := NewAppState()
	root := NewRootComponent(state, func(string) {}, nil)
	state.ModelPicker.Set(&ModelPickerState{Visible: true})
	root.input.Focus()

	if handled := root.input.HandlePaste(gtui.PasteEvent{Text: "hidden\ntext"}); handled {
		t.Fatal("paste should not be handled while a picker overlay is open")
	}
	if got := root.inputText.Get(); got != "" {
		t.Fatalf("input text = %q, want empty", got)
	}
}

func TestPastePopulatesActiveProviderConnectionInput(t *testing.T) {
	state := NewAppState()
	root := NewRootComponent(state, func(string) {}, nil)
	picker := &ModelPickerState{
		Visible:             true,
		Phase:               PickerPhaseConnect,
		ConnectAuthMethods:  []string{"api_key"},
		ConnectSelectedAuth: 0,
		ConnectInputField:   ConnectInputAPIKey,
		ConnectAPIKeyInput:  "sk-",
		ConnectError:        "old error",
	}
	state.ModelPicker.Set(picker)
	root.input.Focus()

	if handled := root.input.HandlePaste(gtui.PasteEvent{Text: "secret\r\n"}); !handled {
		t.Fatal("paste should be handled by the provider connection input")
	}
	if got := state.ModelPicker.Get().ConnectAPIKeyInput; got != "sk-secret" {
		t.Fatalf("API key input = %q, want %q", got, "sk-secret")
	}
	if got := state.ModelPicker.Get().ConnectError; got != "" {
		t.Fatalf("connect error = %q, want cleared", got)
	}
	if got := root.inputText.Get(); got != "" {
		t.Fatalf("composer input = %q, want empty", got)
	}

	picker = state.ModelPicker.Get()
	picker.ConnectInputField = ConnectInputBaseURL
	state.ModelPicker.Set(picker)
	if handled := root.input.HandlePaste(gtui.PasteEvent{Text: "https://gateway.example.com/v1"}); !handled {
		t.Fatal("paste should be handled by the provider base URL input")
	}
	if got := state.ModelPicker.Get().ConnectBaseURLInput; got != "https://gateway.example.com/v1" {
		t.Fatalf("base URL input = %q", got)
	}
}
