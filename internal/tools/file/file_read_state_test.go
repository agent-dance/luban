package file

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestReadFileState_GetSetClear validates the context-scoped lifecycle.
func TestReadFileState_GetSetClear(t *testing.T) {
	s := NewReadFileState()
	if count := readFileStateEntryCount(s); count != 0 {
		t.Fatalf("expected empty state, got entries=%d", count)
	}
	if _, ok := s.GetForContext(context.Background(), "/missing/path"); ok {
		t.Fatal("missing path should return ok=false")
	}

	entry := ReadFileEntry{
		TimestampMs:   1234,
		Offset:        10,
		Limit:         50,
		IsPartialView: true,
		Content:       "hello",
	}
	s.SetForContext(context.Background(), "/tmp/foo.txt", entry)
	if count := readFileStateEntryCount(s); count != 1 {
		t.Fatalf("expected entries=1, got %d", count)
	}
	got, ok := s.GetForContext(context.Background(), "/tmp/foo.txt")
	if !ok {
		t.Fatal("stored entry should return ok=true")
	}
	if got.TimestampMs != 1234 || got.Offset != 10 || got.Limit != 50 || !got.IsPartialView || got.Content != "hello" {
		t.Fatalf("entry mismatch: %+v", got)
	}
	s.ClearForContext(context.Background(), "/tmp/foo.txt")
	if count := readFileStateEntryCount(s); count != 0 {
		t.Fatalf("expected entries=0 after Clear, got %d", count)
	}
}

// TestReadFileState_NormalizesKeys ensures relative and absolute paths
// resolve to the same key.
func TestReadFileState_NormalizesKeys(t *testing.T) {
	s := NewReadFileState()
	abs, err := filepath.Abs("foo/bar.txt")
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	s.SetForContext(context.Background(), "foo/bar.txt", ReadFileEntry{TimestampMs: 99})
	if _, ok := s.GetForContext(context.Background(), abs); !ok {
		t.Fatalf("expected absolute path %q to resolve to same entry", abs)
	}
}

// TestReadFileState_ConcurrentAccess validates that concurrent Set/Get
// operations are race-free.
func TestReadFileState_ConcurrentAccess(t *testing.T) {
	s := NewReadFileState()
	const workers = 32
	const iters = 200
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(id int) {
			defer wg.Done()
			path := filepath.Join("/tmp/r", string(rune('a'+id%26)))
			for i := 0; i < iters; i++ {
				s.SetForContext(context.Background(), path, ReadFileEntry{TimestampMs: int64(i)})
				_, _ = s.GetForContext(context.Background(), path)
			}
		}(w)
	}
	wg.Wait()
}

// TestReadFileState_NilSafe verifies nil-receiver methods are no-ops.
func TestReadFileState_NilSafe(t *testing.T) {
	var s *ReadFileState
	if readFileStateEntryCount(s) != 0 {
		t.Fatal("nil receiver must contain no entries")
	}
	if _, ok := s.GetForContext(context.Background(), "/x"); ok {
		t.Fatal("nil receiver lookup must return ok=false")
	}
	// Context-scoped writes and clears must not panic.
	s.SetForContext(context.Background(), "/x", ReadFileEntry{})
	s.ClearForContext(context.Background(), "/x")
}

func TestP0ReadFileStateConcurrentCoverageUnion(t *testing.T) {
	state := NewReadFileState()
	path := filepath.Join(t.TempDir(), "coverage-union.txt")
	if err := os.WriteFile(path, []byte("one stable file version"), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	const total = 32
	var wg sync.WaitGroup
	for line := 1; line <= total; line++ {
		line := line
		wg.Add(1)
		go func() {
			defer wg.Done()
			state.RecordReadForContext(context.Background(), path, ReadFileEntry{
				TimestampMs: 7, MtimeNs: 7000, TotalBytes: 320, TotalLines: total,
				ContentDigest: fileContentDigest([]byte("one stable file version")),
				FileIdentity:  identity,
				Coverage:      []ReadLineRange{{StartLine: line, EndLine: line + 1}},
				LastTool:      "Read", DedupEligible: true,
			})
		}()
	}
	wg.Wait()
	entry, ok := state.GetForContext(context.Background(), path)
	if !ok || !entry.CoverageComplete || len(entry.Coverage) != 1 || entry.Coverage[0] != (ReadLineRange{StartLine: 1, EndLine: total + 1}) {
		t.Fatalf("concurrent observations lost coverage: %+v", entry)
	}
	if entry.FullSnapshot {
		t.Fatal("range union incorrectly claimed one complete Content snapshot")
	}
}

func TestP0ReadFileStateDoesNotMergeDifferentVersions(t *testing.T) {
	state := NewReadFileState()
	state.RecordReadForContext(context.Background(), "/tmp/versioned.txt", ReadFileEntry{
		TimestampMs: 1, MtimeNs: 1000, TotalBytes: 10, TotalLines: 2,
		ContentDigest: fileContentDigest([]byte("alpha\nbeta")),
		Coverage:      []ReadLineRange{{StartLine: 1, EndLine: 2}},
	})
	state.RecordReadForContext(context.Background(), "/tmp/versioned.txt", ReadFileEntry{
		// Deliberately preserve every legacy version hint. Digest must still
		// prevent the observations from being merged.
		TimestampMs: 1, MtimeNs: 1000, TotalBytes: 10, TotalLines: 2,
		ContentDigest: fileContentDigest([]byte("gamma\nzeta")),
		Coverage:      []ReadLineRange{{StartLine: 2, EndLine: 3}},
	})
	entry, _ := state.GetForContext(context.Background(), "/tmp/versioned.txt")
	if len(entry.Coverage) != 1 || entry.Coverage[0] != (ReadLineRange{StartLine: 2, EndLine: 3}) {
		t.Fatalf("different file versions merged coverage: %+v", entry)
	}
}

func TestP0ReadFileStateStrongEvidenceSurvivesConcurrentOrderPermutations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "order.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree"), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := fileContentDigest([]byte("one\ntwo\nthree"))
	strong := ReadFileEntry{
		MtimeNs: 10, TotalBytes: 13, TotalLines: 3, ContentDigest: digest,
		FileIdentity:     identity,
		Coverage:         []ReadLineRange{{StartLine: 1, EndLine: 4}},
		CoverageComplete: true, FullSnapshot: true, Content: "one\ntwo\nthree",
	}
	weak := ReadFileEntry{
		MtimeNs: 10, TotalBytes: 13, TotalLines: 3, ContentDigest: digest,
		FileIdentity: identity,
		Coverage:     []ReadLineRange{{StartLine: 2, EndLine: 3}}, Content: "two",
	}
	for _, strongFirst := range []bool{true, false} {
		name := "weak-first"
		if strongFirst {
			name = "strong-first"
		}
		t.Run(name, func(t *testing.T) {
			for iteration := 0; iteration < 64; iteration++ {
				state := NewReadFileState()
				firstDone := make(chan struct{})
				var wg sync.WaitGroup
				wg.Add(2)
				go func() {
					defer wg.Done()
					if strongFirst {
						state.RecordReadForContext(context.Background(), path, strong)
					} else {
						state.RecordReadForContext(context.Background(), path, weak)
					}
					close(firstDone)
				}()
				go func() {
					defer wg.Done()
					<-firstDone
					if strongFirst {
						state.RecordReadForContext(context.Background(), path, weak)
					} else {
						state.RecordReadForContext(context.Background(), path, strong)
					}
				}()
				wg.Wait()
				entry, ok := state.GetForContext(context.Background(), path)
				if !ok || !entry.FullSnapshot || !entry.CoverageComplete || entry.Content != strong.Content {
					t.Fatalf("strong evidence was downgraded at iteration %d: %+v", iteration, entry)
				}
			}
		})
	}
}

func readFileStateEntryCount(state *ReadFileState) int {
	if state == nil {
		return 0
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return len(state.entries)
}
