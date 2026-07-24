package tui

import "testing"

func TestRootInputUsesFixedWidthASCIICursor(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	root.input.SetText("abcd")
	root.input.SetCursorPosition(2)
	root.input.Focus()

	rendered := root.input.Render(nil)
	children := rendered.Children()
	if len(children) != 1 {
		t.Fatalf("rendered input children = %d, want 1", len(children))
	}
	if got, want := children[0].Text(), "ab|cd"; got != want {
		t.Fatalf("rendered input = %q, want fixed-width ASCII cursor %q", got, want)
	}
}
