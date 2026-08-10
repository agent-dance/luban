package tui

import (
	"encoding/base64"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

func (c *RootComponent) imageAttachmentAtPoint(x, y int) (ImageAttachment, bool) {
	if position, ok := c.input.PositionAtPoint(x, y); ok {
		text := c.input.Text()
		for _, image := range c.state.PendingImages.Get() {
			span, found := atomicPlaceholderRange(text, imageComposerPlaceholder(image.Placeholder))
			if found && position >= span.Start && position < span.End {
				return image, true
			}
		}
	}
	for key, element := range c.messageImageRefs.All() {
		if element != nil && element.ContainsPoint(x, y) {
			image, exists := c.messageImages[key]
			return image, exists
		}
	}
	return ImageAttachment{}, false
}

func (c *RootComponent) openImageAttachment(image ImageAttachment) {
	path, err := materializeImageAttachment(image)
	if err == nil {
		opener := c.imageOpener
		if opener == nil {
			opener = openImagePath
		}
		err = opener(path)
		if err == nil {
			c.imageOpenMu.Lock()
			c.openedImagePaths = append(c.openedImagePaths, path)
			c.imageOpenMu.Unlock()
		} else {
			_ = os.Remove(path)
		}
	}
	if err != nil {
		wrapped := i18n.WrapError(i18n.KeyImageOpenFailed, err)
		message := wrapped.Error()
		if localizer, ok := wrapped.(interface {
			Localized(i18n.Language) string
		}); ok {
			message = localizer.Localized(c.state.Language.Get())
		}
		c.copyFeedback.Set(message)
		c.scheduleCopyFeedbackClear()
	}
}

func materializeImageAttachment(image ImageAttachment) (path string, err error) {
	data, err := base64.StdEncoding.DecodeString(image.Base64)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", "luban-image-*"+imageFileExtension(image.MediaType))
	if err != nil {
		return "", err
	}
	path = file.Name()
	closed := false
	ok := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if _, err = file.Write(data); err != nil {
		return "", err
	}
	if err = file.Close(); err != nil {
		closed = true
		return "", err
	}
	closed = true
	ok = true
	return path, nil
}

func imageFileExtension(mediaType string) string {
	mediaType = strings.ToLower(strings.TrimSpace(strings.Split(mediaType, ";")[0]))
	switch mediaType {
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}

func openImagePath(path string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command, args = "open", []string{path}
	case "windows":
		command, args = "explorer.exe", []string{path}
	default:
		command, args = "xdg-open", []string{path}
	}
	cmd := exec.Command(command, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func (c *RootComponent) cleanupOpenedImages() {
	c.imageOpenMu.Lock()
	paths := c.openedImagePaths
	c.openedImagePaths = nil
	c.imageOpenMu.Unlock()
	for _, path := range paths {
		_ = os.Remove(path)
	}
}
