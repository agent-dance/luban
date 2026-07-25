package main

import (
	"os"
	"strings"
	"testing"
)

func TestCommandEntrypointOwnsTheOnlyProcessExit(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if strings.Count(text, "os.Exit(") != 1 || !strings.Contains(text, "os.Exit(app.Run())") {
		t.Fatalf("command entrypoint must terminate exactly once with app.Run: %s", text)
	}
}
