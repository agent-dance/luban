package input

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/brand"
)

const (
	// maxHistoryEntries is the maximum number of history lines to retain.
	maxHistoryEntries = 1000
)

// DefaultHistoryPath returns the default LUBAN Code history file path.
func DefaultHistoryPath() string {
	current := brand.HistoryPath()
	if _, err := os.Stat(current); err == nil {
		return current
	}
	for _, legacy := range []string{
		filepath.Join(brand.LegacyDeepSeekUserConfigDir(), "history"),
		filepath.Join(brand.LegacyUserConfigDir(), "history"),
	} {
		if data, err := os.ReadFile(legacy); err == nil {
			_ = migrateHistoryFile(current, data)
			break
		}
	}
	return current
}

func migrateHistoryFile(path string, data []byte) error {
	if err := ensureHistoryDir(path); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

// ensureHistoryDir creates the directory containing the history file if it
// does not already exist.
func ensureHistoryDir(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0o700)
}

// LoadHistory reads history entries from path. It returns an empty slice on
// any error so callers can always range over the result safely.
func LoadHistory(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := strings.TrimRight(sc.Text(), "\r\n"); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// AppendHistory appends entry to the history file at path, then rewrites the
// file with at most maxHistoryEntries lines, deduplicating consecutive
// identical entries.
func AppendHistory(path, entry string) error {
	if entry == "" {
		return nil
	}
	if err := ensureHistoryDir(path); err != nil {
		return err
	}

	existing := LoadHistory(path)
	merged := deduplicateConsecutive(append(existing, entry))

	// Trim to max entries (keep newest).
	if len(merged) > maxHistoryEntries {
		merged = merged[len(merged)-maxHistoryEntries:]
	}

	return writeLines(path, merged)
}

// deduplicateConsecutive removes consecutive duplicate entries while preserving
// order of non-duplicate entries.
func deduplicateConsecutive(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	out := make([]string, 0, len(lines))
	prev := ""
	for _, l := range lines {
		if l != prev {
			out = append(out, l)
		}
		prev = l
	}
	return out
}

// writeLines atomically rewrites the file at path with the given lines.
func writeLines(path string, lines []string) error {
	// Write to a temp file alongside the target, then rename.
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, l := range lines {
		if _, werr := w.WriteString(l + "\n"); werr != nil {
			f.Close()
			os.Remove(tmp)
			return werr
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
