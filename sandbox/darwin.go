//go:build darwin

package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const seatbeltBin = "/usr/bin/sandbox-exec"

// SeatbeltBackend uses macOS sandbox-exec (Seatbelt) for sandboxing.
type SeatbeltBackend struct {
	authority    *executableAuthority
	authorityOps *executableAuthorityOps
}

var defaultSeatbeltAuthority executableAuthority

func init() {
	platformBackends = append(platformBackends, SeatbeltBackend{})
}

func (SeatbeltBackend) Name() string { return "sandbox-exec" }

func (b SeatbeltBackend) authorityRef() *executableAuthority {
	if b.authority != nil {
		return b.authority
	}
	return &defaultSeatbeltAuthority
}

func (b SeatbeltBackend) configureAuthority() *executableAuthority {
	ops := executableAuthorityOps{
		resolve: func() (string, error) { return seatbeltBin, nil },
		probe:   defaultExecutableProbe,
		launch: func(ctx context.Context, executable trustedExecutable, args ...string) *exec.Cmd {
			// macOS does not support execve through /dev/fd. The fixed SIP-owned
			// path is re-probed immediately before construction and any observed
			// identity change permanently poisons the backend.
			return exec.CommandContext(ctx, executable.path, args...)
		},
	}
	if b.authorityOps != nil {
		ops = *b.authorityOps
	}
	authority := b.authorityRef()
	authority.configure(b.Name(), ops)
	return authority
}

func (b SeatbeltBackend) Available() bool {
	return b.configureAuthority().available()
}

func (b SeatbeltBackend) SandboxCapability() (Capability, bool) {
	return b.configureAuthority().snapshot()
}

func (b SeatbeltBackend) Command(ctx context.Context, cfg Config, name string, args ...string) (*exec.Cmd, error) {
	authority := b.configureAuthority()

	// F2: validate all user-supplied paths before embedding in the profile.
	if err := validatePaths(cfg.ReadOnlyPaths); err != nil {
		return nil, err
	}
	if err := validatePaths(cfg.ReadWritePaths); err != nil {
		return nil, err
	}
	if cfg.WorkDir != "" {
		if err := validatePaths([]string{cfg.WorkDir}); err != nil {
			return nil, err
		}
	}

	profile := buildSeatbeltProfile(cfg)

	// sandbox-exec -p <profile> <name> <args...>
	sargs := []string{"-p", profile, name}
	sargs = append(sargs, args...)

	cmd, err := authority.command(ctx, sargs...)
	if err != nil {
		return nil, fmt.Errorf("sandbox: sandbox-exec authority unavailable: %w", err)
	}
	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}
	// F4: filter environment to avoid leaking secrets.
	if cfg.Env != nil {
		cmd.Env = cfg.Env
	} else {
		cmd.Env = SafeEnv(os.Environ())
	}
	return cmd, nil
}

// seatbeltQuote wraps a path in double quotes, strips control characters, and
// escapes backslashes and double-quote characters for use in a Seatbelt (SBPL)
// profile string.
//
// F2: control characters (including newlines) are stripped to prevent profile
// injection attacks.
func seatbeltQuote(path string) string {
	// Strip control characters that could break profile syntax.
	var clean strings.Builder
	for _, r := range path {
		if r < 0x20 || r == 0x7f { // control chars including \n, \r, \t
			continue // strip
		}
		clean.WriteRune(r)
	}
	s := clean.String()
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// buildSeatbeltProfile generates a Seatbelt (SBPL) profile string for the given Config.
func buildSeatbeltProfile(cfg Config) string {
	var sb strings.Builder

	w := func(s string) { sb.WriteString(s + "\n") }

	w("(version 1)")
	w("(deny default)")
	w("(allow process-exec)")
	w("(allow process-fork)")
	w("(allow sysctl-read)")

	// F12: restrict mach-lookup to essential services only instead of allowing all.
	// Allow only essential Mach services for basic command execution.
	w(`(allow mach-lookup (global-name-regex #"^com\.apple\.system\."))`)
	w(`(allow mach-lookup (global-name "com.apple.CoreServices.coreservicesd"))`)
	w(`(allow mach-lookup (global-name "com.apple.SecurityServer"))`)
	w("")

	// Base system paths — always readable.
	systemPaths := []string{
		"/usr",
		"/bin",
		"/Library",
		"/System",
		"/private/etc",
		"/private/tmp",
		"/dev",
		"/Applications",
	}
	w(";; Base system paths (always readable)")
	for _, p := range systemPaths {
		fmt.Fprintf(&sb, "(allow file-read* (subpath %s))\n", seatbeltQuote(p))
	}
	w(`(allow file-read* (literal "/"))`)
	w("")

	// User-specified read-only paths.
	if len(cfg.ReadOnlyPaths) > 0 {
		w(";; User read-only paths")
		for _, p := range cfg.ReadOnlyPaths {
			fmt.Fprintf(&sb, "(allow file-read* (subpath %s))\n", seatbeltQuote(p))
		}
		w("")
	}

	// User-specified read-write paths.
	if len(cfg.ReadWritePaths) > 0 {
		w(";; User read-write paths")
		for _, p := range cfg.ReadWritePaths {
			fmt.Fprintf(&sb, "(allow file-read* (subpath %s))\n", seatbeltQuote(p))
			fmt.Fprintf(&sb, "(allow file-write* (subpath %s))\n", seatbeltQuote(p))
		}
		w("")
	}

	// Temp directories.
	// F6: /tmp is shared across all sandbox invocations (Phase 2 limitation).
	// A future improvement is to create a per-invocation tmpdir and bind-mount it.
	w(";; Temp (shared /tmp — per-invocation isolation is a Phase 2 improvement)")
	w(`(allow file-read* (subpath "/tmp"))`)
	w(`(allow file-write* (subpath "/tmp"))`)
	w(`(allow file-read* (subpath "/private/var"))`)
	w(`(allow file-write* (subpath "/private/var/folders"))`)
	w("")

	// Network rules.
	w(";; Network")
	w(`(allow network* (remote ip "localhost:*"))`)
	if len(cfg.AllowedDomains) == 1 && cfg.AllowedDomains[0] == "*" {
		w("(allow network*)")
	} else if len(cfg.AllowedDomains) > 0 {
		// F3: Specific domain filtering not yet implemented (Phase 2).
		// For safety, deny network rather than silently allowing all.
		// Only localhost (already allowed above) is permitted.
		w(";; Specific domain filtering not yet implemented (Phase 2).")
		w(";; Network denied rather than silently upgrading to allow-all.")
	}
	// Empty AllowedDomains → no extra network rule; default deny applies.

	return sb.String()
}
