// Package tokenizer provides model-text token counters without depending on
// runtime orchestration packages.
package tokenizer

import "github.com/pkoukk/tiktoken-go"

// TiktokenCounter uses the cl100k_base encoding for token estimation. It
// falls back to a four-bytes-per-token estimate if the encoding cannot load.
type TiktokenCounter struct {
	enc *tiktoken.Tiktoken
}

// NewTiktokenCounter creates a counter using the cl100k_base encoding.
func NewTiktokenCounter() *TiktokenCounter {
	enc, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return &TiktokenCounter{}
	}
	return &TiktokenCounter{enc: enc}
}

// Count returns the estimated token count for text.
func (c *TiktokenCounter) Count(text string) int {
	if c == nil || c.enc == nil {
		return len(text) / 4
	}
	return len(c.enc.Encode(text, nil, nil))
}
