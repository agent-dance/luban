package input

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
)

const clipboardTimeout = 5 * time.Second

type localizedClipboardError struct {
	text  string
	cause error
}

func (e *localizedClipboardError) Error() string { return e.text }
func (e *localizedClipboardError) Unwrap() error { return e.cause }

func clipboardError(key i18n.Key, args ...any) error {
	return &localizedClipboardError{text: i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)}
}

func clipboardCauseError(key i18n.Key, cause error, args ...any) error {
	return &localizedClipboardError{text: i18n.Format(i18n.DetectOrLoadLanguage(), key, args...), cause: cause}
}

// imageExtensions lists supported image file extensions for clipboard file references.
// Matches the TS IMAGE_EXTENSION_REGEX: /\.(png|jpe?g|gif|webp)$/i
var imageExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
}

// createTempClipboardFile creates a uniquely-named temporary file for clipboard
// image data. The caller is responsible for removing the file when done.
func createTempClipboardFile() (string, error) {
	f, err := os.CreateTemp("", "claude_clipboard_*.png")
	if err != nil {
		return "", err
	}
	f.Close()
	return f.Name(), nil
}

// HasClipboardImage checks if the system clipboard contains an image.
// Returns false (not true) on unsupported platforms or if no image is present.
func HasClipboardImage() bool {
	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()

	switch runtime.GOOS {
	case "darwin":
		// First try: clipboard contains raw PNG image data (e.g. screenshot, browser "Copy Image")
		cmd := exec.CommandContext(ctx, "osascript", "-e", "the clipboard as «class PNGf»")
		if cmd.Run() == nil {
			return true
		}
		// Second try: clipboard contains a file reference (e.g. Finder Cmd+C on an image file)
		return darwinHasClipboardImageFile(ctx)

	case "linux":
		// Try Wayland first, then X11
		if hasWaylandImage(ctx) {
			return true
		}
		return hasXClipImage(ctx)

	case "windows":
		cmd := exec.CommandContext(ctx,
			"powershell", "-NoProfile", "-Command",
			"(Get-Clipboard -Format Image) -ne $null",
		)
		out, err := cmd.Output()
		if err != nil {
			return false
		}
		return strings.TrimSpace(string(out)) == "True"

	default:
		return false
	}
}

func hasWaylandImage(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "wl-paste", "-l")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return containsImageMIME(string(out))
}

func hasXClipImage(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "xclip", "-selection", "clipboard", "-t", "TARGETS", "-o")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return containsImageMIME(string(out))
}

func containsImageMIME(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "image/png") ||
		strings.Contains(lower, "image/jpeg") ||
		strings.Contains(lower, "image/gif") ||
		strings.Contains(lower, "image/webp")
}

// GetClipboardImage reads an image from the system clipboard.
// Returns base64-encoded data, media type, and any error.
// Returns ("", "", nil) if no image is in the clipboard.
func GetClipboardImage() (base64Data, mediaType string, err error) {
	if !HasClipboardImage() {
		return "", "", nil
	}

	switch runtime.GOOS {
	case "darwin":
		return darwinGetClipboardImage()
	case "linux":
		return linuxGetClipboardImage()
	case "windows":
		return windowsGetClipboardImage()
	default:
		return "", "", clipboardError(i18n.KeyAuxClipboardUnsupported, runtime.GOOS)
	}
}

// darwinGetClipboardImage uses osascript to write clipboard PNG to a temp file,
// then reads and base64-encodes it. Falls back to file reference if PNG read fails.
func darwinGetClipboardImage() (string, string, error) {
	tmp, err := createTempClipboardFile()
	if err != nil {
		return "", "", clipboardCauseError(i18n.KeyAuxClipboardCreateTemp, err, err)
	}
	defer os.Remove(tmp)

	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()

	// Escape backslashes then double-quotes to prevent AppleScript injection.
	escapedTmp := strings.ReplaceAll(tmp, `\`, `\\`)
	escapedTmp = strings.ReplaceAll(escapedTmp, `"`, `\"`)
	cmd := exec.CommandContext(ctx, "osascript",
		"-e", "set png_data to (the clipboard as «class PNGf»)",
		"-e", fmt.Sprintf("set fp to open for access POSIX file \"%s\" with write permission", escapedTmp),
		"-e", "write png_data to fp",
		"-e", "close access fp",
	)
	if out, runErr := cmd.CombinedOutput(); runErr != nil {
		// PNG read failed — fall back to file reference (Finder copy)
		return darwinGetClipboardImageFromFile()
	} else {
		_ = out
	}

	data, err := os.ReadFile(tmp)
	if err != nil {
		return "", "", clipboardCauseError(i18n.KeyAuxClipboardReadTemp, err, err)
	}
	if len(data) == 0 {
		return "", "", nil
	}

	mt := detectMediaType(data)
	return base64.StdEncoding.EncodeToString(data), mt, nil
}

// darwinGetFileRefPath retrieves the POSIX file path from a macOS clipboard
// file reference («class furl»). Returns empty string if no file reference exists.
func darwinGetFileRefPath(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "osascript", "-e",
		"get POSIX path of (the clipboard as «class furl»)")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// isImageExtension checks whether a file path has a supported image extension.
func isImageExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return imageExtensions[ext]
}

// darwinHasClipboardImageFile checks if the clipboard contains a file reference
// pointing to an image file (e.g. Finder Cmd+C on a .png/.jpg file).
func darwinHasClipboardImageFile(ctx context.Context) bool {
	path := darwinGetFileRefPath(ctx)
	if path == "" {
		return false
	}
	return isImageExtension(path)
}

// darwinGetClipboardImageFromFile reads an image file referenced by the clipboard
// file reference («class furl»). Used when the clipboard has a Finder file copy
// instead of raw PNG data.
func darwinGetClipboardImageFromFile() (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()

	path := darwinGetFileRefPath(ctx)
	if path == "" {
		return "", "", clipboardError(i18n.KeyAuxClipboardMissingReference)
	}
	if !isImageExtension(path) {
		return "", "", clipboardError(i18n.KeyAuxClipboardReferenceNotImage, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", clipboardCauseError(i18n.KeyAuxClipboardReadImage, err, path, err)
	}
	if len(data) == 0 {
		return "", "", nil
	}

	mt := detectMediaType(data)
	return base64.StdEncoding.EncodeToString(data), mt, nil
}

// linuxGetClipboardImage tries wl-paste (Wayland) then xclip (X11).
func linuxGetClipboardImage() (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()

	// Try Wayland first
	if hasWaylandImage(ctx) {
		ctx2, cancel2 := context.WithTimeout(context.Background(), clipboardTimeout)
		defer cancel2()
		cmd := exec.CommandContext(ctx2, "wl-paste", "--type", "image/png")
		data, err := cmd.Output()
		if err == nil && len(data) > 0 {
			mt := detectMediaType(data)
			return base64.StdEncoding.EncodeToString(data), mt, nil
		}
		// Try jpeg
		ctx3, cancel3 := context.WithTimeout(context.Background(), clipboardTimeout)
		defer cancel3()
		cmd = exec.CommandContext(ctx3, "wl-paste", "--type", "image/jpeg")
		data, err = cmd.Output()
		if err == nil && len(data) > 0 {
			mt := detectMediaType(data)
			return base64.StdEncoding.EncodeToString(data), mt, nil
		}
	}

	// Fall back to xclip
	ctx4, cancel4 := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel4()
	cmd := exec.CommandContext(ctx4, "xclip", "-selection", "clipboard", "-t", "image/png", "-o")
	data, err := cmd.Output()
	if err == nil && len(data) > 0 {
		mt := detectMediaType(data)
		return base64.StdEncoding.EncodeToString(data), mt, nil
	}

	// Try jpeg via xclip
	ctx5, cancel5 := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel5()
	cmd = exec.CommandContext(ctx5, "xclip", "-selection", "clipboard", "-t", "image/jpeg", "-o")
	data, err = cmd.Output()
	if err == nil && len(data) > 0 {
		mt := detectMediaType(data)
		return base64.StdEncoding.EncodeToString(data), mt, nil
	}

	return "", "", clipboardError(i18n.KeyAuxClipboardLinuxUnavailable)
}

// windowsGetClipboardImage uses PowerShell to save the clipboard image to a temp file.
func windowsGetClipboardImage() (string, string, error) {
	tmp, err := createTempClipboardFile()
	if err != nil {
		return "", "", clipboardCauseError(i18n.KeyAuxClipboardCreateTemp, err, err)
	}
	defer os.Remove(tmp)

	ctx, cancel := context.WithTimeout(context.Background(), clipboardTimeout)
	defer cancel()

	// Escape single quotes to prevent PowerShell injection.
	escapedTmp := strings.ReplaceAll(tmp, `'`, `''`)
	script := fmt.Sprintf(
		`$img = Get-Clipboard -Format Image; if ($img -ne $null) { $img.Save('%s') }`,
		escapedTmp,
	)
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", "", clipboardCauseError(i18n.KeyAuxClipboardPowerShellFailed, err, err, out)
	}

	data, err := os.ReadFile(tmp)
	if err != nil {
		return "", "", clipboardCauseError(i18n.KeyAuxClipboardReadTemp, err, err)
	}
	if len(data) == 0 {
		return "", "", nil
	}

	mt := detectMediaType(data)
	return base64.StdEncoding.EncodeToString(data), mt, nil
}

// detectMediaType inspects the magic bytes of image data and returns the
// corresponding MIME type. Defaults to "image/png" if unrecognised.
func detectMediaType(data []byte) string {
	if len(data) >= 4 &&
		data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "image/png"
	}
	if len(data) >= 3 &&
		data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}
	if len(data) >= 4 && string(data[:4]) == "GIF8" {
		return "image/gif"
	}
	if len(data) >= 12 &&
		string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	return "image/png" // default fallback
}
