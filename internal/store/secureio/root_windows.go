//go:build windows

package secureio

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Root is the Windows compatibility implementation. os.Root confines paths;
// native reparse-point and parent-identity hardening remains a documented
// platform residual.
type Root struct {
	path string
	root *os.Root
}

func Open(path string, create bool) (*Root, error) {
	return open(path, create)
}

func OpenUnowned(path string) (*Root, error) {
	return open(path, false)
}

func open(path string, create bool) (*Root, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	abs = filepath.Clean(abs)
	if abs == filepath.VolumeName(abs)+string(os.PathSeparator) || strings.IndexByte(abs, 0) >= 0 {
		return nil, fs.ErrInvalid
	}
	root, err := os.OpenRoot(abs)
	if errors.Is(err, fs.ErrNotExist) && create {
		if err := os.MkdirAll(abs, 0o700); err != nil {
			return nil, err
		}
		root, err = os.OpenRoot(abs)
	}
	if err != nil {
		return nil, err
	}
	return &Root{path: abs, root: root}, nil
}

func (r *Root) Path(name string) string {
	if name == "." || name == "" {
		return r.path
	}
	return filepath.Join(r.path, name)
}

func (r *Root) Close() error { return r.root.Close() }

func (r *Root) Info() (fs.FileInfo, error) {
	file, err := r.root.Open(".")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return file.Stat()
}

func (r *Root) Validate() error {
	current, err := Open(r.path, false)
	if err != nil {
		return err
	}
	return current.Close()
}

func (r *Root) OpenRoot(name string, create bool) (*Root, error) {
	if create {
		if err := r.root.MkdirAll(name, 0o700); err != nil {
			return nil, err
		}
	}
	child, err := r.root.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	return &Root{path: r.Path(name), root: child}, nil
}

func (r *Root) MkdirAll(name string) error { return r.root.MkdirAll(name, 0o700) }

func (r *Root) OpenRootExclusive(name string) (*Root, error) {
	if err := r.root.Mkdir(name, 0o700); err != nil {
		return nil, err
	}
	child, err := r.OpenRoot(name, false)
	if err != nil {
		_ = r.root.Remove(name)
		return nil, err
	}
	return child, nil
}

func (r *Root) OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	return r.root.OpenFile(name, flag, perm)
}

func (r *Root) Lstat(name string) (fs.FileInfo, error) { return r.root.Lstat(name) }
func (r *Root) ReadDir(name string) ([]fs.DirEntry, error) {
	f, err := r.root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.ReadDir(-1)
}

func (r *Root) CreateTemp(parent, pattern string) (*os.File, string, error) {
	for range 128 {
		name, err := randomTempName(pattern)
		if err != nil {
			return nil, "", err
		}
		rel := filepath.Join(parent, name)
		file, err := r.root.OpenFile(rel, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return file, rel, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", fs.ErrExist
}

func (r *Root) Rename(oldname, newname string) error { return r.root.Rename(oldname, newname) }
func (r *Root) Link(oldname, newname string) error   { return r.root.Link(oldname, newname) }
func (r *Root) Remove(name string) error             { return r.root.Remove(name) }
func (r *Root) RemoveAll(name string) error          { return r.root.RemoveAll(name) }
func (r *Root) Sync(string) error                    { return nil }

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
