package tui

// MarkDirty marks this app as needing a render.
func (a *App) MarkDirty() {
	if a == nil {
		panic("tui: nil app in MarkDirty")
	}
	a.dirty.Store(true)
}

// RequestFullRedraw requests that the next render cycle performs a full
// terminal redraw instead of a diff-based update. Use this when large UI
// regions disappear (e.g. picker/overlay closing) and diff rendering would
// leave stale characters on screen.
func (a *App) RequestFullRedraw() {
	a.needsFullRedraw = true
	a.MarkDirty()
}

func (a *App) checkAndClearDirty() bool {
	if a == nil {
		panic("tui: nil app in checkAndClearDirty")
	}
	return a.dirty.Swap(false)
}
