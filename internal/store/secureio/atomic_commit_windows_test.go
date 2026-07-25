//go:build windows

package secureio

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsAtomicCommitErrorsAreFailClosed(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	temporary := filepath.Join(dir, "temporary")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(target, "sentinel")
	if err := os.WriteFile(sentinel, []byte("protected"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(temporary, []byte("candidate"), 0o600); err != nil {
		t.Fatal(err)
	}

	commitErr := ReplaceFileAtomically(temporary, target)
	if commitErr == nil {
		t.Fatal("Windows replacement unexpectedly replaced a non-empty directory with a file")
	}
	var linkErr *os.LinkError
	if !errors.As(commitErr, &linkErr) {
		t.Fatalf("Windows commit error type = %T, want *os.LinkError", commitErr)
	}
	assertAtomicCommitContent(t, sentinel, "protected")
	assertAtomicCommitContent(t, temporary, "candidate")
}
