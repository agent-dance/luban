package tui

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/agent-dance/luban/types"
	"github.com/agent-dance/luban/ui"
	gtui "github.com/grindlemire/go-tui"
)

func TestTUIDenialDoesNotMutateComposer(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session-a")
	state.SessionEpoch.Set(1)
	root := NewRootComponentWithAdmission(state, func(string) bool { return false }, nil)
	root.input.SetText("draft must survive")
	root.input.SetCursorPosition(5)
	root.input.SelectAll()
	root.input.Focus()

	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	ctx := ui.ToolEventContext{SessionID: "session-a", SessionEpoch: 1, TurnID: "turn-1", ActorID: "assistant", WorkUnitID: "work-1"}
	renderer.RenderToolCall(ctx, types.ToolUseBlock{ID: "bash-1", Name: "Bash", Input: map[string]any{"command": `rm -rf "$tmp"`}})
	renderer.RenderToolResult(ctx, types.ToolResultBlock{ToolUseID: "bash-1", Content: "policy copy", IsError: true, Outcome: types.ToolOutcomeDenied})

	if got := root.input.Text(); got != "draft must survive" {
		t.Fatalf("denial mutated composer text: %q", got)
	}
	if got := root.input.SelectedText(); got != "draft must survive" {
		t.Fatalf("denial mutated composer selection: %q", got)
	}
	if !root.input.IsFocused() {
		t.Fatal("denial changed composer focus")
	}
}

func TestTUIConcurrentDenialsPreserveTerminalBuffer(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session-a")
	state.SessionEpoch.Set(1)
	root := NewRootComponentWithAdmission(state, func(string) bool { return false }, nil)
	root.input.SetText("COMPOSER_SENTINEL")
	root.input.Focus()

	const denials = 50
	queued := make(chan func(), denials)
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { queued <- fn; return true }}
	ctx := ui.ToolEventContext{SessionID: "session-a", SessionEpoch: 1, TurnID: "turn-1", ActorID: "assistant"}
	for index := 0; index < denials; index++ {
		call := types.ToolUseBlock{ID: "bash-" + itoa(index), Name: "Bash", Input: map[string]any{"command": "dangerous"}}
		if err := state.ApplyToolCall(toolObservationContext(ctx, OutcomeRunning), call); err != nil {
			t.Fatal(err)
		}
	}

	const concurrency = 10
	jobs := make(chan int, denials)
	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(concurrency)
	for worker := 0; worker < concurrency; worker++ {
		go func() {
			defer workers.Done()
			<-start
			for index := range jobs {
				renderer.RenderToolResult(ctx, types.ToolResultBlock{
					ToolUseID: "bash-" + itoa(index), Content: "denied", IsError: true, Outcome: types.ToolOutcomeDenied,
				})
			}
		}()
	}
	for index := 0; index < denials; index++ {
		jobs <- index
	}
	close(jobs)
	close(start)
	workers.Wait()
	for index := 0; index < denials; index++ {
		(<-queued)()
	}

	buffer := gtui.NewBuffer(100, 30)
	root.renderAtSize(nil, 100, 30).Render(buffer, 100, 30)
	frame := buffer.String()
	composerRow := ""
	for _, row := range strings.Split(frame, "\n") {
		if strings.Contains(row, "COMPOSER_SENTINEL") {
			composerRow = row
			break
		}
	}
	if composerRow == "" {
		t.Fatalf("composer disappeared after concurrent denials:\n%s", frame)
	}
	if strings.Contains(strings.ToLower(composerRow), "denied") || strings.Contains(composerRow, "Bash") {
		t.Fatalf("denial diagnostic contaminated composer row: %q", composerRow)
	}

	var ansi bytes.Buffer
	terminal := gtui.NewANSITerminalWithCaps(&ansi, strings.NewReader(""), gtui.Capabilities{
		Colors:  gtui.Color256,
		Unicode: true,
	})
	changes := make([]gtui.CellChange, 0, buffer.Width()*buffer.Height())
	for y := 0; y < buffer.Height(); y++ {
		for x := 0; x < buffer.Width(); x++ {
			changes = append(changes, gtui.CellChange{X: x, Y: y, Cell: buffer.Cell(x, y)})
		}
	}
	terminal.Flush(changes)

	physical := newANSIPhysicalScreen(buffer.Width(), buffer.Height())
	physical.parse(ansi.Bytes())
	physical.requireEqualBuffer(t, buffer)
}

// ansiPhysicalScreen interprets the ANSI byte stream emitted by ANSITerminal.
// It deliberately sits above the CellChange layer so this regression catches
// cursor movement, wrapping, control-character, and wide-cell divergence
// between the renderer's back buffer and what a terminal would display.
type ansiPhysicalScreen struct {
	width, height int
	cells         [][]rune
	row, col      int
	wrapPending   bool
}

func newANSIPhysicalScreen(width, height int) *ansiPhysicalScreen {
	cells := make([][]rune, height)
	for row := range cells {
		cells[row] = make([]rune, width)
		for col := range cells[row] {
			cells[row][col] = ' '
		}
	}
	return &ansiPhysicalScreen{width: width, height: height, cells: cells}
}

func (s *ansiPhysicalScreen) parse(data []byte) {
	for index := 0; index < len(data); {
		if data[index] == '\x1b' && index+1 < len(data) && data[index+1] == '[' {
			index += 2
			start := index
			for index < len(data) && (data[index] < 0x40 || data[index] > 0x7e) {
				index++
			}
			if index >= len(data) {
				return
			}
			s.handleCSI(string(data[start:index]), data[index])
			index++
			continue
		}
		r, size := utf8.DecodeRune(data[index:])
		if size == 0 {
			return
		}
		s.writeRune(r)
		index += size
	}
}

func (s *ansiPhysicalScreen) handleCSI(params string, final byte) {
	s.wrapPending = false
	switch final {
	case 'H', 'f':
		row, col := 1, 1
		parts := strings.Split(params, ";")
		if len(parts) > 0 {
			row = positiveANSIParam(parts[0], row)
		}
		if len(parts) > 1 {
			col = positiveANSIParam(parts[1], col)
		}
		s.row = clampTerminalCoordinate(row-1, s.height)
		s.col = clampTerminalCoordinate(col-1, s.width)
	case 'J':
		if params == "2" || params == "3" {
			for row := range s.cells {
				for col := range s.cells[row] {
					s.cells[row][col] = ' '
				}
			}
		}
	case 'm':
		// Style does not change the physical character grid.
	}
}

func positiveANSIParam(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	result := 0
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return fallback
		}
		result = result*10 + int(digit-'0')
	}
	if result <= 0 {
		return fallback
	}
	return result
}

func clampTerminalCoordinate(value, size int) int {
	if size <= 0 || value < 0 {
		return 0
	}
	if value >= size {
		return size - 1
	}
	return value
}

func (s *ansiPhysicalScreen) writeRune(r rune) {
	if s.width == 0 || s.height == 0 {
		return
	}
	if s.wrapPending {
		if s.row < s.height-1 {
			s.row++
		}
		s.col = 0
		s.wrapPending = false
	}
	switch r {
	case '\r':
		s.col = 0
		return
	case '\n':
		if s.row < s.height-1 {
			s.row++
		}
		return
	case '\t':
		next := ((s.col / 8) + 1) * 8
		if next >= s.width {
			s.col = s.width - 1
			s.wrapPending = true
		} else {
			s.col = next
		}
		return
	}

	width := gtui.RuneWidth(r)
	s.cells[s.row][s.col] = r
	for offset := 1; offset < width && s.col+offset < s.width; offset++ {
		s.cells[s.row][s.col+offset] = ' '
	}
	if s.col+width >= s.width {
		s.col = s.width - 1
		s.wrapPending = true
		return
	}
	s.col += width
}

func (s *ansiPhysicalScreen) requireEqualBuffer(t *testing.T, buffer *gtui.Buffer) {
	t.Helper()
	for row := 0; row < buffer.Height(); row++ {
		for col := 0; col < buffer.Width(); col++ {
			expected := buffer.Cell(col, row).Rune
			if expected == 0 || buffer.Cell(col, row).IsContinuation() {
				expected = ' '
			}
			if actual := s.cells[row][col]; actual != expected {
				t.Fatalf("ANSI physical screen diverged from back buffer at (%d,%d): got %q, want %q", col, row, actual, expected)
			}
		}
	}
}

func TestTUIStaleDenialDroppedAfterSessionSwitch(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session-a")
	state.SessionEpoch.Set(1)
	var queued func()
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { queued = fn; return true }}
	renderer.RenderToolResult(ui.ToolEventContext{SessionID: "session-a", SessionEpoch: 1}, types.ToolResultBlock{
		ToolUseID: "bash-stale", Content: "stale denial", IsError: true, Outcome: types.ToolOutcomeDenied,
	})
	if queued == nil {
		t.Fatal("denial was not queued")
	}
	state.SessionID.Set("session-b")
	state.SessionEpoch.Set(2)
	queued()
	if got := state.Messages.Get(); len(got) != 0 {
		t.Fatalf("stale denial crossed session epoch: %+v", got)
	}
	if got := state.Observations.Snapshot(); len(got) != 0 {
		t.Fatalf("stale denial created an observation in the new session: %+v", got)
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	position := len(digits)
	for value > 0 {
		position--
		digits[position] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[position:])
}
