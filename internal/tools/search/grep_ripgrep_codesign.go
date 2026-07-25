// Package search contains the macOS ripgrep signature check.
//
// Mirrors src/utils/ripgrep.ts:619-679 codesignRipgrepIfNecessary. On
// macOS, Apple Silicon Gatekeeper refuses to execute unsigned binaries
// from unknown developers — if the Go distribution ever ships a bundled
// rg, search would fail before the bundled runtime can be used. This file
// runs `codesign --sign - --force <path>` and `xattr -d com.apple.quarantine`
// on first use to neutralise both the missing-signature and the
// quarantine flag. The function is a no-op on non-Darwin platforms.
package search

import (
	"context"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

var (
	codesignOnce sync.Once
	codesignErr  error
)

// codesignRipgrepIfNecessary is the Darwin-only escape hatch for the
// embedded rg copy. Returns nil on non-darwin so callers don't have to
// branch. The first invocation does the work; subsequent calls return
// the cached error.
func codesignRipgrepIfNecessary(path string) error {
	if runtime.GOOS != "darwin" || path == "" {
		return nil
	}
	codesignOnce.Do(func() {
		codesignErr = codesignRipgrepDarwin(path)
	})
	return codesignErr
}

func codesignRipgrepDarwin(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Best-effort: ad-hoc sign with the current identity — failures are
	// non-fatal because the binary may already be properly signed.
	if err := exec.CommandContext(ctx, "codesign", "--sign", "-", "--force", path).Run(); err != nil {
		// fall through to xattr — even if codesign failed we may still
		// be able to remove the quarantine flag.
		_ = err
	}
	if err := exec.CommandContext(ctx, "xattr", "-d", "com.apple.quarantine", path).Run(); err != nil {
		// Same logic — many binaries don't have the quarantine flag.
		_ = err
	}
	return nil
}
