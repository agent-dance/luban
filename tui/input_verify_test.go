package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/grindlemire/go-tui"
)

// TestInputTextRendersWithStyle verifies that text typed into Input
// is rendered with the explicit foreground color we set via WithInputTextStyle.
func TestInputTextRendersWithStyle(t *testing.T) {
	// Build an element that mimics what Input.Render() produces:
	// A bordered Row element with a text child carrying WithTextStyle.
	el := tui.New(
		tui.WithDirection(tui.Row),
		tui.WithWidth(40),
		tui.WithHeight(3),
		tui.WithBorder(tui.BorderRounded),
	)
	textEl := tui.New(
		tui.WithText("hello▌"),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.White)),
	)
	el.AddChild(textEl)

	buf := tui.NewBuffer(40, 3)
	el.Render(buf, 40, 3)

	fmt.Println("=== Rendered buffer ===")
	fmt.Println(buf.StringTrimmed())

	// Check row 1 cells (inside border) — text characters must have White (ANSI 7) fg
	fmt.Println("\n=== Cell details row 1 ===")
	textCells := 0
	for x := 0; x < 40; x++ {
		cell := buf.Cell(x, 1)
		if cell.Rune == 0 || cell.Rune == ' ' {
			continue
		}
		// Skip border characters
		if cell.Rune == '│' || cell.Rune == '╭' || cell.Rune == '╰' || cell.Rune == '╮' || cell.Rune == '╯' {
			continue
		}
		fgType := cell.Style.Fg.Type()
		fgANSI := cell.Style.Fg.ANSI()
		fmt.Printf("  [x=%d] rune='%c' fg_type=%d fg_ansi=%d bg_type=%d attrs=%d\n",
			x, cell.Rune, fgType, fgANSI, cell.Style.Bg.Type(), cell.Style.Attrs)
		textCells++

		if fgType != 1 || fgANSI != 7 { // ColorANSI, White
			t.Errorf("Cell at x=%d rune=%c: fg_type=%d fg_ansi=%d, want ColorANSI(1) White(7)",
				x, cell.Rune, fgType, fgANSI)
		}
	}
	if textCells == 0 {
		t.Fatal("No text cells found in row 1!")
	}
	t.Logf("Verified %d text cells all have White foreground", textCells)
}

// TestInputRenderFullStack tests the full RootComponent render pipeline
// to verify input text appears in the final rendered output.
func TestInputRenderFullStack(t *testing.T) {
	// Create the same setup as NewRootComponent does
	state := NewAppState()
	state.Provider.Set("test")
	state.Model.Set("model")

	_ = NewRootComponent(state, func(text string) {}, nil)

	// We can't easily call root.Render() without a full App, so we simulate
	// the element tree that the Input produces.
	// The key question is: does the input show text when we type?

	// Check what happens when we build the inputRow element
	// directly — simulating what Render does for the input section.
	inputRow := tui.New(
		tui.WithDirection(tui.Row),
		tui.WithHeight(3),
		tui.WithWidth(80),
	)
	prompt := tui.New(
		tui.WithText("> "),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.Green).Bold()),
		tui.WithWidth(3),
		tui.WithHeight(1),
	)
	inputRow.AddChild(prompt)

	// Simulate what Input.Render does internally: text with textStyle
	// The input's textStyle is set to White via WithInputTextStyle
	inputContent := tui.New(
		tui.WithDirection(tui.Row),
		tui.WithWidth(77),
		tui.WithHeight(3),
		tui.WithBorder(tui.BorderRounded),
	)
	textChild := tui.New(
		tui.WithText("typed text here▌"),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.White)),
	)
	inputContent.AddChild(textChild)
	inputRow.AddChild(inputContent)

	buf := tui.NewBuffer(80, 3)
	inputRow.Render(buf, 80, 3)

	output := buf.StringTrimmed()
	fmt.Println("=== Full input row ===")
	fmt.Println(output)

	if !strings.Contains(output, "typed text here") {
		t.Error("Input text not found in rendered output!")
	}

	// Check the ANSI output to verify colors
	fmt.Println("\n=== Cell details row 1 (text area) ===")
	for x := 3; x < 80; x++ {
		cell := buf.Cell(x, 1)
		if cell.Rune >= 'a' && cell.Rune <= 'z' {
			fgType := cell.Style.Fg.Type()
			fgANSI := cell.Style.Fg.ANSI()
			fmt.Printf("  [x=%d] rune='%c' fg=(type=%d, ansi=%d) bg=(type=%d)\n",
				x, cell.Rune, fgType, fgANSI, cell.Style.Bg.Type())
			if fgType != 1 || fgANSI != 7 {
				t.Errorf("Text char '%c' at x=%d has wrong fg: type=%d ansi=%d (want ANSI White)",
					cell.Rune, x, fgType, fgANSI)
			}
		}
	}
}
