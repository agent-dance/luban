package tui

import (
	"strings"
	"testing"
)

type ansiBytesRecorder struct {
	strings.Builder
}

type cursorScreen struct {
	width, height int
	screen        [][]rune
	row, col      int
	wrapPending   bool
}

func newCursorScreen(width, height int) *cursorScreen {
	screen := make([][]rune, height)
	for y := range screen {
		screen[y] = make([]rune, width)
		for x := range screen[y] {
			screen[y][x] = ' '
		}
	}
	return &cursorScreen{
		width:  width,
		height: height,
		screen: screen,
	}
}

func (s *cursorScreen) parseANSI(data string) {
	for i := 0; i < len(data); {
		if data[i] == '\x1b' && i+1 < len(data) && data[i+1] == '[' {
			i += 2
			start := i
			for i < len(data) && (data[i] < 0x40 || data[i] > 0x7e) {
				i++
			}
			if i >= len(data) {
				return
			}
			s.handleCSI(data[start:i], data[i])
			i++
			continue
		}
		r := rune(data[i])
		if r < 0x80 {
			s.writeRune(r)
			i++
			continue
		}
		// Tests below only exercise ASCII payloads.
		i++
	}
}

func (s *cursorScreen) handleCSI(params string, final byte) {
	s.wrapPending = false
	switch final {
	case 'H':
		row, col := 1, 1
		if params != "" {
			var p []int
			for _, part := range strings.Split(params, ";") {
				if part == "" {
					p = append(p, 0)
					continue
				}
				n := 0
				for _, ch := range part {
					n = n*10 + int(ch-'0')
				}
				p = append(p, n)
			}
			if len(p) > 0 && p[0] > 0 {
				row = p[0]
			}
			if len(p) > 1 && p[1] > 0 {
				col = p[1]
			}
		}
		s.row = clampInt(row-1, 0, s.height-1)
		s.col = clampInt(col-1, 0, s.width-1)
	case 'm':
		// Ignore style changes in these tests.
	}
}

func (s *cursorScreen) writeRune(r rune) {
	if s.wrapPending {
		if s.row < s.height-1 {
			s.row++
		}
		s.col = 0
		s.wrapPending = false
	}

	switch r {
	case '\t':
		nextTab := ((s.col / 8) + 1) * 8
		if nextTab >= s.width {
			s.col = s.width - 1
			s.wrapPending = true
			return
		}
		s.col = nextTab
		return
	case '\r':
		s.col = 0
		return
	case '\n':
		if s.row < s.height-1 {
			s.row++
		}
		return
	}

	if s.row < 0 || s.row >= s.height || s.col < 0 || s.col >= s.width {
		return
	}
	s.screen[s.row][s.col] = r
	if s.col == s.width-1 {
		s.wrapPending = true
		return
	}
	s.col++
}

func (s *cursorScreen) rowString(row int) string {
	return strings.TrimRight(string(s.screen[row]), " ")
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func TestANSITerminalFlush_TabCharacterMovesCursorLikeTerminal(t *testing.T) {
	rec := &ansiBytesRecorder{}
	term := NewANSITerminalWithCaps(rec, strings.NewReader(""), Capabilities{Unicode: true})

	changes := []CellChange{
		{X: 0, Y: 0, Cell: NewCell('\t', NewStyle())},
		{X: 1, Y: 0, Cell: NewCell('f', NewStyle())},
		{X: 2, Y: 0, Cell: NewCell('m', NewStyle())},
		{X: 3, Y: 0, Cell: NewCell('t', NewStyle())},
	}
	term.Flush(changes)

	screen := newCursorScreen(16, 2)
	screen.parseANSI(rec.String())

	if got := screen.rowString(0); got == "\tfmt" {
		t.Fatal("expected terminal semantics to diverge from buffer when a raw tab is flushed")
	}
	if got := screen.rowString(0); got != "        fmt" {
		t.Fatalf("terminal row = %q, want %q", got, "        fmt")
	}
}

func TestANSITerminalFlush_LastColumnWrapsOnNextPrintable(t *testing.T) {
	rec := &ansiBytesRecorder{}
	term := NewANSITerminalWithCaps(rec, strings.NewReader(""), Capabilities{Unicode: true})

	changes := []CellChange{
		{X: 4, Y: 0, Cell: NewCell('e', NewStyle())},
		{X: 0, Y: 1, Cell: NewCell('x', NewStyle())},
	}
	term.Flush(changes)

	screen := newCursorScreen(5, 3)
	screen.parseANSI(rec.String())

	if got := screen.rowString(0); got != "    e" {
		t.Fatalf("row 0 = %q, want %q", got, "    e")
	}
	if got := screen.rowString(1); got != "x" {
		t.Fatalf("row 1 = %q, want %q", got, "x")
	}
}
