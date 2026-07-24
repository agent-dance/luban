package input

import (
	"io"
	"strings"

	"github.com/chzyer/readline"
)

// ReaderOpts configures a new Reader.
type ReaderOpts struct {
	// Prompt is the primary prompt string shown before each input line.
	Prompt string

	// HistoryFile is the path to the readline history file.
	// Defaults to DefaultHistoryPath() when empty.
	HistoryFile string

	// MultilineEnabled enables paste-detection and multiline accumulation.
	MultilineEnabled bool
}

// Reader is an enhanced readline wrapper that implements LineReader.
// It supports backslash line continuation and optional multiline mode.
type Reader struct {
	rl   *readline.Instance
	opts ReaderOpts

	// multiline wraps this reader when MultilineEnabled is true.
	ml *MultilineReader
}

// NewReader creates a new Reader with sensible defaults.
func NewReader(opts ReaderOpts) (*Reader, error) {
	if opts.Prompt == "" {
		opts.Prompt = "> "
	}
	histFile := opts.HistoryFile
	if histFile == "" {
		histFile = DefaultHistoryPath()
	}
	if err := ensureHistoryDir(histFile); err != nil {
		// Non-fatal: continue without persistent history.
		histFile = ""
	}

	rlCfg := &readline.Config{
		Prompt:            opts.Prompt,
		HistoryFile:       histFile,
		HistoryLimit:      maxHistoryEntries,
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
	}

	rl, err := readline.NewEx(rlCfg)
	if err != nil {
		return nil, err
	}

	r := &Reader{rl: rl, opts: opts}

	if opts.MultilineEnabled {
		r.ml = NewMultilineReader(r.readRaw, opts.Prompt)
	}

	return r, nil
}

// Readline reads the next logical line from the terminal.
//
// If MultilineEnabled is set the call is delegated to the MultilineReader
// which handles paste-detection and blank-line / Ctrl-D termination.
//
// Otherwise backslash continuation is applied: a line ending with a single
// backslash is joined with the following line (the backslash is stripped).
func (r *Reader) Readline() (string, error) {
	if r.ml != nil {
		return r.ml.Readline()
	}
	return r.readWithContinuation()
}

// readWithContinuation reads one logical line, honouring `\` continuation.
func (r *Reader) readWithContinuation() (string, error) {
	var buf strings.Builder
	continuationPrompt := r.opts.Prompt
	runeLen := len([]rune(continuationPrompt))
	if runeLen > 2 {
		continuationPrompt = strings.Repeat(" ", runeLen-2) + "| "
	} else {
		continuationPrompt = "| "
	}

	prompt := r.opts.Prompt
	for {
		r.rl.SetPrompt(prompt)
		line, err := r.rl.Readline()
		if err != nil {
			if buf.Len() > 0 {
				// Return whatever we have accumulated before propagating the error.
				return buf.String(), err
			}
			return "", err
		}

		if strings.HasSuffix(line, `\`) {
			buf.WriteString(line[:len(line)-1])
			buf.WriteByte('\n')
			prompt = continuationPrompt
			continue
		}

		buf.WriteString(line)
		result := buf.String()
		r.rl.SetPrompt(r.opts.Prompt)
		return result, nil
	}
}

// readRaw reads a single raw line from the underlying readline instance.
// It is used by MultilineReader to fetch individual lines.
func (r *Reader) readRaw(prompt string) (string, error) {
	r.rl.SetPrompt(prompt)
	line, err := r.rl.Readline()
	if err != nil {
		return "", err
	}
	return line, nil
}

// Close releases resources held by the underlying readline instance.
func (r *Reader) Close() error {
	return r.rl.Close()
}

// SetPrompt updates the primary prompt string at runtime.
func (r *Reader) SetPrompt(p string) {
	r.opts.Prompt = p
	r.rl.SetPrompt(p)
}

// Stderr returns the readline-managed stderr writer.
func (r *Reader) Stderr() io.Writer {
	return r.rl.Stderr()
}
