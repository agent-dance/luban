package web

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSecondaryModelPrompt_Preapproved(t *testing.T) {
	got := SecondaryModelPrompt("# Hello", "find rate limits", true)
	if !strings.Contains(got, "Web page content:") {
		t.Fatalf("missing wrapper: %q", got)
	}
	if !strings.Contains(got, "# Hello") {
		t.Fatalf("missing markdown: %q", got)
	}
	if !strings.Contains(got, "find rate limits") {
		t.Fatalf("missing user prompt: %q", got)
	}
	if !strings.Contains(got, "Provide a concise response based on the content above. Include relevant details") {
		t.Fatalf("missing preapproved guidelines: %q", got)
	}
	if strings.Contains(got, "125-character maximum") {
		t.Fatalf("preapproved branch should not include licence preamble: %q", got)
	}
}

func TestSecondaryModelPrompt_GeneralIncludesGuidelines(t *testing.T) {
	got := SecondaryModelPrompt("body", "p", false)
	for _, want := range []string{
		"125-character maximum",
		"Open Source Software is ok",
		"Never produce or reproduce exact song lyrics",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("general guideline missing: %q\nfull: %q", want, got)
		}
	}
}

func TestRunWebFetchSummariser_RejectsNilClient(t *testing.T) {
	_, err := RunWebFetchSummariser(context.Background(), nil, "https://x", "p", "md", false)
	if !errors.Is(err, ErrSummariserUnavailable) {
		t.Fatalf("expected ErrSummariserUnavailable, got %v", err)
	}
	if strings.Contains(err.Error(), "%!(EXTRA") || strings.Count(err.Error(), "WebFetch") != 1 {
		t.Fatalf("unavailable error leaked or duplicated its internal sentinel: %q", err)
	}
}

func TestRunWebFetchSummariser_PassesUserPromptThrough(t *testing.T) {
	var captured SummariserRequest
	client := SummariserFunc(func(ctx context.Context, req SummariserRequest) (string, error) {
		captured = req
		return "summary text", nil
	})
	out, err := RunWebFetchSummariser(context.Background(), client, "https://example.com", "find limits", "<md>", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "summary text" {
		t.Fatalf("unexpected response: %q", out)
	}
	if captured.URL != "https://example.com" {
		t.Fatalf("URL not propagated: %+v", captured)
	}
	if captured.Prompt != "find limits" {
		t.Fatalf("prompt not propagated: %+v", captured)
	}
	if captured.MaxTokens != WebFetchSummariserMaxTokens {
		t.Fatalf("max tokens not capped: %d", captured.MaxTokens)
	}
	if !strings.Contains(captured.UserPrompt, "<md>") {
		t.Fatalf("wrapped prompt missing markdown: %q", captured.UserPrompt)
	}
	if !strings.Contains(captured.UserPrompt, "Provide a concise response based on the content above. Include relevant details") {
		t.Fatalf("expected preapproved guidelines in wrapped prompt")
	}
}

func TestRunWebFetchSummariser_TruncatesOversizedMarkdown(t *testing.T) {
	huge := strings.Repeat("x", MaxMarkdownBytes+1024)
	var captured SummariserRequest
	client := SummariserFunc(func(ctx context.Context, req SummariserRequest) (string, error) {
		captured = req
		return "ok", nil
	})
	if _, err := RunWebFetchSummariser(context.Background(), client, "u", "p", huge, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(captured.UserPrompt, "[Content truncated due to length...]") {
		t.Fatalf("expected truncation marker in wrapped prompt")
	}
	// Heuristic: wrapped prompt size should be near MaxMarkdownBytes plus
	// guidelines/wrapper, not full huge size.
	if len(captured.UserPrompt) > MaxMarkdownBytes+4*1024 {
		t.Fatalf("wrapped prompt too large: %d bytes", len(captured.UserPrompt))
	}
}

func TestRunWebFetchSummariser_EmptyResponseFallback(t *testing.T) {
	client := SummariserFunc(func(ctx context.Context, req SummariserRequest) (string, error) {
		return "   ", nil
	})
	out, err := RunWebFetchSummariser(context.Background(), client, "u", "p", "md", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected a localized empty-response marker")
	}
}

func TestRunWebFetchSummariser_PropagatesError(t *testing.T) {
	want := errors.New("boom")
	client := SummariserFunc(func(ctx context.Context, req SummariserRequest) (string, error) {
		return "", want
	})
	_, err := RunWebFetchSummariser(context.Background(), client, "u", "p", "md", false)
	if err == nil || !errors.Is(err, want) {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}

func TestSummariserFunc_AdaptsToInterface(t *testing.T) {
	var f SummariserClient = SummariserFunc(func(ctx context.Context, req SummariserRequest) (string, error) {
		return "ok", nil
	})
	got, err := f.Summarise(context.Background(), SummariserRequest{})
	if err != nil || got != "ok" {
		t.Fatalf("adapter failure: %v, %q", err, got)
	}
}
