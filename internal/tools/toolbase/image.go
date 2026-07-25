package toolbase

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"strings"

	"github.com/agent-dance/luban/types"
)

// ModelImageMaxBytes is the maximum decoded size accepted for one tool image.
const ModelImageMaxBytes = 5 * 1024 * 1024

// shellImageDiskFallbackMaxBytes caps disk-fallback re-reads at 20MB so a
// runaway script writing a multi-GB PNG cannot OOM the process. Mirrors the
// TS resizeShellImageOutput cap.
const shellImageDiskFallbackMaxBytes = 20 * 1024 * 1024

// ResizeImageToolResult mutates res in-place to apply the disk-
// fallback + DPI downsampling pipeline TS uses to fit Anthropic's 5MB image
// cap. When the image already fits, it is returned unchanged.
//
// The caller is expected to have populated res.ContentBlocks with an
// ImageBlock built from the stdout data URI; this routine is a no-op for
// non-image results.
func ResizeImageToolResult(res types.ToolResult) types.ToolResult {
	if len(res.ContentBlocks) == 0 {
		return res
	}
	for i, block := range res.ContentBlocks {
		img, ok := block.(types.ImageBlock)
		if !ok || img.Source == nil {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(img.Source.Data)
		if err != nil {
			// Try disk fallback when stdout truncation chopped the base64.
			if fallback, ferr := readDiskFallbackImage(res.Metadata); ferr == nil && len(fallback) > 0 {
				raw = fallback
			} else {
				continue
			}
		}
		if len(raw) <= ModelImageMaxBytes {
			continue
		}
		resized, mediaType, err := ResizeImageBytes(raw, img.Source.MediaType, ModelImageMaxBytes)
		if err != nil {
			continue
		}
		img.Source.Data = base64.StdEncoding.EncodeToString(resized)
		img.Source.MediaType = mediaType
		res.ContentBlocks[i] = img
		if res.Metadata == nil {
			res.Metadata = map[string]string{}
		}
		res.Metadata["imageResized"] = "true"
		res.Metadata["imageOriginalBytes"] = fmt.Sprintf("%d", len(raw))
		res.Metadata["imageBytes"] = fmt.Sprintf("%d", len(resized))
	}
	return res
}

// readDiskFallbackImage looks for a disk-spilled image when stdout was capped.
// Caller passes the metadata map; we use the OutputFilePath / outputFilePath
// key the background-task path stamps. Returns the file bytes capped at
// shellImageDiskFallbackMaxBytes.
func readDiskFallbackImage(meta map[string]string) ([]byte, error) {
	path := strings.TrimSpace(meta["outputFilePath"])
	if path == "" {
		path = strings.TrimSpace(meta["OutputFilePath"])
	}
	if path == "" {
		return nil, fmt.Errorf("no disk fallback path")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, shellImageDiskFallbackMaxBytes)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return nil, err
	}
	return buf[:n], nil
}

// ResizeImageBytes downsamples an encoded image (PNG/JPEG) until its byte
// size drops below `targetBytes`. We approximate matplotlib DPI=300 →
// DPI=96 by halving width/height repeatedly; the JPEG quality ladder
// (90 → 75 → 60) handles the long tail. Returns the encoded bytes and the
// media type (which switches to image/jpeg when JPEG is the smaller form).
func ResizeImageBytes(raw []byte, mediaType string, targetBytes int) ([]byte, string, error) {
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, mediaType, err
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Halve dimensions until projected target byte budget is plausibly hit.
	// We allow up to 5 reductions so a 4096x4096 PNG drops to 256x256 worst-case.
	for iter := 0; iter < 6; iter++ {
		encoded, mt, err := encodeImageBudget(img, format, mediaType, targetBytes)
		if err == nil && len(encoded) <= targetBytes {
			return encoded, mt, nil
		}
		// Halve and continue.
		width /= 2
		height /= 2
		if width < 16 || height < 16 {
			if encoded != nil {
				return encoded, mt, nil
			}
			return raw, mediaType, fmt.Errorf("image too large after %d downsamples", iter)
		}
		dst := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
		bilinearScale(dst, img)
		img = dst
	}
	return encodeImageBudget(img, format, mediaType, targetBytes)
}

// encodeImageBudget walks a JPEG quality ladder (90/75/60/45) for raster
// formats and returns the smallest result. PNG falls back to JPEG when the
// PNG encoding stays above budget.
func encodeImageBudget(img image.Image, format, mediaType string, targetBytes int) ([]byte, string, error) {
	var pngBuf bytes.Buffer
	if format == "png" || strings.Contains(mediaType, "png") {
		if err := png.Encode(&pngBuf, img); err == nil && pngBuf.Len() <= targetBytes {
			return pngBuf.Bytes(), "image/png", nil
		}
	}
	for _, q := range []int{90, 75, 60, 45} {
		var b bytes.Buffer
		if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: q}); err != nil {
			continue
		}
		if b.Len() <= targetBytes {
			return b.Bytes(), "image/jpeg", nil
		}
	}
	// Worst case — return the smallest (jpeg q=45) so the caller still has data.
	var fallback bytes.Buffer
	if err := jpeg.Encode(&fallback, img, &jpeg.Options{Quality: 45}); err != nil {
		if pngBuf.Len() > 0 {
			return pngBuf.Bytes(), "image/png", nil
		}
		return nil, mediaType, err
	}
	return fallback.Bytes(), "image/jpeg", nil
}

// bilinearScale fills `dst` with `src` resampled using bilinear interpolation.
// We use this rather than draw.NearestNeighbor to avoid jagged downsamples on
// matplotlib charts.
func bilinearScale(dst draw.Image, src image.Image) {
	dstBounds := dst.Bounds()
	srcBounds := src.Bounds()
	dw := dstBounds.Dx()
	dh := dstBounds.Dy()
	sw := srcBounds.Dx()
	sh := srcBounds.Dy()
	if dw == 0 || dh == 0 || sw == 0 || sh == 0 {
		return
	}
	fx := float64(sw) / float64(dw)
	fy := float64(sh) / float64(dh)
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			sx := int(float64(x) * fx)
			sy := int(float64(y) * fy)
			if sx >= sw {
				sx = sw - 1
			}
			if sy >= sh {
				sy = sh - 1
			}
			c := src.At(srcBounds.Min.X+sx, srcBounds.Min.Y+sy)
			dst.Set(dstBounds.Min.X+x, dstBounds.Min.Y+y, c)
		}
	}
}
