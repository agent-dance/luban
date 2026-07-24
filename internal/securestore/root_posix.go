//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

// Package securestore provides directory-descriptor-relative filesystem
// operations for private runtime state. A Root pins the directory identity for
// its lifetime; every descendant component is opened with O_NOFOLLOW.
package securestore

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Root is a pinned directory descriptor. Operations never resolve through the
// pathname used to construct it, except Validate, which only compares identity.
type Root struct {
	path string
	file *os.File
}

// Open opens path component-by-component without following symbolic links.
// When create is true, missing components are created with mode 0700.
func Open(path string, create bool) (*Root, error) {
	return open(path, create, true)
}

// OpenUnowned pins an existing caller-owned directory without changing its
// mode. Descendants created through the returned root are still private.
func OpenUnowned(path string) (*Root, error) {
	return open(path, false, false)
}

func open(path string, create, tighten bool) (*Root, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	abs, err = canonicalRootPath(filepath.Clean(abs))
	if err != nil {
		return nil, err
	}
	if abs == string(os.PathSeparator) || strings.IndexByte(abs, 0) >= 0 {
		return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	fd, err := unix.Open(string(os.PathSeparator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	components := strings.Split(strings.TrimPrefix(abs, string(os.PathSeparator)), string(os.PathSeparator))
	for index, component := range components {
		if component == "" || component == "." || component == ".." {
			_ = unix.Close(fd)
			return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
		}
		next, openErr := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if openErr != nil && create && errors.Is(openErr, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(fd, component, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				_ = unix.Close(fd)
				return nil, &os.PathError{Op: "mkdir", Path: path, Err: mkdirErr}
			}
			next, openErr = unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		}
		_ = unix.Close(fd)
		if openErr != nil {
			return nil, securePathError("open", path, openErr)
		}
		fd = next
		if tighten && index == len(components)-1 {
			if chmodErr := unix.Fchmod(fd, 0o700); chmodErr != nil {
				_ = unix.Close(fd)
				return nil, &os.PathError{Op: "chmod", Path: path, Err: chmodErr}
			}
		}
	}
	file := os.NewFile(uintptr(fd), abs)
	if file == nil {
		_ = unix.Close(fd)
		return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	return &Root{path: abs, file: file}, nil
}

// canonicalRootPath resolves only ancestors that already exist. The managed
// final component is deliberately not resolved, so a store root that is itself
// a symbolic link remains invalid. This accommodates platform aliases such as
// macOS /var -> /private/var without making a mutable managed component an
// authority.
func canonicalRootPath(abs string) (string, error) {
	components := []string{filepath.Base(abs)}
	ancestor := filepath.Dir(abs)
	for {
		info, err := os.Lstat(ancestor)
		if err == nil {
			if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
				return "", &os.PathError{Op: "open", Path: ancestor, Err: fs.ErrInvalid}
			}
			resolved, err := filepath.EvalSymlinks(ancestor)
			if err != nil {
				return "", err
			}
			for index := len(components) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, components[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if ancestor == string(os.PathSeparator) {
			return "", err
		}
		components = append(components, filepath.Base(ancestor))
		ancestor = filepath.Dir(ancestor)
	}
}

// Path returns the display pathname associated with the pinned root. It must
// not be used as an authority for filesystem access.
func (r *Root) Path(name string) string {
	if name == "." || name == "" {
		return r.path
	}
	return filepath.Join(r.path, filepath.FromSlash(name))
}

func (r *Root) Close() error {
	if r == nil || r.file == nil {
		return nil
	}
	return r.file.Close()
}

func (r *Root) Info() (fs.FileInfo, error) {
	if r == nil || r.file == nil {
		return nil, fs.ErrInvalid
	}
	return r.file.Stat()
}

// Validate securely resolves the original pathname and compares it with the
// pinned descriptor. It detects parent rename and symlink replacement without
// using the newly resolved descriptor for subsequent operations.
func (r *Root) Validate() error {
	if r == nil || r.file == nil {
		return fs.ErrInvalid
	}
	current, err := Open(r.path, false)
	if err != nil {
		return err
	}
	defer current.Close()
	heldInfo, err := r.file.Stat()
	if err != nil {
		return err
	}
	currentInfo, err := current.file.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(heldInfo, currentInfo) {
		return &os.PathError{Op: "open", Path: r.path, Err: fs.ErrInvalid}
	}
	return nil
}

// OpenRoot pins a descendant directory after a no-follow component walk.
func (r *Root) OpenRoot(name string, create bool) (*Root, error) {
	file, err := r.openDir(name, create)
	if err != nil {
		return nil, err
	}
	return &Root{path: r.Path(name), file: file}, nil
}

// MkdirAll creates and validates a private descendant directory tree.
func (r *Root) MkdirAll(name string) error {
	file, err := r.openDir(name, true)
	if err != nil {
		return err
	}
	return file.Close()
}

// OpenRootExclusive creates exactly one new descendant directory and pins it.
func (r *Root) OpenRootExclusive(name string) (*Root, error) {
	parent, base, err := r.openParent(name)
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	if err := unix.Mkdirat(int(parent.Fd()), base, 0o700); err != nil {
		return nil, &os.PathError{Op: "mkdir", Path: r.Path(name), Err: err}
	}
	child, err := r.OpenRoot(name, false)
	if err != nil {
		_ = unix.Unlinkat(int(parent.Fd()), base, unix.AT_REMOVEDIR)
		return nil, err
	}
	return child, nil
}

// OpenFile opens a descendant final component without following symbolic
// links. Intermediate directories are pinned one component at a time.
func (r *Root) OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	parent, base, err := r.openParent(name)
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	unixFlags := flag | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	fd, err := unix.Openat(int(parent.Fd()), base, unixFlags, uint32(perm.Perm()))
	if err != nil {
		return nil, securePathError("open", r.Path(name), err)
	}
	file := os.NewFile(uintptr(fd), r.Path(name))
	if file == nil {
		_ = unix.Close(fd)
		return nil, &os.PathError{Op: "open", Path: r.Path(name), Err: fs.ErrInvalid}
	}
	return file, nil
}

// Lstat returns information for a no-follow-opened descendant. Symbolic links
// and non-openable special files are rejected instead of being followed.
func (r *Root) Lstat(name string) (fs.FileInfo, error) {
	file, err := r.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return file.Stat()
}

func (r *Root) ReadDir(name string) ([]fs.DirEntry, error) {
	file, err := r.openDir(name, false)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return file.ReadDir(-1)
}

// CreateTemp creates a private temporary file in a pinned parent directory.
func (r *Root) CreateTemp(parent, pattern string) (*os.File, string, error) {
	dir, err := r.OpenRoot(parent, false)
	if err != nil {
		return nil, "", err
	}
	defer dir.Close()
	for range 128 {
		name, err := randomTempName(pattern)
		if err != nil {
			return nil, "", err
		}
		file, err := dir.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return file, filepath.ToSlash(filepath.Join(parent, name)), nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", &os.PathError{Op: "open", Path: r.Path(parent), Err: fs.ErrExist}
}

func (r *Root) Rename(oldname, newname string) error {
	oldParent, oldBase, err := r.openParent(oldname)
	if err != nil {
		return err
	}
	defer oldParent.Close()
	newParent, newBase, err := r.openParent(newname)
	if err != nil {
		return err
	}
	defer newParent.Close()
	if err := unix.Renameat(int(oldParent.Fd()), oldBase, int(newParent.Fd()), newBase); err != nil {
		return &os.PathError{Op: "rename", Path: r.Path(newname), Err: err}
	}
	return nil
}

func (r *Root) Link(oldname, newname string) error {
	oldParent, oldBase, err := r.openParent(oldname)
	if err != nil {
		return err
	}
	defer oldParent.Close()
	newParent, newBase, err := r.openParent(newname)
	if err != nil {
		return err
	}
	defer newParent.Close()
	if err := unix.Linkat(int(oldParent.Fd()), oldBase, int(newParent.Fd()), newBase, 0); err != nil {
		return &os.LinkError{Op: "link", Old: r.Path(oldname), New: r.Path(newname), Err: err}
	}
	return nil
}

func (r *Root) Remove(name string) error {
	parent, base, err := r.openParent(name)
	if err != nil {
		return err
	}
	defer parent.Close()
	if err := unix.Unlinkat(int(parent.Fd()), base, 0); err != nil {
		return &os.PathError{Op: "remove", Path: r.Path(name), Err: err}
	}
	return nil
}

func (r *Root) RemoveAll(name string) error {
	clean, err := cleanRelative(name, false)
	if err != nil {
		return err
	}
	parent, base, err := r.openParent(clean)
	if err != nil {
		return err
	}
	defer parent.Close()
	return removeAllAt(int(parent.Fd()), base, r.Path(clean))
}

func (r *Root) Sync(name string) error {
	dir, err := r.openDir(name, false)
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func (r *Root) openDir(name string, create bool) (*os.File, error) {
	clean, err := cleanRelative(name, true)
	if err != nil {
		return nil, err
	}
	// Dup would share a directory stream offset with the pinned descriptor.
	// Reopening "." through openat creates an independent open file
	// description, so an earlier ReadDir cannot make later List calls appear
	// permanently empty.
	fd, err := unix.Openat(int(r.file.Fd()), ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	if clean == "." {
		return os.NewFile(uintptr(fd), r.path), nil
	}
	components := strings.Split(clean, string(os.PathSeparator))
	for _, component := range components {
		next, openErr := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if openErr != nil && create && errors.Is(openErr, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(fd, component, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				_ = unix.Close(fd)
				return nil, &os.PathError{Op: "mkdir", Path: r.Path(name), Err: mkdirErr}
			}
			next, openErr = unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		}
		_ = unix.Close(fd)
		if openErr != nil {
			return nil, securePathError("open", r.Path(name), openErr)
		}
		fd = next
		if chmodErr := unix.Fchmod(fd, 0o700); chmodErr != nil {
			_ = unix.Close(fd)
			return nil, &os.PathError{Op: "chmod", Path: r.Path(name), Err: chmodErr}
		}
	}
	return os.NewFile(uintptr(fd), r.Path(name)), nil
}

func (r *Root) openParent(name string) (*os.File, string, error) {
	clean, err := cleanRelative(name, false)
	if err != nil {
		return nil, "", err
	}
	base := filepath.Base(clean)
	parentName := filepath.Dir(clean)
	parent, err := r.openDir(parentName, false)
	if err != nil {
		return nil, "", err
	}
	return parent, base, nil
}

func cleanRelative(name string, allowDot bool) (string, error) {
	if name == "" || strings.IndexByte(name, 0) >= 0 || filepath.IsAbs(name) {
		return "", fs.ErrInvalid
	}
	clean := filepath.Clean(name)
	if clean == "." && allowDot {
		return clean, nil
	}
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fs.ErrInvalid
	}
	return clean, nil
}

func randomTempName(pattern string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	random := hex.EncodeToString(raw[:])
	if index := strings.LastIndexByte(pattern, '*'); index >= 0 {
		return pattern[:index] + random + pattern[index+1:], nil
	}
	return pattern + random, nil
}

func securePathError(op, path string, err error) error {
	if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) {
		err = fs.ErrInvalid
	}
	return &os.PathError{Op: op, Path: path, Err: err}
}

func removeAllAt(parentFD int, name, display string) error {
	fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		if errors.Is(err, unix.ENOTDIR) || errors.Is(err, unix.ELOOP) {
			if unlinkErr := unix.Unlinkat(parentFD, name, 0); unlinkErr != nil && !errors.Is(unlinkErr, unix.ENOENT) {
				return &os.PathError{Op: "remove", Path: display, Err: unlinkErr}
			}
			return nil
		}
		return securePathError("open", display, err)
	}
	dir := os.NewFile(uintptr(fd), display)
	entries, readErr := dir.ReadDir(-1)
	if readErr != nil {
		_ = dir.Close()
		return readErr
	}
	for _, entry := range entries {
		if err := removeAllAt(fd, entry.Name(), filepath.Join(display, entry.Name())); err != nil {
			_ = dir.Close()
			return err
		}
	}
	if err := dir.Close(); err != nil {
		return err
	}
	if err := unix.Unlinkat(parentFD, name, unix.AT_REMOVEDIR); err != nil && !errors.Is(err, unix.ENOENT) {
		return &os.PathError{Op: "remove", Path: display, Err: err}
	}
	return nil
}
