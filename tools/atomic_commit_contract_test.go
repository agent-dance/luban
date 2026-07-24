package tools

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestAtomicCommitReplaceContract(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	temporary := filepath.Join(dir, "temporary")
	if err := os.WriteFile(target, []byte("previous"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(temporary, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := replaceFileAtomically(temporary, target); err != nil {
		t.Fatalf("atomic replacement failed: %v", err)
	}
	assertAtomicCommitContent(t, target, "replacement")
	if _, err := os.Stat(temporary); !os.IsNotExist(err) {
		t.Fatalf("replacement source still exists or could not be inspected: %v", err)
	}
}

func TestAtomicCommitNoReplacePreservesConcurrentWinner(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	temporary := filepath.Join(dir, "temporary")
	if err := os.WriteFile(target, []byte("winner"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(temporary, []byte("candidate"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := publishFileAtomicallyNoReplace(temporary, target); err == nil {
		t.Fatal("no-replace commit unexpectedly replaced an existing target")
	}
	assertAtomicCommitContent(t, target, "winner")
	assertAtomicCommitContent(t, temporary, "candidate")
}

func TestAtomicCommitNoReplaceHasExactlyOneConcurrentWinner(t *testing.T) {
	const candidates = 16
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	temporaryPaths := make([]string, candidates)
	for index := range candidates {
		temporaryPaths[index] = filepath.Join(dir, fmt.Sprintf("candidate-%02d", index))
		if err := os.WriteFile(temporaryPaths[index], []byte(fmt.Sprintf("value-%02d", index)), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	start := make(chan struct{})
	results := make(chan error, candidates)
	var workers sync.WaitGroup
	workers.Add(candidates)
	for _, temporary := range temporaryPaths {
		go func() {
			defer workers.Done()
			<-start
			results <- publishFileAtomicallyNoReplace(temporary, target)
		}()
	}
	close(start)
	workers.Wait()
	close(results)

	winners := 0
	for err := range results {
		if err == nil {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("no-replace commit winners = %d, want exactly 1", winners)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	matchedCandidate := false
	for index := range candidates {
		if string(data) == fmt.Sprintf("value-%02d", index) {
			matchedCandidate = true
			break
		}
	}
	if !matchedCandidate {
		t.Fatalf("published partial or unknown content %q", data)
	}
}

func TestAtomicWriteFileNeverExposesMissingOrPartialReplacement(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	payloads := [][]byte{
		bytes.Repeat([]byte{'a'}, 256*1024),
		bytes.Repeat([]byte{'b'}, 256*1024),
		bytes.Repeat([]byte{'c'}, 256*1024),
	}
	if err := atomicWriteFile(target, payloads[0], 0o600); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	observations := make(chan error, 1)
	var readers sync.WaitGroup
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				data, err := os.ReadFile(target)
				if err != nil {
					select {
					case observations <- fmt.Errorf("replacement target became unavailable: %w", err):
					default:
					}
					return
				}
				complete := false
				for _, payload := range payloads {
					if bytes.Equal(data, payload) {
						complete = true
						break
					}
				}
				if !complete {
					select {
					case observations <- fmt.Errorf("observed partial replacement with %d bytes", len(data)):
					default:
					}
					return
				}
			}
		}()
	}

	for index := 0; index < 48; index++ {
		if err := atomicWriteFile(target, payloads[index%len(payloads)], 0o600); err != nil {
			close(stop)
			readers.Wait()
			t.Fatalf("atomic replacement %d failed: %v", index, err)
		}
	}
	close(stop)
	readers.Wait()
	select {
	case err := <-observations:
		t.Fatal(err)
	default:
	}
}

func assertAtomicCommitContent(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != expected {
		t.Fatalf("%s content = %q, want %q", path, data, expected)
	}
}
