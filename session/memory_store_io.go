package session

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

func openMemoryStoreRoot(path string, held *os.File) (*os.Root, error) {
	if runtime.GOOS == "js" || runtime.GOOS == "plan9" || runtime.GOOS == "wasip1" {
		return nil, memoryStoreInvalidPath("open", path)
	}
	heldInfo, err := held.Stat()
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	rootInfo, err := root.Stat(".")
	if err != nil || !os.SameFile(heldInfo, rootInfo) {
		_ = root.Close()
		if err != nil {
			return nil, err
		}
		return nil, memoryStoreInvalidPath("open", path)
	}
	return root, nil
}

func readMemoryStoreFile(root *os.Root, path string) ([]byte, error) {
	name := filepath.Base(path)
	f, err := openMemoryStoreFileInRoot(root, name, path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	identity, err := f.Stat()
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	after, err := validateAndTightenPrivateRegularFile(f, path)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(identity, after) {
		return nil, memoryStoreInvalidPath("read", path)
	}
	if err := validateMemoryRegularFileLinkCount(f, path); err != nil {
		return nil, err
	}
	current, err := openMemoryStoreFileInRoot(root, name, path)
	if err != nil {
		return nil, err
	}
	defer current.Close()
	currentInfo, err := current.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(after, currentInfo) {
		return nil, memoryStoreInvalidPath("read", path)
	}
	return data, nil
}

func writeMemoryStoreFileAtomic(root *os.Root, dir *os.File, path string, data []byte) error {
	name := filepath.Base(path)
	before, beforeExists, err := statMemoryStoreFileInRoot(root, name, path)
	if err != nil {
		return err
	}

	tmp, tmpName, err := createMemoryStoreTempInRoot(root, path)
	if err != nil {
		return err
	}
	tmpOpen := true
	defer func() {
		if tmpOpen {
			_ = tmp.Close()
		}
		_ = root.Remove(tmpName)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if n, err := tmp.Write(data); err != nil {
		return err
	} else if n != len(data) {
		return io.ErrShortWrite
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	tmpInfo, err := validateAndTightenPrivateRegularFile(tmp, path)
	if err != nil {
		return err
	}
	if err := validateMemoryRegularFileLinkCount(tmp, path); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		tmpOpen = false
		return err
	}
	tmpOpen = false

	current, currentInfo, err := openAndStatMemoryStoreFileInRoot(root, name, path)
	if err != nil {
		return err
	}
	if current != nil {
		defer current.Close()
	}
	currentExists := currentInfo != nil
	if beforeExists != currentExists || beforeExists && !os.SameFile(before, currentInfo) {
		return memoryStoreInvalidPath("rename", path)
	}
	if err := root.Rename(tmpName, name); err != nil {
		return &os.PathError{Op: "rename", Path: path, Err: err}
	}

	published, err := openMemoryStoreFileInRoot(root, name, path)
	if err != nil {
		return err
	}
	defer published.Close()
	publishedInfo, err := published.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(tmpInfo, publishedInfo) {
		return memoryStoreInvalidPath("rename", path)
	}
	if err := dir.Sync(); err != nil && runtime.GOOS != "windows" {
		return err
	}
	return nil
}

func openAndStatMemoryStoreFileInRoot(root *os.Root, name, path string) (*os.File, fs.FileInfo, error) {
	f, err := openMemoryStoreFileInRoot(root, name, path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return f, info, nil
}

func statMemoryStoreFileInRoot(root *os.Root, name, path string) (fs.FileInfo, bool, error) {
	f, info, err := openAndStatMemoryStoreFileInRoot(root, name, path)
	if f != nil {
		defer f.Close()
	}
	return info, info != nil, err
}

func openMemoryStoreFileInRoot(root *os.Root, name, path string) (*os.File, error) {
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if err := validatePrivateRegularFileInfo(path, "open", before); err != nil {
		return nil, err
	}
	f, err := root.OpenFile(name, os.O_RDONLY|memoryStoreNonblockFlag(), 0)
	if err != nil {
		return nil, err
	}
	after, err := validateAndTightenPrivateRegularFile(f, path)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !os.SameFile(before, after) {
		_ = f.Close()
		return nil, memoryStoreInvalidPath("open", path)
	}
	if err := validateMemoryRegularFileLinkCount(f, path); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func createMemoryStoreTempInRoot(root *os.Root, path string) (*os.File, string, error) {
	for range 32 {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, "", err
		}
		name := ".memory-" + hex.EncodeToString(random[:])
		f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return f, name, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", memoryStoreInvalidPath("open", path)
}

func memoryStoreInvalidPath(op, path string) error {
	return &os.PathError{Op: op, Path: path, Err: fs.ErrInvalid}
}
