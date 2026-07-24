package tui

import (
	"testing"

	"github.com/agent-dance/luban/terminaltheme"
	gotui "github.com/grindlemire/go-tui"
)

func TestApplyTerminalThemeUsesSharedAccent(t *testing.T) {
	t.Cleanup(func() {
		terminaltheme.Configure("system")
		applyTerminalTheme()
	})
	terminaltheme.Configure("burgundy")
	applyTerminalTheme()

	want, err := gotui.HexColor(terminaltheme.Current().Accent)
	if err != nil {
		t.Fatal(err)
	}
	if !gotui.Cyan.Equal(want) || !gotui.BrightCyan.Equal(want) {
		t.Fatalf("accent colors = cyan:%+v brightCyan:%+v, want %+v", gotui.Cyan, gotui.BrightCyan, want)
	}
}

func TestApplyTerminalThemePreservesTerminalDefaultColors(t *testing.T) {
	t.Cleanup(func() {
		terminaltheme.Configure("system")
		applyTerminalTheme()
	})
	terminaltheme.Configure("system")
	applyTerminalTheme()

	if !gotui.Black.IsDefault() || !gotui.White.IsDefault() {
		t.Fatalf("system theme colors = background:%v foreground:%v, want terminal defaults", gotui.Black, gotui.White)
	}

	root := NewRootComponent(NewAppState(), nil, nil)
	buffer := gotui.NewBuffer(80, 24)
	root.renderAtSize(nil, 80, 24).Render(buffer, 80, 24)
	for _, point := range [][2]int{{0, 0}, {79, 0}, {0, 23}, {79, 23}, {40, 12}} {
		cell := buffer.Cell(point[0], point[1])
		if !cell.Style.Bg.IsDefault() {
			t.Fatalf("root cell (%d,%d) background = %+v, want terminal default", point[0], point[1], cell.Style.Bg)
		}
	}
}
