package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/grindlemire/go-tui"
)

// TestInputLayoutInRealTUI simulates the exact layout from RootComponent.Render()
// to check if the input text ends up in the correct screen position.
// This was the test that discovered the original bug: inputRow used
// WithDirection(Row) but lacked WithDisplay(DisplayFlex), so the layout
// engine treated it as Column (DisplayBlock default), pushing the input
// element below the visible screen area.
func TestInputLayoutInRealTUI(t *testing.T) {
	termWidth := 120
	termHeight := 40

	// Recreate the exact layout from RootComponent.Render()
	root := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithHeightPercent(100),
		tui.WithWidthPercent(100),
	)

	// Banner (3 rows)
	banner := tui.New(
		tui.WithBorder(tui.BorderRounded),
		tui.WithBorderStyle(tui.NewStyle().Foreground(tui.Cyan)),
		tui.WithPaddingTRBL(0, 1, 0, 1),
		tui.WithHeight(3),
		tui.WithWidthPercent(100),
	)
	banner.AddChild(tui.New(
		tui.WithText("LUBAN Code — test/model"),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.Cyan).Bold()),
	))
	root.AddChild(banner)

	// Message area (flex grow)
	msgArea := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithFlexGrow(1),
		tui.WithWidthPercent(100),
		tui.WithScrollable(tui.ScrollVertical),
		tui.WithScrollOffset(0, 0),
		tui.WithGap(0),
	)
	root.AddChild(msgArea)

	// Status bar (1 row)
	status := tui.New(
		tui.WithText(" "),
		tui.WithTextStyle(tui.NewStyle().Dim()),
		tui.WithHeight(1),
		tui.WithWidthPercent(100),
	)
	root.AddChild(status)

	// Input area with external border container (no border on TextArea itself).
	// Simulates 1 line of text: content height=1 + border=2 = total 3 rows.
	inputRowHeight := 3 // 1 line content + 2 border
	inputBorder := tui.New(
		tui.WithDisplay(tui.DisplayFlex),
		tui.WithDirection(tui.Row),
		tui.WithHeight(inputRowHeight),
		tui.WithWidthPercent(100),
		tui.WithBorder(tui.BorderRounded),
		tui.WithBorderStyle(tui.NewStyle().Foreground(tui.Cyan)),
	)
	prompt := tui.New(
		tui.WithText("> "),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.Green).Bold()),
		tui.WithWidth(3),
		tui.WithHeight(1), // content height only, border is on parent
	)
	inputBorder.AddChild(prompt)

	// Simulate what TextArea.Render() produces (no border, 1 line)
	inputWidth := termWidth - 3 - 2 // minus prompt minus border
	inputEl := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithHeight(1),
		tui.WithWidth(inputWidth),
		tui.WithFocusable(true),
	)
	textEl := tui.New(
		tui.WithText("user typed text here"),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.White)),
	)
	inputEl.AddChild(textEl)
	inputBorder.AddChild(inputEl)
	root.AddChild(inputBorder)

	// Render to buffer
	buf := tui.NewBuffer(termWidth, termHeight)
	root.Render(buf, termWidth, termHeight)

	// Debug: check element rects after layout
	t.Logf("=== Element Rects ===")
	t.Logf("root:        %v", root.Rect())
	t.Logf("banner:      %v", banner.Rect())
	t.Logf("msgArea:     %v", msgArea.Rect())
	t.Logf("status:      %v", status.Rect())
	t.Logf("inputBorder: %v", inputBorder.Rect())
	t.Logf("prompt:      %v", prompt.Rect())
	t.Logf("inputEl:     %v", inputEl.Rect())
	t.Logf("textEl:      %v", textEl.Rect())

	// Verify inputBorder is within visible bounds
	inputBorderRect := inputBorder.Rect()
	if inputBorderRect.Y+inputBorderRect.Height > termHeight {
		t.Errorf("inputBorder overflows screen: y=%d height=%d, screen height=%d",
			inputBorderRect.Y, inputBorderRect.Height, termHeight)
	}

	// Verify inputEl is within the border container (accounting for border offset)
	inputElRect := inputEl.Rect()
	if inputElRect.X < 3 {
		t.Errorf("inputEl should start at x>=3 (after prompt), got x=%d", inputElRect.X)
	}

	// Verify inputEl is within visible bounds
	if inputElRect.Y+inputElRect.Height > termHeight {
		t.Errorf("inputEl overflows screen: y=%d height=%d, screen height=%d",
			inputElRect.Y, inputElRect.Height, termHeight)
	}

	// Verify textEl is within visible bounds
	textElRect := textEl.Rect()
	if textElRect.Y >= termHeight {
		t.Errorf("textEl is off-screen: y=%d, screen height=%d", textElRect.Y, termHeight)
	}

	// Find "user typed" text in the buffer
	found := false
	for y := 0; y < termHeight; y++ {
		for x := 0; x < termWidth-4; x++ {
			cell := buf.Cell(x, y)
			if cell.Rune == 'u' {
				word := ""
				for dx := 0; dx < 4 && x+dx < termWidth; dx++ {
					c := buf.Cell(x+dx, y)
					word += string(c.Rune)
				}
				if word == "user" {
					t.Logf("Found 'user' at row=%d col=%d (within visible area)", y, x)
					// Verify it has white foreground
					for dx := 0; dx < 4; dx++ {
						c := buf.Cell(x+dx, y)
						if c.Style.Fg.Type() != 1 || c.Style.Fg.ANSI() != 7 {
							t.Errorf("char '%c' at [%d,%d] expected white (type=1,ansi=7), got type=%d,ansi=%d",
								c.Rune, x+dx, y, c.Style.Fg.Type(), c.Style.Fg.ANSI())
						}
					}
					found = true
					break
				}
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Error("Could not find 'user typed text here' in rendered output!")
		// Dump last few rows for debugging
		t.Log("=== Last 5 rows ===")
		for y := termHeight - 5; y < termHeight; y++ {
			var line strings.Builder
			for x := 0; x < min(termWidth, 80); x++ {
				c := buf.Cell(x, y)
				if c.Rune == 0 {
					line.WriteByte(' ')
				} else {
					fmt.Fprintf(&line, "%c", c.Rune)
				}
			}
			t.Logf("row %d: %s", y, line.String())
		}
	}
}
