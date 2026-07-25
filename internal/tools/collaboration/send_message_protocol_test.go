package collaboration

import (
	"sync"
	"testing"
)

func TestShutdownRequestIDsRemainUniqueUnderConcurrency(t *testing.T) {
	const count = 1000
	ids := make([]string, count)
	var wait sync.WaitGroup
	wait.Add(count)
	for index := range count {
		index := index
		go func() {
			defer wait.Done()
			ids[index] = generateShutdownRequestID("worker-1")
		}()
	}
	wait.Wait()
	seen := make(map[string]struct{}, count)
	for _, id := range ids {
		if _, duplicate := seen[id]; duplicate {
			t.Fatalf("duplicate request id %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestMailboxRetryClassification(t *testing.T) {
	tests := map[string]bool{
		"locked: another writer":           true,
		"resource temporarily unavailable": true,
		"context deadline exceeded":        true,
		"would block":                      true,
		"json marshal: invalid":            false,
		"":                                 false,
	}
	for input, want := range tests {
		var err error
		if input != "" {
			err = protocolTestError(input)
		}
		if got := mailboxErrorRetryable(err); got != want {
			t.Fatalf("mailboxErrorRetryable(%q) = %t, want %t", input, got, want)
		}
	}
}

type protocolTestError string

func (err protocolTestError) Error() string { return string(err) }
