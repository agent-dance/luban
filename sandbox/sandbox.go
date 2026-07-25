package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Config defines what the sandboxed process is allowed to do.
type Config struct {
	// ReadOnlyPaths are absolute paths the process can read but not write.
	ReadOnlyPaths []string

	// ReadWritePaths are absolute paths the process can read and write.
	ReadWritePaths []string

	// AllowedDomains are domain names the process can access via network.
	// Empty = no network. Use ["*"] to allow all.
	AllowedDomains []string

	// WorkDir is the working directory for the sandboxed process.
	WorkDir string

	// Env is the environment for the sandboxed process.
	// If nil, inherits from parent (filtered for safety).
	Env []string
}

// Backend wraps command execution with OS-level sandboxing.
type Backend interface {
	// Name returns the backend identifier (e.g., "bwrap", "sandbox-exec", "none").
	Name() string

	// Available reports whether this backend can run on the current system.
	Available() bool

	// Command returns a sandboxed exec.Cmd.
	// The returned command has NOT been started — the caller calls cmd.Start().
	Command(ctx context.Context, cfg Config, name string, args ...string) (*exec.Cmd, error)
}

// Capability is an immutable description of the executable authority behind
// a real sandbox backend. ExecutableIdentity is intentionally opaque outside
// this package; callers bind the stable ID into permission receipts instead of
// making policy decisions from individual stat fields.
type Capability struct {
	Backend            string
	ExecutablePath     string
	ExecutableIdentity string
	// protections is deliberately package-private. An exported third-party
	// Backend may publish executable identity for fail-closed execution, but it
	// cannot self-assert a protection property used for permission auto-approval.
	protections CapabilityProtection
}

// CapabilityProtection records isolation properties that are actually
// enforced by a backend. Auto-approval must require the relevant property;
// executable identity alone does not make a broad read-write bind safe.
type CapabilityProtection uint64

const (
	// ProtectionProtectedPaths proves that commands cannot mutate the runtime's
	// protected credential, configuration, and VCS paths through indirect code.
	ProtectionProtectedPaths CapabilityProtection = 1 << iota
)

func (c Capability) Enforces(protection CapabilityProtection) bool {
	return protection != 0 && c.protections&protection == protection
}

// ID returns the stable digest used to bind permission preflight, approval,
// and execution to the same sandbox executable authority.
func (c Capability) ID() string {
	if strings.TrimSpace(c.Backend) == "" || !filepath.IsAbs(c.ExecutablePath) || strings.TrimSpace(c.ExecutableIdentity) == "" {
		return ""
	}
	encoded := fmt.Sprintf("%s\x00%s\x00%s\x00%d", c.Backend, filepath.Clean(c.ExecutablePath), c.ExecutableIdentity, c.protections)
	digest := sha256.Sum256([]byte(encoded))
	return hex.EncodeToString(digest[:])
}

// CapabilityProvider is implemented by backends whose OS isolation boundary
// is rooted in a prepared, immutable executable authority. Merely satisfying
// Backend is not sufficient for permission auto-approval: third-party
// backends must explicitly publish and continuously validate a capability.
type CapabilityProvider interface {
	SandboxCapability() (Capability, bool)
}

// Snapshot returns a currently valid immutable capability for backend. It
// rejects name-only backends and mismatched backend identities.
func Snapshot(backend Backend) (Capability, bool) {
	if backend == nil || backend.Name() == "none" {
		return Capability{}, false
	}
	provider, ok := backend.(CapabilityProvider)
	if !ok {
		return Capability{}, false
	}
	capability, ok := provider.SandboxCapability()
	if !ok || capability.Backend != backend.Name() || capability.ID() == "" {
		return Capability{}, false
	}
	return capability, true
}

// IsRealBackend reports whether backend provides an actual OS isolation
// boundary. NoopBackend remains an unsandboxed execution backend, but it
// must never be treated as sandbox authority for permission auto-approval.
func IsRealBackend(backend Backend) bool {
	_, ok := Snapshot(backend)
	return ok
}

// platformBackends is set by platform-specific init() functions.
var platformBackends []Backend

// Detect returns the best available backend for the current platform.
// Returns a NoopBackend if no sandboxing is available.
func Detect() Backend {
	for _, b := range platformBackends {
		if b.Available() {
			return b
		}
	}
	return NoopBackend{}
}

// NoopBackend passes commands through without sandboxing.
type NoopBackend struct{}

func (NoopBackend) Name() string    { return "none" }
func (NoopBackend) Available() bool { return true }
func (NoopBackend) Command(ctx context.Context, cfg Config, name string, args ...string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}
	if cfg.Env != nil {
		cmd.Env = cfg.Env
	}
	return cmd, nil
}

// validatePaths checks all paths are absolute and contain no control characters.
func validatePaths(paths []string) error {
	for _, p := range paths {
		if p == "" {
			return fmt.Errorf("sandbox: empty path")
		}
		if !filepath.IsAbs(p) {
			return fmt.Errorf("sandbox: relative path not allowed: %q", p)
		}
		for _, r := range p {
			if r < 0x20 || r == 0x7f {
				return fmt.Errorf("sandbox: path contains control character: %q", p)
			}
		}
	}
	return nil
}

// SafeEnv returns a filtered copy of the environment, stripping secrets.
// It uses an allowlist of safe variable names/prefixes and a denylist of
// patterns that indicate sensitive values (keys, tokens, passwords, etc.).
func SafeEnv(env []string) []string {
	// Allowlist of safe exact names and prefix families (suffix "_" = prefix match).
	safe := map[string]bool{
		"PATH": true, "HOME": true, "USER": true, "LOGNAME": true,
		"LANG": true, "LC_": true, "TERM": true, "SHELL": true,
		"TMPDIR": true, "TZ": true, "EDITOR": true, "VISUAL": true,
		"XDG_": true, "DISPLAY": true, "COLORTERM": true,
		"NO_COLOR": true, "FORCE_COLOR": true,
	}
	// Denylist substrings (case-insensitive match against the key).
	deny := []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "CREDENTIAL", "AUTH"}

	var filtered []string
	for _, e := range env {
		k, _, _ := strings.Cut(e, "=")
		upper := strings.ToUpper(k)

		// Check denylist first.
		denied := false
		for _, d := range deny {
			if strings.Contains(upper, d) {
				denied = true
				break
			}
		}
		if denied {
			continue
		}

		// Check allowlist (prefix match for entries ending with "_").
		allowed := false
		for s := range safe {
			if strings.HasSuffix(s, "_") {
				if strings.HasPrefix(k, s) {
					allowed = true
					break
				}
			} else {
				if k == s {
					allowed = true
					break
				}
			}
		}
		if allowed {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
