package tui

// ForkEntry describes one complete user/assistant conversation turn. Entries
// are supplied newest-first and MessageEnd is the exclusive transcript index
// retained by the fork.
type ForkEntry struct {
	MessageEnd    int
	UserText      string
	AssistantText string
}

// ForkPickerState tracks the /fork history picker overlay.
type ForkPickerState struct {
	Visible  bool
	Entries  []ForkEntry
	Selected int
	OnSelect func(ForkEntry)
	OnCancel func()
}

func (s *ForkPickerState) clamp() {
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

func forkPickerVisibleRange(total, selected, limit int) (int, int) {
	if total <= 0 || limit <= 0 {
		return 0, 0
	}
	if total <= limit {
		return 0, total
	}
	start := selected - limit/2
	if start < 0 {
		start = 0
	}
	if start > total-limit {
		start = total - limit
	}
	return start, start + limit
}
