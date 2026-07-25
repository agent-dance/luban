//go:build linux || darwin

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

type trustedExecutable struct {
	path     string
	identity string
	file     *os.File
}

func (e trustedExecutable) close() {
	if e.file != nil {
		_ = e.file.Close()
	}
}

type executableAuthorityOps struct {
	resolve func() (string, error)
	probe   func(string) (trustedExecutable, error)
	launch  func(context.Context, trustedExecutable, ...string) *exec.Cmd
}

type executableAuthority struct {
	mu       sync.Mutex
	name     string
	ops      executableAuthorityOps
	prepared bool
	poisoned bool
	exec     trustedExecutable
	cap      Capability
}

// configure is called before the authority is published. Once preparation has
// succeeded, its resolver/probe/launch functions and capability never change.
func (a *executableAuthority) configure(name string, ops executableAuthorityOps) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.name != "" {
		return
	}
	a.name = name
	a.ops = ops
}

func (a *executableAuthority) available() bool {
	_, ok := a.snapshot()
	return ok
}

func (a *executableAuthority) snapshot() (Capability, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.validateLocked(); err != nil {
		return Capability{}, false
	}
	return a.cap, true
}

func (a *executableAuthority) command(ctx context.Context, args ...string) (*exec.Cmd, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.validateLocked(); err != nil {
		return nil, err
	}
	if a.ops.launch == nil {
		return nil, errors.New("sandbox: executable authority has no launcher")
	}
	return a.ops.launch(ctx, a.exec, args...), nil
}

func (a *executableAuthority) validateLocked() error {
	if a.poisoned {
		return errors.New("sandbox: executable authority is invalid")
	}
	if strings.TrimSpace(a.name) == "" || a.ops.resolve == nil || a.ops.probe == nil {
		a.poisoned = true
		return errors.New("sandbox: executable authority is not configured")
	}
	candidate, err := a.ops.resolve()
	if err != nil {
		if a.prepared {
			a.poisoned = true
		}
		return fmt.Errorf("sandbox: executable resolution failed: %w", err)
	}
	if !filepath.IsAbs(candidate) {
		if a.prepared {
			a.poisoned = true
		}
		return fmt.Errorf("sandbox: executable path is not absolute: %q", candidate)
	}
	current, err := a.ops.probe(candidate)
	if err != nil {
		if a.prepared {
			a.poisoned = true
		}
		return fmt.Errorf("sandbox: executable authority is not trusted: %w", err)
	}
	if !filepath.IsAbs(current.path) || strings.TrimSpace(current.identity) == "" {
		current.close()
		if a.prepared {
			a.poisoned = true
		}
		return errors.New("sandbox: executable probe returned an invalid identity")
	}
	current.path = filepath.Clean(current.path)
	if !a.prepared {
		a.exec = current
		a.cap = Capability{
			Backend:            a.name,
			ExecutablePath:     current.path,
			ExecutableIdentity: current.identity,
		}
		if a.cap.ID() == "" {
			a.exec.close()
			a.exec = trustedExecutable{}
			return errors.New("sandbox: executable capability is invalid")
		}
		a.prepared = true
		return nil
	}
	current.close()
	if current.path != a.exec.path || current.identity != a.exec.identity {
		a.poisoned = true
		return errors.New("sandbox: executable authority changed after preparation")
	}
	return nil
}

func defaultExecutableProbe(candidate string) (trustedExecutable, error) {
	if !filepath.IsAbs(candidate) {
		return trustedExecutable{}, fmt.Errorf("path is not absolute: %q", candidate)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return trustedExecutable{}, err
	}
	resolved = filepath.Clean(resolved)
	if !filepath.IsAbs(resolved) {
		return trustedExecutable{}, fmt.Errorf("resolved path is not absolute: %q", resolved)
	}
	if err := validateExecutableAuthorityPath(resolved); err != nil {
		return trustedExecutable{}, err
	}
	file, err := os.Open(resolved)
	if err != nil {
		return trustedExecutable{}, err
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = file.Close()
		}
	}()
	openedInfo, err := file.Stat()
	if err != nil {
		return trustedExecutable{}, err
	}
	pathInfo, err := os.Stat(resolved)
	if err != nil {
		return trustedExecutable{}, err
	}
	if !os.SameFile(openedInfo, pathInfo) {
		return trustedExecutable{}, errors.New("path changed while executable was opened")
	}
	if !openedInfo.Mode().IsRegular() || openedInfo.Mode().Perm()&0o111 == 0 {
		return trustedExecutable{}, errors.New("sandbox executable is not a regular executable file")
	}
	identity, owner, ok := executableIdentity(openedInfo)
	if !ok || identity == "" {
		return trustedExecutable{}, errors.New("sandbox executable identity is unavailable")
	}
	if owner != 0 || openedInfo.Mode().Perm()&0o022 != 0 {
		return trustedExecutable{}, errors.New("sandbox executable has writable non-system authority")
	}
	if unix.Access(resolved, unix.W_OK) == nil {
		return trustedExecutable{}, errors.New("sandbox executable is writable by the current process authority")
	}
	closeOnError = false
	return trustedExecutable{path: resolved, identity: identity, file: file}, nil
}

func validateExecutableAuthorityPath(path string) error {
	volume := filepath.VolumeName(path)
	root := volume + string(filepath.Separator)
	current := filepath.Clean(filepath.Dir(path))
	for {
		info, err := os.Stat(current)
		if err != nil {
			return err
		}
		owner, ok := executableOwner(info)
		if !ok || !info.IsDir() || owner != 0 || info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("sandbox executable directory is not system-controlled: %s", current)
		}
		if unix.Access(current, unix.W_OK) == nil {
			return fmt.Errorf("sandbox executable directory is writable by the current process authority: %s", current)
		}
		if current == root {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return nil
}
