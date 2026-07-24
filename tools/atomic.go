package tools

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"

	"github.com/agent-dance/luban/i18n"
)

// atomicWriteFile writes data to a file atomically: write to a temp file in the
// same directory, fsync, then rename over the target. This prevents corruption
// on crash or disk-full during write.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, ".atomic-*")
	if err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicCreateTemporary, err)
	}
	tmpPath := tmp.Name()

	// Clean up temp file on any error path
	success := false
	defer func() {
		if !success {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()

	if err := writeAll(tmp, data); err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicWriteTemporary, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicChmodTemporary, err)
	}

	// Fsync to ensure data is on disk before rename
	if err := tmp.Sync(); err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicSyncTemporary, err)
	}

	if err := tmp.Close(); err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicCloseTemporary, err)
	}

	if err := replaceFileAtomically(tmpPath, path); err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicReplaceTarget, err)
	}

	success = true
	if err := syncRuntimeDirectory(dir); err != nil && !errors.Is(err, fs.ErrInvalid) {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicSyncTemporary, err)
	}
	return nil
}

// Private runtime stores contain transcripts, task results, and session
// recovery state. These helpers deliberately apply only to those stores; the
// generic atomic writer above also serves user-owned project files and must not
// silently change their permissions.
const (
	privateRuntimeDirectoryMode os.FileMode = 0o700
	privateRuntimeFileMode      os.FileMode = 0o600
)

func ensurePrivateRuntimeDirectory(path string) error {
	path = filepath.Clean(privateRuntimePathOrDot(path))
	if path == "." || privateRuntimeVolumeRoot(path) {
		return &os.PathError{Op: "mkdir", Path: path, Err: fs.ErrInvalid}
	}
	parent := filepath.Dir(path)
	parentInfo, parentErr := os.Lstat(parent)
	if errors.Is(parentErr, fs.ErrNotExist) {
		if parentErr = os.MkdirAll(parent, privateRuntimeDirectoryMode); parentErr == nil {
			parentInfo, parentErr = os.Lstat(parent)
		}
	}
	if parentErr != nil {
		return parentErr
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
		return &os.PathError{Op: "mkdir", Path: parent, Err: fs.ErrInvalid}
	}

	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		if err = os.Mkdir(path, privateRuntimeDirectoryMode); errors.Is(err, fs.ErrExist) {
			err = nil
		}
		if err == nil {
			info, err = os.Lstat(path)
		}
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return &os.PathError{Op: "mkdir", Path: path, Err: fs.ErrInvalid}
	}

	for range 32 {
		dir, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		opened, statErr := dir.Stat()
		if statErr == nil && opened.IsDir() && os.SameFile(info, opened) {
			chmodErr := dir.Chmod(privateRuntimeDirectoryMode)
			closeErr := dir.Close()
			if chmodErr != nil {
				return chmodErr
			}
			return closeErr
		}
		_ = dir.Close()
		info, err = os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return &os.PathError{Op: "mkdir", Path: path, Err: fs.ErrInvalid}
		}
	}
	return &os.PathError{Op: "mkdir", Path: path, Err: fs.ErrInvalid}
}

func validatePrivateRuntimeDirectory(path string) error {
	path = filepath.Clean(privateRuntimePathOrDot(path))
	if path == "." || privateRuntimeVolumeRoot(path) {
		return &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	for range 32 {
		before, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if before.Mode()&os.ModeSymlink != 0 || !before.IsDir() {
			return &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
		}
		dir, err := os.Open(path)
		if err != nil {
			return err
		}
		after, statErr := dir.Stat()
		closeErr := dir.Close()
		if statErr == nil && after.IsDir() && os.SameFile(before, after) {
			return closeErr
		}
	}
	return &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
}

func privateRuntimePathOrDot(path string) string {
	if path == "" {
		return "."
	}
	return path
}

func privateRuntimeVolumeRoot(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return true
	}
	volume := filepath.VolumeName(abs)
	return filepath.Clean(abs) == filepath.Clean(volume+string(os.PathSeparator))
}

func tightenPrivateRuntimeRegularFile(path string, missingOK bool) (fs.FileInfo, error) {
	for range 32 {
		before, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) && missingOK {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() || privateRuntimeFileHasMultipleLinks(before) {
			return nil, &os.PathError{Op: "lstat", Path: path, Err: fs.ErrInvalid}
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		after, statErr := f.Stat()
		if statErr == nil && after.Mode().IsRegular() && !privateRuntimeFileHasMultipleLinks(after) && os.SameFile(before, after) {
			chmodErr := f.Chmod(privateRuntimeFileMode)
			closeErr := f.Close()
			if chmodErr != nil {
				return nil, chmodErr
			}
			if closeErr != nil {
				return nil, closeErr
			}
			return after, nil
		}
		_ = f.Close()
	}
	return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
}

func openPrivateRuntimeRegularFile(path string) (*os.File, error) {
	for range 32 {
		before, err := tightenPrivateRuntimeRegularFile(path, false)
		if err != nil {
			return nil, err
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		after, err := f.Stat()
		if err == nil && after.Mode().IsRegular() && !privateRuntimeFileHasMultipleLinks(after) && os.SameFile(before, after) {
			return f, nil
		}
		_ = f.Close()
	}
	return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
}

func readPrivateRuntimeRegularFile(path string) ([]byte, error) {
	f, err := openPrivateRuntimeRegularFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func openPrivateRuntimeAppendFile(path string) (*os.File, error) {
	return openPrivateRuntimeAppendFileWithDirectoryPolicy(path, true)
}

func openPrivateRuntimeAppendFileWithoutDirectoryMutation(path string) (*os.File, error) {
	return openPrivateRuntimeAppendFileWithDirectoryPolicy(path, false)
}

func openPrivateRuntimeAppendFileWithDirectoryPolicy(path string, manageDirectory bool) (*os.File, error) {
	dir := filepath.Dir(path)
	var dirErr error
	if manageDirectory {
		dirErr = ensurePrivateRuntimeDirectory(dir)
	} else {
		dirErr = validatePrivateRuntimeDirectory(dir)
	}
	if dirErr != nil {
		return nil, dirErr
	}
	before, err := tightenPrivateRuntimeRegularFile(path, true)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, privateRuntimeFileMode)
	if err != nil {
		return nil, err
	}
	opened, statErr := f.Stat()
	pathInfo, pathErr := os.Lstat(path)
	valid := statErr == nil && pathErr == nil && opened.Mode().IsRegular() && !privateRuntimeFileHasMultipleLinks(opened) &&
		pathInfo.Mode()&os.ModeSymlink == 0 && pathInfo.Mode().IsRegular() && os.SameFile(opened, pathInfo)
	if valid && before != nil {
		valid = os.SameFile(before, opened)
	}
	if !valid {
		_ = f.Close()
		return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	if err := f.Chmod(privateRuntimeFileMode); err != nil {
		_ = f.Close()
		return nil, err
	}
	if before == nil {
		if err := f.Sync(); err != nil {
			_ = f.Close()
			return nil, err
		}
		if err := syncRuntimeDirectory(dir); err != nil {
			_ = f.Close()
			return nil, err
		}
	}
	return f, nil
}

func privateRuntimeFileHasMultipleLinks(info fs.FileInfo) bool {
	if info == nil || info.Sys() == nil {
		return false
	}
	value := reflect.ValueOf(info.Sys())
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return false
	}
	links := value.FieldByName("Nlink")
	if !links.IsValid() {
		return false
	}
	switch links.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return links.Uint() > 1
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return links.Int() > 1
	default:
		return false
	}
}

func atomicWritePrivateRuntimeFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := ensurePrivateRuntimeDirectory(dir); err != nil {
		return err
	}
	if _, err := tightenPrivateRuntimeRegularFile(path, true); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".private-runtime-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpPath)
	}()
	if err := tmp.Chmod(privateRuntimeFileMode); err != nil {
		return err
	}
	if err := writeAll(tmp, data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	if err := replaceFileAtomically(tmpPath, path); err != nil {
		return err
	}
	return syncRuntimeDirectory(dir)
}

func preparePrivateRuntimeLock(path string) error {
	if err := ensurePrivateRuntimeDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	_, err := tightenPrivateRuntimeRegularFile(path, true)
	return err
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func syncRuntimeDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
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
