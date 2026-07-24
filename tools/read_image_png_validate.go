// Package tools — read_image_png_validate.go implements an early-reject
// PNG dimension guard. Mirrors src/utils/imageResizer.ts:404-411 which
// reads the IHDR chunk directly from raw bytes to bail out before
// invoking the full image decoder when a crafted PNG declares pathological
// dimensions (e.g. 100000x100000 with 1px of pixel data).
//
// Reading the 24-byte PNG header is O(1) bytes and avoids forcing
// image.Decode to allocate a backing buffer based on the malicious header.
package tools

import (
	"bytes"
	"encoding/binary"

	"github.com/agent-dance/luban/i18n"
)

// pngOverdimAbsoluteLimit is the largest dimension we'll accept on either
// axis before refusing to decode. Mirrors TS readPng safety cap.
const pngOverdimAbsoluteLimit = 16384

// pngMagic is the PNG file signature (\x89PNG\r\n\x1a\n).
var pngMagic = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

// validatePNGDimensionsFromBytes reads the IHDR chunk to retrieve the
// declared width/height and rejects the file if either dimension exceeds
// pngOverdimAbsoluteLimit. Returns (width, height, err). When data is not
// a PNG, returns (0, 0, nil) — caller continues with normal decode.
func validatePNGDimensionsFromBytes(data []byte) (int, int, error) {
	if len(data) < 24 {
		return 0, 0, nil
	}
	if !bytes.HasPrefix(data, pngMagic) {
		return 0, 0, nil
	}
	// PNG layout after 8-byte magic:
	//   4 bytes: chunk length (must be 13 for IHDR)
	//   4 bytes: chunk type ("IHDR")
	//   4 bytes: width (big-endian uint32)
	//   4 bytes: height (big-endian uint32)
	if !bytes.Equal(data[12:16], []byte("IHDR")) {
		return 0, 0, nil
	}
	width := binary.BigEndian.Uint32(data[16:20])
	height := binary.BigEndian.Uint32(data[20:24])

	if width == 0 || height == 0 {
		return 0, 0, i18n.NewError(i18n.KeyToolSourceSinkReadPNGInvalidSize, width, height)
	}
	if width > pngOverdimAbsoluteLimit || height > pngOverdimAbsoluteLimit {
		return int(width), int(height), i18n.NewError(
			i18n.KeyToolSourceSinkReadPNGTooLarge,
			width, height, pngOverdimAbsoluteLimit,
		)
	}
	return int(width), int(height), nil
}
