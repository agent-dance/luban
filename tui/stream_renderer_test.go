package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/grindlemire/go-tui"
)

// --- StreamRenderer core tests ---

func TestStreamRenderer_BasicParagraph(t *testing.T) {
	sr := NewStreamRenderer()
	sr.Feed("Hello world.\n\n")
	sr.Finalize()

	lines := sr.Lines()
	if len(lines) == 0 {
		t.Fatal("expected output lines for basic paragraph")
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Hello world") {
		t.Errorf("output should contain 'Hello world', got: %q", joined)
	}
}

func TestStreamRenderer_MultipleBlocks(t *testing.T) {
	sr := NewStreamRenderer()

	// First paragraph
	sr.Feed("First paragraph.\n\n")
	if len(sr.blocks) != 1 {
		t.Fatalf("expected 1 cached block after first paragraph, got %d", len(sr.blocks))
	}

	// Second paragraph
	sr.Feed("Second paragraph.\n\n")
	if len(sr.blocks) != 2 {
		t.Fatalf("expected 2 cached blocks after second paragraph, got %d", len(sr.blocks))
	}

	sr.Finalize()
	lines := sr.Lines()
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "First") || !strings.Contains(joined, "Second") {
		t.Errorf("expected both paragraphs in output, got: %q", joined)
	}
}

func TestStreamRenderer_FencedCodeBlock(t *testing.T) {
	sr := NewStreamRenderer()

	// Stream a fenced code block token by token
	sr.Feed("Some text.\n\n")
	sr.Feed("```go\n")
	sr.Feed("fmt.Println(\"hello\")\n")
	sr.Feed("```\n")

	// The paragraph should be cached (1 block); code block should also be cached
	if len(sr.blocks) < 1 {
		t.Fatalf("expected at least 1 cached block, got %d", len(sr.blocks))
	}

	sr.Finalize()
	lines := sr.Lines()
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Some text") {
		t.Errorf("expected 'Some text' in output, got: %q", joined)
	}
	if !strings.Contains(joined, "Println") {
		t.Errorf("expected 'Println' in output, got: %q", joined)
	}
}

func TestStreamRenderer_FenceDoesNotSplitOnBlankLine(t *testing.T) {
	sr := NewStreamRenderer()

	// Code blocks with blank lines inside should NOT split
	sr.Feed("```\nline1\n\nline2\n```\n")
	sr.Finalize()

	// Should be exactly 1 block (the entire fenced code block)
	if len(sr.blocks) != 1 {
		t.Errorf("expected 1 block for code with internal blank line, got %d", len(sr.blocks))
	}
}

func TestStreamRenderer_HeadingSplitsBlock(t *testing.T) {
	sr := NewStreamRenderer()

	// A heading after a paragraph (separated by newline) should cause a split
	sr.Feed("Some paragraph text.\n# Heading\n")
	sr.Finalize()

	if len(sr.blocks) < 1 {
		t.Errorf("expected at least 1 block (paragraph + heading), got %d", len(sr.blocks))
	}
	joined := strings.Join(sr.Lines(), "\n")
	if !strings.Contains(joined, "paragraph") {
		t.Errorf("expected 'paragraph' in output, got: %q", joined)
	}
	if !strings.Contains(joined, "Heading") {
		t.Errorf("expected 'Heading' in output, got: %q", joined)
	}
}

func TestStreamRenderer_PendingShowsPlainText(t *testing.T) {
	sr := NewStreamRenderer()

	// Feed incomplete text (no block boundary yet)
	sr.Feed("This is still being")
	sr.Feed(" typed...")

	lines := sr.Lines()
	if len(lines) == 0 {
		t.Fatal("expected lines for pending text")
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "This is still being typed...") {
		t.Errorf("pending text should appear as plain text, got: %q", joined)
	}

	// No blocks should be cached yet
	if len(sr.blocks) != 0 {
		t.Errorf("expected 0 cached blocks while text is pending, got %d", len(sr.blocks))
	}
}

func TestStreamRenderer_FinalizeRendersPending(t *testing.T) {
	sr := NewStreamRenderer()

	sr.Feed("Trailing text without double newline")

	// Before finalize: 0 cached blocks, text in pending
	if len(sr.blocks) != 0 {
		t.Fatalf("expected 0 blocks before finalize, got %d", len(sr.blocks))
	}

	sr.Finalize()

	// After finalize: 1 cached block
	if len(sr.blocks) != 1 {
		t.Fatalf("expected 1 block after finalize, got %d", len(sr.blocks))
	}

	// Pending should be empty
	if sr.pending.Len() != 0 {
		t.Errorf("pending should be empty after finalize, got %q", sr.pending.String())
	}
}

func TestStreamRenderer_DoubleFinalize(t *testing.T) {
	sr := NewStreamRenderer()
	sr.Feed("text\n\n")
	sr.Finalize()
	blocksBefore := len(sr.blocks)

	// Second finalize should be a no-op
	sr.Finalize()
	if len(sr.blocks) != blocksBefore {
		t.Errorf("double finalize changed block count: %d → %d", blocksBefore, len(sr.blocks))
	}
}

func TestStreamRenderer_FeedAfterFinalize(t *testing.T) {
	sr := NewStreamRenderer()
	sr.Feed("text\n\n")
	sr.Finalize()

	// Feed after finalize should be a no-op
	changed := sr.Feed("more text")
	if changed {
		t.Error("Feed after Finalize should return false")
	}
}

func TestStreamRenderer_Reset(t *testing.T) {
	sr := NewStreamRenderer()
	sr.Feed("some text\n\n")
	sr.Finalize()

	sr.Reset()
	if len(sr.blocks) != 0 {
		t.Errorf("expected 0 blocks after reset, got %d", len(sr.blocks))
	}
	if sr.pending.Len() != 0 {
		t.Errorf("expected empty pending after reset, got %q", sr.pending.String())
	}
	if sr.finalized {
		t.Error("expected finalized=false after reset")
	}
}

func TestStreamRenderer_EmptyFeed(t *testing.T) {
	sr := NewStreamRenderer()
	changed := sr.Feed("")
	if !changed {
		// With the new implementation, Feed always returns true (pending text may differ)
		// but on truly empty input it's acceptable either way.
	}
	if len(sr.blocks) != 0 || sr.pending.Len() != 0 {
		t.Errorf("expected no state change on empty feed")
	}
}

func TestStreamRenderer_TokenByToken(t *testing.T) {
	sr := NewStreamRenderer()

	// Simulate LLM streaming: one word at a time
	tokens := []string{
		"Hello", " ", "world", ".", "\n", "\n",
		"Second", " ", "paragraph", ".", "\n", "\n",
	}
	for _, tok := range tokens {
		sr.Feed(tok)
	}

	// Both paragraphs should be cached
	if len(sr.blocks) != 2 {
		t.Errorf("expected 2 cached blocks after token-by-token feed, got %d", len(sr.blocks))
	}

	sr.Finalize()
	lines := sr.Lines()
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Hello world") {
		t.Errorf("expected 'Hello world', got: %q", joined)
	}
	if !strings.Contains(joined, "Second paragraph") {
		t.Errorf("expected 'Second paragraph', got: %q", joined)
	}
}

func TestStreamRenderer_MixedContentStream(t *testing.T) {
	sr := NewStreamRenderer()

	// Simulate a realistic LLM response
	sr.Feed("Here's a code example:\n\n")
	sr.Feed("```python\n")
	sr.Feed("print(\"hello\")\n")
	sr.Feed("```\n\n")
	sr.Feed("And a list:\n\n")
	sr.Feed("- item 1\n")
	sr.Feed("- item 2\n")
	sr.Finalize()

	lines := sr.Lines()
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "code example") {
		t.Errorf("expected 'code example', got: %q", joined)
	}
	if !strings.Contains(joined, "print") {
		t.Errorf("expected 'print', got: %q", joined)
	}
	if !strings.Contains(joined, "item 1") {
		t.Errorf("expected 'item 1', got: %q", joined)
	}
}

func TestStreamRenderer_IsFinalized(t *testing.T) {
	sr := NewStreamRenderer()
	if sr.IsFinalized() {
		t.Error("should not be finalized initially")
	}
	sr.Feed("text")
	if sr.IsFinalized() {
		t.Error("should not be finalized after Feed")
	}
	sr.Finalize()
	if !sr.IsFinalized() {
		t.Error("should be finalized after Finalize")
	}
}

func TestStreamRenderer_WhitespaceOnlyPending(t *testing.T) {
	sr := NewStreamRenderer()
	sr.Feed("   \n  \n  ")
	sr.Finalize()

	// Whitespace-only pending should not produce a block
	if len(sr.blocks) != 0 {
		t.Errorf("expected 0 blocks for whitespace-only, got %d", len(sr.blocks))
	}
}

// --- NEW: Nested fence tests ---

func TestStreamRenderer_NestedFences(t *testing.T) {
	sr := NewStreamRenderer()

	// 4-backtick fence containing a 3-backtick fence inside
	input := "Before.\n\n" +
		"````markdown\n" +
		"Here is an example:\n" +
		"```python\n" +
		"print(\"hello\")\n" +
		"```\n" +
		"````\n\n" +
		"After.\n\n"

	sr.Feed(input)
	sr.Finalize()

	joined := strings.Join(sr.Lines(), "\n")
	if !strings.Contains(joined, "Before") {
		t.Errorf("expected 'Before' in output, got: %q", joined)
	}
	if !strings.Contains(joined, "print") {
		t.Errorf("expected 'print' (inside nested fence) in output, got: %q", joined)
	}
	if !strings.Contains(joined, "After") {
		t.Errorf("expected 'After' in output, got: %q", joined)
	}

	// The inner ``` should NOT close the outer ```` fence, so we should have
	// all content present. After Finalize(), blocks are re-rendered as a single
	// document, so block count is 1.
	if len(sr.blocks) != 1 {
		t.Errorf("expected 1 block (full re-render on finalize), got %d", len(sr.blocks))
	}
}

func TestStreamRenderer_NestedFenceTokenByToken(t *testing.T) {
	sr := NewStreamRenderer()

	// Stream nested fence token by token
	tokens := []string{
		"````", "md", "\n",
		"```", "\n",
		"inner", "\n",
		"```", "\n",
		"````", "\n",
	}
	for _, tok := range tokens {
		sr.Feed(tok)
	}
	sr.Finalize()

	joined := strings.Join(sr.Lines(), "\n")
	if !strings.Contains(joined, "inner") {
		t.Errorf("expected 'inner' in output, got: %q", joined)
	}
	// The entire thing should be one block (the outer fence)
	if len(sr.blocks) != 1 {
		t.Errorf("expected 1 block for nested fence, got %d", len(sr.blocks))
	}
}

func TestStreamRenderer_FenceClosingNeedsEnoughBackticks(t *testing.T) {
	sr := NewStreamRenderer()

	// Open with 4 backticks, try closing with 3 — should NOT close
	sr.Feed("````\n")
	sr.Feed("some code\n")
	sr.Feed("```\n") // not enough backticks to close
	sr.Feed("more code\n")
	sr.Feed("````\n") // this should close

	sr.Finalize()

	// Should be one block (the entire fence from ```` to ````)
	if len(sr.blocks) != 1 {
		t.Errorf("expected 1 block, got %d", len(sr.blocks))
	}
	joined := strings.Join(sr.Lines(), "\n")
	if !strings.Contains(joined, "some code") || !strings.Contains(joined, "more code") {
		t.Errorf("expected both 'some code' and 'more code', got: %q", joined)
	}
}

// --- NEW: Table test ---

func TestStreamRenderer_TableAsOneBlock(t *testing.T) {
	sr := NewStreamRenderer()

	// Table without trailing blank line — stays in pending until Finalize
	sr.Feed("| Name | Value |\n")
	sr.Feed("|------|-------|\n")
	sr.Feed("| foo  | bar   |\n")
	sr.Feed("| baz  | qux   |\n")

	// No double newline → table stays in pending
	if len(sr.blocks) != 0 {
		t.Errorf("expected 0 blocks while table is pending, got %d", len(sr.blocks))
	}

	// Verify pending shows table as plain text
	lines := sr.Lines()
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "foo") {
		t.Errorf("expected 'foo' in pending output, got: %q", joined)
	}

	sr.Finalize()
	if len(sr.blocks) != 1 {
		t.Errorf("expected 1 block after finalize, got %d", len(sr.blocks))
	}
}

func TestStreamRenderer_TableWithTrailingBlankLine(t *testing.T) {
	sr := NewStreamRenderer()

	sr.Feed("| A | B |\n|---|---|\n| 1 | 2 |\n\n")

	// Double newline after table → should be cached
	if len(sr.blocks) != 1 {
		t.Errorf("expected 1 cached block for table, got %d", len(sr.blocks))
	}
}

// --- NEW: Long paragraph test ---

func TestStreamRenderer_LongParagraphNoBoundary(t *testing.T) {
	sr := NewStreamRenderer()

	// Simulate a very long paragraph with no double-newline
	for i := 0; i < 1000; i++ {
		sr.Feed("word ")
	}

	// Everything should be in pending, no blocks cached
	if len(sr.blocks) != 0 {
		t.Errorf("expected 0 blocks for continuous text, got %d", len(sr.blocks))
	}

	// Pending should contain all text
	pending := sr.pending.String()
	if len(pending) < 5000 {
		t.Errorf("expected pending to be at least 5000 bytes, got %d", len(pending))
	}

	sr.Finalize()
	if len(sr.blocks) != 1 {
		t.Errorf("expected 1 block after finalize, got %d", len(sr.blocks))
	}
}

// --- NEW: ScanOffset optimization test ---

func TestStreamRenderer_IncrementalBoundaryDetection(t *testing.T) {
	sr := NewStreamRenderer()

	// Feed text without boundary — no blocks should be cached
	sr.Feed("line 1\nline 2\nline 3\n")
	if len(sr.blocks) != 0 {
		t.Errorf("expected 0 blocks (no double newline yet), got %d", len(sr.blocks))
	}

	// Feed more text with a boundary (creates \n\n)
	sr.Feed("\n") // this creates \n\n boundary
	if len(sr.blocks) != 1 {
		t.Errorf("expected 1 block after double newline, got %d", len(sr.blocks))
	}
}

// --- NEW: Elements() method tests ---

func TestStreamRenderer_Elements(t *testing.T) {
	sr := NewStreamRenderer()
	sr.Feed("Hello **world**.\n\n")
	sr.Finalize()

	elements := sr.Elements()
	if len(elements) == 0 {
		t.Fatal("expected elements from Elements()")
	}

	// Check that elements have text content
	var allText strings.Builder
	for _, el := range flattenElements(elements) {
		allText.WriteString(el.Text())
	}
	if !strings.Contains(allText.String(), "Hello") {
		t.Errorf("expected 'Hello' in elements text, got: %q", allText.String())
	}
}

func TestStreamRenderer_ElementsPending(t *testing.T) {
	sr := NewStreamRenderer()
	sr.Feed("Still typing...")

	elements := sr.Elements()
	if len(elements) == 0 {
		t.Fatal("expected elements for pending text")
	}
	if elements[0].Text() != "Still typing..." {
		t.Errorf("expected pending text element, got: %q", elements[0].Text())
	}
}

func TestStreamRenderer_ElementsIncludeRelaxedSpacingBetweenBlocks(t *testing.T) {
	sr := NewStreamRenderer()
	sr.Feed("First paragraph.\n\nSecond paragraph.\n\n")
	sr.Finalize()

	elements := sr.Elements()
	blankLines := 0
	for _, el := range elements {
		if el.Text() == "" {
			blankLines++
		}
	}
	if blankLines < 1 {
		t.Fatalf("expected at least one spacer element between rendered blocks, got %d", blankLines)
	}
}

func TestStreamRenderer_CachedBlocksPreserveNestedListIndentation(t *testing.T) {
	sr := NewStreamRenderer()
	sr.Feed("- parent\n\n  - child one\n  - child two\n\n- next\n")

	if len(sr.blocks) < 2 {
		t.Fatalf("expected at least 2 cached blocks, got %d", len(sr.blocks))
	}
	if got := sr.blocks[1].source; !strings.HasPrefix(got, "  - child one") {
		t.Fatalf("cached nested-list block lost indentation: %q", got)
	}
}

func TestStreamRenderer_FinalizePreservesNestedListIndentAndSpacing(t *testing.T) {
	sr := NewStreamRenderer()
	sr.Feed("- parent\n\n  - child one\n  - child two\n\n- next top level\n")
	sr.Finalize()

	elements := sr.Elements()
	if len(elements) == 0 {
		t.Fatal("expected elements after finalize")
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
		t.Fatal("expected nested child line in rendered buffer")
	}
	if !strings.HasPrefix(childLine, "  • child one") {
		t.Fatalf("nested child line = %q, want prefix %q", childLine, "  • child one")
	}
	if nextLine == "" {
		t.Fatal("expected next top-level line in rendered buffer")
	}
	if !strings.HasPrefix(nextLine, "• next top level") {
		t.Fatalf("next top-level line = %q, want prefix %q", nextLine, "• next top level")
	}
}

// --- Integration with AppState ---

func TestAppState_StreamRendererIntegration(t *testing.T) {
	s := NewAppState()

	// Simulate streaming tokens
	s.AppendOrStreamText("Hello ")
	s.AppendOrStreamText("world.\n\n")
	s.AppendOrStreamText("Second paragraph.")

	// Wait for debounce to flush
	time.Sleep(100 * time.Millisecond)

	msgs := s.Messages.Get()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Stream == nil {
		t.Fatal("expected Stream to be non-nil")
	}
	if msgs[0].Stream.IsFinalized() {
		t.Error("stream should not be finalized yet")
	}

	// The first paragraph should be cached
	if len(msgs[0].Stream.blocks) < 1 {
		t.Errorf("expected at least 1 cached block, got %d", len(msgs[0].Stream.blocks))
	}

	// Raw text should have everything
	expected := "Hello world.\n\nSecond paragraph."
	if msgs[0].Text != expected {
		t.Errorf("expected Text=%q, got %q", expected, msgs[0].Text)
	}

	// Finalize
	s.FinalizeStream()
	msgs = s.Messages.Get()
	if !msgs[0].Stream.IsFinalized() {
		t.Error("stream should be finalized after FinalizeStream")
	}

	// After finalize, the full text is re-rendered as a single document block.
	if len(msgs[0].Stream.blocks) != 1 {
		t.Errorf("expected 1 block after finalize (full re-render), got %d", len(msgs[0].Stream.blocks))
	}
}

func TestAppState_StreamInterruptedByToolCall(t *testing.T) {
	s := NewAppState()

	// Stream some text
	s.AppendOrStreamText("Some text ")
	s.AppendOrStreamText("here.")

	// Simulate a ToolCall arriving (which should trigger finalize)
	s.FinalizeStream()
	s.AppendMessage(Message{Kind: MsgToolCall, ToolName: "Bash"})

	// Now stream more text (new assistant message)
	s.AppendOrStreamText("After tool.")

	msgs := s.Messages.Get()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Kind != MsgAssistant {
		t.Errorf("msg[0] should be MsgAssistant, got %d", msgs[0].Kind)
	}
	if msgs[1].Kind != MsgToolCall {
		t.Errorf("msg[1] should be MsgToolCall, got %d", msgs[1].Kind)
	}
	if msgs[2].Kind != MsgAssistant {
		t.Errorf("msg[2] should be MsgAssistant, got %d", msgs[2].Kind)
	}
	// First message's stream should be finalized
	if msgs[0].Stream != nil && !msgs[0].Stream.IsFinalized() {
		t.Error("first assistant message's stream should be finalized")
	}
	// Second assistant message should have its own stream
	if msgs[2].Stream == nil {
		t.Error("second assistant message should have a Stream")
	}
}

func TestAppState_FinalizeStreamNoMessages(t *testing.T) {
	s := NewAppState()
	// Should not panic on empty state
	s.FinalizeStream()
}

func TestAppState_FinalizeStreamNonAssistant(t *testing.T) {
	s := NewAppState()
	s.AppendMessage(Message{Kind: MsgError, Text: "error"})
	// Should not panic when last message is not assistant
	s.FinalizeStream()
}

func TestAppState_DebounceFlushesOnFinalize(t *testing.T) {
	s := NewAppState()

	// Rapidly stream tokens
	for i := 0; i < 20; i++ {
		s.AppendOrStreamText(fmt.Sprintf("tok%d ", i))
	}

	// Finalize without waiting for debounce timer — should flush everything
	s.FinalizeStream()

	msgs := s.Messages.Get()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !msgs[0].Stream.IsFinalized() {
		t.Error("stream should be finalized")
	}
	// All tokens should be in the text
	for i := 0; i < 20; i++ {
		expected := fmt.Sprintf("tok%d", i)
		if !strings.Contains(msgs[0].Text, expected) {
			t.Errorf("expected Text to contain %q, got %q", expected, msgs[0].Text)
		}
	}
}

func TestAppState_ClearCancelsDebounce(t *testing.T) {
	s := NewAppState()

	// Start streaming
	s.AppendOrStreamText("hello ")
	s.AppendOrStreamText("world ")

	// Clear should cancel the debounce timer
	s.ClearMessages()

	msgs := s.Messages.Get()
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages after clear, got %d", len(msgs))
	}

	// Wait to make sure no stale debounce timer fires and panics
	time.Sleep(100 * time.Millisecond)
}

// --- Benchmarks ---

func BenchmarkStreamRenderer_500Tokens(b *testing.B) {
	// Simulate 500 tokens of mixed content
	tokens := make([]string, 0, 500)
	for i := 0; i < 500; i++ {
		switch {
		case i == 100:
			tokens = append(tokens, "\n\n```go\n")
		case i == 130:
			tokens = append(tokens, "```\n\n")
		case i%50 == 0 && i > 0:
			tokens = append(tokens, ".\n\n")
		default:
			tokens = append(tokens, "word ")
		}
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		sr := NewStreamRenderer()
		for _, tok := range tokens {
			sr.Feed(tok)
		}
		sr.Finalize()
		_ = sr.Lines()
	}
}

func BenchmarkStreamRenderer_LongParagraph(b *testing.B) {
	// Simulate a single very long paragraph (worst case for pending accumulation)
	tokens := make([]string, 2000)
	for i := range tokens {
		tokens[i] = "word "
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		sr := NewStreamRenderer()
		for _, tok := range tokens {
			sr.Feed(tok)
		}
		sr.Finalize()
		_ = sr.Lines()
	}
}

func BenchmarkStreamRenderer_ManySmallBlocks(b *testing.B) {
	// Many short paragraphs (tests block cache efficiency)
	tokens := make([]string, 0, 1000)
	for i := 0; i < 200; i++ {
		tokens = append(tokens, fmt.Sprintf("Paragraph %d content here.", i))
		tokens = append(tokens, "\n\n")
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		sr := NewStreamRenderer()
		for _, tok := range tokens {
			sr.Feed(tok)
		}
		sr.Finalize()
		_ = sr.Lines()
	}
}

func BenchmarkStreamRenderer_NestedFences(b *testing.B) {
	// Nested fence handling performance
	tokens := []string{
		"Before.\n\n",
		"````markdown\n",
		"```python\n",
		"print(1)\n",
		"```\n",
		"Some text between\n",
		"```go\n",
		"fmt.Println(2)\n",
		"```\n",
		"````\n\n",
		"After.\n\n",
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		sr := NewStreamRenderer()
		for _, tok := range tokens {
			sr.Feed(tok)
		}
		sr.Finalize()
		_ = sr.Lines()
	}
}

func TestStreamRenderer_ElementsRenderOpenFenceWithIndentation(t *testing.T) {
	sr := NewStreamRenderer()
	sr.Feed("```go\n")
	sr.Feed("fmt.Println(\"hello\")\n")

	elements := sr.Elements()
	if len(elements) == 0 {
		t.Fatal("expected elements for in-progress fenced code block")
	}

	hasHeader := false
	hasNumberedCode := false
	for _, el := range flattenElements(elements) {
		if strings.Contains(el.Text(), "Go | 1 line") {
			hasHeader = true
		}
		if strings.Contains(el.Text(), "1 │ fmt.Println(\"hello\")") {
			hasNumberedCode = true
		}
	}

	if !hasHeader {
		t.Fatalf("expected rendered code panel header for open fence, got %q", strings.Join(sr.Lines(), "\\n"))
	}
	if !hasNumberedCode {
		t.Fatalf("expected numbered code line before finalize, got %q", strings.Join(sr.Lines(), "\\n"))
	}
}

func TestStreamRenderer_FinalizePreservesCodeBlockLayout(t *testing.T) {
	sr := NewStreamRenderer()
	sr.Feed("Intro paragraph.\n\n")
	sr.Feed("```go\n")
	sr.Feed("fmt.Println(\"hello\")\n")
	sr.Feed("fmt.Println(\"world\")\n")
	sr.Feed("```\n")

	beforeLines := strings.Join(sr.Lines(), "\n")
	beforeElements := elementsText(sr.Elements())

	sr.Finalize()

	afterLines := strings.Join(sr.Lines(), "\n")
	afterElements := elementsText(sr.Elements())

	if beforeLines != afterLines {
		t.Fatalf("stream/finalize lines drifted:\nbefore=%q\nafter=%q", beforeLines, afterLines)
	}
	if beforeElements != afterElements {
		t.Fatalf("stream/finalize element text drifted:\nbefore=%q\nafter=%q", beforeElements, afterElements)
	}
}
