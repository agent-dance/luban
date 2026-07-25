package file

import (
	"bytes"
	"strings"
	"testing"
)

func TestP0DetectFileEncodingCompletesUTF8RuneAtSampleBoundary(t *testing.T) {
	for _, value := range []string{"é", "中", "😀"} {
		encoded := []byte(value)
		for split := 1; split < len(encoded); split++ {
			prefix := strings.Repeat("a", 8192-split)
			data := append([]byte(prefix), encoded...)
			data = append(data, []byte("tail")...)
			if got := detectFileEncoding(data).Encoding; got != EncodingUTF8 {
				t.Errorf("%q split at %d detected as %q", value, split, got)
			}
		}
	}
}

func TestP0DetectFileEncodingDoesNotForgiveTruncatedUTF8(t *testing.T) {
	data := append([]byte(strings.Repeat("a", 8191)), 0xE4, 0xB8)
	if got := detectFileEncoding(data).Encoding; got != EncodingLatin1 {
		t.Fatalf("truncated UTF-8 detected as %q", got)
	}
}

func TestFileReadEncoding_UTF8(t *testing.T) {
	res := detectFileEncoding([]byte("hello world"))
	if res.Encoding != EncodingUTF8 {
		t.Fatalf("expected utf8, got %s", res.Encoding)
	}
	if len(res.BOM) != 0 {
		t.Fatalf("expected no BOM, got %v", res.BOM)
	}
}

func TestFileReadEncoding_UTF8BOM(t *testing.T) {
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte("hello")...)
	res := detectFileEncoding(data)
	if res.Encoding != EncodingUTF8BOM {
		t.Fatalf("expected utf-8-bom, got %s", res.Encoding)
	}
	if !bytes.Equal(res.BOM, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatalf("expected utf8 BOM bytes, got %v", res.BOM)
	}
	if got := decodeFileBytes(data, res); got != "hello" {
		t.Fatalf("decode: expected hello, got %q", got)
	}
}

func TestFileReadEncoding_UTF16LE_BOM(t *testing.T) {
	// "Hi" in UTF-16LE: 0x48 0x00 0x69 0x00
	data := append([]byte{0xFF, 0xFE}, 0x48, 0x00, 0x69, 0x00)
	res := detectFileEncoding(data)
	if res.Encoding != EncodingUTF16LE {
		t.Fatalf("expected utf-16le, got %s", res.Encoding)
	}
	if got := decodeFileBytes(data, res); got != "Hi" {
		t.Fatalf("decode: expected Hi, got %q", got)
	}
}

func TestFileReadEncoding_UTF16BE_BOM(t *testing.T) {
	data := append([]byte{0xFE, 0xFF}, 0x00, 0x48, 0x00, 0x69)
	res := detectFileEncoding(data)
	if res.Encoding != EncodingUTF16BE {
		t.Fatalf("expected utf-16be, got %s", res.Encoding)
	}
	if got := decodeFileBytes(data, res); got != "Hi" {
		t.Fatalf("decode: expected Hi, got %q", got)
	}
}

func TestFileReadEncoding_UTF16LE_NoBOM(t *testing.T) {
	// "Hello" in UTF-16LE without BOM
	data := []byte{0x48, 0x00, 0x65, 0x00, 0x6C, 0x00, 0x6C, 0x00, 0x6F, 0x00}
	res := detectFileEncoding(data)
	if res.Encoding != EncodingUTF16LE {
		t.Fatalf("expected utf-16le from heuristic, got %s", res.Encoding)
	}
	if got := decodeFileBytes(data, res); got != "Hello" {
		t.Fatalf("decode: expected Hello, got %q", got)
	}
}

func TestFileReadEncoding_Latin1(t *testing.T) {
	// é = 0xE9 in Latin-1, invalid UTF-8 by itself
	data := []byte{0x48, 0xE9, 0x6C, 0x6C, 0x6F}
	res := detectFileEncoding(data)
	if res.Encoding != EncodingLatin1 {
		t.Fatalf("expected latin1, got %s", res.Encoding)
	}
	got := decodeFileBytes(data, res)
	if got != "Héllo" {
		t.Fatalf("decode: expected Héllo, got %q", got)
	}
}

func TestFileReadEncoding_Empty(t *testing.T) {
	res := detectFileEncoding(nil)
	if res.Encoding != EncodingUTF8 {
		t.Fatalf("expected utf8 for empty buffer, got %s", res.Encoding)
	}
}

func TestFileReadEncoding_RoundTripUTF16LE(t *testing.T) {
	out := encodeStringForEncoding("Hello", EncodingUTF16LE)
	expected := []byte{0x48, 0x00, 0x65, 0x00, 0x6C, 0x00, 0x6C, 0x00, 0x6F, 0x00}
	if !bytes.Equal(out, expected) {
		t.Fatalf("expected %v, got %v", expected, out)
	}
}

func TestFileReadEncoding_RoundTripUTF16BE(t *testing.T) {
	out := encodeStringForEncoding("Hi", EncodingUTF16BE)
	expected := []byte{0x00, 0x48, 0x00, 0x69}
	if !bytes.Equal(out, expected) {
		t.Fatalf("expected %v, got %v", expected, out)
	}
}
