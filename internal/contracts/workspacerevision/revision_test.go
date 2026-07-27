package workspacerevision

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReceiptBindsChangedPathsAndScopeEpoch(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "first.txt")
	secondPath := filepath.Join(root, "second.txt")
	if err := os.WriteFile(firstPath, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger := NewLedger()
	first, err := ledger.Commit(root, []string{firstPath})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Valid() || first.Epoch() != 1 || first.Digest() == "" {
		t.Fatalf("first receipt = epoch %d digest %q", first.Epoch(), first.Digest())
	}
	if err := ledger.Validate(first); err != nil {
		t.Fatalf("fresh receipt: %v", err)
	}
	if err := os.WriteFile(firstPath, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Validate(first); !errors.Is(err, ErrRevisionChanged) {
		t.Fatalf("changed path validation = %v", err)
	}

	second, err := ledger.Commit(root, []string{secondPath})
	if err != nil {
		t.Fatal(err)
	}
	if second.Epoch() != 2 {
		t.Fatalf("second epoch = %d, want 2", second.Epoch())
	}
	if err := ledger.Validate(first); !errors.Is(err, ErrRevisionChanged) {
		t.Fatalf("superseded receipt validation = %v", err)
	}
}

func TestReceiptAcceptsCommittedDeletionAndSeparatesWorktrees(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	deleted := filepath.Join(rootA, "deleted.txt")
	ledger := NewLedger()
	receiptA, err := ledger.Commit(rootA, []string{deleted})
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(rootB, "other.txt")
	if err := os.WriteFile(other, []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	receiptB, err := ledger.Commit(rootB, []string{other})
	if err != nil {
		t.Fatal(err)
	}
	if receiptA.Epoch() != 1 || receiptB.Epoch() != 1 {
		t.Fatalf("worktree epochs = %d, %d", receiptA.Epoch(), receiptB.Epoch())
	}
	if err := ledger.Validate(receiptA); err != nil {
		t.Fatalf("other worktree invalidated deletion receipt: %v", err)
	}
}
