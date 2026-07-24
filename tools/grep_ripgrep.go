// Package tools — grep_ripgrep.go centralises ripgrep binary location.
//
// Mirrors src/tools/GrepTool/ripgrepRunner.ts. Lookup order, in priority:
//
//  1. CLAUDE_RG_PATH environment override (test hook + power-user escape).
//  2. exec.LookPath("rg") — the system PATH copy.
//  3. The embedded fallback path under .claude/bin/rg (.exe on Windows) when
//     bundled with the CLI distribution.
//
// When none of those produce an executable the call returns an
// errRipgrepUnavailable. Grep never silently downgrades in production; the
// Go scanner is available only through an explicit fallback environment flag.
package tools

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// The tools package exercises many process-heavy lifecycle paths in one
// process. Keep the locator probe bounded, but leave enough headroom that a
// valid bundled binary is not rejected solely because the host is saturated.
const ripgrepVersionProbeTimeout = 15 * time.Second

// LocateRipgrep returns the absolute path to a usable rg binary, or an error
// wrapped with errRipgrepUnavailable when none can be found. Results are
// cached for the lifetime of the process via locateRipgrepOnce; tests that
// need to mutate the discovered path can call ResetRipgrepLocation.
func LocateRipgrep() (string, error) {
	locateRipgrepMu.Lock()
	defer locateRipgrepMu.Unlock()
	locateRipgrepOnce.Do(loadRipgrepLocation)
	if locatedRipgrepErr != nil {
		return "", locatedRipgrepErr
	}
	return locatedRipgrepPath, nil
}

// verifyRipgrepBinary runs `rg --version` and confirms the output starts
// with "ripgrep ". Location resolution itself is cached, so a second global
// once here would incorrectly reuse verification after a test/runtime reset
// changes the candidate path.
func verifyRipgrepBinary(path string) bool {
	if path == "" {
		return false
	}
	output, err := os.CreateTemp("", "claude-rg-version-*")
	if err != nil {
		return false
	}
	outputPath := output.Name()
	defer func() {
		_ = output.Close()
		_ = os.Remove(outputPath)
	}()
	if err := output.Chmod(0o600); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), ripgrepVersionProbeTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, path, "--version")
	// A real file avoids os/exec's capture pipe. A malformed candidate can
	// otherwise leave a descendant holding that pipe open and make a completed
	// version probe appear to hang until the context deadline.
	command.Stdout = output
	command.Stderr = output
	if err := command.Run(); err != nil {
		return false
	}
	if _, err := output.Seek(0, io.SeekStart); err != nil {
		return false
	}
	version, err := io.ReadAll(io.LimitReader(output, 4096))
	return err == nil && strings.HasPrefix(strings.TrimSpace(string(version)), "ripgrep ")
}

// ResetRipgrepLocation is intended for tests; it forces the next LocateRipgrep
// call to re-resolve. Production code should never call this.
func ResetRipgrepLocation() {
	locateRipgrepMu.Lock()
	defer locateRipgrepMu.Unlock()
	locateRipgrepOnce = sync.Once{}
	locatedRipgrepPath = ""
	locatedRipgrepErr = nil
	codesignOnce = sync.Once{}
	codesignErr = nil
}

var (
	locateRipgrepOnce  sync.Once
	locateRipgrepMu    sync.Mutex
	locatedRipgrepPath string
	locatedRipgrepErr  error
)

func loadRipgrepLocation() {
	if override := strings.TrimSpace(os.Getenv("CLAUDE_RG_PATH")); override != "" {
		if isExecutable(override) {
			setLocatedRipgrep(override)
			return
		}
		locatedRipgrepErr = ripgrepUnavailableError("CLAUDE_RG_PATH=%s is not an executable file", override)
		return
	}
	if userWantsSystemRipgrep() {
		if path, err := exec.LookPath("rg"); err == nil {
			setLocatedRipgrep(path)
			return
		}
	}
	if embedded, ok := findEmbeddedRipgrep(); ok {
		setLocatedRipgrep(embedded)
		return
	}
	// Go development builds do not carry the npm vendored tree. Keep PATH as
	// a final locator candidate, but never use a Go scanner when it is absent.
	if path, err := exec.LookPath("rg"); err == nil {
		setLocatedRipgrep(path)
		return
	}
	locatedRipgrepErr = ripgrepUnavailableError("ripgrep (rg) was not found in PATH or bundled location")
}

func setLocatedRipgrep(path string) {
	if !verifyRipgrepBinary(path) {
		locatedRipgrepErr = ripgrepUnavailableError("ripgrep at %q failed --version sanity check (not real rg?)", path)
		return
	}
	locatedRipgrepPath = path
}

func userWantsSystemRipgrep() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("USE_BUILTIN_RIPGREP"))) {
	case "0", "false", "no", "off":
		return true
	default:
		return false
	}
}

// findEmbeddedRipgrep walks the conventional bundled-binary locations for an
// rg copy ('.claude/bin/rg' relative to the running executable, then the
// repository root, then ~/.claude/bin). Returns the first hit that is
// executable.
func findEmbeddedRipgrep() (string, bool) {
	exeName := "rg"
	if runtime.GOOS == "windows" {
		exeName = "rg.exe"
	}
	candidates := []string{}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, exeName),
			filepath.Join(exeDir, ".claude", "bin", exeName),
			filepath.Join(exeDir, "..", ".claude", "bin", exeName),
		)
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, ".claude", "bin", exeName))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".claude", "bin", exeName))
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if isExecutable(candidate) {
			// grep-codesign-mac: ad-hoc-sign + de-quarantine the bundled
			// rg on macOS first use so Gatekeeper doesn't block exec.
			_ = CodesignRipgrepIfNecessary(candidate)
			return candidate, true
		}
	}
	return "", false
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	mode := info.Mode().Perm()
	return mode&0o111 != 0
}

// SlowFallbackEnabled reports whether the explicitly gated Go grep fallback
// is permitted. Production defaults to false so missing/broken rg is visible.
func SlowFallbackEnabled() bool {
	if searchEnvTruthy(os.Getenv("CLAUDE_CODE_DISABLE_SEARCH_FALLBACK")) {
		return false
	}
	return searchEnvTruthy(os.Getenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK")) ||
		searchEnvTruthy(os.Getenv("CLAUDE_CODE_ALLOW_SEARCH_FALLBACK"))
}
