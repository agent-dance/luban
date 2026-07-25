// Package tools — local LLM summariser used by WebFetch.
//
// Mirrors applyPromptToMarkdown in src/tools/WebFetchTool/utils.ts: feed
// the truncated markdown plus the user's prompt to a small/fast model
// (Haiku class) and return the model's text response. The summariser is
// pluggable so providers can supply the active small-model client.
package web

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// WebFetchSummariserMaxTokens caps the secondary-model output to keep the
// returned summary bounded. Mirrors the behaviour described by the task
// spec (4096 tokens).
const WebFetchSummariserMaxTokens = 4096

// SummariserSecondaryPromptGuidelines are the trailing instructions the TS
// makeSecondaryModelPrompt appends. Two flavours: preapproved domains get
// a concise-response prompt, everything else gets the licence/safety
// preamble. Kept in sync with prompt.ts so behaviour and audits match.
const summariserGuidelinesPreapproved = `Provide a concise response based on the content above. Include relevant details, code examples, and documentation excerpts as needed.`

const summariserGuidelinesGeneral = `Provide a concise response based only on the content above. In your response:
 - Enforce a strict 125-character maximum for quotes from any source document. Open Source Software is ok as long as we respect the license.
 - Use quotation marks for exact language from articles; any language outside of the quotation should never be word-for-word the same.
 - You are not a lawyer and never comment on the legality of your own prompts and responses.
 - Never produce or reproduce exact song lyrics.`

// SecondaryModelPrompt formats markdown content + user prompt into the
// exact wrapper that the TS reference uses.
func SecondaryModelPrompt(markdownContent, prompt string, isPreapprovedDomain bool) string {
	guidelines := summariserGuidelinesGeneral
	if isPreapprovedDomain {
		guidelines = summariserGuidelinesPreapproved
	}
	return fmt.Sprintf(`
Web page content:
---
%s
---

%s

%s
`, markdownContent, prompt, guidelines)
}

// SummariserClient is the minimal interface the summariser needs. It lets
// us swap the production Anthropic client for fakes in tests without
// pulling the SDK into this file.
type SummariserClient interface {
	// Summarise sends a single user message + system prompt to a fast model
	// and returns the assistant text. ctx, maxTokens, and the system prompt
	// are honoured by the implementation; cancellation surfaces as
	// ctx.Err().
	Summarise(ctx context.Context, req SummariserRequest) (string, error)
}

// SummariserRequest captures the inputs to a single summariser call.
type SummariserRequest struct {
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
	// Source URL — used by the implementation for telemetry/auth scoping.
	URL string
	// Original user prompt (without markdown wrapper) — passed through so
	// providers that prefer message arrays can build them.
	Prompt string
}

// SummariserFunc adapts a function to the SummariserClient interface so
// tests can supply an inline closure.
type SummariserFunc func(ctx context.Context, req SummariserRequest) (string, error)

// Summarise implements SummariserClient.
func (f SummariserFunc) Summarise(ctx context.Context, req SummariserRequest) (string, error) {
	return f(ctx, req)
}

// ErrSummariserUnavailable is returned by RunWebFetchSummariser when no
// client has been configured. Callers should fall back to the structured
// payload built from the markdown directly.
var ErrSummariserUnavailable = errors.New("WebFetch summariser is not configured")

// RunWebFetchSummariser truncates `markdown` to MaxMarkdownBytes, formats
// the secondary-model prompt via SecondaryModelPrompt, and invokes the
// supplied client. Returns the model's text or an error.
func RunWebFetchSummariser(
	ctx context.Context,
	client SummariserClient,
	url, userPrompt, markdown string,
	isPreapprovedDomain bool,
) (string, error) {
	if client == nil {
		return "", i18n.WrapInternalError(i18n.KeyToolWebSummariserUnavailable, ErrSummariserUnavailable)
	}
	truncated := markdown
	if len(truncated) > MaxMarkdownBytes {
		truncated = truncated[:MaxMarkdownBytes] + markdownTruncationMarker
	}
	wrapped := SecondaryModelPrompt(truncated, userPrompt, isPreapprovedDomain)
	resp, err := client.Summarise(ctx, SummariserRequest{
		SystemPrompt: "",
		UserPrompt:   wrapped,
		MaxTokens:    WebFetchSummariserMaxTokens,
		URL:          url,
		Prompt:       userPrompt,
	})
	if err != nil {
		return "", i18n.WrapError(i18n.KeyToolWebSummariserFailed, err)
	}
	resp = strings.TrimSpace(resp)
	if resp == "" {
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolWebNoModelResponse), nil
	}
	return resp, nil
}
