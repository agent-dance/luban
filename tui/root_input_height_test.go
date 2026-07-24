package tui

import "testing"

func TestTextAreaVisibleLinesCountsWideRunesByDisplayWidth(t *testing.T) {
	text := "你看我多行展示的时候这个输入区域"
	if got := textAreaVisibleLines(text, 10); got != 4 {
		t.Fatalf("textAreaVisibleLines(%q, 10) = %d, want 4", text, got)
	}
}

func TestTextAreaVisibleLinesPreservesExplicitNewlines(t *testing.T) {
	text := "abc\n\n你好世界"
	if got := textAreaVisibleLines(text, 4); got != 4 {
		t.Fatalf("textAreaVisibleLines(%q, 4) = %d, want 4", text, got)
	}
}

func TestTextAreaVisibleLinesClampsEmptyToOneLine(t *testing.T) {
	if got := textAreaVisibleLines("", 20); got != 1 {
		t.Fatalf("textAreaVisibleLines(empty) = %d, want 1", got)
	}
}
