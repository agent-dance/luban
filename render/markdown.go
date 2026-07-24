package render

import (
	"sync"

	glamour "charm.land/glamour/v2"
)

// glamourRenderer is a lazily-initialized, reusable glamour TermRenderer.
var (
	glamourOnce     sync.Once
	glamourRenderer *glamour.TermRenderer
)

func getGlamourRenderer() *glamour.TermRenderer {
	glamourOnce.Do(func() {
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(80),
		)
		if err != nil {
			glamourRenderer = nil
			return
		}
		glamourRenderer = r
	})
	return glamourRenderer
}

// Markdown converts Markdown text to ANSI-colored terminal output using
// charmbracelet/glamour. It supports headings, bold, italic, inline code,
// fenced code blocks (with syntax highlighting), lists, horizontal rules,
// links, tables, blockquotes, strikethrough, and more.
//
// If glamour is unavailable, the input is returned as-is.
func Markdown(input string) string {
	r := getGlamourRenderer()
	if r == nil {
		return input
	}

	out, err := r.Render(input)
	if err != nil {
		return input
	}

	return out
}
