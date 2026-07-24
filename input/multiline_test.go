package input

import (
	"io"
	"testing"
)

// mockFetcher creates a lineFetcher that returns lines from the provided slice
// in order, then returns io.EOF.
func mockFetcher(lines []string) lineFetcher {
	i := 0
	return func(prompt string) (string, error) {
		if i >= len(lines) {
			return "", io.EOF
		}
		line := lines[i]
		i++
		return line, nil
	}
}

func TestMultilineReader_ShortLine_SingleLine(t *testing.T) {
	// A short first line should not trigger multiline mode.
	fetch := mockFetcher([]string{"hello"})
	r := NewMultilineReader(fetch, "> ")

	got, err := r.Readline()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestMultilineReader_Close(t *testing.T) {
	fetch := mockFetcher(nil)
	r := NewMultilineReader(fetch, "> ")
	if err := r.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestMultilineReader_LinePrompt(t *testing.T) {
	r := NewMultilineReader(nil, "> ")
	tests := []struct {
		n    int
		want string
	}{
		{1, "[line 1] "},
		{2, "[line 2] "},
		{10, "[line 10] "},
	}
	for _, tc := range tests {
		got := r.linePrompt(tc.n)
		if got != tc.want {
			t.Errorf("linePrompt(%d): got %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestMultilineReader_EOFOnFirst(t *testing.T) {
	fetch := mockFetcher(nil) // immediately EOF
	r := NewMultilineReader(fetch, "> ")

	_, err := r.Readline()
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestMultilineReader_Accumulate_BlankTerminator(t *testing.T) {
	// Simulate accumulate: first two lines already gathered, then more lines.
	r := NewMultilineReader(nil, "> ")

	fetch := mockFetcher([]string{"third line", "fourth line", ""})
	r.fetch = fetch

	existing := []string{"first line", "second line"}
	got, err := r.accumulate(existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"first line", "second line", "third line", "fourth line"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("line[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestMultilineReader_Accumulate_EOFTerminator(t *testing.T) {
	r := NewMultilineReader(nil, "> ")

	fetch := mockFetcher([]string{"extra"}) // EOF after "extra"
	r.fetch = fetch

	existing := []string{"first"}
	got, err := r.accumulate(existing)
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
	want := []string{"first", "extra"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
