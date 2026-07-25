package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/grindlemire/go-tui"
)

type terminalBackgroundPreference uint8

const (
	terminalBackgroundDark terminalBackgroundPreference = iota
	terminalBackgroundLight
)

type codeHighlightPalette struct {
	Style *chroma.Style
	Theme codeHighlightTheme
}

type codeHighlightTheme struct {
	PanelBackground  tui.Style
	HeaderBackground tui.Style
	GutterBackground tui.Style
	BorderStyle      tui.Style
	HeaderText       tui.Style
	HeaderMeta       tui.Style
	LineNumber       tui.Style
	Separator        tui.Style
	PlainText        tui.Style
}

type highlightedCodeBlock struct {
	Language string
	Lines    [][]tui.StyledSpan
}

type codeBlockChromePalette struct {
	PanelBackground  tui.Color
	HeaderBackground tui.Color
	GutterBackground tui.Color
	BodyText         tui.Color
	MutedText        tui.Color
	AccentText       tui.Color
}

func detectTerminalBackgroundPreference() terminalBackgroundPreference {
	raw := strings.TrimSpace(os.Getenv("COLORFGBG"))
	if raw == "" {
		return terminalBackgroundDark
	}

	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == ':'
	})
	if len(parts) == 0 {
		return terminalBackgroundDark
	}

	bgIndex, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return terminalBackgroundDark
	}
	if bgIndex >= 7 {
		return terminalBackgroundLight
	}
	return terminalBackgroundDark
}

func currentCodeHighlightPalette() codeHighlightPalette {
	return newCodeHighlightPalette(detectTerminalBackgroundPreference())
}

func newCodeHighlightPalette(pref terminalBackgroundPreference) codeHighlightPalette {
	switch pref {
	case terminalBackgroundLight:
		style := styles.Get("github")
		return codeHighlightPalette{
			Style: style,
			Theme: newCodeHighlightTheme(style, codeBlockChromePalette{
				PanelBackground:  tui.DefaultColor(),
				HeaderBackground: tui.DefaultColor(),
				GutterBackground: tui.DefaultColor(),
				BodyText:         tui.RGBColor(36, 41, 47),
				MutedText:        tui.RGBColor(101, 109, 118),
				AccentText:       tui.RGBColor(140, 149, 159),
			}),
		}
	default:
		style := styles.Get("github-dark")
		return codeHighlightPalette{
			Style: style,
			Theme: newCodeHighlightTheme(style, codeBlockChromePalette{
				PanelBackground:  tui.DefaultColor(),
				HeaderBackground: tui.DefaultColor(),
				GutterBackground: tui.DefaultColor(),
				BodyText:         tui.RGBColor(230, 237, 243),
				MutedText:        tui.RGBColor(143, 151, 163),
				AccentText:       tui.RGBColor(110, 118, 129),
			}),
		}
	}
}

func newCodeHighlightTheme(style *chroma.Style, chrome codeBlockChromePalette) codeHighlightTheme {
	panelBg := tui.NewStyle().Background(chrome.PanelBackground)
	headerBg := tui.NewStyle().Background(chrome.HeaderBackground)
	gutterBg := tui.NewStyle().Background(chrome.GutterBackground)

	plainText := chromaEntryToTuiStyle(style.Get(chroma.Text))
	if plainText.Fg.IsDefault() {
		plainText = plainText.Foreground(chrome.BodyText)
	}
	plainText = plainText.Background(panelBg.Bg)

	lineNumber := tui.NewStyle().
		Foreground(chrome.MutedText).
		Background(gutterBg.Bg)

	headerText := plainText.Bold().Background(headerBg.Bg)
	headerMeta := tui.NewStyle().
		Foreground(chrome.MutedText).
		Dim().
		Background(headerBg.Bg)
	borderStyle := tui.NewStyle().
		Foreground(chrome.AccentText).
		Background(panelBg.Bg)

	return codeHighlightTheme{
		PanelBackground:  panelBg,
		HeaderBackground: headerBg,
		GutterBackground: gutterBg,
		BorderStyle:      borderStyle,
		HeaderText:       headerText,
		HeaderMeta:       headerMeta,
		LineNumber:       lineNumber,
		Separator:        lineNumber.Dim().Background(gutterBg.Bg),
		PlainText:        plainText,
	}
}

func renderCodeBlockPanel(code, lang string, depth int) []*tui.Element {
	palette := currentCodeHighlightPalette()
	block := highlightCodeBlock(code, lang, palette)
	indentCells := markdownIndentCells(depth)
	lineCount := len(block.Lines)
	if lineCount == 0 {
		lineCount = 1
	}
	lineNumberWidth := len(fmt.Sprintf("%d", lineCount))

	children := []*tui.Element{
		newCodeBlockHeaderElement(block.Language, lineCount, palette.Theme),
	}
	for i, lineSpans := range block.Lines {
		children = append(children, newCodeBlockLineElement(i+1, lineNumberWidth, lineSpans, palette.Theme))
	}
	if len(block.Lines) == 0 {
		children = append(children, newCodeBlockLineElement(1, lineNumberWidth, nil, palette.Theme))
	}

	opts := []tui.Option{
		tui.WithDisplay(tui.DisplayFlex),
		tui.WithDirection(tui.Column),
		tui.WithWidthPercent(100),
		tui.WithBorder(tui.BorderRounded),
		tui.WithBorderStyle(palette.Theme.BorderStyle),
		tui.WithPaddingTRBL(0, 1, 0, 1),
		tui.WithChildren(children...),
		tui.WithMarginTRBL(0, 0, 0, indentCells),
	}

	return []*tui.Element{tui.New(opts...)}
}

func newCodeBlockHeaderElement(language string, lineCount int, theme codeHighlightTheme) *tui.Element {
	spans := mergeAdjacentSpans([]tui.StyledSpan{
		{Text: language, Style: theme.HeaderText},
		{Text: " | " + i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeCodeLineCount, lineCount), Style: theme.HeaderMeta},
	})

	return tui.New(
		tui.WithStyledSpans(spans),
		tui.WithBackground(theme.HeaderBackground),
		tui.WithWidthPercent(100),
		tui.WithWrap(false),
		tui.WithTruncate(true),
	)
}

func newCodeBlockLineElement(lineNumber, lineNumberWidth int, codeSpans []tui.StyledSpan, theme codeHighlightTheme) *tui.Element {
	spans := make([]tui.StyledSpan, 0, len(codeSpans)+2)
	spans = append(spans,
		tui.StyledSpan{Text: fmt.Sprintf(" %*d", lineNumberWidth, lineNumber), Style: theme.LineNumber},
		tui.StyledSpan{Text: " │ ", Style: theme.Separator},
	)
	if len(codeSpans) == 0 {
		spans = append(spans, tui.StyledSpan{Text: "", Style: theme.PlainText})
	} else {
		spans = append(spans, codeSpans...)
	}

	return tui.New(
		tui.WithStyledSpans(mergeAdjacentSpans(spans)),
		tui.WithBackground(theme.PanelBackground),
		tui.WithWidthPercent(100),
		tui.WithWrap(false),
		tui.WithTruncate(true),
	)
}

func highlightCodeBlock(code, lang string, palette codeHighlightPalette) highlightedCodeBlock {
	lexer, displayLanguage := resolveCodeLexer(code, lang)
	coalesced := chroma.Coalesce(lexer)
	syntaxThemeBackground := chromaEntryToTuiStyle(palette.Style.Get(chroma.Background)).Bg

	iterator, err := coalesced.Tokenise(nil, code)
	if err != nil {
		return highlightedCodeBlock{
			Language: displayLanguage,
			Lines:    plainCodeLines(code, palette.Theme.PlainText),
		}
	}

	tokens := iterator.Tokens()
	if len(tokens) == 0 {
		return highlightedCodeBlock{
			Language: displayLanguage,
			Lines:    plainCodeLines(code, palette.Theme.PlainText),
		}
	}

	tokenLines := chroma.SplitTokensIntoLines(tokens)
	lines := make([][]tui.StyledSpan, 0, len(tokenLines))
	for _, tokenLine := range tokenLines {
		var lineSpans []tui.StyledSpan
		for _, tok := range tokenLine {
			text := strings.TrimSuffix(tok.Value, "\n")
			if text == "" {
				continue
			}
			style := chromaEntryToTuiStyle(palette.Style.Get(tok.Type))
			if style.Fg.IsDefault() {
				style.Fg = palette.Theme.PlainText.Fg
			}
			if style.Bg.IsDefault() || style.Bg == syntaxThemeBackground {
				style.Bg = palette.Theme.PanelBackground.Bg
			}
			lineSpans = append(lineSpans, tui.StyledSpan{
				Text:  text,
				Style: style,
			})
		}
		if len(lineSpans) == 0 {
			lines = append(lines, nil)
			continue
		}
		lines = append(lines, mergeAdjacentSpans(lineSpans))
	}

	if len(lines) == 0 {
		lines = [][]tui.StyledSpan{{}}
	}

	return highlightedCodeBlock{
		Language: displayLanguage,
		Lines:    lines,
	}
}

func resolveCodeLexer(code, lang string) (chroma.Lexer, string) {
	lang = strings.TrimSpace(lang)

	var lexer chroma.Lexer
	if lang != "" {
		lexer = lexers.Get(lang)
		if lexer == nil {
			lexer = lexers.Get(strings.ToLower(lang))
		}
	}
	if lexer == nil {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}

	displayLanguage := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeCodePlainText)
	if cfg := lexer.Config(); cfg != nil && cfg.Name != "" && lexer != lexers.Fallback {
		displayLanguage = cfg.Name
	} else if lang != "" {
		displayLanguage = lang
	}

	return lexer, displayLanguage
}

func plainCodeLines(code string, plainTextStyle tui.Style) [][]tui.StyledSpan {
	lines := strings.Split(code, "\n")
	out := make([][]tui.StyledSpan, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			out = append(out, nil)
			continue
		}
		out = append(out, []tui.StyledSpan{
			{Text: line, Style: plainTextStyle},
		})
	}
	return out
}

// chromaEntryToTuiStyle converts a chroma.StyleEntry into a go-tui Style.
func chromaEntryToTuiStyle(entry chroma.StyleEntry) tui.Style {
	style := tui.NewStyle()
	if entry.Colour.IsSet() {
		style = style.Foreground(
			tui.RGBColor(entry.Colour.Red(), entry.Colour.Green(), entry.Colour.Blue()),
		)
	}
	if entry.Background.IsSet() {
		style = style.Background(
			tui.RGBColor(entry.Background.Red(), entry.Background.Green(), entry.Background.Blue()),
		)
	}
	if entry.Bold == chroma.Yes {
		style = style.Bold()
	}
	if entry.Italic == chroma.Yes {
		style = style.Italic()
	}
	if entry.Underline == chroma.Yes {
		style = style.Underline()
	}
	return style
}
