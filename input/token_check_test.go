package input

import (
	"strings"
	"testing"
)

func TestCheckInputSize_BelowThreshold(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"short", "hello world"},
		{"exactly at threshold", strings.Repeat("a", warnCharThreshold)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckInputSize(tc.input)
			if got != "" {
				t.Errorf("expected empty warning for input len %d, got %q", len(tc.input), got)
			}
		})
	}
}

func TestCheckInputSize_AboveThreshold(t *testing.T) {
	// 52000 chars => ~52K chars, ~13K tokens
	input := strings.Repeat("a", 52_000)
	got := CheckInputSize(input)
	if got == "" {
		t.Fatal("expected a warning message, got empty string")
	}
	if !strings.Contains(got, "⚠️") {
		t.Errorf("expected warning emoji in message, got %q", got)
	}
	if !strings.Contains(got, "52K chars") {
		t.Errorf("expected '52K chars' in message, got %q", got)
	}
	if !strings.Contains(got, "13K tokens") {
		t.Errorf("expected '13K tokens' in message, got %q", got)
	}
	if !strings.Contains(got, "[Y/n]") {
		t.Errorf("expected '[Y/n]' in message, got %q", got)
	}
}

func TestCheckInputSize_JustOverThreshold(t *testing.T) {
	input := strings.Repeat("x", warnCharThreshold+1)
	got := CheckInputSize(input)
	if got == "" {
		t.Errorf("expected warning for input len %d", len(input))
	}
}

func TestCheckInputSize_VeryLargeInput(t *testing.T) {
	// 200K chars => ~200K chars, ~50K tokens
	input := strings.Repeat("z", 200_000)
	got := CheckInputSize(input)
	if got == "" {
		t.Fatal("expected warning for 200K char input")
	}
	if !strings.Contains(got, "200K chars") {
		t.Errorf("expected '200K chars' in message, got %q", got)
	}
	if !strings.Contains(got, "50K tokens") {
		t.Errorf("expected '50K tokens' in message, got %q", got)
	}
}
