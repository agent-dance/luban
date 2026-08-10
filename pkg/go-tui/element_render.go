package tui

import (
	"strings"

	"github.com/grindlemire/go-tui/internal/debug"
)

// inheritedStyle carries cascading visual properties down the element tree.
// Text style (Fg, Attrs) and background color cascade from parent to child.
// Each field is only used when the child does not explicitly set its own value.
type inheritedStyle struct {
	textStyle Style
	bg        *Style // nil = no inherited background
}

// effectiveStyles returns the resolved text style and background for an element,
// taking inheritance into account. If the element explicitly set its textStyle,
// that is used; otherwise the inherited textStyle is used. Similarly for background.
//
// Automatic contrast: If a non-default background is set without an explicit
// foreground, choose the palette foreground that contrasts with that background.
func effectiveStyles(e *Element, inherited inheritedStyle) (textStyle Style, bg *Style) {
	if e.textStyleSet {
		textStyle = e.textStyle
	} else {
		textStyle = inherited.textStyle
	}

	if e.background != nil {
		bg = e.background
	} else {
		bg = inherited.bg
	}

	textStyle = styleWithForegroundContrast(textStyle, bg)

	return textStyle, bg
}

func styleWithForegroundContrast(style Style, bg *Style) Style {
	if bg == nil || bg.Bg.IsDefault() || !style.Fg.IsDefault() {
		return style
	}
	if bg.Bg.IsLight() {
		style.Fg = Black
	} else {
		style.Fg = White
	}
	return style
}

// renderContext holds resolved state for rendering an element.
type renderContext struct {
	textStyle      Style
	bg             *Style
	childInherited inheritedStyle
}

// resolveRenderContext resolves effective styles and builds child inherited style.
// Handles style inheritance and <th> bold default.
func resolveRenderContext(e *Element, inherited inheritedStyle) renderContext {
	textStyle, bg := effectiveStyles(e, inherited)
	if e.tag == "th" && !e.textStyleSet {
		textStyle = textStyle.Bold()
	}
	return renderContext{
		textStyle: textStyle,
		bg:        bg,
		childInherited: inheritedStyle{
			textStyle: textStyle,
			bg:        bg,
		},
	}
}

// RenderTree traverses the Element tree and renders to the buffer.
// This renders the element and all its descendants.
func RenderTree(buf *Buffer, root *Element) {
	renderElement(buf, root, inheritedStyle{})
}

// renderElement renders a single element and recurses to its children.
func renderElement(buf *Buffer, e *Element, inherited inheritedStyle) {
	if e.hidden {
		return
	}

	// Call pre-render hook for custom update logic (polling, animations, etc.)
	if e.onUpdate != nil {
		e.onUpdate()
	}

	rect := e.Rect()

	// Skip if outside buffer bounds
	bufRect := buf.Rect()
	if !rect.Intersects(bufRect) {
		return
	}

	// Use custom render hook if set (used by wrappers)
	if e.onRender != nil {
		e.onRender(e, buf)
		return
	}

	// Resolve effective styles (inheritance + <th> bold)
	rc := resolveRenderContext(e, inherited)

	// Handle HR specially - draws a horizontal line and returns (no children)
	if e.hr {
		renderHR(buf, e, styleWithInheritedBackground(rc.textStyle, rc.bg))
		return
	}

	// 1. Fill background
	if e.bgGradient != nil {
		bgStyle := NewStyle()
		if rc.bg != nil {
			bgStyle = *rc.bg
		}
		buf.FillGradient(rect, ' ', *e.bgGradient, bgStyle)
	} else if rc.bg != nil {
		buf.Fill(rect, ' ', *rc.bg)
	}

	// Debug: highlight containers whose children overflow
	if debug.OverflowHighlight() && !e.IsScrollable() && len(e.children) > 0 {
		contentRect := e.ContentRect()
		for _, child := range e.children {
			if !contentRect.ContainsRect(child.Rect()) {
				debugBg := NewStyle().Background(BrightRed)
				buf.Fill(rect, ' ', debugBg)
				break
			}
		}
	}

	// 2. Draw border. Foreground/attributes stay local, while an unspecified
	// background inherits so an opaque application root cannot be punched
	// through by child borders.
	if e.border != BorderNone {
		borderStyle := styleWithInheritedBackground(e.borderStyle, rc.bg)
		if e.borderGradient != nil {
			DrawBoxGradient(buf, rect, e.border, *e.borderGradient, borderStyle)
		} else {
			DrawBox(buf, rect, e.border, borderStyle)
		}
	}

	// 3. Draw text content if present
	if e.text != "" {
		renderTextContent(buf, e, rc.textStyle, rc.bg)
	}

	// 4. Render children (with scroll handling if scrollable, or clipping if overflow-hidden)
	if e.IsScrollable() {
		renderScrollableChildren(buf, e, rc.childInherited)
	} else if e.overflow == OverflowHidden {
		clipRect := e.ContentRect()
		for _, child := range e.children {
			if child.overlay {
				continue
			}
			renderClippedElement(buf, child, clipRect, 0, 0, clipRect.X, clipRect.Y, rc.childInherited)
		}
	} else {
		for _, child := range e.children {
			if child.overlay {
				continue
			}
			renderElement(buf, child, rc.childInherited)
		}
	}
}

// renderScrollableChildren renders children with scroll offset and clipping.
func renderScrollableChildren(buf *Buffer, e *Element, childInherited inheritedStyle) {
	// First, do scroll-aware layout
	e.layoutScrollContent()

	// Get viewport (clip region)
	clipRect := e.ContentRect()

	// Reserve space for vertical scrollbar if needed
	if e.needsVerticalScrollbar() {
		clipRect.Width = max(0, clipRect.Width-1)
	}

	// Render each child with scroll offset and clipping
	for _, child := range e.children {
		renderClippedElement(buf, child, clipRect, e.scrollX, e.scrollY, clipRect.X, clipRect.Y, childInherited)
	}

	// Draw scrollbar with local foreground/attributes and inherited background.
	if e.needsVerticalScrollbar() {
		renderVerticalScrollbar(buf, e, childInherited.bg)
	}
}

// renderClippedElement renders an element with scroll offset and clipping.
func renderClippedElement(buf *Buffer, e *Element, clipRect Rect, scrollX, scrollY, viewportX, viewportY int, inherited inheritedStyle) {
	if e.hidden {
		return
	}

	childRect := e.Rect()

	// Translate from content space to screen space
	// Children are laid out starting from (0,0) in content space
	// We add viewport origin and subtract scroll offset
	screenX := viewportX + childRect.X - scrollX
	screenY := viewportY + childRect.Y - scrollY

	screenRect := Rect{
		X:      screenX,
		Y:      screenY,
		Width:  childRect.Width,
		Height: childRect.Height,
	}

	// Check if visible within clip region
	visibleRect := screenRect.Intersect(clipRect)
	if visibleRect.IsEmpty() {
		return
	}

	// Resolve effective styles (inheritance + <th> bold)
	rc := resolveRenderContext(e, inherited)

	// Handle HR specially - draws a horizontal line and returns (no children)
	if e.hr {
		char := hrCharacter(e.border)
		for x := visibleRect.X; x < visibleRect.Right(); x++ {
			buf.SetRune(x, screenY, char, rc.textStyle)
		}
		return
	}

	// Render background (only visible portion)
	if e.bgGradient != nil {
		bgStyle := NewStyle()
		if rc.bg != nil {
			bgStyle = *rc.bg
		}
		buf.FillGradient(visibleRect, ' ', *e.bgGradient, bgStyle)
	} else if rc.bg != nil {
		buf.Fill(visibleRect, ' ', *rc.bg)
	}

	// Render border clipped to viewport with the inherited background.
	if e.border != BorderNone {
		borderStyle := styleWithInheritedBackground(e.borderStyle, rc.bg)
		if e.borderGradient != nil {
			DrawBoxGradientClipped(buf, screenRect, e.border, *e.borderGradient, borderStyle, clipRect)
		} else {
			DrawBoxClipped(buf, screenRect, e.border, borderStyle, clipRect)
		}
	}

	// Render text with clipping
	if e.text != "" {
		textBaseX := screenX + e.style.Padding.Left
		textBaseY := screenY + e.style.Padding.Top
		if e.border != BorderNone {
			textBaseX += 1
			textBaseY += 1
		}

		availTextWidth := childRect.Width - e.style.Padding.Horizontal()
		if e.border != BorderNone {
			availTextWidth -= 2
		}

		var lines []string
		if !e.noWrap && availTextWidth > 0 {
			lines = wrapText(e.text, availTextWidth)
		} else {
			lines = []string{e.text}
		}

		if e.truncate {
			if e.noWrap {
				lines[0] = truncateText(lines[0], availTextWidth)
			} else {
				availTextHeight := childRect.Height - e.style.Padding.Vertical()
				if e.border != BorderNone {
					availTextHeight -= 2
				}
				if len(lines) > availTextHeight && availTextHeight > 0 {
					lines = lines[:availTextHeight]
					lines[availTextHeight-1] = truncateText(lines[availTextHeight-1], availTextWidth)
				}
			}
		}

		ts := rc.textStyle
		if rc.bg != nil && !rc.bg.Bg.IsDefault() {
			ts.Bg = rc.bg.Bg
		}
		hasSpans := len(e.styledSpans) > 0
		needPerCell := e.textGradient != nil || ts.Bg.IsDefault() || e.textPrefixLen > 0 || hasSpans

		var prefixStyle Style
		if e.textPrefixLen > 0 {
			prefixStyle = e.textPrefixStyle
			if rc.bg != nil && !rc.bg.Bg.IsDefault() && prefixStyle.Bg.IsDefault() {
				prefixStyle.Bg = rc.bg.Bg
			}
		}

		clippedSpanIdx := 0
		clippedSpanRuneOff := 0
		globalRuneOff := 0

		for lineIdx, line := range lines {
			textY := textBaseY + lineIdx
			if textY < clipRect.Y || textY >= clipRect.Bottom() {
				globalRuneOff += len([]rune(line))
				continue
			}

			lineWidth := stringWidth(line)
			textX := textBaseX

			if availTextWidth > lineWidth {
				switch e.textAlign {
				case TextAlignCenter:
					textX += (availTextWidth - lineWidth) / 2
				case TextAlignRight:
					textX += availTextWidth - lineWidth
				}
			}

			if needPerCell {
				clusters := splitTextGraphemes(line)
				if len(clusters) > 0 {
					if hasSpans {
						remaining := globalRuneOff
						clippedSpanIdx = 0
						clippedSpanRuneOff = 0
						for clippedSpanIdx < len(e.styledSpans) {
							spanRunes := len([]rune(e.styledSpans[clippedSpanIdx].Text))
							if remaining < spanRunes {
								clippedSpanRuneOff = remaining
								break
							}
							remaining -= spanRunes
							clippedSpanIdx++
						}
						if clippedSpanIdx >= len(e.styledSpans) {
							clippedSpanRuneOff = 0
						}
					}
					curX := textX
					for i, cluster := range clusters {
						if curX >= clipRect.Right() {
							globalRuneOff += graphemeRuneCount(clusters[i:])
							break
						}
						style := nextTextGraphemeStyle(e, ts, prefixStyle, rc.bg, &clippedSpanIdx, &clippedSpanRuneOff, &globalRuneOff, cluster.runeCount)
						if curX < clipRect.X {
							curX += cluster.width
							continue
						}
						if cluster.width > 1 && curX+cluster.width > clipRect.Right() {
							globalRuneOff += graphemeRuneCount(clusters[i+1:])
							break
						}
						if curX >= clipRect.X && curX < clipRect.Right() {
							if style.Bg.IsDefault() {
								cellBg := buf.Cell(curX, textY).Style.Bg
								if !cellBg.IsDefault() {
									style.Bg = cellBg
								}
							}
							if e.textGradient != nil {
								t := float64(i) / float64(len(clusters)-1)
								if len(clusters) == 1 {
									t = 0
								}
								style.Fg = e.textGradient.At(t)
							}
							buf.SetGrapheme(curX, textY, cluster.text, style)
						}
						curX += cluster.width
					}
				}
			} else {
				buf.SetStringClipped(textX, textY, line, ts, clipRect)
			}
		}
	}

	// Recurse to children
	// Propagate the original viewport and scroll offsets rather than re-basing
	// to the parent's screen position. Child Rect() values are absolute in the
	// temp container's coordinate space (from layoutScrollContent), so the same
	// viewport+scroll translation applies at every depth.
	for _, child := range e.children {
		renderClippedElement(buf, child, clipRect, scrollX, scrollY, viewportX, viewportY, rc.childInherited)
	}
}

// renderVerticalScrollbar draws the vertical scrollbar for a scrollable element.
func renderVerticalScrollbar(buf *Buffer, e *Element, bg *Style) {
	viewportRect := e.ContentRect()

	// Scrollbar position: right edge of content area
	trackX := viewportRect.Right() - 1
	trackTop := viewportRect.Y
	trackHeight := viewportRect.Height

	if trackHeight <= 0 {
		return
	}

	viewportHeight := viewportRect.Height
	contentHeight := e.contentHeight

	if contentHeight <= viewportHeight {
		return
	}

	// Thumb size proportional to viewport/content ratio
	thumbHeight := max(1, trackHeight*viewportHeight/contentHeight)

	// Thumb position based on scroll offset
	maxScroll := contentHeight - viewportHeight
	if maxScroll <= 0 {
		return
	}
	thumbTop := e.scrollY * (trackHeight - thumbHeight) / maxScroll

	// Draw track and thumb
	for y := 0; y < trackHeight; y++ {
		screenY := trackTop + y
		if y >= thumbTop && y < thumbTop+thumbHeight {
			buf.SetRune(trackX, screenY, '█', styleWithInheritedBackground(e.scrollbarThumbStyle, bg))
		} else {
			buf.SetRune(trackX, screenY, '│', styleWithInheritedBackground(e.scrollbarStyle, bg))
		}
	}
}

func styleWithInheritedBackground(style Style, bg *Style) Style {
	style = styleWithForegroundContrast(style, bg)
	if bg != nil && style.Bg.IsDefault() && !bg.Bg.IsDefault() {
		style.Bg = bg.Bg
	}
	return style
}

// truncateText truncates a string to fit within maxWidth terminal cells,
// replacing the overflow with an ellipsis character (…).
func truncateText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	textWidth := stringWidth(text)
	if textWidth <= maxWidth {
		return text
	}
	// Fit only complete grapheme clusters before the ellipsis.
	clusters := splitTextGraphemes(text)
	ellipsisWidth := StringWidth("…")
	curWidth := 0
	var prefix strings.Builder
	for _, cluster := range clusters {
		if curWidth+cluster.width+ellipsisWidth > maxWidth {
			return prefix.String() + "…"
		}
		prefix.WriteString(cluster.text)
		curWidth += cluster.width
	}
	return text
}

// spanStyleAtRune returns the Style for the next rune in the StyledSpan stream.
// Callers must iterate runes in order and pass cursor state by pointer.
func spanStyleAtRune(spans []StyledSpan, spanIdx *int, spanRuneOff *int) Style {
	for *spanIdx < len(spans) {
		spanRunes := len([]rune(spans[*spanIdx].Text))
		if *spanRuneOff < spanRunes {
			break
		}
		*spanRuneOff -= spanRunes
		*spanIdx++
	}
	var style Style
	if *spanIdx < len(spans) {
		style = spans[*spanIdx].Style
	}
	*spanRuneOff++
	return style
}

func nextTextRuneStyle(
	e *Element,
	baseStyle Style,
	prefixStyle Style,
	bg *Style,
	spanIdx *int,
	spanRuneOff *int,
	globalRuneOffset *int,
) Style {
	var style Style
	if len(e.styledSpans) > 0 {
		style = spanStyleAtRune(e.styledSpans, spanIdx, spanRuneOff)
	} else {
		style = baseStyle
		if e.textPrefixLen > 0 && *globalRuneOffset < e.textPrefixLen {
			style = prefixStyle
		}
	}
	style = styleWithForegroundContrast(style, bg)
	if bg != nil && !bg.Bg.IsDefault() && style.Bg.IsDefault() {
		style.Bg = bg.Bg
	}
	*globalRuneOffset++
	return style
}

func nextTextGraphemeStyle(
	e *Element,
	baseStyle Style,
	prefixStyle Style,
	bg *Style,
	spanIdx *int,
	spanRuneOff *int,
	globalRuneOffset *int,
	runeCount int,
) Style {
	style := baseStyle
	for index := 0; index < max(runeCount, 1); index++ {
		next := nextTextRuneStyle(e, baseStyle, prefixStyle, bg, spanIdx, spanRuneOff, globalRuneOffset)
		if index == 0 {
			style = next
		}
	}
	return style
}

// renderTextContent draws the text content within the element's content rect.
//
// Text is wrapped to fit within the content rect width when wrapping is enabled
// (the default). Each wrapped line is aligned independently according to textAlign.
//
// When the element width equals text width (intrinsic sizing), the text is drawn
// at the content rect origin - the parent's AlignItems handles centering.
//
// When the element width is larger than text width (explicit sizing), text-level
// alignment is applied. This supports use cases like centered text in a fixed-width
// button, while avoiding jitter for intrinsic-width text in a centered layout.
func renderTextContent(buf *Buffer, e *Element, textStyle Style, bg *Style) {
	contentRect := e.ContentRect()

	// Skip if content rect is empty or outside buffer
	if contentRect.IsEmpty() {
		return
	}

	// Compute wrapped lines
	var lines []string
	if !e.noWrap && contentRect.Width > 0 {
		lines = wrapText(e.text, contentRect.Width)
	} else {
		lines = []string{e.text}
	}

	// Apply truncation
	if e.truncate {
		if e.noWrap {
			// Single-line truncation (existing behavior)
			lines[0] = truncateText(lines[0], contentRect.Width)
		} else if len(lines) > contentRect.Height && contentRect.Height > 0 {
			// Truncate at last visible line
			lines = lines[:contentRect.Height]
			lines[contentRect.Height-1] = truncateText(lines[contentRect.Height-1], contentRect.Width)
		}
	}

	ts := textStyle
	// Merge background color into text style so text preserves the background
	if bg != nil && !bg.Bg.IsDefault() {
		ts.Bg = bg.Bg
	}

	// Determine visible line count
	maxLines := len(lines)
	if contentRect.Height > 0 && maxLines > contentRect.Height {
		maxLines = contentRect.Height
	}

	// Auto-scroll: if wrapped content overflows, use scrollY as line offset
	startLine := 0
	if len(lines) > contentRect.Height && contentRect.Height > 0 {
		e.contentHeight = len(lines) // store for scroll bounds
		startLine = e.scrollY
		if startLine > len(lines)-contentRect.Height {
			startLine = len(lines) - contentRect.Height
		}
		if startLine < 0 {
			startLine = 0
		}
		maxLines = contentRect.Height
	}

	// Total grapheme count across visible lines (for gradient calculation).
	totalGraphemes := 0
	if e.textGradient != nil {
		for i := startLine; i < startLine+maxLines && i < len(lines); i++ {
			totalGraphemes += len(splitTextGraphemes(lines[i]))
		}
	}

	hasSpans := len(e.styledSpans) > 0
	needPerCell := e.textGradient != nil || ts.Bg.IsDefault() || e.textPrefixLen > 0 || hasSpans
	graphemeOffset := 0 // running offset for gradient

	var prefixStyle Style
	if e.textPrefixLen > 0 {
		prefixStyle = e.textPrefixStyle
		if bg != nil && !bg.Bg.IsDefault() && prefixStyle.Bg.IsDefault() {
			prefixStyle.Bg = bg.Bg
		}
	}

	globalRuneOffset := 0
	for i := 0; i < startLine && i < len(lines); i++ {
		globalRuneOffset += len([]rune(lines[i]))
	}

	spanIdx := 0
	spanRuneOff := 0
	if hasSpans {
		remaining := globalRuneOffset
		for spanIdx < len(e.styledSpans) {
			spanRunes := len([]rune(e.styledSpans[spanIdx].Text))
			if remaining < spanRunes {
				spanRuneOff = remaining
				break
			}
			remaining -= spanRunes
			spanIdx++
		}
		if spanIdx >= len(e.styledSpans) {
			spanRuneOff = 0
		}
	}
	for lineIdx := 0; lineIdx < maxLines; lineIdx++ {
		srcIdx := startLine + lineIdx
		if srcIdx >= len(lines) {
			break
		}
		line := lines[srcIdx]
		lineWidth := stringWidth(line)
		x := contentRect.X
		y := contentRect.Y + lineIdx

		if y >= contentRect.Bottom() {
			break
		}

		// Per-line text alignment
		if contentRect.Width > lineWidth {
			switch e.textAlign {
			case TextAlignCenter:
				x += (contentRect.Width - lineWidth) / 2
			case TextAlignRight:
				x += contentRect.Width - lineWidth
			}
		}

		if needPerCell {
			clusters := splitTextGraphemes(line)
			curX := x
			for i, cluster := range clusters {
				if curX >= contentRect.Right() {
					globalRuneOffset += graphemeRuneCount(clusters[i:])
					break
				}
				if cluster.width > 1 && curX+cluster.width > contentRect.Right() {
					globalRuneOffset += graphemeRuneCount(clusters[i:])
					break
				}
				style := nextTextGraphemeStyle(e, ts, prefixStyle, bg, &spanIdx, &spanRuneOff, &globalRuneOffset, cluster.runeCount)
				if style.Bg.IsDefault() {
					cellBg := buf.Cell(curX, y).Style.Bg
					if !cellBg.IsDefault() {
						style.Bg = cellBg
					}
				}
				if e.textGradient != nil {
					t := 0.0
					if totalGraphemes > 1 {
						t = float64(graphemeOffset) / float64(totalGraphemes-1)
					}
					style.Fg = e.textGradient.At(t)
					graphemeOffset++
				}
				buf.SetGrapheme(curX, y, cluster.text, style)
				curX += cluster.width
			}
		} else {
			buf.SetStringClipped(x, y, line, ts, contentRect)
		}
	}
}

// Render calculates layout (if needed) and renders the entire tree to the buffer.
// This is the main entry point for rendering an Element tree.
// Note: onUpdate hooks are called in renderElement for each element in the tree.
func (e *Element) Render(buf *Buffer, width, height int) {
	if e.dirty {
		Calculate(e, width, height)
	}
	RenderTree(buf, e)
}

// hrCharacter returns the horizontal rule character based on border style.
func hrCharacter(border BorderStyle) rune {
	switch border {
	case BorderDouble:
		return '═' // U+2550
	case BorderThick:
		return '━' // U+2501
	default:
		return '─' // U+2500
	}
}

// renderHR draws a horizontal rule across the element's width.
func renderHR(buf *Buffer, e *Element, textStyle Style) {
	rect := e.ContentRect()
	char := hrCharacter(e.border)

	for x := rect.X; x < rect.Right(); x++ {
		buf.SetRune(x, rect.Y, char, textStyle)
	}
}
