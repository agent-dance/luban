package input

import (
	"fmt"
	"io"
	"strings"
)

const (
	// pasteDetectChars is the minimum number of characters in a single line
	// that triggers paste-detection mode.
	pasteDetectChars = 10
)

// lineFetcher is the function signature used by MultilineReader to request
// individual raw lines from an underlying source (e.g. readline).
type lineFetcher func(prompt string) (string, error)

// MultilineReader wraps an inner lineFetcher and adds:
//   - Paste detection: if a line arrives with >pasteDetectChars chars AND
//     another line follows quickly, switch to multiline accumulation.
//   - Multiline accumulation: collect lines until a blank line or io.EOF /
//     Ctrl-D is received.
//   - A "[line N] " indicator prompt displayed during multiline mode.
//
// MultilineReader implements the legacy line-reader abstraction used by the
// previous readline-based interactive path.
type MultilineReader struct {
	fetch       lineFetcher
	primaryPrompt string
}

// NewMultilineReader returns a MultilineReader backed by fetch.
// primaryPrompt is shown for the first line; subsequent lines show a counter.
func NewMultilineReader(fetch lineFetcher, primaryPrompt string) *MultilineReader {
	return &MultilineReader{
		fetch:         fetch,
		primaryPrompt: primaryPrompt,
	}
}

// Readline implements LineReader. It reads one logical (possibly multiline)
// input block from the terminal.
//
// Algorithm:
//  1. Read the first line with the primary prompt.
//  2. If the first line is long (>pasteDetectChars), treat it as a potential
//     paste and switch to multiline accumulation mode immediately.
//  3. In multiline mode, keep reading lines (showing "[line N] ") until:
//     - a blank line is entered, or
//     - io.EOF / Ctrl-D is signalled on the inner reader.
//  4. Return the accumulated text, joining lines with newlines.
//
// No goroutines are spawned; readline is only ever called from this goroutine.
func (m *MultilineReader) Readline() (string, error) {
	// --- Step 1: read first line ---
	first, err := m.fetch(m.primaryPrompt)
	if err != nil {
		return first, err
	}

	// --- Step 2: paste heuristic ---
	// If the first line is short, it was almost certainly typed (not pasted).
	// Return it immediately without entering multiline mode.
	if len(first) <= pasteDetectChars {
		return first, nil
	}

	// Long input detected — treat as start of a paste / multiline block and
	// continue accumulating lines until a blank line or EOF.
	lines := []string{first}
	rest, accErr := m.accumulate(lines)
	if accErr != nil && accErr != io.EOF {
		return strings.Join(rest, "\n"), accErr
	}
	return strings.Join(rest, "\n"), nil
}

// accumulate continues reading lines into existing and returns the full slice.
// It stops on a blank line or io.EOF.
func (m *MultilineReader) accumulate(existing []string) ([]string, error) {
	lines := existing
	for {
		lineNum := len(lines) + 1
		prompt := m.linePrompt(lineNum)
		line, err := m.fetch(prompt)
		if err == io.EOF {
			return lines, io.EOF
		}
		if err != nil {
			return lines, err
		}
		// Blank line terminates multiline input.
		if strings.TrimSpace(line) == "" {
			return lines, nil
		}
		lines = append(lines, line)
	}
}

// linePrompt returns the prompt shown for line number n during multiline mode.
// Example: "[line 3] "
func (m *MultilineReader) linePrompt(n int) string {
	return fmt.Sprintf("[line %d] ", n)
}

// Close is a no-op for MultilineReader; it satisfies the LineReader interface.
// The underlying reader is closed via the outer Reader.Close().
func (m *MultilineReader) Close() error {
	return nil
}
