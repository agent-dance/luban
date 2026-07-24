//go:build darwin || linux

package tools

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/agent-dance/luban/i18n"
	"golang.org/x/sys/unix"
)

const (
	privateFileHistoryDirMode  os.FileMode = 0o700
	privateFileHistoryFileMode os.FileMode = 0o600
)

// privateFileHistoryDir pins the history directory by descriptor. Every data,
// lock, and temporary-file operation is relative to this descriptor, so a
// concurrent rename of the pathname cannot redirect an open or chmod to an
// attacker-selected symlink target.
type privateFileHistoryDir struct {
	file *os.File
	path string
}

type privateFileHistoryIdentity struct {
	device uint64
	inode  uint64
}

func appendPrivateFileHistory(root, name string, line []byte) error {
	dir, err := openPrivateFileHistoryDir(root)
	if err != nil {
		return err
	}
	defer dir.file.Close()

	unlock, err := dir.lock(name + ".lock")
	if err != nil {
		return err
	}
	defer unlock()

	existing, identity, found, err := dir.read(name)
	if err != nil {
		return err
	}
	// Encoder-produced legacy journals always end at a record boundary. A
	// partial tail means the old writer crashed mid-record; appending to it
	// would make the new record undecodable and falsely imply a committed edit.
	if found && !validPrivateFileHistoryJSONL(existing) {
		return &os.PathError{Op: "append", Path: filepath.Join(root, name), Err: fs.ErrInvalid}
	}

	payload := make([]byte, 0, len(existing)+len(line))
	payload = append(payload, existing...)
	payload = append(payload, line...)
	return dir.replace(name, payload, identity, found)
}

func readPrivateFileHistory(root, name string) ([]byte, bool, error) {
	dir, err := openPrivateFileHistoryDir(root)
	if err != nil {
		return nil, false, err
	}
	defer dir.file.Close()

	unlock, err := dir.lock(name + ".lock")
	if err != nil {
		return nil, false, err
	}
	defer unlock()

	data, _, found, err := dir.read(name)
	return data, found, err
}

func openPrivateFileHistoryDir(root string) (*privateFileHistoryDir, error) {
	if invalidPrivateFileHistoryRoot(root) {
		return nil, &os.PathError{Op: "open", Path: root, Err: fs.ErrInvalid}
	}
	abs, err := filepath.Abs(root)
	if err != nil || privateRuntimeVolumeRoot(abs) {
		return nil, &os.PathError{Op: "open", Path: root, Err: fs.ErrInvalid}
	}

	parent, err := openOrCreatePrivateFileHistoryParent(filepath.Dir(abs))
	if err != nil {
		return nil, err
	}
	defer parent.Close()

	leaf := filepath.Base(abs)
	if err := unix.Mkdirat(int(parent.Fd()), leaf, uint32(privateFileHistoryDirMode.Perm())); err != nil && !errors.Is(err, unix.EEXIST) {
		return nil, &os.PathError{Op: "mkdir", Path: abs, Err: err}
	}
	dir, err := openPrivateFileHistoryDirectoryAt(parent, leaf, abs)
	if err != nil {
		return nil, err
	}
	if err := dir.Chmod(privateFileHistoryDirMode); err != nil {
		_ = dir.Close()
		return nil, err
	}
	if err := dir.Sync(); err != nil && !errors.Is(err, fs.ErrInvalid) {
		_ = dir.Close()
		return nil, err
	}
	return &privateFileHistoryDir{file: dir, path: abs}, nil
}

// openOrCreatePrivateFileHistoryParent creates only missing components, with
// private permissions. Existing parent directories are never chmodded because
// the workspace/config directory can contain unrelated user-owned data. The
// final existing component is opened with O_NOFOLLOW, so a symlinked config
// directory is rejected instead of followed.
func openOrCreatePrivateFileHistoryParent(path string) (*os.File, error) {
	path = filepath.Clean(path)
	missing := make([]string, 0, 4)
	anchor := path
	for {
		info, err := os.Lstat(anchor)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return nil, &os.PathError{Op: "open", Path: anchor, Err: fs.ErrInvalid}
			}
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		base := filepath.Base(anchor)
		if base == "." || base == string(os.PathSeparator) || base == "" {
			return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
		}
		missing = append(missing, base)
		next := filepath.Dir(anchor)
		if next == anchor {
			return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
		}
		anchor = next
	}

	current, err := openPrivateFileHistoryDirectoryPath(anchor)
	if err != nil {
		return nil, err
	}
	for i := len(missing) - 1; i >= 0; i-- {
		component := missing[i]
		display := filepath.Join(anchor, filepath.Join(reverseStrings(missing[i:])...))
		if err := unix.Mkdirat(int(current.Fd()), component, uint32(privateFileHistoryDirMode.Perm())); err != nil && !errors.Is(err, unix.EEXIST) {
			_ = current.Close()
			return nil, &os.PathError{Op: "mkdir", Path: display, Err: err}
		}
		next, err := openPrivateFileHistoryDirectoryAt(current, component, display)
		if err != nil {
			_ = current.Close()
			return nil, err
		}
		_ = current.Close()
		current = next
	}
	return current, nil
}

func reverseStrings(values []string) []string {
	result := make([]string, len(values))
	for i := range values {
		result[i] = values[len(values)-1-i]
	}
	return result
}

func openPrivateFileHistoryDirectoryPath(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_DIRECTORY, 0)
	if err != nil {
		return nil, historyPathError("open", path, err)
	}
	f := os.NewFile(uintptr(fd), path)
	if err := validatePrivateFileHistoryDirectory(f, path); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func openPrivateFileHistoryDirectoryAt(parent *os.File, name, display string) (*os.File, error) {
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_DIRECTORY, 0)
	if err != nil {
		return nil, historyPathError("open", display, err)
	}
	f := os.NewFile(uintptr(fd), display)
	if err := validatePrivateFileHistoryDirectory(f, display); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func validatePrivateFileHistoryDirectory(f *os.File, path string) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	return nil
}

func invalidPrivateFileHistoryRoot(path string) bool {
	if strings.TrimSpace(path) == "" || strings.IndexByte(path, 0) >= 0 {
		return true
	}
	clean := filepath.Clean(path)
	if clean == "." || clean != path {
		return true
	}
	volume := filepath.VolumeName(path)
	remainder := strings.TrimPrefix(path, volume)
	for _, component := range strings.Split(remainder, string(os.PathSeparator)) {
		if component == ".." {
			return true
		}
	}
	return false
}

func validPrivateFileHistoryJSONL(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if data[len(data)-1] != '\n' {
		return false
	}
	for _, line := range strings.Split(string(data[:len(data)-1]), "\n") {
		if len(line) == 0 || !json.Valid([]byte(line)) {
			return false
		}
	}
	return true
}

func validPrivateFileHistoryName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsRune(name, os.PathSeparator)
}

func (d *privateFileHistoryDir) openRegular(name string, flag int, create bool) (*os.File, *privateFileHistoryIdentity, bool, error) {
	if !validPrivateFileHistoryName(name) {
		return nil, nil, false, &os.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	for range 32 {
		fd, err := unix.Openat(int(d.file.Fd()), name, flag|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		created := false
		if errors.Is(err, unix.ENOENT) && create {
			fd, err = unix.Openat(int(d.file.Fd()), name, flag|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CREAT|unix.O_EXCL, uint32(privateFileHistoryFileMode.Perm()))
			created = err == nil
			if errors.Is(err, unix.EEXIST) {
				continue
			}
		}
		if err != nil {
			if errors.Is(err, unix.ENOENT) {
				return nil, nil, false, nil
			}
			return nil, nil, false, historyPathError("open", filepath.Join(d.path, name), err)
		}
		f := os.NewFile(uintptr(fd), filepath.Join(d.path, name))
		identity, err := validateAndTightenPrivateFileHistoryRegular(f, filepath.Join(d.path, name))
		if err != nil {
			_ = f.Close()
			return nil, nil, false, err
		}
		if created {
			if err := f.Sync(); err != nil {
				_ = f.Close()
				return nil, nil, false, err
			}
			if err := d.file.Sync(); err != nil && !errors.Is(err, fs.ErrInvalid) {
				_ = f.Close()
				return nil, nil, false, err
			}
		}
		return f, identity, true, nil
	}
	return nil, nil, false, &os.PathError{Op: "open", Path: filepath.Join(d.path, name), Err: fs.ErrInvalid}
}

func validateAndTightenPrivateFileHistoryRegular(f *os.File, path string) (*privateFileHistoryIdentity, error) {
	before, err := f.Stat()
	if err != nil {
		return nil, err
	}
	identity, err := privateFileHistoryRegularIdentity(path, before)
	if err != nil {
		return nil, err
	}
	if err := f.Chmod(privateFileHistoryFileMode); err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	afterIdentity, err := privateFileHistoryRegularIdentity(path, after)
	if err != nil {
		return nil, err
	}
	if *identity != *afterIdentity || !os.SameFile(before, after) {
		return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	return identity, nil
}

func privateFileHistoryRegularIdentity(path string, info fs.FileInfo) (*privateFileHistoryIdentity, error) {
	if info == nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil || stat.Nlink != 1 {
		return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	return &privateFileHistoryIdentity{device: uint64(stat.Dev), inode: uint64(stat.Ino)}, nil
}

func (d *privateFileHistoryDir) lock(name string) (func(), error) {
	f, identity, _, err := d.openRegular(name, unix.O_RDWR, true)
	if err != nil {
		return nil, err
	}
	const retryInterval = 25 * time.Millisecond
	deadline := time.Now().Add(30 * time.Second)
	for {
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			_ = f.Close()
			return nil, err
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, i18n.NewError(i18n.KeyToolFileLockTimedOut, filepath.Join(d.path, name))
		}
		time.Sleep(retryInterval)
	}

	// The pathname must still identify the inode that was locked. Otherwise a
	// replacement lock entry could split cooperating writers across two locks.
	check, checkIdentity, found, checkErr := d.openRegular(name, unix.O_RDWR, false)
	if checkErr != nil || !found || checkIdentity == nil || *identity != *checkIdentity {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
		if check != nil {
			_ = check.Close()
		}
		if checkErr != nil {
			return nil, checkErr
		}
		return nil, &os.PathError{Op: "lock", Path: filepath.Join(d.path, name), Err: fs.ErrInvalid}
	}
	_ = check.Close()
	return func() {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
	}, nil
}

func (d *privateFileHistoryDir) read(name string) ([]byte, *privateFileHistoryIdentity, bool, error) {
	f, identity, found, err := d.openRegular(name, unix.O_RDONLY, false)
	if err != nil || !found {
		return nil, nil, found, err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, nil, false, err
	}
	after, err := validateAndTightenPrivateFileHistoryRegular(f, filepath.Join(d.path, name))
	if err != nil || *identity != *after {
		if err != nil {
			return nil, nil, false, err
		}
		return nil, nil, false, &os.PathError{Op: "read", Path: filepath.Join(d.path, name), Err: fs.ErrInvalid}
	}
	return data, identity, true, nil
}

func (d *privateFileHistoryDir) replace(name string, payload []byte, expected *privateFileHistoryIdentity, existed bool) error {
	tmp, tmpName, err := d.createTemporary()
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = unix.Unlinkat(int(d.file.Fd()), tmpName, 0)
	}()
	if err := writeAll(tmp, payload); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if _, err := validateAndTightenPrivateFileHistoryRegular(tmp, filepath.Join(d.path, tmpName)); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return err
	}
	closed = true

	if err := d.validateExpectedTarget(name, expected, existed); err != nil {
		return err
	}
	if err := unix.Renameat(int(d.file.Fd()), tmpName, int(d.file.Fd()), name); err != nil {
		return historyPathError("rename", filepath.Join(d.path, name), err)
	}
	if err := d.file.Sync(); err != nil && !errors.Is(err, fs.ErrInvalid) {
		return err
	}
	return nil
}

func (d *privateFileHistoryDir) validateExpectedTarget(name string, expected *privateFileHistoryIdentity, existed bool) error {
	f, identity, found, err := d.openRegular(name, unix.O_RDONLY, false)
	if f != nil {
		defer f.Close()
	}
	if err != nil {
		return err
	}
	if found != existed || found && (expected == nil || identity == nil || *expected != *identity) {
		return &os.PathError{Op: "replace", Path: filepath.Join(d.path, name), Err: fs.ErrInvalid}
	}
	return nil
}

func (d *privateFileHistoryDir) createTemporary() (*os.File, string, error) {
	for range 32 {
		var entropy [16]byte
		if _, err := rand.Read(entropy[:]); err != nil {
			return nil, "", err
		}
		name := ".file-history-" + hex.EncodeToString(entropy[:]) + ".tmp"
		fd, err := unix.Openat(int(d.file.Fd()), name, unix.O_WRONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CREAT|unix.O_EXCL, uint32(privateFileHistoryFileMode.Perm()))
		if errors.Is(err, unix.EEXIST) {
			continue
		}
		if err != nil {
			return nil, "", historyPathError("create", filepath.Join(d.path, name), err)
		}
		f := os.NewFile(uintptr(fd), filepath.Join(d.path, name))
		if _, err := validateAndTightenPrivateFileHistoryRegular(f, filepath.Join(d.path, name)); err != nil {
			_ = f.Close()
			_ = unix.Unlinkat(int(d.file.Fd()), name, 0)
			return nil, "", err
		}
		return f, name, nil
	}
	return nil, "", &os.PathError{Op: "create", Path: d.path, Err: fs.ErrExist}
}

func historyPathError(op, path string, err error) error {
	if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) || errors.Is(err, unix.EISDIR) || errors.Is(err, unix.ENXIO) {
		err = fs.ErrInvalid
	}
	return &os.PathError{Op: op, Path: path, Err: err}
}
