package tui

import (
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// Cell represents a single character cell in the terminal buffer.
// Wide characters (CJK, emoji) occupy multiple cells; the first cell holds
// the rune, subsequent cells are marked as continuations.
type Cell struct {
	Rune  rune   // First rune of the grapheme (0 for continuation cells)
	Tail  string // Remaining runes in the grapheme cluster
	Style Style  // Visual styling
	Width uint8  // Display width (usually 1 or 2; 0 for continuation)
}

// NewCell creates a new Cell with automatic width detection.
func NewCell(r rune, style Style) Cell {
	return Cell{
		Rune:  r,
		Style: style,
		Width: uint8(RuneWidth(r)),
	}
}

// NewCellWithWidth creates a new Cell with an explicit width.
// Use this for continuation cells (width 0) or when width is already known.
func NewCellWithWidth(r rune, style Style, width uint8) Cell {
	return Cell{
		Rune:  r,
		Style: style,
		Width: width,
	}
}

func (c Cell) displayText() string {
	if c.Rune == 0 {
		return ""
	}
	if c.Tail == "" {
		return string(c.Rune)
	}
	return string(c.Rune) + c.Tail
}

// IsContinuation returns true if this cell is a continuation of a wide character.
// Continuation cells have Width == 0 and are placed after the primary cell.
func (c Cell) IsContinuation() bool {
	return c.Width == 0
}

// Equal returns true if both cells are identical.
func (c Cell) Equal(other Cell) bool {
	return c.Rune == other.Rune && c.Tail == other.Tail && c.Style.Equal(other.Style) && c.Width == other.Width
}

// IsEmpty returns true if this cell represents an empty/blank cell.
// A cell is empty if it's a space (or zero rune) with default styling.
func (c Cell) IsEmpty() bool {
	// Zero rune with any style is considered empty
	if c.Rune == 0 {
		return true
	}
	// Space with default style is considered empty
	if c.Rune == ' ' {
		return c.Style.Equal(NewStyle())
	}
	return false
}

// RuneWidth returns the display width of a rune in terminal cells.
// Returns 1 for most characters, 2 for wide characters (CJK/fullwidth, emoji).
//
// Note: this cell model reserves Width==0 for continuation cells only.
// Runes that are logically zero-width (combining marks, variation selectors,
// format controls) are explicitly recognized but still treated as width 1.
func RuneWidth(r rune) int {
	// Keep invalid/control runes narrow so they don't disrupt layout.
	if r < 0 || r > unicode.MaxRune {
		return 1
	}

	// C0 and C1 controls.
	if r < 0x20 || (r >= 0x7F && r < 0xA0) {
		return 1
	}

	// Combining marks and format code points are logically zero-width, but this
	// buffer model uses Width==0 only for continuation cells.
	if isZeroWidthRune(r) {
		return 1
	}

	if inRuneRanges(r, eastAsianWideRanges) || inRuneRanges(r, emojiWideRanges) {
		return 2
	}

	return 1
}

// StringWidth returns the physical terminal-cell width of a string. Unlike a
// sum of RuneWidth calls, it treats a user-perceived grapheme cluster as one
// unit, so sequences such as "✏️" and "👩🏽‍💻" match terminal cursor movement.
func StringWidth(s string) int {
	return uniseg.StringWidth(s)
}

type textGrapheme struct {
	text      string
	width     int
	runeCount int
}

func splitTextGraphemes(text string) []textGrapheme {
	if text == "" {
		return nil
	}
	clusters := make([]textGrapheme, 0, utf8.RuneCountInString(text))
	graphemes := uniseg.NewGraphemes(text)
	for graphemes.Next() {
		cluster := graphemes.Str()
		clusters = append(clusters, textGrapheme{
			text:      cluster,
			width:     graphemes.Width(),
			runeCount: utf8.RuneCountInString(cluster),
		})
	}
	return clusters
}

func graphemeRuneCount(clusters []textGrapheme) int {
	count := 0
	for _, cluster := range clusters {
		count += cluster.runeCount
	}
	return count
}

type runeRange struct {
	min rune
	max rune
}

// East Asian wide/fullwidth code point ranges.
var eastAsianWideRanges = []runeRange{
	{min: 0x1100, max: 0x115F},   // Hangul Jamo init. consonants
	{min: 0x2329, max: 0x232A},   // Angle brackets
	{min: 0x2E80, max: 0x303E},   // CJK radicals + punctuation (excluding U+303F)
	{min: 0x3040, max: 0xA4CF},   // Hiragana/Katakana/Bopomofo/CJK/Yi
	{min: 0xAC00, max: 0xD7A3},   // Hangul syllables
	{min: 0xF900, max: 0xFAFF},   // CJK compatibility ideographs
	{min: 0xFE10, max: 0xFE19},   // Vertical forms
	{min: 0xFE30, max: 0xFE6F},   // CJK compatibility forms + small forms
	{min: 0xFF00, max: 0xFF60},   // Fullwidth forms
	{min: 0xFFE0, max: 0xFFE6},   // Fullwidth symbol variants
	{min: 0x1B000, max: 0x1B12F}, // Kana supplement + Kana ext. A
	{min: 0x1B130, max: 0x1B167}, // Kana extended B
	{min: 0x20000, max: 0x2FFFD}, // CJK extensions
	{min: 0x30000, max: 0x3FFFD}, // CJK extensions
}

// Emoji ranges that terminals commonly render as 2-cell glyphs.
// Includes Miscellaneous Symbols (U+2600-U+26FF), Dingbats (U+2700-U+27BF),
// and Miscellaneous Technical symbols (U+2300-U+23FF) that terminals render wide.
var emojiWideRanges = []runeRange{
	{min: 0x231A, max: 0x231B},   // Watch, Hourglass
	{min: 0x23E9, max: 0x23F3},   // Double fast arrows, hourglass (⏩⏪⏫⏬⏭⏮⏯⏰⏱⏲⏳)
	{min: 0x23F8, max: 0x23FA},   // Pause, record, stop (⏸⏹⏺)
	{min: 0x25AA, max: 0x25AB},   // Small squares
	{min: 0x25B6, max: 0x25B6},   // Right-pointing triangle
	{min: 0x25C0, max: 0x25C0},   // Left-pointing triangle
	{min: 0x25FB, max: 0x25FE},   // Medium/small squares
	{min: 0x2600, max: 0x2604},   // Sun, cloud, umbrella, snowman, comet
	{min: 0x260E, max: 0x260E},   // Telephone
	{min: 0x2611, max: 0x2611},   // Ballot box with check
	{min: 0x2614, max: 0x2615},   // Umbrella, hot beverage
	{min: 0x2618, max: 0x2618},   // Shamrock
	{min: 0x261D, max: 0x261D},   // Index pointing up
	{min: 0x2620, max: 0x2620},   // Skull and crossbones
	{min: 0x2622, max: 0x2623},   // Radioactive, biohazard
	{min: 0x2626, max: 0x2626},   // Orthodox cross
	{min: 0x262A, max: 0x262A},   // Star and crescent
	{min: 0x262E, max: 0x262F},   // Peace symbol, yin yang
	{min: 0x2638, max: 0x263A},   // Wheel of dharma, smiley faces
	{min: 0x2640, max: 0x2640},   // Female sign
	{min: 0x2642, max: 0x2642},   // Male sign
	{min: 0x2648, max: 0x2653},   // Zodiac signs (♈-♓)
	{min: 0x265F, max: 0x2660},   // Chess pawn, spade
	{min: 0x2663, max: 0x2663},   // Club suit
	{min: 0x2665, max: 0x2666},   // Heart, diamond suits
	{min: 0x2668, max: 0x2668},   // Hot springs
	{min: 0x267B, max: 0x267B},   // Recycling symbol
	{min: 0x267E, max: 0x267F},   // Infinity, wheelchair
	{min: 0x2692, max: 0x2697},   // Hammers, scales, alembic
	{min: 0x2699, max: 0x2699},   // Gear
	{min: 0x269B, max: 0x269C},   // Atom, fleur-de-lis
	{min: 0x26A0, max: 0x26A1},   // Warning, high voltage (⚠⚡)
	{min: 0x26A7, max: 0x26A7},   // Transgender symbol
	{min: 0x26AA, max: 0x26AB},   // White/black circles (⚪⚫)
	{min: 0x26B0, max: 0x26B1},   // Coffin, funeral urn
	{min: 0x26BD, max: 0x26BE},   // Soccer, baseball
	{min: 0x26C4, max: 0x26C5},   // Snowman, partly cloudy
	{min: 0x26CE, max: 0x26CE},   // Ophiuchus
	{min: 0x26CF, max: 0x26CF},   // Pick
	{min: 0x26D1, max: 0x26D1},   // Helmet with cross
	{min: 0x26D3, max: 0x26D4},   // Chains, no entry
	{min: 0x26E9, max: 0x26EA},   // Shinto shrine, church
	{min: 0x26F0, max: 0x26F5},   // Mountain, umbrella, ferry, etc.
	{min: 0x26F7, max: 0x26FA},   // Skier, ice skate, tent, etc.
	{min: 0x26FD, max: 0x26FD},   // Fuel pump
	{min: 0x2702, max: 0x2702},   // Scissors
	{min: 0x2705, max: 0x2705},   // Check mark
	{min: 0x2708, max: 0x270D},   // Airplane, envelope, pen, etc.
	{min: 0x270F, max: 0x270F},   // Pencil
	{min: 0x2712, max: 0x2712},   // Black nib
	{min: 0x2714, max: 0x2714},   // Heavy check mark
	{min: 0x2716, max: 0x2716},   // Heavy multiplication
	{min: 0x271D, max: 0x271D},   // Latin cross
	{min: 0x2721, max: 0x2721},   // Star of David
	{min: 0x2728, max: 0x2728},   // Sparkles
	{min: 0x2733, max: 0x2734},   // Eight-spoked asterisk
	{min: 0x2744, max: 0x2744},   // Snowflake
	{min: 0x2747, max: 0x2747},   // Sparkle
	{min: 0x274C, max: 0x274C},   // Cross mark
	{min: 0x274E, max: 0x274E},   // Cross mark with outline
	{min: 0x2753, max: 0x2755},   // Question/exclamation marks (❓❔❕)
	{min: 0x2757, max: 0x2757},   // Exclamation mark (❗)
	{min: 0x2763, max: 0x2764},   // Heart exclamation, heavy heart
	{min: 0x2795, max: 0x2797},   // Heavy plus/minus/division
	{min: 0x27A1, max: 0x27A1},   // Right arrow
	{min: 0x27B0, max: 0x27B0},   // Curly loop
	{min: 0x27BF, max: 0x27BF},   // Double curly loop
	{min: 0x2934, max: 0x2935},   // Arrow right up/down
	{min: 0x2B05, max: 0x2B07},   // Left/up/down arrows
	{min: 0x2B1B, max: 0x2B1C},   // Black/white large squares
	{min: 0x2B50, max: 0x2B50},   // Star
	{min: 0x2B55, max: 0x2B55},   // Heavy large circle
	{min: 0x3030, max: 0x3030},   // Wavy dash
	{min: 0x303D, max: 0x303D},   // Part alternation mark
	{min: 0x3297, max: 0x3297},   // Circled ideograph congratulation
	{min: 0x3299, max: 0x3299},   // Circled ideograph secret
	{min: 0x1F004, max: 0x1F004}, // Mahjong tile red dragon
	{min: 0x1F0CF, max: 0x1F0CF}, // Playing card black joker
	{min: 0x1F170, max: 0x1F171}, // Negative squared A/B
	{min: 0x1F17E, max: 0x1F17F}, // Negative squared O/P
	{min: 0x1F18E, max: 0x1F18E}, // Negative squared AB
	{min: 0x1F191, max: 0x1F19A}, // Squared symbols
	{min: 0x1F1E6, max: 0x1F1FF}, // Regional indicator symbols (flags)
	{min: 0x1F201, max: 0x1F202}, // Squared Katakana words
	{min: 0x1F21A, max: 0x1F21A}, // Squared CJK ideograph
	{min: 0x1F22F, max: 0x1F22F}, // Squared CJK ideograph
	{min: 0x1F232, max: 0x1F23A}, // Squared CJK ideographs
	{min: 0x1F250, max: 0x1F251}, // Circled ideographs
	{min: 0x1F300, max: 0x1F64F}, // Pictographs + emoticons
	{min: 0x1F680, max: 0x1F6FF}, // Transport/map symbols
	{min: 0x1F7E0, max: 0x1F7EB}, // Large colored circles/squares
	{min: 0x1F900, max: 0x1F9FF}, // Supplemental symbols/pictographs
	{min: 0x1FA70, max: 0x1FAFF}, // Symbols/pictographs ext. A
}

func isZeroWidthRune(r rune) bool {
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Variation_Selector, unicode.Join_Control)
}

func inRuneRanges(r rune, ranges []runeRange) bool {
	for _, rr := range ranges {
		if r >= rr.min && r <= rr.max {
			return true
		}
	}
	return false
}
