package tui

import "testing"

func TestTableRender(t *testing.T) {
	type tc struct {
		buildTable  func() *Element
		width       int
		height      int
		expectCells map[[2]int]rune // (x,y) -> expected rune
		expectBold  map[[2]int]bool // (x,y) -> expected bold
	}

	tests := map[string]tc{
		"cells align across rows": {
			buildTable: func() *Element {
				table := New(WithTag("table"))
				row1 := New(WithTag("tr"))
				row1.AddChild(New(WithTag("td"), WithText("Hi")))
				row1.AddChild(New(WithTag("td"), WithText("World")))
				table.AddChild(row1)

				row2 := New(WithTag("tr"))
				row2.AddChild(New(WithTag("td"), WithText("Hello")))
				row2.AddChild(New(WithTag("td"), WithText("Go")))
				table.AddChild(row2)

				return table
			},
			width:  80,
			height: 24,
			// Col 0 width = max("Hi"=2, "Hello"=5) = 5
			// Col 1 starts at x=6 (5 + 1 gap)
			// Row 0: "Hi" at x=0, "World" at x=6
			// Row 1: "Hello" at x=0, "Go" at x=6
			expectCells: map[[2]int]rune{
				{0, 0}: 'H', {1, 0}: 'i',
				{6, 0}: 'W', {7, 0}: 'o', {8, 0}: 'r', {9, 0}: 'l', {10, 0}: 'd',
				{0, 1}: 'H', {1, 1}: 'e', {2, 1}: 'l', {3, 1}: 'l', {4, 1}: 'o',
				{6, 1}: 'G', {7, 1}: 'o',
			},
		},
		"th renders bold": {
			buildTable: func() *Element {
				table := New(WithTag("table"))
				row := New(WithTag("tr"))
				row.AddChild(New(WithTag("th"), WithText("Name")))
				table.AddChild(row)
				return table
			},
			width:  80,
			height: 24,
			expectBold: map[[2]int]bool{
				{0, 0}: true, {1, 0}: true, {2, 0}: true, {3, 0}: true,
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			table := tt.buildTable()
			buf := NewBuffer(tt.width, tt.height)
			table.Render(buf, tt.width, tt.height)

			for pos, expectedRune := range tt.expectCells {
				cell := buf.Cell(pos[0], pos[1])
				if cell.Rune != expectedRune {
					t.Errorf("at (%d,%d): expected rune %q, got %q",
						pos[0], pos[1], string(expectedRune), string(cell.Rune))
				}
			}

			for pos, expectedBold := range tt.expectBold {
				cell := buf.Cell(pos[0], pos[1])
				isBold := cell.Style.Attrs&AttrBold != 0
				if isBold != expectedBold {
					t.Errorf("at (%d,%d): expected bold=%v, got bold=%v",
						pos[0], pos[1], expectedBold, isBold)
				}
			}
		})
	}
}

func TestDrawTableGrid_SingleBorder(t *testing.T) {
	// Draw a grid for a 2-column, 2-row table with a header separator.
	// Layout: border(1) + col0(3) + colSep(1) + col1(4) + border(1) = 10 wide
	//         border(1) + header(1) + hdrSep(1) + body(1) + border(1) = 5 tall
	buf := NewBuffer(20, 10)
	info := TableGridInfo{
		ColWidths:     []int{3, 4},
		RowHeights:    []int{1, 1},
		HeaderRows:    1,
		RowSeparators: false,
	}
	rect := Rect{X: 0, Y: 0, Width: 10, Height: 5}
	DrawTableGrid(buf, rect, BorderSingle, NewStyle(), info)

	// Check corners
	assertRune(t, buf, 0, 0, '┌', "top-left corner")
	assertRune(t, buf, 9, 0, '┐', "top-right corner")
	assertRune(t, buf, 0, 4, '└', "bottom-left corner")
	assertRune(t, buf, 9, 4, '┘', "bottom-right corner")

	// Check top edge: position 4 should be TopTee (after border + 3 col0 content)
	assertRune(t, buf, 4, 0, '┬', "top tee at column separator")

	// Check bottom edge: position 4 should be BottomTee
	assertRune(t, buf, 4, 4, '┴', "bottom tee at column separator")

	// Check header separator line at Y=2
	assertRune(t, buf, 0, 2, '├', "left tee at header separator")
	assertRune(t, buf, 9, 2, '┤', "right tee at header separator")
	assertRune(t, buf, 4, 2, '┼', "cross at header separator × column separator")

	// Check horizontal content of header separator
	assertRune(t, buf, 1, 2, '─', "horizontal line in header separator")
	assertRune(t, buf, 5, 2, '─', "horizontal line in header separator (col1)")

	// Check vertical column separator between content rows
	assertRune(t, buf, 4, 1, '│', "vertical separator in header row")
	assertRune(t, buf, 4, 3, '│', "vertical separator in body row")
}

func TestDrawTableGrid_WithRowSeparators(t *testing.T) {
	// 1 header + 2 body rows, with row separators between body rows.
	// Width: 1 + 5 + 1 + 5 + 1 = 13
	// Height: 1 + 1 + 1 + 1 + 1 + 1 + 1 = 7
	//   top-border(1) + header(1) + headerSep(1) + body1(1) + rowSep(1) + body2(1) + bottom-border(1)
	buf := NewBuffer(20, 10)
	info := TableGridInfo{
		ColWidths:     []int{5, 5},
		RowHeights:    []int{1, 1, 1},
		HeaderRows:    1,
		RowSeparators: true,
	}
	rect := Rect{X: 0, Y: 0, Width: 13, Height: 7}
	DrawTableGrid(buf, rect, BorderSingle, NewStyle(), info)

	// Header separator at Y=2
	assertRune(t, buf, 0, 2, '├', "left tee at header separator")
	assertRune(t, buf, 12, 2, '┤', "right tee at header separator")

	// Row separator between body1 and body2 at Y=4
	assertRune(t, buf, 0, 4, '├', "left tee at body row separator")
	assertRune(t, buf, 12, 4, '┤', "right tee at body row separator")
	assertRune(t, buf, 6, 4, '┼', "cross at body row sep × column sep")
}

func TestTableRender_WithBorder(t *testing.T) {
	// Full integration: table with border, header, and body should render grid lines.
	table := New(
		WithTag("table"),
		WithBorder(BorderSingle),
		WithTableHeaderRows(1),
	)

	headerRow := New(WithTag("tr"))
	headerRow.AddChild(New(WithTag("th"), WithText("A")))
	headerRow.AddChild(New(WithTag("th"), WithText("BB")))
	table.AddChild(headerRow)

	bodyRow := New(WithTag("tr"))
	bodyRow.AddChild(New(WithTag("td"), WithText("x")))
	bodyRow.AddChild(New(WithTag("td"), WithText("yy")))
	table.AddChild(bodyRow)

	buf := NewBuffer(40, 10)
	table.Render(buf, 40, 10)

	// The table should render with a border. Let's check corner characters.
	// Table intrinsic:
	//   Col 0: max("A"=1, "x"=1) = 1
	//   Col 1: max("BB"=2, "yy"=2) = 2, gap = 1
	//   Content width = 1 + 1 + 2 = 4, plus border 2 = 6
	//   Content height: 2 rows + 1 header sep = 3, plus border 2 = 5
	topLeft := buf.Cell(0, 0)
	if topLeft.Rune != '┌' {
		t.Errorf("top-left corner: expected ┌, got %q", string(topLeft.Rune))
	}

	// Check that header text is bold (th auto-bold)
	// "A" is at (1,1) — inside border at (0,0)
	aCell := buf.Cell(1, 1)
	if aCell.Rune != 'A' {
		t.Errorf("header cell A: expected 'A', got %q", string(aCell.Rune))
	}
	if aCell.Style.Attrs&AttrBold == 0 {
		t.Error("header cell A should be bold")
	}
}

func TestTableRender_WithChildren(t *testing.T) {
	// Test that WithChildren option works for building table trees.
	table := New(
		WithTag("table"),
		WithChildren(
			New(WithTag("tr"), WithChildren(
				New(WithTag("td"), WithText("Hi")),
				New(WithTag("td"), WithText("Go")),
			)),
		),
	)

	buf := NewBuffer(40, 10)
	table.Render(buf, 40, 10)

	// Just verify it doesn't panic and renders something
	hiCell := buf.Cell(0, 0)
	if hiCell.Rune != 'H' {
		t.Errorf("first cell: expected 'H', got %q", string(hiCell.Rune))
	}
}

func assertRune(t *testing.T, buf *Buffer, x, y int, expected rune, msg string) {
	t.Helper()
	cell := buf.Cell(x, y)
	if cell.Rune != expected {
		t.Errorf("%s: at (%d,%d) expected %q, got %q", msg, x, y, string(expected), string(cell.Rune))
	}
}
