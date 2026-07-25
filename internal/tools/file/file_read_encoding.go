// Package file — file_read_encoding.go implements BOM/encoding detection
// for FileReadTool. Mirrors the encoding-detection portion of TS
// readFileSyncWithMetadata in src/tools/FileReadTool/FileReadTool.ts.
//
// Supported encodings (decode + reencode-to-UTF-8 transparently):
//   - UTF-8 with or without BOM
//   - UTF-16 LE (BOM)
//   - UTF-16 BE (BOM)
//   - UTF-16 LE/BE (no BOM, heuristic by null distribution)
//   - Latin-1 / Windows-1252 (heuristic fallback when bytes are clearly
//     non-UTF-8 but printable in 8-bit ASCII range)
//
// The result is a pair of strings: the canonical encoding label (matching
// the TS-side names: utf8, utf-8-bom, utf-16le, utf-16be, latin1) plus the
// raw BOM bytes that were stripped. Callers that want to round-trip the
// original encoding (FileWrite preservation) can capture this metadata.
package file

import (
	"bytes"
	"unicode/utf16"
	"unicode/utf8"
)

// FileEncoding is the canonical label set we use for encoded files.
type FileEncoding string

const (
	EncodingUTF8    FileEncoding = "utf8"
	EncodingUTF8BOM FileEncoding = "utf-8-bom"
	EncodingUTF16LE FileEncoding = "utf-16le"
	EncodingUTF16BE FileEncoding = "utf-16be"
	EncodingLatin1  FileEncoding = "latin1"
	EncodingUnknown FileEncoding = "unknown"
)

// Standard BOM byte sequences.
var (
	bomUTF8    = []byte{0xEF, 0xBB, 0xBF}
	bomUTF16LE = []byte{0xFF, 0xFE}
	bomUTF16BE = []byte{0xFE, 0xFF}
)

// EncodingDetectResult captures the detected encoding metadata for a
// raw byte buffer. BOM holds the literal BOM bytes (empty when none).
type EncodingDetectResult struct {
	Encoding FileEncoding
	BOM      []byte
}

// detectFileEncoding inspects the first ~8KB of a buffer to classify its
// encoding. The returned result preserves the BOM bytes (when present)
// so a write path can reapply them.
//
// Heuristics, in priority order:
//  1. Explicit BOM (UTF-8 / UTF-16 LE / UTF-16 BE) — definitive.
//  2. Null-byte parity for UTF-16 LE/BE without BOM (>=20% nulls in even
//     or odd positions).
//  3. Valid UTF-8 — accept as UTF-8.
//  4. Otherwise: Latin-1 fallback so callers can still display the file.
func detectFileEncoding(data []byte) EncodingDetectResult {
	if len(data) == 0 {
		return EncodingDetectResult{Encoding: EncodingUTF8}
	}

	if bytes.HasPrefix(data, bomUTF8) {
		return EncodingDetectResult{Encoding: EncodingUTF8BOM, BOM: append([]byte(nil), bomUTF8...)}
	}
	if bytes.HasPrefix(data, bomUTF16LE) {
		return EncodingDetectResult{Encoding: EncodingUTF16LE, BOM: append([]byte(nil), bomUTF16LE...)}
	}
	if bytes.HasPrefix(data, bomUTF16BE) {
		return EncodingDetectResult{Encoding: EncodingUTF16BE, BOM: append([]byte(nil), bomUTF16BE...)}
	}

	sampleEnd := len(data)
	if sampleEnd > 8192 {
		sampleEnd = 8192
	}
	utf16Sample := data[:sampleEnd]

	// Heuristic UTF-16 detection: count null bytes at even vs odd positions.
	if enc, ok := guessUTF16WithoutBOM(utf16Sample); ok {
		return EncodingDetectResult{Encoding: enc}
	}

	// A fixed 8 KiB sample may end in the middle of a valid UTF-8 rune. When
	// lookahead bytes are available, extend through continuation bytes to the
	// next rune boundary before validating.
	utf8End := sampleEnd
	for utf8End < len(data) && utf8End < sampleEnd+utf8.UTFMax-1 && !utf8.RuneStart(data[utf8End]) {
		utf8End++
	}
	if utf8.Valid(data[:utf8End]) {
		return EncodingDetectResult{Encoding: EncodingUTF8}
	}

	return EncodingDetectResult{Encoding: EncodingLatin1}
}

// guessUTF16WithoutBOM detects BOM-less UTF-16 by looking at the parity
// of null-byte positions in ASCII-heavy text.
func guessUTF16WithoutBOM(sample []byte) (FileEncoding, bool) {
	if len(sample) < 4 {
		return "", false
	}
	if len(sample)%2 != 0 {
		// odd-length is unlikely to be UTF-16
		return "", false
	}
	evenNulls, oddNulls := 0, 0
	for i := 0; i < len(sample); i++ {
		if sample[i] == 0 {
			if i%2 == 0 {
				evenNulls++
			} else {
				oddNulls++
			}
		}
	}
	pairs := len(sample) / 2
	// Require a strong skew (>=30%) on one side to call UTF-16.
	if oddNulls*100/pairs >= 30 && evenNulls == 0 {
		return EncodingUTF16LE, true
	}
	if evenNulls*100/pairs >= 30 && oddNulls == 0 {
		return EncodingUTF16BE, true
	}
	return "", false
}

// decodeFileBytes returns the UTF-8 string representation of data given
// the detected encoding. The BOM (if any) is stripped from the result so
// downstream rendering does not include it in the visible text.
func decodeFileBytes(data []byte, det EncodingDetectResult) string {
	body := data
	if len(det.BOM) > 0 && bytes.HasPrefix(body, det.BOM) {
		body = body[len(det.BOM):]
	}
	switch det.Encoding {
	case EncodingUTF8, EncodingUTF8BOM:
		return string(body)
	case EncodingUTF16LE:
		return decodeUTF16(body, false)
	case EncodingUTF16BE:
		return decodeUTF16(body, true)
	case EncodingLatin1:
		return decodeLatin1(body)
	default:
		return string(body)
	}
}

// decodeUTF16 converts a byte slice (already BOM-stripped) into a UTF-8
// string using the requested byte order.
func decodeUTF16(b []byte, bigEndian bool) string {
	if len(b) == 0 {
		return ""
	}
	// Trim a trailing odd byte rather than panic.
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	u16 := make([]uint16, len(b)/2)
	for i := 0; i < len(u16); i++ {
		hi := b[2*i]
		lo := b[2*i+1]
		if bigEndian {
			u16[i] = uint16(hi)<<8 | uint16(lo)
		} else {
			u16[i] = uint16(lo)<<8 | uint16(hi)
		}
	}
	return string(utf16.Decode(u16))
}

// decodeLatin1 maps each byte to the Unicode codepoint with the same
// numeric value (ISO-8859-1 round-trips into Unicode by definition).
func decodeLatin1(b []byte) string {
	r := make([]rune, len(b))
	for i, v := range b {
		r[i] = rune(v)
	}
	return string(r)
}

// encodeStringForEncoding takes a UTF-8 string and re-emits the bytes in
// the requested encoding (without BOM — caller prepends if required).
// Returns nil on unknown encodings — caller should fall back to UTF-8.
func encodeStringForEncoding(s string, enc FileEncoding) []byte {
	switch enc {
	case EncodingUTF8, EncodingUTF8BOM, "":
		return []byte(s)
	case EncodingUTF16LE:
		return encodeUTF16([]rune(s), false)
	case EncodingUTF16BE:
		return encodeUTF16([]rune(s), true)
	case EncodingLatin1:
		return encodeLatin1(s)
	default:
		return nil
	}
}

func encodeUTF16(r []rune, bigEndian bool) []byte {
	u16 := utf16.Encode(r)
	out := make([]byte, len(u16)*2)
	for i, c := range u16 {
		if bigEndian {
			out[2*i] = byte(c >> 8)
			out[2*i+1] = byte(c & 0xFF)
		} else {
			out[2*i] = byte(c & 0xFF)
			out[2*i+1] = byte(c >> 8)
		}
	}
	return out
}

func encodeLatin1(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if r < 0x100 {
			out = append(out, byte(r))
		} else {
			// substitute character: '?'
			out = append(out, '?')
		}
	}
	return out
}
