package tui

import (
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

type textAreaGrapheme struct {
	text      string
	runeCount int
	width     int
}

func textAreaGraphemes(text string) []textAreaGrapheme {
	clusters := make([]textAreaGrapheme, 0, utf8.RuneCountInString(text))
	graphemes := uniseg.NewGraphemes(text)
	for graphemes.Next() {
		clusters = append(clusters, textAreaGrapheme{
			text:      graphemes.Str(),
			runeCount: utf8.RuneCountInString(graphemes.Str()),
			width:     graphemes.Width(),
		})
	}
	return clusters
}

// textAreaGraphemeBoundaries returns rune offsets for every user-perceived
// character boundary, including 0 and the end of the string. TextArea keeps
// rune offsets for persistence compatibility, while editing snaps to these
// boundaries so combining sequences and emoji stay intact.
func textAreaGraphemeBoundaries(text string) []int {
	boundaries := []int{0}
	runeOffset := 0
	for _, grapheme := range textAreaGraphemes(text) {
		runeOffset += grapheme.runeCount
		boundaries = append(boundaries, runeOffset)
	}
	return boundaries
}

func textAreaBoundaryAtOrBefore(text string, pos int) int {
	boundaries := textAreaGraphemeBoundaries(text)
	if pos <= 0 {
		return 0
	}
	result := 0
	for _, boundary := range boundaries {
		if boundary > pos {
			break
		}
		result = boundary
	}
	return result
}

func textAreaBoundaryAtOrAfter(text string, pos int) int {
	boundaries := textAreaGraphemeBoundaries(text)
	for _, boundary := range boundaries {
		if boundary >= pos {
			return boundary
		}
	}
	return boundaries[len(boundaries)-1]
}

func textAreaPreviousGraphemeBoundary(text string, pos int) int {
	boundaries := textAreaGraphemeBoundaries(text)
	result := 0
	for _, boundary := range boundaries {
		if boundary >= pos {
			break
		}
		result = boundary
	}
	return result
}

func textAreaNextGraphemeBoundary(text string, pos int) int {
	boundaries := textAreaGraphemeBoundaries(text)
	for _, boundary := range boundaries {
		if boundary > pos {
			return boundary
		}
	}
	return boundaries[len(boundaries)-1]
}
