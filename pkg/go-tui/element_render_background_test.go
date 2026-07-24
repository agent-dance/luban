package tui

import "testing"

func TestInheritedBackgroundCoversChildBorderAndRule(t *testing.T) {
	root := New(
		WithDirection(Column),
		WithWidth(12),
		WithHeight(6),
		WithBackground(NewStyle().Background(Black)),
	)
	root.AddChild(New(
		WithBorder(BorderSingle),
		WithBorderStyle(NewStyle().Foreground(Cyan)),
		WithWidth(12),
		WithHeight(4),
		WithText("opaque"),
	))
	root.AddChild(New(
		WithHR(),
		WithWidth(12),
		WithHeight(1),
	))

	buffer := NewBuffer(12, 6)
	root.Render(buffer, 12, 6)
	for _, point := range [][2]int{{0, 0}, {11, 0}, {0, 3}, {11, 3}, {0, 4}, {6, 4}} {
		cell := buffer.Cell(point[0], point[1])
		if !cell.Style.Bg.Equal(Black) {
			t.Fatalf("cell (%d,%d) background = %+v, want black", point[0], point[1], cell.Style.Bg)
		}
	}
}
