package tokenizer

import "testing"

func TestNewTiktokenCounterCountsText(t *testing.T) {
	counter := NewTiktokenCounter()
	if counter == nil {
		t.Fatal("NewTiktokenCounter returned nil")
	}
	if got := counter.Count("hello world"); got <= 0 {
		t.Fatalf("Count(hello world) = %d, want a positive count", got)
	}
	if got := counter.Count(""); got != 0 {
		t.Fatalf("Count(empty) = %d, want 0", got)
	}
}

func TestTiktokenCounterFallback(t *testing.T) {
	counter := &TiktokenCounter{}
	if got := counter.Count("12345678"); got != 2 {
		t.Fatalf("fallback Count = %d, want 2", got)
	}
	var nilCounter *TiktokenCounter
	if got := nilCounter.Count("12345678"); got != 2 {
		t.Fatalf("nil fallback Count = %d, want 2", got)
	}
}
