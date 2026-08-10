package tui

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/agent-dance/luban/i18n"
	gtui "github.com/grindlemire/go-tui"
)

func dispatchRootKeyForTest(t *testing.T, root *RootComponent, ke gtui.KeyEvent) bool {
	t.Helper()
	for _, binding := range root.KeyMap() {
		if slashAwareKeyMatches(binding.Pattern, ke) {
			binding.Handler(ke)
			return binding.Stop
		}
	}
	return false
}

func TestRootEscapeKeepsInputFocusedWithoutSuggestions(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	root.input.Blur()

	if stopped := dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyEscape}); !stopped {
		t.Fatal("expected root Escape handler to consume the event")
	}
	if !root.input.IsFocused() {
		t.Fatal("expected Escape with no overlay or suggestions to keep input focused")
	}
}

func TestRootRejectsImagePasteForTextOnlyModel(t *testing.T) {
	state := NewAppState()
	state.ModelCanSeeImages.Set(false)
	root := NewRootComponent(state, nil, nil)

	root.handleImagePaste()

	if got := len(state.PendingImages.Get()); got != 0 {
		t.Fatalf("pending images = %d, want 0", got)
	}
	if got := root.copyFeedback.Get(); !strings.Contains(got, "does not support image input") {
		t.Fatalf("copy feedback = %q, want unsupported-image message", got)
	}
}

func TestRootPastedImageIsAtomicInsideComposer(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	state.ModelCanSeeImages.Set(true)
	root := NewRootComponent(state, nil, nil)
	root.input.SetText("before  after")
	root.input.SetCursorPosition(utf8.RuneCountInString("before "))
	root.attachPastedImage("image-data", "image/png")

	const placeholder = "[Image #1]"
	if got, want := root.input.Text(), "before  "+placeholder+"  after"; got != want {
		t.Fatalf("composer text = %q, want %q", got, want)
	}
	end := utf8.RuneCountInString("before  " + placeholder + " ")
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyLeft})
	if got, want := root.input.CursorPosition(), utf8.RuneCountInString("before "); got != want {
		t.Fatalf("cursor after Left = %d, want image start %d", got, want)
	}
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyRight})
	if got := root.input.CursorPosition(); got != end {
		t.Fatalf("cursor after Right = %d, want image end %d", got, end)
	}
	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyBackspace})
	if got := root.input.Text(); got != "before  after" {
		t.Fatalf("composer after Backspace = %q, want whole placeholder removed", got)
	}
	if got := len(state.PendingImages.Get()); got != 0 {
		t.Fatalf("pending images after Backspace = %d, want 0", got)
	}
}

func TestRootImagePlaceholderIsDisplayOnlyOnSubmit(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	var submitted string
	var submittedImages []ImageAttachment
	root := NewRootComponent(state, func(text string) {
		submitted = text
		submittedImages = append([]ImageAttachment(nil), state.PendingImages.Get()...)
	}, nil)
	root.input.SetText("describe ")
	root.attachPastedImage("image-data", "image/png")
	if got := root.input.Text(); got != "describe  [Image #1] " {
		t.Fatalf("composer text = %q, want padded image placeholder", got)
	}

	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyEnter})

	if submitted != "describe " {
		t.Fatalf("submitted text = %q, want image placeholder stripped", submitted)
	}
	if len(submittedImages) != 1 || submittedImages[0].Base64 != "image-data" {
		t.Fatalf("submitted images = %+v, want pasted image", submittedImages)
	}
}

func TestRootAdmissionPreservesImagePlaceholderForOrdering(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	var submitted string
	root := NewRootComponentWithAdmission(state, func(text string) bool {
		submitted = text
		return true
	}, nil)
	root.input.SetText("before ")
	root.attachPastedImage("image-data", "image/png")
	root.input.InsertText(" after")
	root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyEnter})

	if submitted != "before  [Image #1]  after" {
		t.Fatalf("admitted text = %q, want inline image marker", submitted)
	}
}

func TestPendingImageClickOpensPrivateDecodedFile(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	root := NewRootComponent(state, nil, nil)
	payload := []byte("image payload")
	root.attachPastedImage(base64.StdEncoding.EncodeToString(payload), "image/png")

	var openedPath string
	root.imageOpener = func(path string) error {
		openedPath = path
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(data) != string(payload) {
			t.Fatalf("opened image data = %q", data)
		}
		if info, err := os.Stat(path); err != nil || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("opened image permissions are not private: info=%v err=%v", info, err)
		}
		return nil
	}

	const width, height = 80, 24
	frame := root.renderAtSize(nil, width, height)
	buffer := gtui.NewBuffer(width, height)
	frame.Render(buffer, width, height)
	x, y := -1, -1
	for row := 0; row < height && x < 0; row++ {
		for column := 0; column < width; column++ {
			if _, ok := root.imageAttachmentAtPoint(column, row); ok {
				x, y = column, row
				break
			}
		}
	}
	if x < 0 || !root.HandleMouse(gtui.MouseEvent{Button: gtui.MouseLeft, Action: gtui.MousePress, X: x, Y: y}) {
		t.Fatal("image tag click was not consumed")
	}
	if openedPath == "" {
		t.Fatal("image opener was not called")
	}
	root.cleanupOpenedImages()
	if _, err := os.Stat(openedPath); !os.IsNotExist(err) {
		t.Fatalf("opened image was not removed during cleanup: %v", err)
	}
}

func TestSentImageTagClickOpensAttachment(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	state.AppendMessage(Message{
		Kind: MsgUser,
		Text: "sent image",
		Images: []ImageAttachment{{
			ID: 9, Base64: base64.StdEncoding.EncodeToString([]byte("sent payload")), MediaType: "image/png",
		}},
	})
	root := NewRootComponent(state, nil, nil)
	var openedPath string
	root.imageOpener = func(path string) error {
		openedPath = path
		return nil
	}

	const width, height = 80, 24
	frame := root.renderAtSize(nil, width, height)
	buffer := gtui.NewBuffer(width, height)
	frame.Render(buffer, width, height)
	clicked := false
	for row := 0; row < height && !clicked; row++ {
		for column := 0; column < width; column++ {
			if _, ok := root.imageAttachmentAtPoint(column, row); !ok {
				continue
			}
			clicked = root.HandleMouse(gtui.MouseEvent{Button: gtui.MouseLeft, Action: gtui.MousePress, X: column, Y: row})
			break
		}
	}
	if !clicked || openedPath == "" {
		t.Fatal("sent image tag did not invoke opener")
	}
	root.cleanupOpenedImages()
	if _, err := os.Stat(openedPath); !os.IsNotExist(err) {
		t.Fatalf("sent image temporary file was not cleaned up: %v", err)
	}
}
