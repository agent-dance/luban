package tui

import (
	"strings"
	"testing"

	gtui "github.com/grindlemire/go-tui"
)

func firePermissionBinding(t *testing.T, root *RootComponent, key gtui.Key, r rune) {
	t.Helper()
	for _, binding := range root.KeyMap() {
		if r != 0 {
			if binding.Pattern.Rune == r {
				binding.Handler(gtui.KeyEvent{Key: gtui.KeyRune, Rune: r})
				return
			}
			continue
		}
		if binding.Pattern.Key == key && !binding.Pattern.AnyKey {
			binding.Handler(gtui.KeyEvent{Key: key})
			return
		}
	}
	t.Fatalf("permission binding not found for key=%v rune=%q", key, r)
}

func readPermissionResponse(t *testing.T, state *AppState) string {
	t.Helper()
	select {
	case resp := <-state.PermResp:
		return resp
	default:
		t.Fatal("expected permission response")
		return ""
	}
}

func TestPermissionChoiceNavigationAndResponse(t *testing.T) {
	if got := nextPermissionChoice(permissionChoiceAllow, -1); got != permissionChoiceAlways {
		t.Fatalf("previous from allow = %d, want always", got)
	}
	if got := nextPermissionChoice(permissionChoiceAlways, 1); got != permissionChoiceAllow {
		t.Fatalf("next from always = %d, want allow", got)
	}
	if got := permissionChoiceResponse(permissionChoiceAlways); got != "a" {
		t.Fatalf("permissionChoiceResponse(always) = %q, want a", got)
	}
}

func TestPermissionDialogConsumesInputFocusKeys(t *testing.T) {
	state := NewAppState()
	root := NewRootComponent(state, nil, nil)

	if root.input.KeyMap() == nil {
		t.Fatal("expected text area keymap before permission modal opens")
	}

	state.PermReq.Set(&PermissionReq{ToolName: "Bash"})
	if root.input.KeyMap() != nil {
		t.Fatal("expected permission modal to suppress text area keymap")
	}
}

func TestPermissionDialogKeyMapNavigatesAndConfirmsSelection(t *testing.T) {
	state := NewAppState()
	state.PermReq.Set(&PermissionReq{ToolName: "Bash"})
	root := NewRootComponent(state, nil, nil)

	firePermissionBinding(t, root, gtui.KeyRight, 0)
	if got := state.PermSelected.Get(); got != permissionChoiceDeny {
		t.Fatalf("right selected %d, want deny", got)
	}
	firePermissionBinding(t, root, gtui.KeyDown, 0)
	if got := state.PermSelected.Get(); got != permissionChoiceAlways {
		t.Fatalf("down selected %d, want always", got)
	}
	firePermissionBinding(t, root, gtui.KeyEnter, 0)
	if got := readPermissionResponse(t, state); got != "a" {
		t.Fatalf("enter response = %q, want a", got)
	}
}

func TestPermissionDialogDirectShortcutBypassesSelection(t *testing.T) {
	state := NewAppState()
	state.PermReq.Set(&PermissionReq{ToolName: "Bash"})
	state.PermSelected.Set(permissionChoiceAlways)
	root := NewRootComponent(state, nil, nil)

	firePermissionBinding(t, root, 0, 'n')
	if got := readPermissionResponse(t, state); got != "n" {
		t.Fatalf("n response = %q, want n", got)
	}
}

func TestPermissionDialogFixedHeightAndCompactPreview(t *testing.T) {
	state := NewAppState()
	root := NewRootComponent(state, nil, nil)
	root.termWidth = 80

	dialog := root.renderPermissionDialog(&PermissionReq{
		ToolName: "SendUserMessage",
		Input: map[string]any{
			"message":     "line one\nline two with enough text to be truncated because permission dialogs must stay compact",
			"attachments": []string{},
		},
		RiskLevel: 2,
	})

	if got := dialog.HeightForWidth(100); got != permissionDialogRows {
		t.Fatalf("dialog height = %d, want %d", got, permissionDialogRows)
	}

	text := collectElementText(dialog)
	if strings.Count(text, "Input:") != 1 {
		t.Fatalf("expected a single compact input preview row, got %q", text)
	}
	if strings.Contains(text, "line one\nline two") {
		t.Fatalf("expected newlines in input preview to be collapsed, got %q", text)
	}
}

func TestPermissionDialogSelectedActionUsesReadableOutline(t *testing.T) {
	el := permissionActionElement(permissionChoiceAllow, "[y]", "Allow", gtui.Green, permissionChoiceAllow)

	text := collectElementText(el)
	if text != "> [y] Allow <" {
		t.Fatalf("selected action text = %q, want readable outline", text)
	}
}
