package commands_test

import (
	"testing"

	"github.com/agent-dance/luban/commands"
)

// TestPasteCmd_Name verifies that PasteCmd reports the correct command name.
func TestPasteCmd_Name(t *testing.T) {
	c := &commands.PasteCmd{}
	if got := c.Name(); got != "paste" {
		t.Fatalf("Name() = %q, want %q", got, "paste")
	}
}

// TestPasteCmd_Description verifies that the description is non-empty.
func TestPasteCmd_Description(t *testing.T) {
	c := &commands.PasteCmd{}
	if c.Description() == "" {
		t.Fatal("Description() must be non-empty")
	}
}

// TestPasteCmd_Execute_NoClipboard verifies that Execute handles the
// "no clipboard image" case gracefully: it should return nil and leave
// ImageBlock unset.
//
// On CI (and most test environments) the clipboard is unavailable, so
// HasClipboardImage() returns false and Execute returns early without error.
func TestPasteCmd_Execute_NoClipboard(t *testing.T) {
	c := &commands.PasteCmd{}
	var events []string
	ctx := &commands.Context{
		QueryLoop: &stubQL{},
		OnEvent:   func(s string) { events = append(events, s) },
	}

	if err := c.Execute(ctx, ""); err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
	if c.ImageBlock != nil {
		t.Fatal("ImageBlock should be nil when no clipboard image is present")
	}
}
