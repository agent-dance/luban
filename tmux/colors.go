package tmux

import "github.com/agent-dance/luban/terminaltheme"

// Colors is the ordered palette used for teammate pane borders and titles.
var Colors = burgundyColors()

func burgundyColors() []struct {
	Name     string
	TmuxCode string
} {
	p := terminaltheme.Current()
	return []struct {
		Name     string
		TmuxCode string
	}{
		{"burgundy", p.Accent},
		{"wine", p.Success},
		{"rose", p.Warning},
		{"claret", p.Danger},
		{"muted-wine", p.Muted},
	}
}

// AssignColor returns the name and tmux color code for the given index
// using round-robin assignment over the palette. Safe for negative indices.
func AssignColor(index int) (name, tmuxCode string) {
	n := len(Colors)
	if n == 0 {
		return "white", "white"
	}
	c := Colors[((index%n)+n)%n]
	return c.Name, c.TmuxCode
}
