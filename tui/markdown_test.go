package tui

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/grindlemire/go-tui"
)

func TestRenderMarkdown_BasicText(t *testing.T) {
	elements := renderMarkdown("hello world")
	if len(elements) == 0 {
		t.Fatal("expected at least 1 element for basic text")
	}
	// Output should contain "hello world" as plain text
	allText := elementsText(elements)
	if !strings.Contains(allText, "hello world") {
		t.Errorf("expected 'hello world' in output, got: %q", allText)
	}
}

func TestRenderMarkdown_CodeBlock(t *testing.T) {
	input := "```go\nfmt.Println(\"hello\")\nfmt.Println(\"world\")\n```"
	elements := renderMarkdown(input)
	if len(elements) == 0 {
		t.Fatal("expected elements for code block")
	}
	codeBlock := findCodeBlockContainer(elements)
	if codeBlock == nil {
		t.Fatal("expected bordered code block container")
	}
	if len(codeBlock.Children()) < 3 {
		t.Fatalf("expected header + code lines inside code block, got %d child elements", len(codeBlock.Children()))
	}
	allText := elementsText(elements)
	if !strings.Contains(allText, "Go | 2 lines") {
		t.Errorf("expected code panel header in output, got: %q", allText)
	}
	if !strings.Contains(allText, "1 │ fmt.Println(\"hello\")") {
		t.Errorf("expected numbered first code line in output, got: %q", allText)
	}
	if !strings.Contains(allText, "2 │ fmt.Println(\"world\")") {
		t.Errorf("expected numbered second code line in output, got: %q", allText)
	}
}

func TestRenderMarkdown_Headings(t *testing.T) {
	input := "# Title\n\n## Subtitle\n\n### Section"
	elements := renderMarkdown(input)
	if len(elements) == 0 {
		t.Fatal("expected elements for headings")
	}
	allText := elementsText(elements)
	if !strings.Contains(allText, "# Title") {
		t.Errorf("expected '# Title' in output, got: %q", allText)
	}
	if !strings.Contains(allText, "## Subtitle") {
		t.Errorf("expected '## Subtitle' in output, got: %q", allText)
	}
}

func TestRenderMarkdown_BulletList(t *testing.T) {
	input := "- item one\n- item two\n- item three"
	elements := renderMarkdown(input)
	if len(elements) == 0 {
		t.Fatal("expected elements for bullet list")
	}
	allText := elementsText(elements)
	if !strings.Contains(allText, "item one") {
		t.Errorf("expected 'item one' in output, got: %q", allText)
	}
}

func TestRenderMarkdown_HorizontalRule(t *testing.T) {
	input := "---"
	elements := renderMarkdown(input)
	if len(elements) == 0 {
		t.Fatal("expected elements for horizontal rule")
	}
}

func TestRenderMarkdown_MixedContent(t *testing.T) {
	input := "# Hello\n\nSome **bold** text.\n\n```bash\necho hi\n```\n\n- item one\n- item two"
	elements := renderMarkdown(input)
	if len(elements) < 3 {
		t.Errorf("expected at least 3 elements for mixed content, got %d", len(elements))
	}
	allText := elementsText(elements)
	if !strings.Contains(allText, "Hello") {
		t.Errorf("expected 'Hello' in output, got: %q", allText)
	}
	if !strings.Contains(allText, "echo hi") {
		t.Errorf("expected 'echo hi' in output, got: %q", allText)
	}
}

func TestRenderMarkdown_Blockquote(t *testing.T) {
	input := "> important note"
	elements := renderMarkdown(input)
	if len(elements) == 0 {
		t.Fatal("expected elements for blockquote")
	}
	allText := elementsText(elements)
	if !strings.Contains(allText, "important note") {
		t.Errorf("expected 'important note' in output, got: %q", allText)
	}
}

func TestRenderMarkdown_NumberedList(t *testing.T) {
	input := "1. first\n2. second\n3. third"
	elements := renderMarkdown(input)
	if len(elements) == 0 {
		t.Fatal("expected elements for numbered list")
	}
	allText := elementsText(elements)
	if !strings.Contains(allText, "first") {
		t.Errorf("expected 'first' in output, got: %q", allText)
	}
}

func TestRenderMarkdown_NestedListIndentAndSpacing(t *testing.T) {
	input := "- top level\n  - child one\n  - child two\n- next top level"
	elements := renderMarkdown(input)
	if len(elements) == 0 {
		t.Fatal("expected elements for nested list")
	}

	var childOne *tui.Element
	childTwoIdx := -1
	nextIdx := -1
	for i, el := range elements {
		text := el.Text()
		switch {
		case strings.Contains(text, "child one"):
			childOne = el
		case strings.Contains(text, "child two"):
			childTwoIdx = i
		case strings.Contains(text, "next top level"):
			nextIdx = i
		}
	}

	if childOne == nil {
		t.Fatal("expected nested child list item element")
	}
	if got := childOne.Style().Padding.Left; got != markdownIndentCells(1) {
		t.Fatalf("nested child padding left = %d, want %d", got, markdownIndentCells(1))
	}
	if !childOne.HasStyledSpans() {
		t.Fatal("expected nested child to use styled spans")
	}
	spans := childOne.StyledSpans()
	if len(spans) == 0 || spans[0].Text != "• " {
		t.Fatalf("nested child marker span = %q, want %q", firstSpanText(spans), "• ")
	}

	if childTwoIdx < 0 || nextIdx < 0 {
		t.Fatalf("expected child two and next top level elements, got childTwoIdx=%d nextIdx=%d", childTwoIdx, nextIdx)
	}
	if nextIdx-childTwoIdx < 2 {
		t.Fatalf("expected spacer between nested list and next top-level item, indexes childTwo=%d next=%d", childTwoIdx, nextIdx)
	}
	if spacer := elements[childTwoIdx+1]; spacer.Text() != "" {
		t.Fatalf("expected blank spacer element after nested list, got %q", spacer.Text())
	}

	root := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithChildren(elements...),
	)
	buf := tui.NewBuffer(40, 12)
	root.Render(buf, 40, 12)

	var childLine string
	var nextLine string
	for y := 0; y < 12; y++ {
		line := extractBufferLine(buf, y, 40)
		switch {
		case strings.Contains(line, "child one"):
			childLine = line
		case strings.Contains(line, "next top level"):
			nextLine = line
		}
	}

	if childLine == "" {
		t.Fatal("expected rendered buffer line for nested child item")
	}
	if !strings.HasPrefix(childLine, "  • child one") {
		t.Fatalf("nested child line = %q, want prefix %q", childLine, "  • child one")
	}
	if nextLine == "" {
		t.Fatal("expected rendered buffer line for next top-level item")
	}
	if !strings.HasPrefix(nextLine, "• next top level") {
		t.Fatalf("next top-level line = %q, want prefix %q", nextLine, "• next top level")
	}
}

func TestRenderMarkdown_Table(t *testing.T) {
	input := "| Name | Age |\n|------|-----|\n| Alice | 30 |\n| Bob | 25 |"
	elements := renderMarkdown(input)
	if len(elements) == 0 {
		t.Fatal("expected elements for table")
	}

	// Find the <table> element — it may be a top-level element or nested
	// inside a wrapper, depending on the current renderTable implementation.
	var tableEl *tui.Element
	for _, el := range elements {
		if el.Tag() == "table" {
			tableEl = el
			break
		}
		for _, child := range el.Children() {
			if child.Tag() == "table" {
				tableEl = child
				break
			}
		}
		if tableEl != nil {
			break
		}
	}

	if tableEl == nil {
		t.Fatal("expected a <table> element in the output tree")
	}

	// Table should have a border
	if tableEl.Border() == tui.BorderNone {
		t.Error("table element should have a border")
	}

	// Table should have header rows set
	if tableEl.TableHeaderRows() != 1 {
		t.Errorf("table header rows = %d, want 1", tableEl.TableHeaderRows())
	}

	// Table should have 3 <tr> children (1 header + 2 body)
	rows := tableEl.Children()
	if len(rows) != 3 {
		t.Fatalf("table should have 3 <tr> children, got %d", len(rows))
	}

	// First row should be header (with th children)
	firstRowChildren := rows[0].Children()
	if len(firstRowChildren) != 2 {
		t.Fatalf("header row should have 2 cells, got %d", len(firstRowChildren))
	}
	if firstRowChildren[0].Tag() != "th" {
		t.Errorf("first header cell tag = %q, want 'th'", firstRowChildren[0].Tag())
	}

	// Body rows should have td children
	bodyRowChildren := rows[1].Children()
	if len(bodyRowChildren) != 2 {
		t.Fatalf("body row should have 2 cells, got %d", len(bodyRowChildren))
	}
	if bodyRowChildren[0].Tag() != "td" {
		t.Errorf("first body cell tag = %q, want 'td'", bodyRowChildren[0].Tag())
	}
}

func TestRenderMarkdown_TableRendersGrid(t *testing.T) {
	// Full integration: render markdown table to a buffer and check grid characters
	input := "| A | B |\n|---|---|\n| x | y |"
	elements := renderMarkdown(input)
	if len(elements) == 0 {
		t.Fatal("expected elements for table")
	}

	// Render to buffer
	root := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithChildren(elements...),
	)
	buf := tui.NewBuffer(40, 20)
	root.Render(buf, 40, 20)

	// The table should be somewhere in the buffer with rounded border corners
	// Scan for ╭ (BorderRounded TopLeft character)
	found := false
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			cell := buf.Cell(x, y)
			if cell.Rune == '╭' {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Error("expected rounded border top-left corner ╭ in rendered table")
	}
}

func TestRenderMarkdown_Empty(t *testing.T) {
	elements := renderMarkdown("")
	if len(elements) != 0 {
		t.Errorf("expected 0 elements for empty input, got %d", len(elements))
	}
	elements = renderMarkdown("   \n  \n  ")
	if len(elements) != 0 {
		t.Errorf("expected 0 elements for whitespace-only input, got %d", len(elements))
	}
}

func TestRenderMarkdown_UnclosedCodeBlock(t *testing.T) {
	input := "```python\nprint('hello')\nprint('world')"
	elements := renderMarkdown(input)
	// goldmark handles unclosed code blocks gracefully
	if len(elements) == 0 {
		t.Fatal("expected elements even for unclosed code block")
	}
}

func TestIsDiffContent(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"--- a/file.go\n+++ b/file.go\n@@ -1,3 +1,4 @@\n+new line", true},
		{"just normal text", false},
		{"diff --git a/f b/f\n--- a/f\n+++ b/f", true},
		{"+added\n-removed", false}, // not enough indicators
	}
	for _, tt := range tests {
		got := isDiffContent(tt.input)
		if got != tt.want {
			preview := tt.input
			if len(preview) > 30 {
				preview = preview[:30]
			}
			t.Errorf("isDiffContent(%q) = %v, want %v", preview, got, tt.want)
		}
	}
}

func TestRenderDiffLines(t *testing.T) {
	input := "--- a/file.go\n+++ b/file.go\n@@ -1,3 +1,4 @@\n context\n+added line\n-removed line"
	elements := renderDiffLines(input)
	if len(elements) != 6 {
		t.Fatalf("expected 6 diff line elements, got %d", len(elements))
	}
}

func TestRenderPlainText(t *testing.T) {
	elements := renderPlainText("line1\nline2\nline3")
	if len(elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(elements))
	}
}

func TestIsMarkdownHR(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"---", true},
		{"***", true},
		{"___", true},
		{"- - -", true},
		{"--", false},
		{"hello", false},
	}
	for _, tt := range tests {
		got := isMarkdownHR(tt.input)
		if got != tt.want {
			t.Errorf("isMarkdownHR(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestRenderMarkdown_InlineFormatting(t *testing.T) {
	// With native rendering, inline formatting is flattened to plain text.
	// Bold/italic markers are stripped, inline code is wrapped with backticks.
	input := "This is **bold** and *italic* and `code`."
	elements := renderMarkdown(input)
	if len(elements) == 0 {
		t.Fatal("expected elements for inline formatting")
	}

	allText := elementsText(elements)
	if !strings.Contains(allText, "bold") {
		t.Errorf("output should contain 'bold', got: %s", allText)
	}
	if !strings.Contains(allText, "italic") {
		t.Errorf("output should contain 'italic', got: %s", allText)
	}
	if !strings.Contains(allText, "code") {
		t.Errorf("output should contain 'code', got: %s", allText)
	}
}

func TestRenderMarkdown_RelaxedSpacingBetweenBlocks(t *testing.T) {
	input := "First paragraph.\n\nSecond paragraph.\n\n- item one\n- item two"
	elements := renderMarkdown(input)
	if len(elements) == 0 {
		t.Fatal("expected elements for mixed blocks")
	}

	blankLines := 0
	for _, el := range elements {
		if el.Text() == "" {
			blankLines++
		}
	}
	if blankLines < 2 {
		t.Fatalf("expected relaxed spacing blank lines between blocks, got %d", blankLines)
	}
}

func TestRenderMarkdown_CodeAndTableKeepSingleBlankLineAroundBlock(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		tag      string
	}{
		{
			name:     "code block",
			markdown: "Before text that wraps onto a second line in a narrow viewport before the block.\n\n```sh\necho hi\n```\n\nAfter.",
		},
		{
			name:     "code block after wrapped heading",
			markdown: "# Before heading that wraps onto another line in a narrow viewport before the block\n\n```sh\necho hi\n```\n\nAfter.",
		},
		{
			name:     "table",
			markdown: "Before.\n\n| A | B |\n|---|---|\n| x | y |\n\nAfter.",
			tag:      "table",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elements := renderMarkdown(tt.markdown)
			for range 8 {
				elements = append(elements, tui.New(tui.WithText("extra row"), tui.WithWidthPercent(100)))
			}
			root := tui.New(
				tui.WithDirection(tui.Column),
				tui.WithWidth(40),
				tui.WithHeight(8),
				tui.WithScrollable(tui.ScrollVertical),
				tui.WithChildren(elements...),
			)
			buf := tui.NewBuffer(40, 8)
			root.Render(buf, 40, 8)

			var block *tui.Element
			if tt.tag == "table" {
				block = findElementByTag(elements, tt.tag)
			} else {
				block = findCodeBlockContainer(elements)
			}
			if block == nil {
				t.Fatalf("expected %s element", tt.name)
			}
			before := findElementContainingText(elements, "Before")
			if before == nil {
				t.Fatal("expected paragraph before block")
			}
			after := findElementContainingText(elements, "After.")
			if after == nil {
				t.Fatal("expected paragraph after block")
			}
			if got := block.Rect().Y - before.Rect().Bottom(); got != 1 {
				t.Fatalf("blank rows before %s = %d, want exactly 1", tt.name, got)
			}
			lastTextRow := -1
			for y := before.Rect().Y; y < block.Rect().Y; y++ {
				if strings.TrimSpace(extractBufferLine(buf, y, 39)) != "" {
					lastTextRow = y
				}
			}
			if lastTextRow < 0 {
				t.Fatal("expected rendered paragraph text before block")
			}
			if got := block.Rect().Y - lastTextRow - 1; got != 1 {
				t.Fatalf("visible blank rows before %s = %d, want exactly 1", tt.name, got)
			}
			if got := after.Rect().Y - block.Rect().Bottom(); got != 1 {
				t.Fatalf("blank rows after %s = %d, want exactly 1", tt.name, got)
			}
		})
	}
}

func TestRenderMarkdown_CodeAndTableFitBesideVerticalScrollbar(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		tag      string
	}{
		{
			name:     "code block",
			markdown: "```sh\necho this line is much wider than the viewport and must stay inside the panel\n```",
		},
		{
			name:     "table",
			markdown: "| Long heading | Value |\n|---|---|\n| content that is much wider than the viewport | value that is also too wide |",
			tag:      "table",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const width = 24
			elements := renderMarkdown(tt.markdown)
			for range 8 {
				elements = append(elements, tui.New(tui.WithText("extra row"), tui.WithWidthPercent(100)))
			}
			scroller := tui.New(
				tui.WithDirection(tui.Column),
				tui.WithWidth(width),
				tui.WithHeight(6),
				tui.WithScrollable(tui.ScrollVertical),
				tui.WithChildren(elements...),
			)
			buf := tui.NewBuffer(width, 6)
			scroller.Render(buf, width, 6)

			var block *tui.Element
			if tt.tag == "table" {
				block = findElementByTag(elements, tt.tag)
			} else {
				block = findCodeBlockContainer(elements)
			}
			if block == nil {
				t.Fatalf("expected %s element", tt.name)
			}
			viewportWidth, _ := scroller.ViewportSize()
			contentRight := scroller.ContentRect().X + viewportWidth - 1
			if got := block.Rect().Right(); got > contentRight {
				t.Fatalf("%s right edge = %d, want <= %d beside scrollbar", tt.name, got, contentRight)
			}
			contentRow := block.Children()[1]
			rightBorder := buf.Cell(block.Rect().Right()-1, contentRow.Rect().Y).Rune
			if rightBorder != '│' {
				t.Fatalf("%s content overflowed right border: got %q, want %q", tt.name, rightBorder, '│')
			}
		})
	}
}

func TestRenderMarkdown_CodeBlockPreservesInternalBlankLines(t *testing.T) {
	input := "```go\nline1\n\nline3\n```"
	elements := renderMarkdown(input)
	allText := elementsText(elements)
	if !strings.Contains(allText, "line1") || !strings.Contains(allText, "line3") {
		t.Fatalf("expected code block content to be preserved, got %q", allText)
	}

	hasBlankCodeLine := false
	blankCodeLine := " 2 │ "
	for _, el := range flattenElements(elements) {
		if el.Text() == blankCodeLine {
			hasBlankCodeLine = true
			break
		}
	}
	if !hasBlankCodeLine {
		t.Fatal("expected internal blank line inside code block to be preserved")
	}
}

func TestRenderMarkdown_CodeBlockNormalizesCRLF(t *testing.T) {
	input := "```go\r\npackage main\r\n\r\nimport \"fmt\"\r\n```\r\n"
	elements := renderMarkdown(input)
	if len(elements) == 0 {
		t.Fatal("expected elements for CRLF code block")
	}

	allText := elementsText(elements)
	if strings.ContainsRune(allText, '\r') {
		t.Fatalf("expected code block render to strip carriage returns, got %q", allText)
	}
	if !strings.Contains(allText, "package main") {
		t.Fatalf("expected normalized code content, got %q", allText)
	}
	if !strings.Contains(allText, "import \"fmt\"") {
		t.Fatalf("expected normalized code content, got %q", allText)
	}
}

func TestNormalizeCodeBlockText_ExpandsTabsAndControls(t *testing.T) {
	input := "\tfunc main() {\n\t\tfmt.Println(\"hi\")\x08\n}\n"
	got := normalizeCodeBlockText(input)

	if strings.ContainsRune(got, '\t') {
		t.Fatalf("expected tabs to be expanded, got %q", got)
	}
	if strings.ContainsRune(got, '\x08') {
		t.Fatalf("expected control characters to be neutralised, got %q", got)
	}
	want := "    func main() {\n        fmt.Println(\"hi\") \n}"
	if got != want {
		t.Fatalf("normalizeCodeBlockText() = %q, want %q", got, want)
	}
}

func TestRenderMarkdown_CodeBlockExpandsTabs(t *testing.T) {
	input := "```go\n\tfmt.Println(\"hi\")\n```\n"
	elements := renderMarkdown(input)
	if len(elements) == 0 {
		t.Fatal("expected elements for tab-indented code block")
	}

	allText := elementsText(elements)
	if strings.ContainsRune(allText, '\t') {
		t.Fatalf("expected rendered code block to contain no raw tabs, got %q", allText)
	}
	if !strings.Contains(allText, "1 │     fmt.Println(\"hi\")") {
		t.Fatalf("expected code block tabs to expand to spaces, got %q", allText)
	}
}

func TestRenderMarkdown_CodeBlockHighlightsTokens(t *testing.T) {
	t.Setenv("COLORFGBG", "15;0")

	input := "```go\npackage main\n```"
	elements := renderMarkdown(input)
	codeBlock := findCodeBlockContainer(elements)
	if codeBlock == nil || len(codeBlock.Children()) < 2 {
		t.Fatalf("expected bordered code block with header + line children, got %+v", elements)
	}

	lineSpans := codeBlock.Children()[1].StyledSpans()
	if len(lineSpans) == 0 {
		t.Fatal("expected styled spans for highlighted code line")
	}

	palette := currentCodeHighlightPalette()
	expected := chromaEntryToTuiStyle(palette.Style.Get(chroma.KeywordNamespace)).Fg
	found := false
	for _, span := range lineSpans {
		if span.Text == "package" {
			found = true
			if span.Style.Fg != expected {
				t.Fatalf("keyword color = %v, want %v", span.Style.Fg, expected)
			}
			if !span.Style.Bg.IsDefault() {
				t.Fatalf("keyword background = %v, want default background", span.Style.Bg)
			}
		}
	}
	if !found {
		t.Fatalf("expected a span for the 'package' keyword, got %+v", lineSpans)
	}
}

func TestRenderMarkdown_CodeBlockUsesBorderedContainer(t *testing.T) {
	t.Setenv("COLORFGBG", "15;0")

	input := "```go\nfmt.Println(\"hello\")\n```"
	elements := renderMarkdown(input)
	codeBlock := findCodeBlockContainer(elements)
	if codeBlock == nil {
		t.Fatal("expected bordered code block container")
	}
	if codeBlock.Border() != tui.BorderRounded {
		t.Fatalf("code block border = %v, want rounded border", codeBlock.Border())
	}
	root := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithChildren(elements...),
	)
	buf := tui.NewBuffer(40, 10)
	root.Render(buf, 40, 10)

	rect := codeBlock.Rect()
	if rect.X != 0 {
		t.Fatalf("top-level code block x = %d, want 0", rect.X)
	}
	if rect.Width != 40 {
		t.Fatalf("top-level code block width = %d, want 40", rect.Width)
	}
	corners := []struct {
		x, y int
		r    rune
	}{
		{rect.X, rect.Y, '╭'},
		{rect.X + rect.Width - 1, rect.Y, '╮'},
		{rect.X, rect.Y + rect.Height - 1, '╰'},
		{rect.X + rect.Width - 1, rect.Y + rect.Height - 1, '╯'},
	}
	for _, corner := range corners {
		if cell := buf.Cell(corner.x, corner.y); cell.Rune != corner.r {
			t.Fatalf("border cell(%d,%d) = %q, want %q", corner.x, corner.y, cell.Rune, corner.r)
		}
	}

	contentX := rect.X + 2
	contentY := rect.Y + 2
	if cell := buf.Cell(contentX, contentY); !cell.Style.Bg.IsDefault() {
		t.Fatalf("content cell(%d,%d).Style.Bg = %v, want default background", contentX, contentY, cell.Style.Bg)
	}
}

func TestRenderMarkdown_CodeBlockFollowsListIndentWithoutExtraOffset(t *testing.T) {
	t.Setenv("COLORFGBG", "15;0")

	input := "- item\n\n  ```go\n  fmt.Println(\"hello\")\n  ```"
	elements := renderMarkdown(input)
	codeBlock := findCodeBlockContainer(elements)
	if codeBlock == nil {
		t.Fatal("expected bordered nested code block container")
	}

	root := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithChildren(elements...),
	)
	buf := tui.NewBuffer(40, 12)
	root.Render(buf, 40, 12)

	rect := codeBlock.Rect()
	wantIndent := markdownIndentCells(1)
	if rect.X != wantIndent {
		t.Fatalf("nested code block x = %d, want %d", rect.X, wantIndent)
	}
	if rect.Width != 40-wantIndent {
		t.Fatalf("nested code block width = %d, want %d", rect.Width, 40-wantIndent)
	}
	if cell := buf.Cell(rect.X, rect.Y); cell.Rune != '╭' {
		t.Fatalf("nested code block top-left border = %q, want %q", cell.Rune, '╭')
	}
}

func TestDetectTerminalBackgroundPreference(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want terminalBackgroundPreference
	}{
		{name: "missing env defaults dark", env: "", want: terminalBackgroundDark},
		{name: "dark background index", env: "15;0", want: terminalBackgroundDark},
		{name: "light background index", env: "0;15", want: terminalBackgroundLight},
		{name: "colon separated", env: "0:7", want: terminalBackgroundLight},
		{name: "invalid env defaults dark", env: "bogus", want: terminalBackgroundDark},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env == "" {
				t.Setenv("COLORFGBG", "")
			} else {
				t.Setenv("COLORFGBG", tt.env)
			}
			if got := detectTerminalBackgroundPreference(); got != tt.want {
				t.Fatalf("detectTerminalBackgroundPreference() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewCodeHighlightPalette_UsesDefaultBlockBackground(t *testing.T) {
	for _, pref := range []terminalBackgroundPreference{terminalBackgroundDark, terminalBackgroundLight} {
		palette := newCodeHighlightPalette(pref)
		if !palette.Theme.PanelBackground.Bg.IsDefault() {
			t.Fatalf("panel background = %v, want default background", palette.Theme.PanelBackground.Bg)
		}
		if !palette.Theme.HeaderBackground.Bg.IsDefault() {
			t.Fatalf("header background = %v, want default background", palette.Theme.HeaderBackground.Bg)
		}
		if !palette.Theme.GutterBackground.Bg.IsDefault() {
			t.Fatalf("gutter background = %v, want default background", palette.Theme.GutterBackground.Bg)
		}
		if palette.Theme.BorderStyle.Fg.IsDefault() {
			t.Fatal("border foreground should stay explicit for structure cues")
		}
	}
}

func elementsText(elements []*tui.Element) string {
	var sb strings.Builder
	for _, el := range flattenElements(elements) {
		sb.WriteString(el.Text())
		sb.WriteByte('\n')
	}
	return sb.String()
}

func flattenElements(elements []*tui.Element) []*tui.Element {
	var out []*tui.Element
	var walk func(*tui.Element)
	walk = func(el *tui.Element) {
		if el == nil {
			return
		}
		out = append(out, el)
		for _, child := range el.Children() {
			walk(child)
		}
	}
	for _, el := range elements {
		walk(el)
	}
	return out
}

func findCodeBlockContainer(elements []*tui.Element) *tui.Element {
	for _, el := range flattenElements(elements) {
		if el.Border() == tui.BorderRounded && len(el.Children()) > 0 {
			return el
		}
	}
	return nil
}

func findElementByTag(elements []*tui.Element, tag string) *tui.Element {
	for _, el := range flattenElements(elements) {
		if el.Tag() == tag {
			return el
		}
	}
	return nil
}

func findElementContainingText(elements []*tui.Element, text string) *tui.Element {
	for _, el := range flattenElements(elements) {
		if strings.Contains(el.Text(), text) {
			return el
		}
	}
	return nil
}

func firstSpanText(spans []tui.StyledSpan) string {
	if len(spans) == 0 {
		return ""
	}
	return spans[0].Text
}

func extractBufferLine(buf *tui.Buffer, y, width int) string {
	var row strings.Builder
	for x := 0; x < width; x++ {
		r := buf.Cell(x, y).Rune
		if r == 0 {
			r = ' '
		}
		row.WriteRune(r)
	}
	return strings.TrimRight(row.String(), " ")
}
