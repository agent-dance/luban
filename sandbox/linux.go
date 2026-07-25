//go:build linux

package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func init() {
	platformBackends = append(platformBackends, BwrapBackend{})
}

// BwrapBackend uses bubblewrap (bwrap) for sandboxing on Linux.
type BwrapBackend struct {
	authority    *executableAuthority
	authorityOps *executableAuthorityOps
}

var defaultBwrapAuthority executableAuthority

func (BwrapBackend) Name() string { return "bwrap" }

func (b BwrapBackend) authorityRef() *executableAuthority {
	if b.authority != nil {
		return b.authority
	}
	return &defaultBwrapAuthority
}

func (b BwrapBackend) configureAuthority() *executableAuthority {
	ops := executableAuthorityOps{
		resolve: func() (string, error) { return exec.LookPath("bwrap") },
		probe:   defaultExecutableProbe,
		launch: func(ctx context.Context, executable trustedExecutable, args ...string) *exec.Cmd {
			return launchPinnedExecutable(ctx, executable, "/proc/self/fd/3", args...)
		},
	}
	if b.authorityOps != nil {
		ops = *b.authorityOps
	}
	authority := b.authorityRef()
	authority.configure(b.Name(), ops)
	return authority
}

func launchPinnedExecutable(ctx context.Context, executable trustedExecutable, descriptorPath string, args ...string) *exec.Cmd {
	launchPath := executable.path
	if executable.file != nil {
		launchPath = descriptorPath
	}
	cmd := exec.CommandContext(ctx, launchPath, args...)
	if executable.file != nil {
		// ExtraFiles[0] is fd 3 in the child. Execute the file opened during
		// trusted preparation instead of resolving the mutable path again.
		cmd.ExtraFiles = []*os.File{executable.file}
		cmd.Args[0] = executable.path
	}
	return cmd
}

func (b BwrapBackend) Available() bool {
	return b.configureAuthority().available()
}

func (b BwrapBackend) SandboxCapability() (Capability, bool) {
	return b.configureAuthority().snapshot()
}

func (b BwrapBackend) Command(ctx context.Context, cfg Config, name string, args ...string) (*exec.Cmd, error) {
	authority := b.configureAuthority()

	// F2: validate all user-supplied paths before building bwrap args.
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

	// F5: resolve symlinks on user-supplied paths to prevent symlink escape.
	resolvedRO := make([]string, len(cfg.ReadOnlyPaths))
	copy(resolvedRO, cfg.ReadOnlyPaths)
	for i, p := range resolvedRO {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			resolvedRO[i] = resolved
		}
	}

	resolvedRW := make([]string, len(cfg.ReadWritePaths))
	copy(resolvedRW, cfg.ReadWritePaths)
	for i, p := range resolvedRW {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			resolvedRW[i] = resolved
		}
	}
	allowHostFilesystem := false
	scopedRW := make([]string, 0, len(resolvedRW))
	for _, p := range resolvedRW {
		if filepath.Clean(p) == string(filepath.Separator) {
			allowHostFilesystem = true
			continue
		}
		scopedRW = append(scopedRW, p)
	}

	var bwrapArgs []string

	if allowHostFilesystem {
		// Bind the host root first so later virtual filesystem mounts are not
		// obscured. Normal OS ownership and mode bits still apply.
		bwrapArgs = append(bwrapArgs, "--bind", "/", "/")
	} else {
		// Always-present read-only system paths.
		systemROPaths := []string{"/usr", "/bin", "/lib", "/etc"}
		for _, p := range systemROPaths {
			if _, err := os.Stat(p); err == nil {
				bwrapArgs = append(bwrapArgs, "--ro-bind", p, p)
			}
		}
		// Optional system paths.
		optionalROPaths := []string{"/lib64", "/opt"}
		for _, p := range optionalROPaths {
			if _, err := os.Stat(p); err == nil {
				bwrapArgs = append(bwrapArgs, "--ro-bind", p, p)
			}
		}
	}

	// Virtual filesystems.
	bwrapArgs = append(bwrapArgs, "--proc", "/proc")
	bwrapArgs = append(bwrapArgs, "--dev", "/dev")
	if !allowHostFilesystem {
		bwrapArgs = append(bwrapArgs, "--tmpfs", "/tmp")
	}

	// User-specified read-only paths (symlink-resolved).
	for _, p := range resolvedRO {
		bwrapArgs = append(bwrapArgs, "--ro-bind", p, p)
	}

	// User-specified read-write paths (symlink-resolved).
	for _, p := range scopedRW {
		bwrapArgs = append(bwrapArgs, "--bind", p, p)
	}

	// Network: unshare unless AllowedDomains == ["*"].
	// F3: specific domain entries (not "*") also use --unshare-net; domain
	// filtering is Phase 2. Never silently upgrade to allow-all.
	if !allowAll(cfg.AllowedDomains) {
		bwrapArgs = append(bwrapArgs, "--unshare-net")
	}

	// Working directory.
	if cfg.WorkDir != "" {
		bwrapArgs = append(bwrapArgs, "--chdir", cfg.WorkDir)
	}

	bwrapArgs = append(bwrapArgs, "--die-with-parent")
	// F10: --new-session prevents the sandboxed process from accessing the
	// parent terminal via TIOCSTI and similar ioctl attacks.
	bwrapArgs = append(bwrapArgs, "--new-session")
	bwrapArgs = append(bwrapArgs, "--")
	bwrapArgs = append(bwrapArgs, name)
	bwrapArgs = append(bwrapArgs, args...)

	cmd, err := authority.command(ctx, bwrapArgs...)
	if err != nil {
		return nil, fmt.Errorf("sandbox: bwrap authority unavailable: %w", err)
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

// allowAll returns true if domains is exactly ["*"].
func allowAll(domains []string) bool {
	return len(domains) == 1 && domains[0] == "*"
}
