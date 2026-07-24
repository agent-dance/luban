// Package terminaltheme owns the shared visual palette for terminal surfaces.
package terminaltheme

import (
	"strings"
	"sync"
)

// Palette names the semantic colors used by the interactive TUI, print mode,
// and tmux panes. Keeping roles rather than component-specific colors makes a
// theme switch a single configuration change.
type Palette struct {
	Name       string
	Background string
	Foreground string
	Muted      string
	Accent     string
	Success    string
	Warning    string
	Danger     string
}

var burgundy = Palette{
	Name:       "burgundy",
	Background: "#1A090B",
	Foreground: "#F7E6E8",
	Muted:      "#A77B82",
	Accent:     "#D65A6A",
	Success:    "#C97983",
	Warning:    "#E6A1A8",
	Danger:     "#A82A3A",
}

// system leaves the terminal-owned foreground and background unset while
// retaining the product's semantic accent colors. This lets light and dark
// terminal profiles remain authoritative for ordinary text and empty cells.
var system = Palette{
	Name:       "system",
	Background: "",
	Foreground: "",
	Muted:      "",
	Accent:     burgundy.Accent,
	Success:    burgundy.Success,
	Warning:    burgundy.Warning,
	Danger:     burgundy.Danger,
}

var current = struct {
	sync.RWMutex
	palette Palette
}{palette: system}

// Configure selects the palette named by the single "theme" configuration
// value. Empty, system, and light values preserve the terminal's own default
// foreground and background. Unknown values use that same safe fallback.
func Configure(name string) {
	current.Lock()
	defer current.Unlock()

	switch strings.ToLower(strings.TrimSpace(name)) {
	case "burgundy", "wine", "crimson", "dark":
		current.palette = burgundy
	case "", "system", "default", "auto", "light":
		current.palette = system
	default:
		current.palette = system
	}
}

// Current returns the active immutable palette.
func Current() Palette {
	current.RLock()
	defer current.RUnlock()
	return current.palette
}
