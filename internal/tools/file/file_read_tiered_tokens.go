// Package file — file_read_tiered_tokens.go implements a two-stage
// token check for FileRead. Mirrors TS validateContentTokens which
// estimates tokens from byte size first and only invokes the precise
// tokenizer when the rough estimate is within striking distance of the
// limit.
//
// The fast estimate uses bytes / 4 (≈ 4 chars per token in English text)
// — the 0.25 ratio is conservative for code, a bit liberal for prose.
// We bracket a safety margin (1.25× of fast estimate) before triggering
// the precise tokenizer so borderline files still get an accurate count.
package file

import "context"

// fastTokenEstimate returns a cheap upper-bound estimate of the number
// of tokens in a UTF-8 byte slice. Worst case is ASCII text → 1 token
// per 3 chars, so we use len/3 as the conservative cap.
func fastTokenEstimate(content string) int {
	if len(content) == 0 {
		return 0
	}
	return (len(content) + 2) / 3
}

func (t *FileReadTool) tieredReadTokenCount(ctx context.Context, content string, maxTokens int) (int, bool) {
	fast := fastTokenEstimate(content)
	if maxTokens > 0 && fast <= maxTokens/4 {
		return fast, false
	}
	if t != nil && t.PreciseTokenCounter != nil {
		if count, err := t.PreciseTokenCounter(ctx, content); err == nil && count >= 0 {
			return count, true
		}
	}
	return estimateReadTokens(content), true
}
