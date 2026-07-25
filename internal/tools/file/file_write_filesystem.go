package file

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/tools/toolbase"
)

type fileWriteTarget struct {
	DisplayPath string
	TargetPath  string
	Info        os.FileInfo
	Exists      bool
	IsSymlink   bool
	Mode        os.FileMode
	Before      string
	Encoding    FileEncoding
	BOM         []byte
}

func inspectFileWriteTarget(absPath string) (fileWriteTarget, error) {
	target := fileWriteTarget{DisplayPath: absPath, TargetPath: absPath, Mode: 0o644}
	linkInfo, err := os.Lstat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return target, nil
		}
		return target, i18n.WrapError(i18n.KeyToolFileHelperStatFailed, err)
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 {
		resolved, resolveErr := filepath.EvalSymlinks(absPath)
		if resolveErr != nil {
			return target, i18n.WrapError(i18n.KeyToolFileHelperResolveSymlinkFailed, resolveErr, absPath)
		}
		target.TargetPath = resolved
		target.IsSymlink = true
	}
	info, err := os.Stat(target.TargetPath)
	if err != nil {
		return target, i18n.WrapError(i18n.KeyToolFileHelperStatFailed, err)
	}
	if info.IsDir() {
		return target, i18n.NewError(i18n.KeyToolFileHelperPathIsDirectory, absPath)
	}
	target.Exists = true
	target.Info = info
	target.Mode = info.Mode().Perm()
	if target.Mode == 0 {
		target.Mode = 0o644
	}
	raw, err := os.ReadFile(target.TargetPath)
	if err != nil {
		return target, i18n.WrapError(i18n.KeyToolFileHelperReadFailed, err)
	}
	target.Before, target.Encoding, target.BOM = decodeFileWriteBytes(raw)
	return target, nil
}

// decodeFileWriteBytes treats an explicit UTF-16LE BOM as UTF-16 and every
// other byte sequence as UTF-8. It normalizes CRLF for comparison and diff
// display while deliberately leaving bare CR untouched.
func decodeFileWriteBytes(raw []byte) (string, FileEncoding, []byte) {
	if bytes.HasPrefix(raw, bomUTF16LE) {
		content := decodeFileBytes(raw, EncodingDetectResult{Encoding: EncodingUTF16LE, BOM: bomUTF16LE})
		return strings.ReplaceAll(content, "\r\n", "\n"), EncodingUTF16LE, append([]byte(nil), bomUTF16LE...)
	}
	content := string(raw)
	return strings.ReplaceAll(content, "\r\n", "\n"), EncodingUTF8, nil
}

func encodeFileWriteBytes(content string, encoding FileEncoding, bom []byte) []byte {
	if encoding == EncodingUTF16LE {
		return encodeWriteBytes(content, EncodingUTF16LE, bom)
	}
	return []byte(content)
}

func (t *FileWriteTool) allowedDirs() []string {
	if t != nil && t.Runtime != nil {
		if dirs := t.runtimeSnapshot().AllowedDirs; dirs != nil {
			return append([]string(nil), dirs...)
		}
	}
	if t == nil {
		return nil
	}
	return append([]string(nil), t.AllowedDirs...)
}

func recheckFileWriteTarget(target fileWriteTarget, allowedDirs []string) error {
	if err := checkAllowedPath(target.DisplayPath, allowedDirs); err != nil {
		return i18n.WrapError(i18n.KeyToolFileHelperWriteTargetOutsideAllowed, err)
	}
	if !target.Exists {
		if _, err := os.Lstat(target.DisplayPath); err == nil {
			return i18n.NewError(i18n.KeyToolFileHelperCreatedAfterCheck)
		} else if !os.IsNotExist(err) {
			return i18n.WrapError(i18n.KeyToolFileHelperRecheckWriteTargetFailed, err)
		}
		return nil
	}
	if target.IsSymlink {
		resolved, err := filepath.EvalSymlinks(target.DisplayPath)
		if err != nil || toolbase.CanonicalPath(resolved) != toolbase.CanonicalPath(target.TargetPath) {
			return i18n.NewError(i18n.KeyToolFileHelperSymlinkTargetChanged)
		}
	}
	current, err := os.Stat(target.TargetPath)
	if err != nil {
		return i18n.WrapError(i18n.KeyToolFileHelperRecheckWriteTargetFailed, err)
	}
	if target.Info != nil && !os.SameFile(target.Info, current) {
		return i18n.NewError(i18n.KeyToolFileHelperWriteTargetReplaced)
	}
	return nil
}
