package tui

import "time"

// SessionEntry is the TUI-facing view of a resumable session.
type SessionEntry struct {
	ID           string
	ProjectDir   string
	Title        string
	UpdatedAt    time.Time
	CreatedAt    time.Time
	MessageCount int
	CWD          string
	GitBranch    string
	PreviewText  string
	Messages     []Message
}

// SessionPickerState tracks the current resume-session picker modal.
type SessionPickerState struct {
	Visible  bool
	Entries  []SessionEntry
	Selected int
	Query    string
	OnSelect func(SessionEntry)
	OnCancel func()
}

func (s *SessionPickerState) clamp() {
	if len(s.Entries) == 0 {
		s.Selected = 0
		return
	}
	if s.Selected < 0 {
		s.Selected = 0
	}
	if s.Selected >= len(s.Entries) {
		s.Selected = len(s.Entries) - 1
	}
}
