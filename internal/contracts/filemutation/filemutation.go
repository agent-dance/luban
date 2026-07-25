// Package filemutation defines the capability boundary used by tools that
// mutate files through a command or another non-file-domain executor.
package filemutation

import "context"

// Target identifies one primary mutation path and its optional backup path.
// The coordinator owns the exact validation and evidence semantics for each.
type Target struct {
	Path       string
	BackupPath string
}

// Coordinator serializes cooperating file mutations and maintains the
// read-before-write evidence owned by the file tool domain.
type Coordinator interface {
	Lock(targets []Target) func()
	ValidateFullRead(ctx context.Context, targets []Target) error
	Commit(ctx context.Context, targets []Target, source string) error
	Invalidate(ctx context.Context, targets []Target)
}
