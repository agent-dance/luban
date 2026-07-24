package tui

// Compile-time check that Element implements the required interfaces.
var (
	_ Viewable   = (*Element)(nil)
	_ Focusable  = (*Element)(nil)
	_ Layoutable = (*Element)(nil)
)

// TextAlign specifies how text is aligned within its content area.
type TextAlign int

const (
	// TextAlignLeft aligns text to the left edge (default).
	TextAlignLeft TextAlign = iota
	// TextAlignCenter centers text horizontally.
	TextAlignCenter
	// TextAlignRight aligns text to the right edge.
	TextAlignRight
)

// ScrollMode specifies how an element scrolls its content.
type ScrollMode int

const (
	// ScrollNone disables scrolling (default).
	ScrollNone ScrollMode = iota
	// ScrollVertical enables vertical scrolling.
	ScrollVertical
	// ScrollHorizontal enables horizontal scrolling.
	ScrollHorizontal
	// ScrollBoth enables both vertical and horizontal scrolling.
	ScrollBoth
)

// OverflowMode specifies how an element handles content that exceeds its bounds.
type OverflowMode int

const (
	// OverflowVisible allows content to render outside the element's bounds (default).
	OverflowVisible OverflowMode = iota
	// OverflowHidden clips content at the element's bounds without scrollbars.
	OverflowHidden
)

// StyledSpan represents a contiguous segment of text with a specific style.
// When an Element has StyledSpans set, the text field is ignored for rendering
// purposes; instead, the spans are concatenated and each span's style is used
// during rendering.
type StyledSpan struct {
	Text  string // the text content of this span
	Style Style  // visual style for this span
}

// Element is a layout container with visual properties.
// It implements Layoutable and owns its children directly.
type Element struct {
	// Tree structure (single source of truth)
	children []*Element
	parent   *Element
	app      *App

	// Layout properties
	style  LayoutStyle
	layout LayoutResult
	dirty  bool

	// Visual properties
	border      BorderStyle
	borderStyle Style
	background  *Style // nil = transparent

	// Text properties
	text         string
	textStyle    Style
	textStyleSet bool // true if textStyle was explicitly configured (false = inherit from parent)
	textAlign    TextAlign
	truncate     bool
	noWrap       bool // true = wrapping disabled; false (default) = wrapping enabled

	// Rich text: a sequence of styled spans that replaces text + textStyle
	// when non-nil. Each span carries its own style, enabling inline formatting
	// (bold, italic, colored code, links, etc.) within a single Element.
	styledSpans []StyledSpan

	// Styled text prefix: the first len([]rune(textPrefix)) runes of text
	// are rendered with textPrefixStyle instead of textStyle.
	// The prefix is part of the text string (prepended in WithText or set
	// separately). textPrefixLen stores the rune count for O(1) lookup.
	textPrefixLen   int
	textPrefixStyle Style

	// Focus properties
	focusable        bool
	tabStop          bool // whether this element appears in Tab/Shift+Tab navigation
	focused          bool
	autoFocus        bool // request initial focus on this element
	onFocus          func(*Element)
	onBlur           func(*Element)
	onActivate       func() // called when Enter is pressed while focused (e.g. modal buttons)
	savedBorderStyle Style  // border style saved before focus highlight
	hasSavedBorder   bool   // true if savedBorderStyle is valid

	// Tree notification
	onChildAdded     func(*Element)
	onFocusableAdded func(Focusable)

	// Custom render hook (used by wrappers that need custom rendering)
	onRender func(*Element, *Buffer)

	// Scroll properties
	scrollMode            ScrollMode
	scrollX               int  // Current horizontal scroll offset
	scrollY               int  // Current vertical scroll offset
	contentWidth          int  // Computed content width (may exceed viewport)
	contentHeight         int  // Computed content height (may exceed viewport)
	scrollToBottomPending bool // Scroll to bottom after next layout

	// Scrollbar styles
	scrollbarStyle      Style
	scrollbarThumbStyle Style

	// HR properties
	hr bool // true if this element is a horizontal rule

	// Visibility
	hidden bool

	// Overlay flag - element renders in overlay pass, not in normal tree
	overlay bool

	// Overflow clipping
	overflow OverflowMode

	// Gradient properties (nil = no gradient, use solid color)
	textGradient   *Gradient
	bgGradient     *Gradient
	borderGradient *Gradient

	// Pre-render hook for custom update logic (polling, animations, etc.)
	onUpdate func()

	// Watchers attached to this element (timers, channel watchers, etc.)
	watchers []Watcher

	// Tag identifies the element type for layout dispatch (e.g., "table", "tr", "td", "th")
	tag string

	// Table rendering options (only meaningful when tag == "table")
	tableHeaderRows   int  // number of header rows for header separator line
	tableRowSeparator bool // whether to draw separators between body rows

	// Component that produced this element (set by Mount, read during tree walks)
	component Component
}

// New creates a new Element with the given options.
// By default, an Element has Auto width/height (flexes to fill available space).
func New(opts ...Option) *Element {
	e := &Element{
		style: DefaultLayoutStyle(),
		dirty: true,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}
