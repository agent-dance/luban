package file

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
)

// craftPNGHeader builds a fake PNG header (just enough for IHDR parsing)
// declaring the given width/height. The body after IHDR is intentionally
// trash — we only care about the early-reject path.
func craftPNGHeader(width, height uint32) []byte {
	var buf bytes.Buffer
	buf.Write(pngMagic)
	// Length = 13 (IHDR is always 13 bytes of payload)
	binary.Write(&buf, binary.BigEndian, uint32(13))
	buf.WriteString("IHDR")
	binary.Write(&buf, binary.BigEndian, width)
	binary.Write(&buf, binary.BigEndian, height)
	// Pad with extra body bytes so length>=24
	buf.Write(make([]byte, 16))
	return buf.Bytes()
}

func TestValidatePNGDimensionsFromBytes_OK(t *testing.T) {
	data := craftPNGHeader(1024, 768)
	w, h, err := validatePNGDimensionsFromBytes(data)
	if err != nil {
		t.Fatalf("expected no error for 1024x768, got %v", err)
	}
	if w != 1024 || h != 768 {
		t.Fatalf("expected 1024x768, got %dx%d", w, h)
	}
}

func TestValidatePNGDimensionsFromBytes_Overdim(t *testing.T) {
	data := craftPNGHeader(100000, 100000)
	_, _, err := validatePNGDimensionsFromBytes(data)
	if err == nil {
		t.Fatal("expected reject for 100000x100000 PNG")
	}
}

func TestValidatePNGDimensionsFromBytes_NotPNG(t *testing.T) {
	data := []byte("\x89JPG_not_a_png_at_all_just_some_data_padding_padding")
	w, h, err := validatePNGDimensionsFromBytes(data)
	if err != nil {
		t.Fatalf("expected nil error for non-PNG, got %v", err)
	}
	if w != 0 || h != 0 {
		t.Fatalf("expected (0,0) for non-PNG, got (%d,%d)", w, h)
	}
}

func TestValidatePNGDimensionsFromBytes_TooShort(t *testing.T) {
	_, _, err := validatePNGDimensionsFromBytes([]byte("short"))
	if err != nil {
		t.Fatalf("short data should not error, got %v", err)
	}
}

func TestValidatePNGDimensionsFromBytes_ZeroDim(t *testing.T) {
	data := craftPNGHeader(0, 100)
	if _, _, err := validatePNGDimensionsFromBytes(data); err == nil {
		t.Fatal("expected reject for 0-width PNG")
	}
}

func TestClassifyImageError_Categories(t *testing.T) {
	cases := []struct {
		msg  string
		want ImageErrorCategory
	}{
		{"image is empty", ImageErrCategoryEmpty},
		{"PNG dimensions 100000x100000 exceed the maximum allowed", ImageErrCategoryDimensionOOM},
		{"image content (5000 tokens) exceeds maximum allowed", ImageErrCategoryTooLarge},
		{"failed to decode image", ImageErrCategoryDecodeFailed},
		{"jpeg encode failed", ImageErrCategoryEncodeFailed},
		{"unsupported format", ImageErrCategoryFormat},
		{"permission denied", ImageErrCategoryPermission},
		{"random unhelpful message", ImageErrCategoryUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.msg, func(t *testing.T) {
			err := classifyImageError(errors.New(tc.msg))
			var classified *classifiedImageError
			if !errors.As(err, &classified) {
				t.Fatalf("classify(%q) returned %T", tc.msg, err)
			}
			if got := classified.Category; got != tc.want {
				t.Errorf("classify(%q) -> %d, want %d", tc.msg, got, tc.want)
			}
		})
	}
}

func TestClassifyImageError_PreClassifiedNoOp(t *testing.T) {
	pre := classifyImageError(fmt.Errorf("permission denied"))
	again := classifyImageError(pre)
	var classified *classifiedImageError
	if !errors.As(again, &classified) || classified.Category != ImageErrCategoryPermission {
		t.Fatal("re-classify should preserve original category")
	}
}
