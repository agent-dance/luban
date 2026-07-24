package input

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/brand"
)

const maxPromptHistoryRecordBytes = 16 << 20

var promptHistoryMu sync.Mutex

// PromptHistoryEntry is one durable TUI prompt-history record. Display may
// contain newlines; JSONL encoding keeps it in a single filesystem record.
type PromptHistoryEntry struct {
	Display   string `json:"display"`
	Timestamp int64  `json:"timestamp"`
	Project   string `json:"project"`
	SessionID string `json:"sessionId,omitempty"`
}

// DefaultPromptHistoryPath returns the Claude-compatible JSONL history path
// for the TUI. The legacy readline history remains a separate plain-text file.
func DefaultPromptHistoryPath() string {
	return filepath.Join(brand.UserConfigDir(), "history.jsonl")
}

// LoadPromptHistory returns newest-first entries for project, prioritizing the
// active session before other sessions in the same project.
func LoadPromptHistory(path, project, sessionID string, limit int) []PromptHistoryEntry {
	if path == "" || limit <= 0 {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	current := make([]PromptHistoryEntry, 0, limit)
	other := make([]PromptHistoryEntry, 0, limit)
	_ = readPromptHistoryLines(f, func(line []byte) {
		var entry PromptHistoryEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return
		}
		if strings.TrimSpace(entry.Display) == "" || entry.Project != project {
			return
		}
		if entry.SessionID == sessionID {
			current = appendBoundedPromptHistory(current, entry, limit)
		} else {
			other = appendBoundedPromptHistory(other, entry, limit)
		}
	})
	reversePromptHistory(current)
	reversePromptHistory(other)

	result := append(current, other...)
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

// AppendPromptHistory appends one JSONL record unless it is a consecutive
// duplicate for the same project and session.
func AppendPromptHistory(path string, entry PromptHistoryEntry) error {
	if path == "" || strings.TrimSpace(entry.Display) == "" {
		return nil
	}
	if entry.Timestamp == 0 {
		entry.Timestamp = time.Now().UnixMilli()
	}
	if err := ensureHistoryDir(path); err != nil {
		return err
	}

	promptHistoryMu.Lock()
	defer promptHistoryMu.Unlock()

	if previous, ok := lastPromptHistoryEntry(path); ok &&
		previous.Display == entry.Display &&
		previous.Project == entry.Project &&
		previous.SessionID == entry.SessionID {
		return nil
	}

	encoded, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	encoded = append(encoded, '\n')
	if _, err := f.Write(encoded); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func lastPromptHistoryEntry(path string) (PromptHistoryEntry, bool) {
	f, err := os.Open(path)
	if err != nil {
		return PromptHistoryEntry{}, false
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return PromptHistoryEntry{}, false
	}

	end := stat.Size()
	for end > 0 {
		line, nextEnd, ok, readErr := readPreviousPromptHistoryLine(f, end)
		if readErr != nil || !ok {
			return PromptHistoryEntry{}, false
		}
		end = nextEnd
		if len(line) == 0 {
			continue
		}
		var entry PromptHistoryEntry
		if err := json.Unmarshal(line, &entry); err == nil {
			return entry, true
		}
	}
	return PromptHistoryEntry{}, false
}

func readPromptHistoryLines(f *os.File, visit func([]byte)) error {
	reader := bufio.NewReaderSize(f, 64<<10)
	line := make([]byte, 0, 64<<10)
	oversized := false
	for {
		fragment, err := reader.ReadSlice('\n')
		if !oversized {
			if len(line)+len(fragment) > maxPromptHistoryRecordBytes {
				line = line[:0]
				oversized = true
			} else {
				line = append(line, fragment...)
			}
		}

		switch err {
		case nil:
			if !oversized {
				visit(bytes.TrimRight(line, "\r\n"))
			}
			line = line[:0]
			oversized = false
		case bufio.ErrBufferFull:
			continue
		case io.EOF:
			if !oversized && len(line) > 0 {
				visit(bytes.TrimRight(line, "\r\n"))
			}
			return nil
		default:
			return err
		}
	}
}

func appendBoundedPromptHistory(entries []PromptHistoryEntry, entry PromptHistoryEntry, limit int) []PromptHistoryEntry {
	if len(entries) < limit {
		return append(entries, entry)
	}
	copy(entries, entries[1:])
	entries[len(entries)-1] = entry
	return entries
}

func reversePromptHistory(entries []PromptHistoryEntry) {
	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}
}

func readPreviousPromptHistoryLine(f *os.File, end int64) ([]byte, int64, bool, error) {
	var single [1]byte
	for end > 0 {
		if _, err := f.ReadAt(single[:], end-1); err != nil {
			return nil, end, false, err
		}
		if single[0] != '\n' && single[0] != '\r' {
			break
		}
		end--
	}
	if end == 0 {
		return nil, 0, false, nil
	}

	const chunkSize int64 = 64 << 10
	parts := make([][]byte, 0, 4)
	total := 0
	oversized := false
	position := end
	for position > 0 {
		start := position - chunkSize
		if start < 0 {
			start = 0
		}
		chunk := make([]byte, position-start)
		if _, err := f.ReadAt(chunk, start); err != nil && err != io.EOF {
			return nil, end, false, err
		}
		if newline := bytes.LastIndexByte(chunk, '\n'); newline >= 0 {
			fragment := chunk[newline+1:]
			if !oversized {
				if total+len(fragment) > maxPromptHistoryRecordBytes {
					oversized = true
					parts = nil
				} else {
					parts = append(parts, fragment)
					total += len(fragment)
				}
			}
			nextEnd := start + int64(newline)
			if oversized {
				return nil, nextEnd, true, nil
			}
			return joinReversePromptHistoryParts(parts, total), nextEnd, true, nil
		}
		if !oversized {
			if total+len(chunk) > maxPromptHistoryRecordBytes {
				oversized = true
				parts = nil
			} else {
				parts = append(parts, chunk)
				total += len(chunk)
			}
		}
		position = start
	}
	if oversized {
		return nil, 0, true, nil
	}
	return joinReversePromptHistoryParts(parts, total), 0, true, nil
}

func joinReversePromptHistoryParts(parts [][]byte, total int) []byte {
	line := make([]byte, 0, total)
	for i := len(parts) - 1; i >= 0; i-- {
		line = append(line, parts[i]...)
	}
	return line
}
