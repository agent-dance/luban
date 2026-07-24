package tui

import (
	"strings"
	"testing"
)

type emulatorWriter struct {
	emu *EmulatorTerminal
}

func (w emulatorWriter) Write(p []byte) (int, error) {
	return w.emu.WriteDirect(p)
}

func trimmedViewportPrefix(buf *Buffer, width, height, visibleWidth int) string {
	var rows []string
	for y := 0; y < height; y++ {
		var sb strings.Builder
		for x := 0; x < visibleWidth && x < width; x++ {
			cell := buf.Cell(x, y)
			if cell.IsContinuation() {
				continue
			}
			if cell.Rune == 0 {
				sb.WriteRune(' ')
			} else {
				sb.WriteRune(cell.Rune)
			}
		}
		rows = append(rows, strings.TrimRight(sb.String(), " "))
	}
	return strings.Join(rows, "\n")
}

func trimmedScreenPrefix(emu *EmulatorTerminal, visibleWidth int) string {
	var rows []string
	for y := 0; y < emu.height; y++ {
		row := []rune(emu.ScreenRow(y))
		if len(row) > visibleWidth {
			row = row[:visibleWidth]
		}
		rows = append(rows, strings.TrimRight(string(row), " "))
	}
	return strings.Join(rows, "\n")
}

func TestANSITerminal_DiffRender_MatchesBufferForScrollableCodeLikeContent(t *testing.T) {
	const width = 40
	const height = 6

	codeLine := func(parts ...StyledSpan) *Element {
		return New(
			WithStyledSpans(parts),
			WithWidthPercent(100),
			WithWrap(false),
		)
	}

	scrollable := New(
		WithSize(width, height),
		WithScrollable(ScrollVertical),
		WithDirection(Column),
	)
	scrollable.AddChild(codeLine(
		StyledSpan{Text: "```go", Style: NewStyle().Foreground(BrightBlack)},
	))
	scrollable.AddChild(codeLine(
		StyledSpan{Text: "package", Style: NewStyle().Foreground(BrightMagenta)},
		StyledSpan{Text: " main", Style: NewStyle()},
	))
	scrollable.AddChild(codeLine(
		StyledSpan{Text: "import", Style: NewStyle().Foreground(BrightMagenta)},
		StyledSpan{Text: " \"fmt\"", Style: NewStyle().Foreground(BrightGreen)},
	))
	scrollable.AddChild(codeLine(
		StyledSpan{Text: "func", Style: NewStyle().Foreground(BrightBlue)},
		StyledSpan{Text: " main() {", Style: NewStyle()},
	))
	scrollable.AddChild(codeLine(
		StyledSpan{Text: "    fmt.Println", Style: NewStyle().Foreground(Cyan)},
		StyledSpan{Text: "(\"hello\")", Style: NewStyle().Foreground(Yellow)},
	))
	scrollable.AddChild(codeLine(
		StyledSpan{Text: "}", Style: NewStyle()},
	))
	scrollable.AddChild(codeLine(
		StyledSpan{Text: "```", Style: NewStyle().Foreground(BrightBlack)},
	))
	scrollable.AddChild(codeLine(
		StyledSpan{Text: "after block", Style: NewStyle()},
	))

	emu := NewEmulatorTerminal(width, height)
	term := NewANSITerminalWithCaps(emulatorWriter{emu: emu}, strings.NewReader(""), emu.Caps())
	buf := NewBuffer(width, height)

	renderFrame := func(full bool) {
		buf.Clear()
		scrollable.Render(buf, width, height)
		if full {
			RenderFull(term, buf)
		} else {
			Render(term, buf)
		}
		// Ignore the last column because the scrollbar uses Unicode glyphs and
		// the emulator test helper stores bytes, not decoded runes.
		if got, want := trimmedScreenPrefix(emu, width-1), trimmedViewportPrefix(buf, width, height, width-1); got != want {
			t.Fatalf("terminal screen diverged from buffer\n--- got ---\n%s\n--- want ---\n%s", got, want)
		}
	}

	renderFrame(true)

	for _, y := range []int{1, 2, 3, 2, 0} {
		scrollable.ScrollTo(0, y)
		renderFrame(false)
	}
}
