package commands

import (
	"strings"
	"testing"
)

func TestForkCommandOpensConfiguredPicker(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltins(registry)

	command := registry.Find("fork")
	if command == nil {
		t.Fatal("/fork is not registered")
	}

	opened := 0
	err := command.Execute(&Context{
		OpenForkPicker: func() error {
			opened++
			return nil
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if opened != 1 {
		t.Fatalf("fork picker opened %d times, want 1", opened)
	}
}

func TestForkCommandRejectsArgumentsAndMissingPicker(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltins(registry)
	command := registry.Find("fork")

	if err := command.Execute(&Context{OpenForkPicker: func() error { return nil }}, "latest"); err == nil || !strings.Contains(err.Error(), "usage: /fork") {
		t.Fatalf("argument error = %v, want usage", err)
	}
	if err := command.Execute(&Context{}, ""); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("missing picker error = %v", err)
	}
	if err := command.Execute(nil, ""); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("nil context error = %v", err)
	}
}
