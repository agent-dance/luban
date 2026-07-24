package tui

import (
	"bytes"
	"time"
)

// escapeSequenceTimeout is the maximum time the event reader waits to
// disambiguate a standalone Escape key from the ESC prefix used by legacy
// Alt/Meta key encodings. It mirrors the short escape delay used by terminal
// multiplexers and editors without making Escape feel laggy.
const escapeSequenceTimeout = 25 * time.Millisecond

// parseInputWithRemainder parses input and returns incomplete trailing bytes.
// It is shared by Unix and Windows readers so large pastes behave identically.
func parseInputWithRemainder(data []byte) ([]Event, []byte) {
	if pasteStart := findUnterminatedBracketedPaste(data); pasteStart >= 0 {
		remaining := append([]byte(nil), data[pasteStart:]...)
		return parseInput(data[:pasteStart]), remaining
	}

	escRemaining := findIncompleteEscapeSequence(data)
	if len(escRemaining) > 0 {
		if len(escRemaining) < len(data) {
			data = data[:len(data)-len(escRemaining)]
		} else {
			// A read boundary is not an input boundary. Preserve the whole
			// incomplete sequence so split legacy Meta, CSI, Kitty, and SGR mouse
			// encodings can be completed by the next PollEvent read.
			data = nil
		}
	}

	remaining := findIncompleteUTF8Suffix(data)
	if len(remaining) > 0 {
		data = data[:len(data)-len(remaining)]
	}

	events := parseInput(data)
	if len(escRemaining) > 0 {
		return events, escRemaining
	}
	return events, remaining
}

func hasPendingEscapePrefix(data []byte) bool {
	return len(data) == 1 && data[0] == 0x1b
}

func hasPendingEscapeSequence(data []byte) bool {
	return len(data) > 0 && data[0] == 0x1b
}

func escapePollTimeout(requested time.Duration) time.Duration {
	if requested < 0 || requested > escapeSequenceTimeout {
		return escapeSequenceTimeout
	}
	return requested
}

func findUnterminatedBracketedPaste(data []byte) int {
	searchFrom := 0
	for {
		startOffset := bytes.Index(data[searchFrom:], []byte(bracketedPasteStart))
		if startOffset < 0 {
			return -1
		}
		start := searchFrom + startOffset
		payloadStart := start + len(bracketedPasteStart)
		endOffset := bytes.Index(data[payloadStart:], []byte(bracketedPasteEnd))
		if endOffset < 0 {
			return start
		}
		searchFrom = payloadStart + endOffset + len(bracketedPasteEnd)
	}
}

func findIncompleteEscapeSequence(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	searchStart := len(data) - 64
	if searchStart < 0 {
		searchStart = 0
	}
	for i := len(data) - 1; i >= searchStart; i-- {
		if data[i] != 0x1b {
			continue
		}
		suffix := data[i:]
		if len(suffix) == 1 {
			return suffix
		}
		switch suffix[1] {
		case '[':
			if len(suffix) == 2 {
				return suffix
			}
			if suffix[2] == '<' {
				for j := 3; j < len(suffix); j++ {
					if suffix[j] == 'M' || suffix[j] == 'm' {
						break
					}
					if suffix[j] != ';' && (suffix[j] < '0' || suffix[j] > '9') {
						break
					}
					if j == len(suffix)-1 {
						return suffix
					}
				}
			} else {
				for j := 2; j < len(suffix); j++ {
					if suffix[j] >= 0x40 && suffix[j] <= 0x7e {
						break
					}
					if j == len(suffix)-1 {
						return suffix
					}
				}
			}
		case 'O':
			if len(suffix) == 2 {
				return suffix
			}
		}
	}
	return nil
}

func findIncompleteUTF8Suffix(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	for i := 1; i <= 3 && i <= len(data); i++ {
		b := data[len(data)-i]
		if b >= 0xC0 {
			expectedLen := 4
			if b < 0xE0 {
				expectedLen = 2
			} else if b < 0xF0 {
				expectedLen = 3
			}
			if i < expectedLen {
				return data[len(data)-i:]
			}
			return nil
		}
		if b >= 0x80 && b < 0xC0 {
			continue
		}
		return nil
	}
	return nil
}
