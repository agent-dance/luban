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

func TestTerminalLayoutHelpersKeepGraphemeClustersAtomic(t *testing.T) {
	if got := textAreaVisibleLines("✏️✏️", 2); got != 2 {
		t.Fatalf("variation-selector graphemes used %d lines, want 2", got)
	}

	lines := wrapTerminalCellLines("✏️A", 2)
	if len(lines) != 2 || lines[0] != "✏️" || lines[1] != "A" {
		t.Fatalf("wrapTerminalCellLines split a grapheme: %#v", lines)
	}

	const mixed = "👩🏽‍💻A"
	if got := terminalCellWidth(mixed); got != 3 {
		t.Fatalf("terminalCellWidth(%q) = %d, want 3", mixed, got)
	}
	if got := truncateTerminalCells(mixed, 3); got != mixed {
		t.Fatalf("truncateTerminalCells changed an exact-width grapheme string: %q", got)
	}
}
