package tui

import "strings"

const maxPromptHistoryEntries = 100

// promptHistoryNavigator owns the reversible, in-memory portion of prompt
// history navigation. Entries are stored newest first; index -1 represents
// the live composer draft.
type promptHistoryNavigator struct {
	entries []string
	index   int
	draft   string
	edits   map[int]string
}

func newPromptHistoryNavigator(entries []string) *promptHistoryNavigator {
	history := &promptHistoryNavigator{}
	history.Replace(entries)
	return history
}

func (h *promptHistoryNavigator) Replace(entries []string) {
	h.entries = h.entries[:0]
	for _, entry := range entries {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		h.entries = append(h.entries, entry)
		if len(h.entries) == maxPromptHistoryEntries {
			break
		}
	}
	h.ResetNavigation()
}

func (h *promptHistoryNavigator) Add(value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if len(h.entries) == 0 || h.entries[0] != value {
		h.entries = append([]string{value}, h.entries...)
		if len(h.entries) > maxPromptHistoryEntries {
			h.entries = h.entries[:maxPromptHistoryEntries]
		}
	}
	h.ResetNavigation()
}

func (h *promptHistoryNavigator) Previous(current string) (string, bool) {
	next := h.index + 1
	if next >= len(h.entries) {
		return "", false
	}
	if h.index < 0 {
		if strings.TrimSpace(current) != "" {
			h.draft = current
		} else {
			h.draft = ""
		}
	} else {
		h.edits[h.index] = current
	}
	h.index = next
	return h.valueAt(next), true
}

func (h *promptHistoryNavigator) Next(current string) (string, bool) {
	if h.index < 0 {
		return "", false
	}
	h.edits[h.index] = current
	h.index--
	if h.index < 0 {
		return h.draft, true
	}
	return h.valueAt(h.index), true
}

func (h *promptHistoryNavigator) ResetNavigation() {
	h.index = -1
	h.draft = ""
	h.edits = make(map[int]string)
}

func (h *promptHistoryNavigator) valueAt(index int) string {
	if edited, ok := h.edits[index]; ok {
		return edited
	}
	return h.entries[index]
}
