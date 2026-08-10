package tui

// BorderStyle represents different styles of box borders.
type BorderStyle int

const (
	// BorderNone indicates no border should be drawn.
	BorderNone BorderStyle = iota
	// BorderSingle uses single-line box-drawing characters (─, │, ┌, etc.)
	BorderSingle
	// BorderDouble uses double-line box-drawing characters (═, ║, ╔, etc.)
	BorderDouble
	// BorderRounded uses rounded corner characters (─, │, ╭, ╮, ╰, ╯)
	BorderRounded
	// BorderThick uses thick/heavy box-drawing characters (━, ┃, ┏, etc.)
	BorderThick
)

// BorderChars holds the characters used to draw a box border.
type BorderChars struct {
	TopLeft     rune
	Top         rune
	TopRight    rune
	Left        rune
	Right       rune
	BottomLeft  rune
	Bottom      rune
	BottomRight rune

	// Table-specific junction characters for internal grid lines.
	// These are used by DrawTableGrid to draw header separators,
	// row separators, and column separators within a table.
	TopTee     rune // ┬ — top edge meets vertical separator
	BottomTee  rune // ┴ — bottom edge meets vertical separator
	LeftTee    rune // ├ — left edge meets horizontal separator
	RightTee   rune // ┤ — right edge meets horizontal separator
	Cross      rune // ┼ — horizontal and vertical separators cross
	Horizontal rune // ─ — horizontal separator (same as Top/Bottom)
	Vertical   rune // │ — vertical separator (same as Left/Right)
}

// Chars returns the box-drawing characters for this border style.
func (b BorderStyle) Chars() BorderChars {
	switch b {
	case BorderSingle:
		return BorderChars{
			TopLeft:     '┌',
			Top:         '─',
			TopRight:    '┐',
			Left:        '│',
			Right:       '│',
			BottomLeft:  '└',
			Bottom:      '─',
			BottomRight: '┘',
			TopTee:      '┬',
			BottomTee:   '┴',
			LeftTee:     '├',
			RightTee:    '┤',
			Cross:       '┼',
			Horizontal:  '─',
			Vertical:    '│',
		}
	case BorderDouble:
		return BorderChars{
			TopLeft:     '╔',
			Top:         '═',
			TopRight:    '╗',
			Left:        '║',
			Right:       '║',
			BottomLeft:  '╚',
			Bottom:      '═',
			BottomRight: '╝',
			TopTee:      '╦',
			BottomTee:   '╩',
			LeftTee:     '╠',
			RightTee:    '╣',
			Cross:       '╬',
			Horizontal:  '═',
			Vertical:    '║',
		}
	case BorderRounded:
		return BorderChars{
			TopLeft:     '╭',
			Top:         '─',
			TopRight:    '╮',
			Left:        '│',
			Right:       '│',
			BottomLeft:  '╰',
			Bottom:      '─',
			BottomRight: '╯',
			TopTee:      '┬',
			BottomTee:   '┴',
			LeftTee:     '├',
			RightTee:    '┤',
			Cross:       '┼',
			Horizontal:  '─',
			Vertical:    '│',
		}
	case BorderThick:
		return BorderChars{
			TopLeft:     '┏',
			Top:         '━',
			TopRight:    '┓',
			Left:        '┃',
			Right:       '┃',
			BottomLeft:  '┗',
			Bottom:      '━',
			BottomRight: '┛',
			TopTee:      '┳',
			BottomTee:   '┻',
			LeftTee:     '┣',
			RightTee:    '┫',
			Cross:       '╋',
			Horizontal:  '━',
			Vertical:    '┃',
		}
	default:
		// BorderNone or unknown - return spaces
		return BorderChars{
			TopLeft:     ' ',
			Top:         ' ',
			TopRight:    ' ',
			Left:        ' ',
			Right:       ' ',
			BottomLeft:  ' ',
			Bottom:      ' ',
			BottomRight: ' ',
			TopTee:      ' ',
			BottomTee:   ' ',
			LeftTee:     ' ',
			RightTee:    ' ',
			Cross:       ' ',
			Horizontal:  ' ',
			Vertical:    ' ',
		}
	}
}

// DrawBox draws a box border on the buffer at the specified rectangle.
// The box is drawn using the specified border style and style (colors/attributes).
// If the rectangle is smaller than 2x2, the function does nothing.
func DrawBox(buf *Buffer, rect Rect, border BorderStyle, style Style) {
	if rect.Width < 2 || rect.Height < 2 {
		return
	}
	if border == BorderNone {
		return
	}

	chars := border.Chars()

	// Clip rect to buffer bounds
	bufRect := buf.Rect()
	rect = rect.Intersect(bufRect)
	if rect.IsEmpty() || rect.Width < 2 || rect.Height < 2 {
		return
	}

	left := rect.X
	right := rect.Right() - 1
	top := rect.Y
	bottom := rect.Bottom() - 1

	// Draw corners
	buf.SetRune(left, top, chars.TopLeft, style)
	buf.SetRune(right, top, chars.TopRight, style)
	buf.SetRune(left, bottom, chars.BottomLeft, style)
	buf.SetRune(right, bottom, chars.BottomRight, style)

	// Draw top and bottom edges
	for x := left + 1; x < right; x++ {
		buf.SetRune(x, top, chars.Top, style)
		buf.SetRune(x, bottom, chars.Bottom, style)
	}

	// Draw left and right edges
	for y := top + 1; y < bottom; y++ {
		buf.SetRune(left, y, chars.Left, style)
		buf.SetRune(right, y, chars.Right, style)
	}
}

// DrawBoxGradient draws a box border with a gradient applied around the perimeter.
// The gradient is applied based on its direction:
// - Horizontal: left to right along top/bottom edges, top to bottom along left/right edges
// - Vertical: top to bottom along all edges
// - DiagonalDown: top-left to bottom-right
// - DiagonalUp: bottom-left to top-right
func DrawBoxGradient(buf *Buffer, rect Rect, border BorderStyle, g Gradient, baseStyle Style) {
	if rect.Width < 2 || rect.Height < 2 {
		return
	}
	if border == BorderNone {
		return
	}

	chars := border.Chars()

	// Clip rect to buffer bounds
	bufRect := buf.Rect()
	rect = rect.Intersect(bufRect)
	if rect.IsEmpty() || rect.Width < 2 || rect.Height < 2 {
		return
	}

	left := rect.X
	right := rect.Right() - 1
	top := rect.Y
	bottom := rect.Bottom() - 1
	width := float64(rect.Width)
	height := float64(rect.Height)
	perimeter := 2*width + 2*height - 4 // Subtract 4 for corners counted twice

	// Helper to calculate t along the perimeter, mirrored so the gradient
	// goes Start→End over the first half and End→Start over the second half.
	// This avoids a jarring color discontinuity where the perimeter wraps.
	getPerimeterT := func(x, y int) float64 {
		// Calculate position along perimeter: start at top-left, go clockwise
		var pos float64
		if y == top {
			// Top edge
			pos = float64(x - left)
		} else if x == right {
			// Right edge
			pos = width - 1 + float64(y-top)
		} else if y == bottom {
			// Bottom edge (right to left)
			pos = width - 1 + height - 1 + float64(right-x)
		} else {
			// Left edge (bottom to top)
			pos = width - 1 + height - 1 + width - 1 + float64(bottom-y)
		}
		t := pos / perimeter
		// Mirror: 0→1 for first half, 1→0 for second half
		if t <= 0.5 {
			return 2 * t
		}
		return 2 * (1 - t)
	}

	// Draw corners with gradient
	style := baseStyle
	style.Fg = g.At(getPerimeterT(left, top))
	buf.SetRune(left, top, chars.TopLeft, style)

	style.Fg = g.At(getPerimeterT(right, top))
	buf.SetRune(right, top, chars.TopRight, style)

	style.Fg = g.At(getPerimeterT(left, bottom))
	buf.SetRune(left, bottom, chars.BottomLeft, style)

	style.Fg = g.At(getPerimeterT(right, bottom))
	buf.SetRune(right, bottom, chars.BottomRight, style)

	// Draw top and bottom edges with gradient
	for x := left + 1; x < right; x++ {
		style.Fg = g.At(getPerimeterT(x, top))
		buf.SetRune(x, top, chars.Top, style)

		style.Fg = g.At(getPerimeterT(x, bottom))
		buf.SetRune(x, bottom, chars.Bottom, style)
	}

	// Draw left and right edges with gradient
	for y := top + 1; y < bottom; y++ {
		style.Fg = g.At(getPerimeterT(left, y))
		buf.SetRune(left, y, chars.Left, style)

		style.Fg = g.At(getPerimeterT(right, y))
		buf.SetRune(right, y, chars.Right, style)
	}
}

// DrawBoxClipped draws a box border clipped to the given clipRect.
// Positions are computed from the full rect, but only characters within
// clipRect are actually drawn. This enables partial border rendering
// when an element is partially scrolled out of view.
func DrawBoxClipped(buf *Buffer, rect Rect, border BorderStyle, style Style, clipRect Rect) {
	if rect.Width < 2 || rect.Height < 2 {
		return
	}
	if border == BorderNone {
		return
	}

	chars := border.Chars()

	left := rect.X
	right := rect.Right() - 1
	top := rect.Y
	bottom := rect.Bottom() - 1

	// Draw corners (only if within clip region)
	if clipRect.Contains(left, top) {
		buf.SetRune(left, top, chars.TopLeft, style)
	}
	if clipRect.Contains(right, top) {
		buf.SetRune(right, top, chars.TopRight, style)
	}
	if clipRect.Contains(left, bottom) {
		buf.SetRune(left, bottom, chars.BottomLeft, style)
	}
	if clipRect.Contains(right, bottom) {
		buf.SetRune(right, bottom, chars.BottomRight, style)
	}

	// Draw top and bottom edges
	for x := left + 1; x < right; x++ {
		if clipRect.Contains(x, top) {
			buf.SetRune(x, top, chars.Top, style)
		}
		if clipRect.Contains(x, bottom) {
			buf.SetRune(x, bottom, chars.Bottom, style)
		}
	}

	// Draw left and right edges
	for y := top + 1; y < bottom; y++ {
		if clipRect.Contains(left, y) {
			buf.SetRune(left, y, chars.Left, style)
		}
		if clipRect.Contains(right, y) {
			buf.SetRune(right, y, chars.Right, style)
		}
	}
}

// DrawBoxGradientClipped draws a gradient box border clipped to the given clipRect.
// Positions and gradient colors are computed from the full rect, but only
// characters within clipRect are actually drawn.
func DrawBoxGradientClipped(buf *Buffer, rect Rect, border BorderStyle, g Gradient, baseStyle Style, clipRect Rect) {
	if rect.Width < 2 || rect.Height < 2 {
		return
	}
	if border == BorderNone {
		return
	}

	chars := border.Chars()

	left := rect.X
	right := rect.Right() - 1
	top := rect.Y
	bottom := rect.Bottom() - 1
	width := float64(rect.Width)
	height := float64(rect.Height)
	perimeter := 2*width + 2*height - 4

	// Mirrored perimeter t: Start→End over first half, End→Start over second half.
	getPerimeterT := func(x, y int) float64 {
		var pos float64
		if y == top {
			pos = float64(x - left)
		} else if x == right {
			pos = width - 1 + float64(y-top)
		} else if y == bottom {
			pos = width - 1 + height - 1 + float64(right-x)
		} else {
			pos = width - 1 + height - 1 + width - 1 + float64(bottom-y)
		}
		t := pos / perimeter
		if t <= 0.5 {
			return 2 * t
		}
		return 2 * (1 - t)
	}

	style := baseStyle

	// Draw corners with gradient (only if within clip region)
	if clipRect.Contains(left, top) {
		style.Fg = g.At(getPerimeterT(left, top))
		buf.SetRune(left, top, chars.TopLeft, style)
	}
	if clipRect.Contains(right, top) {
		style.Fg = g.At(getPerimeterT(right, top))
		buf.SetRune(right, top, chars.TopRight, style)
	}
	if clipRect.Contains(left, bottom) {
		style.Fg = g.At(getPerimeterT(left, bottom))
		buf.SetRune(left, bottom, chars.BottomLeft, style)
	}
	if clipRect.Contains(right, bottom) {
		style.Fg = g.At(getPerimeterT(right, bottom))
		buf.SetRune(right, bottom, chars.BottomRight, style)
	}

	// Draw top and bottom edges with gradient
	for x := left + 1; x < right; x++ {
		if clipRect.Contains(x, top) {
			style.Fg = g.At(getPerimeterT(x, top))
			buf.SetRune(x, top, chars.Top, style)
		}
		if clipRect.Contains(x, bottom) {
			style.Fg = g.At(getPerimeterT(x, bottom))
			buf.SetRune(x, bottom, chars.Bottom, style)
		}
	}

	// Draw left and right edges with gradient
	for y := top + 1; y < bottom; y++ {
		if clipRect.Contains(left, y) {
			style.Fg = g.At(getPerimeterT(left, y))
			buf.SetRune(left, y, chars.Left, style)
		}
		if clipRect.Contains(right, y) {
			style.Fg = g.At(getPerimeterT(right, y))
			buf.SetRune(right, y, chars.Right, style)
		}
	}
}

// DrawBoxWithTitle draws a box border with a title in the top border.
// The title is centered in the top border and truncated if too long.
// If the rectangle is smaller than 2x2, the function does nothing.
func DrawBoxWithTitle(buf *Buffer, rect Rect, border BorderStyle, title string, style Style) {
	if rect.Width < 2 || rect.Height < 2 {
		return
	}
	if border == BorderNone {
		return
	}

	// First draw the box
	DrawBox(buf, rect, border, style)

	// Now add the title if there's room
	if len(title) == 0 {
		return
	}

	// Calculate available space for title (leave at least 1 char on each side for corners)
	availableWidth := rect.Width - 2
	if availableWidth <= 0 {
		return
	}

	// Truncate the title at grapheme boundaries so emoji sequences and
	// combining marks occupy the same number of cells as they do in a terminal.
	titleGraphemes := splitTextGraphemes(title)
	titleWidth := 0
	truncatedGraphemes := make([]textGrapheme, 0, len(titleGraphemes))

	for _, grapheme := range titleGraphemes {
		if titleWidth+grapheme.width > availableWidth {
			break
		}
		truncatedGraphemes = append(truncatedGraphemes, grapheme)
		titleWidth += grapheme.width
	}

	if len(truncatedGraphemes) == 0 {
		return
	}

	// Center the title in the available space
	startX := rect.X + 1 + (availableWidth-titleWidth)/2

	// Draw the title
	x := startX
	for _, grapheme := range truncatedGraphemes {
		buf.SetGrapheme(x, rect.Y, grapheme.text, style)
		x += grapheme.width
	}
}

// FillBox fills the interior of a box (excluding the border) with a character and style.
// This is useful for clearing the interior before drawing content.
func FillBox(buf *Buffer, rect Rect, r rune, style Style) {
	if rect.Width <= 2 || rect.Height <= 2 {
		return
	}

	interior := rect.Inset(EdgeAll(1))
	buf.Fill(interior, r, style)
}

// TableGridInfo describes the grid structure for DrawTableGrid.
type TableGridInfo struct {
	// ColWidths contains the content width of each column (excluding separators).
	ColWidths []int
	// RowHeights contains the content height of each row (excluding separators).
	RowHeights []int
	// HeaderRows is the number of header rows (a separator is drawn after them).
	// Set to 0 for no header separator, 1 for a single header row, etc.
	HeaderRows int
	// RowSeparators controls whether separator lines are drawn between body rows.
	RowSeparators bool
}

// DrawTableGrid draws a complete table grid on the buffer at the specified rectangle.
// It draws:
//   - The outer border (top, bottom, left, right edges with corners)
//   - Vertical column separators throughout the table
//   - A horizontal separator after header rows (if HeaderRows > 0)
//   - Horizontal separators between body rows (if RowSeparators is true)
//
// The rect must be large enough to contain the grid. The grid structure is:
//
//	┌──────┬──────┬──────┐  ← top border with TopTee at column separators
//	│ col1 │ col2 │ col3 │  ← header row
//	├──────┼──────┼──────┤  ← header separator (LeftTee, Cross, RightTee)
//	│ val1 │ val2 │ val3 │  ← body row
//	├──────┼──────┼──────┤  ← row separator (if RowSeparators=true)
//	│ val4 │ val5 │ val6 │  ← body row
//	└──────┴──────┴──────┘  ← bottom border with BottomTee at column separators
func DrawTableGrid(buf *Buffer, rect Rect, border BorderStyle, style Style, info TableGridInfo) {
	if border == BorderNone || rect.Width < 2 || rect.Height < 2 {
		return
	}
	if len(info.ColWidths) == 0 || len(info.RowHeights) == 0 {
		return
	}

	chars := border.Chars()

	// Clip rect to buffer bounds
	bufRect := buf.Rect()
	rect = rect.Intersect(bufRect)
	if rect.IsEmpty() || rect.Width < 2 || rect.Height < 2 {
		return
	}

	left := rect.X
	top := rect.Y
	right := rect.Right() - 1
	bottom := rect.Bottom() - 1

	// Precompute column separator X positions (absolute).
	// These are the X coordinates where vertical separators are drawn.
	colSepXs := make([]int, 0, len(info.ColWidths)-1)
	x := left
	for i, cw := range info.ColWidths {
		x += 1 + cw // skip left-border/separator + content width
		if i < len(info.ColWidths)-1 {
			if x <= right {
				colSepXs = append(colSepXs, x)
			}
		}
	}

	// Precompute row separator Y positions (absolute).
	// These are the Y coordinates where horizontal separators are drawn.
	rowSepYs := make([]int, 0, len(info.RowHeights))
	y := top
	for i, rh := range info.RowHeights {
		y += 1 + rh // skip top-border/separator + content height
		if i < len(info.RowHeights)-1 {
			isHeaderSep := info.HeaderRows > 0 && i == info.HeaderRows-1
			if isHeaderSep || info.RowSeparators {
				if y <= bottom {
					rowSepYs = append(rowSepYs, y)
				}
			}
		}
	}

	// Build a set for quick row-sep Y lookup
	rowSepSet := make(map[int]bool, len(rowSepYs))
	for _, sy := range rowSepYs {
		rowSepSet[sy] = true
	}

	// 1. Draw four corners
	buf.SetRune(left, top, chars.TopLeft, style)
	buf.SetRune(right, top, chars.TopRight, style)
	buf.SetRune(left, bottom, chars.BottomLeft, style)
	buf.SetRune(right, bottom, chars.BottomRight, style)

	// 2. Draw top edge with TopTee at column separator positions
	colSepSet := make(map[int]bool, len(colSepXs))
	for _, sx := range colSepXs {
		colSepSet[sx] = true
	}
	for x := left + 1; x < right; x++ {
		if colSepSet[x] {
			buf.SetRune(x, top, chars.TopTee, style)
		} else {
			buf.SetRune(x, top, chars.Top, style)
		}
	}

	// 3. Draw bottom edge with BottomTee at column separator positions
	for x := left + 1; x < right; x++ {
		if colSepSet[x] {
			buf.SetRune(x, bottom, chars.BottomTee, style)
		} else {
			buf.SetRune(x, bottom, chars.Bottom, style)
		}
	}

	// 4. Draw left and right edges (skipping row separator Y positions)
	for y := top + 1; y < bottom; y++ {
		if rowSepSet[y] {
			buf.SetRune(left, y, chars.LeftTee, style)
			buf.SetRune(right, y, chars.RightTee, style)
		} else {
			buf.SetRune(left, y, chars.Left, style)
			buf.SetRune(right, y, chars.Right, style)
		}
	}

	// 5. Draw vertical column separators (full height, adjusting for row separators)
	for _, sx := range colSepXs {
		for y := top + 1; y < bottom; y++ {
			if rowSepSet[y] {
				buf.SetRune(sx, y, chars.Cross, style)
			} else {
				buf.SetRune(sx, y, chars.Vertical, style)
			}
		}
	}

	// 6. Draw horizontal row separators
	for _, sy := range rowSepYs {
		for x := left + 1; x < right; x++ {
			if colSepSet[x] {
				// Already handled by step 5 (Cross character)
				continue
			}
			buf.SetRune(x, sy, chars.Horizontal, style)
		}
	}
}

// DrawTableGridClipped draws a complete table grid clipped to the given clipRect.
// It is the clipped counterpart to DrawTableGrid, used when rendering a table
// inside a scrollable or overflow-hidden container.
func DrawTableGridClipped(buf *Buffer, rect Rect, border BorderStyle, style Style, info TableGridInfo, clipRect Rect) {
	if border == BorderNone || rect.Width < 2 || rect.Height < 2 {
		return
	}
	if len(info.ColWidths) == 0 || len(info.RowHeights) == 0 {
		return
	}

	chars := border.Chars()

	left := rect.X
	top := rect.Y
	right := rect.Right() - 1
	bottom := rect.Bottom() - 1

	// Precompute column separator X positions (absolute).
	colSepXs := make([]int, 0, len(info.ColWidths)-1)
	x := left
	for i, cw := range info.ColWidths {
		x += 1 + cw
		if i < len(info.ColWidths)-1 {
			if x <= right {
				colSepXs = append(colSepXs, x)
			}
		}
	}

	// Precompute row separator Y positions (absolute).
	rowSepYs := make([]int, 0, len(info.RowHeights))
	y := top
	for i, rh := range info.RowHeights {
		y += 1 + rh
		if i < len(info.RowHeights)-1 {
			isHeaderSep := info.HeaderRows > 0 && i == info.HeaderRows-1
			if isHeaderSep || info.RowSeparators {
				if y <= bottom {
					rowSepYs = append(rowSepYs, y)
				}
			}
		}
	}

	// Build sets for quick lookup
	rowSepSet := make(map[int]bool, len(rowSepYs))
	for _, sy := range rowSepYs {
		rowSepSet[sy] = true
	}
	colSepSet := make(map[int]bool, len(colSepXs))
	for _, sx := range colSepXs {
		colSepSet[sx] = true
	}

	// Helper: only draw if within clipRect
	set := func(px, py int, r rune) {
		if clipRect.Contains(px, py) {
			buf.SetRune(px, py, r, style)
		}
	}

	// 1. Draw four corners
	set(left, top, chars.TopLeft)
	set(right, top, chars.TopRight)
	set(left, bottom, chars.BottomLeft)
	set(right, bottom, chars.BottomRight)

	// 2. Draw top edge with TopTee at column separator positions
	for x := left + 1; x < right; x++ {
		if colSepSet[x] {
			set(x, top, chars.TopTee)
		} else {
			set(x, top, chars.Top)
		}
	}

	// 3. Draw bottom edge with BottomTee at column separator positions
	for x := left + 1; x < right; x++ {
		if colSepSet[x] {
			set(x, bottom, chars.BottomTee)
		} else {
			set(x, bottom, chars.Bottom)
		}
	}

	// 4. Draw left and right edges
	for y := top + 1; y < bottom; y++ {
		if rowSepSet[y] {
			set(left, y, chars.LeftTee)
			set(right, y, chars.RightTee)
		} else {
			set(left, y, chars.Left)
			set(right, y, chars.Right)
		}
	}

	// 5. Draw vertical column separators
	for _, sx := range colSepXs {
		for y := top + 1; y < bottom; y++ {
			if rowSepSet[y] {
				set(sx, y, chars.Cross)
			} else {
				set(sx, y, chars.Vertical)
			}
		}
	}

	// 6. Draw horizontal row separators
	for _, sy := range rowSepYs {
		for x := left + 1; x < right; x++ {
			if colSepSet[x] {
				continue // Already handled by step 5
			}
			set(x, sy, chars.Horizontal)
		}
	}
}
