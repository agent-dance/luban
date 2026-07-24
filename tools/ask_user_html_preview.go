package tools

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
)

// validateHtmlPreview mirrors src/tools/AskUserQuestionTool/AskUserQuestionTool.tsx:250-265
// Lightweight HTML fragment check for previews when previewFormat=html.
//
// Rules:
//   - Reject full-document tags: <html>, <body>, <!DOCTYPE>
//   - Reject executable/style tags: <script>, <style>
//   - Require at least one HTML tag
//
// Returns nil when valid, or an error describing why validation failed.
var (
	htmlPreviewFullDocRe = regexp.MustCompile(`(?i)<\s*(html|body|!doctype)\b`)
	htmlPreviewBadTagRe  = regexp.MustCompile(`(?i)<\s*(script|style)\b`)
	htmlPreviewAnyTagRe  = regexp.MustCompile(`(?i)<[a-z][^>]*>`)
)

// ValidateHtmlPreview validates an option preview as an HTML fragment.
// Empty preview is treated as valid (caller decides if preview is required).
func ValidateHtmlPreview(preview string) error {
	if preview == "" {
		return nil
	}
	if htmlPreviewFullDocRe.MatchString(preview) {
		return fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRuntimeAskUserPreviewHTMLFragment))
	}
	if htmlPreviewBadTagRe.MatchString(preview) {
		return fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRuntimeAskUserPreviewHTMLUnsafe))
	}
	if !htmlPreviewAnyTagRe.MatchString(preview) {
		return fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRuntimeAskUserPreviewHTMLRequired))
	}
	return nil
}

// ValidateHtmlPreviewForQuestions runs ValidateHtmlPreview against all option
// previews when the global preview format is "html". Returns the first error.
func ValidateHtmlPreviewForQuestions(questions []QuestionSpec, previewFormat string) error {
	if previewFormat != "html" {
		return nil
	}
	for _, q := range questions {
		for _, opt := range q.Options {
			if err := ValidateHtmlPreview(opt.Preview); err != nil {
				return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeAskUserPreviewOptionContext, opt.Label, q.Question, err))
			}
		}
	}
	return nil
}

// askUserPreviewFormatMu / askUserPreviewFormat hold the host-configured
// preview format that the AskUserQuestion tool advertises. Empty means the
// host has not opted in; "html" enables fragment-shape validation. The
// CLAUDE_ASKUSER_PREVIEW_FORMAT env var is the runtime escape hatch.
var (
	askUserPreviewFormatMu sync.RWMutex
	askUserPreviewFormat   string
)

// SetAskUserPreviewFormat configures the host preview format. Pass "" to
// clear. Recognised values: "html", "markdown", "plain".
func SetAskUserPreviewFormat(format string) {
	askUserPreviewFormatMu.Lock()
	askUserPreviewFormat = strings.ToLower(strings.TrimSpace(format))
	askUserPreviewFormatMu.Unlock()
}

// ResolveAskUserPreviewFormat returns the active preview format (env override
// wins over SetAskUserPreviewFormat). Empty string means no validation/no
// inject of the format-specific prompt.
func ResolveAskUserPreviewFormat() string {
	if env := strings.ToLower(strings.TrimSpace(os.Getenv("CLAUDE_ASKUSER_PREVIEW_FORMAT"))); env != "" {
		return env
	}
	askUserPreviewFormatMu.RLock()
	defer askUserPreviewFormatMu.RUnlock()
	return askUserPreviewFormat
}

// askUserPreviewFormatPrompts maps a preview format to the format-specific
// guidance the model needs to produce correctly-rendered previews. Mirrors
// PREVIEW_FEATURE_PROMPT in AskUserQuestionTool.tsx:117-124.
var askUserPreviewFormatPrompts = map[string]string{
	"html": "When supplying option.preview, return a small self-contained HTML fragment " +
		"(no <html>/<body>/<!DOCTYPE>; no <script>/<style>). Use semantic tags " +
		"like <div>, <pre>, <code>, <ul>, <table>. Inline styles via the style attribute are allowed.",
	"markdown": "When supplying option.preview, return Markdown only. Code blocks must use " +
		"fenced ``` syntax. Do not embed raw HTML.",
	"plain": "When supplying option.preview, return plain text only. Do not include " +
		"Markdown or HTML.",
}

// AskUserPreviewFormatPromptSuffix returns the format-specific guidance to
// append to the AskUserQuestion tool prompt so the model emits previews in
// the host's expected shape. Returns "" when no format is configured.
func AskUserPreviewFormatPromptSuffix() string {
	format := ResolveAskUserPreviewFormat()
	if format == "" {
		return ""
	}
	if msg, ok := askUserPreviewFormatPrompts[format]; ok {
		return msg
	}
	return ""
}
