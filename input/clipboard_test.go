package input

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectMediaType(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "PNG magic bytes",
			data:     []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			expected: "image/png",
		},
		{
			name:     "JPEG magic bytes",
			data:     []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10},
			expected: "image/jpeg",
		},
		{
			name:     "GIF magic bytes",
			data:     []byte("GIF89a\x01\x00\x01\x00"),
			expected: "image/gif",
		},
		{
			name: "WebP magic bytes",
			data: func() []byte {
				b := make([]byte, 12)
				copy(b[0:4], "RIFF")
				copy(b[8:12], "WEBP")
				return b
			}(),
			expected: "image/webp",
		},
		{
			name:     "unknown defaults to image/png",
			data:     []byte{0x00, 0x01, 0x02, 0x03},
			expected: "image/png",
		},
		{
			name:     "empty data defaults to image/png",
			data:     []byte{},
			expected: "image/png",
		},
		{
			name:     "short data defaults to image/png",
			data:     []byte{0x89},
			expected: "image/png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectMediaType(tt.data)
			if got != tt.expected {
				t.Errorf("detectMediaType() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestHasClipboardImageNoPanic(t *testing.T) {
	// In CI or any environment, HasClipboardImage must not panic.
	// It may return false, but must not crash.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("HasClipboardImage() panicked: %v", r)
		}
	}()
	_ = HasClipboardImage()
}

func TestGetClipboardImageNoPanic(t *testing.T) {
	// GetClipboardImage must not panic even when no image is available.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("GetClipboardImage() panicked: %v", r)
		}
	}()
	// Since HasClipboardImage will return false in CI, GetClipboardImage
	// should return ("", "", nil) without error.
	b64, mt, err := GetClipboardImage()
	if err != nil {
		// An error is acceptable in CI; just log it.
		t.Logf("GetClipboardImage() returned error (expected in CI): %v", err)
	}
	// If no image, both strings must be empty.
	if b64 == "" && mt != "" {
		t.Errorf("expected empty mediaType when base64Data is empty, got %q", mt)
	}
}

func TestCreateTempClipboardFileCleanup(t *testing.T) {
	tmp, err := createTempClipboardFile()
	if err != nil {
		t.Fatalf("createTempClipboardFile() error: %v", err)
	}

	// Write a sentinel value to confirm the file is usable.
	if err := os.WriteFile(tmp, []byte("test"), 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	// Verify it exists.
	if _, err := os.Stat(tmp); os.IsNotExist(err) {
		t.Fatal("temp file should exist after writing")
	}
	// Remove it (simulating the defer os.Remove(tmp) in the real functions).
	os.Remove(tmp)
	// Verify it is gone.
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("temp file should have been removed")
	}
}

func TestCreateTempClipboardFilePath(t *testing.T) {
	tmp, err := createTempClipboardFile()
	if err != nil {
		t.Fatalf("createTempClipboardFile() error: %v", err)
	}
	defer os.Remove(tmp)

	// Must be in the OS temp directory (compare cleaned paths to normalise trailing slashes).
	if filepath.Clean(filepath.Dir(tmp)) != filepath.Clean(os.TempDir()) {
		t.Errorf("createTempClipboardFile() dir = %q, want %q", filepath.Dir(tmp), os.TempDir())
	}
	// Must have a .png extension.
	if filepath.Ext(tmp) != ".png" {
		t.Errorf("createTempClipboardFile() ext = %q, want .png", filepath.Ext(tmp))
	}
	// Each call must return a unique path.
	tmp2, err := createTempClipboardFile()
	if err != nil {
		t.Fatalf("createTempClipboardFile() second call error: %v", err)
	}
	defer os.Remove(tmp2)
	if tmp == tmp2 {
		t.Error("createTempClipboardFile() should return unique paths on each call")
	}
}

func TestIsImageExtension(t *testing.T) {
	tests := []struct {
		path   string
		expect bool
	}{
		{"/Users/me/photo.png", true},
		{"/Users/me/photo.PNG", true},
		{"/tmp/shot.jpg", true},
		{"/tmp/shot.JPG", true},
		{"/tmp/shot.jpeg", true},
		{"/tmp/shot.JPEG", true},
		{"/tmp/anim.gif", true},
		{"/tmp/anim.GIF", true},
		{"/tmp/pic.webp", true},
		{"/tmp/pic.WebP", true},
		{"/Users/me/doc.pdf", false},
		{"/Users/me/file.txt", false},
		{"/Users/me/image.bmp", false},
		{"/Users/me/image.tiff", false},
		{"", false},
		{"/Users/me/noext", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isImageExtension(tt.path)
			if got != tt.expect {
				t.Errorf("isImageExtension(%q) = %v, want %v", tt.path, got, tt.expect)
			}
		})
	}
}

