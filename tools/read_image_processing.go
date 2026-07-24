package tools

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"net/http"
	"strings"

	"github.com/agent-dance/luban/i18n"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	readImageTargetRawSize = (5 * 1024 * 1024 * 3) / 4
	readImageMaxWidth      = 2000
	readImageMaxHeight     = 2000
	readImageMinDimension  = 128
)

type readImageDimensions struct {
	OriginalWidth  int
	OriginalHeight int
	DisplayWidth   int
	DisplayHeight  int
}

type preparedImage struct {
	Data       []byte
	MediaType  string
	Dimensions *readImageDimensions
}

type encodedImageCandidate struct {
	Data      []byte
	MediaType string
	Width     int
	Height    int
}

func prepareImageForRead(filePath string, data []byte, maxTokens int) (preparedImage, error) {
	if len(data) == 0 {
		return preparedImage{}, i18n.NewError(i18n.KeyToolSourceSinkReadImageEmpty, filePath)
	}

	// Early reject for crafted PNGs declaring pathological dimensions.
	// Mirrors TS imageResizer:404-411 — we look at the IHDR chunk only,
	// which is O(1) bytes, before letting image.Decode allocate.
	if _, _, err := validatePNGDimensionsFromBytes(data); err != nil {
		return preparedImage{}, classifyImageError(err)
	}

	mediaType := detectImageMediaType(data, filePath)
	if width, height, ok := decodeImageDimensions(data); ok {
		if len(data) <= readImageTargetRawSize && width <= readImageMaxWidth && height <= readImageMaxHeight && estimateImageTokens(len(data)) <= maxTokens {
			return preparedImage{
				Data:      data,
				MediaType: mediaType,
				Dimensions: &readImageDimensions{
					OriginalWidth:  width,
					OriginalHeight: height,
					DisplayWidth:   width,
					DisplayHeight:  height,
				},
			}, nil
		}
	} else if estimateImageTokens(len(data)) <= maxTokens {
		return preparedImage{Data: data, MediaType: mediaType}, nil
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		if tokens := estimateImageTokens(len(data)); tokens > maxTokens {
			return preparedImage{}, i18n.NewError(
				i18n.KeyToolSourceSinkReadImageTokenLimit,
				tokens,
				maxTokens,
			)
		}
		return preparedImage{Data: data, MediaType: mediaType}, nil
	}

	bounds := img.Bounds()
	originalWidth := bounds.Dx()
	originalHeight := bounds.Dy()
	displayWidth, displayHeight := fitImageDimensions(originalWidth, originalHeight, readImageMaxWidth, readImageMaxHeight)
	working := img
	if displayWidth != originalWidth || displayHeight != originalHeight {
		working = resizeImage(img, displayWidth, displayHeight)
	}

	candidates := buildImageCandidates(working, mediaType, displayWidth, displayHeight, readImageTargetRawSize)
	best := pickBestImageCandidate(candidates, maxTokens)
	if best == nil {
		best = shrinkImageToTokenBudget(img, mediaType, originalWidth, originalHeight, maxTokens)
	}
	if best == nil {
		best = smallestImageCandidate(candidates)
	}
	if best == nil {
		return preparedImage{}, i18n.NewError(i18n.KeyToolSourceSinkReadImagePrepare)
	}

	return preparedImage{
		Data:      best.Data,
		MediaType: best.MediaType,
		Dimensions: &readImageDimensions{
			OriginalWidth:  originalWidth,
			OriginalHeight: originalHeight,
			DisplayWidth:   best.Width,
			DisplayHeight:  best.Height,
		},
	}, nil
}

func decodeImageDimensions(data []byte) (width int, height int, ok bool) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

func detectImageMediaType(data []byte, filePath string) string {
	sample := data
	if len(sample) > 512 {
		sample = sample[:512]
	}
	mediaType := strings.TrimSpace(http.DetectContentType(sample))
	if idx := strings.IndexByte(mediaType, ';'); idx >= 0 {
		mediaType = strings.TrimSpace(mediaType[:idx])
	}
	if strings.HasPrefix(mediaType, "image/") {
		if mediaType == "image/jpg" {
			return "image/jpeg"
		}
		return mediaType
	}
	return imageMediaTypeForPath(filePath)
}

func estimateImageTokens(rawSize int) int {
	return int(math.Ceil(float64(base64.StdEncoding.EncodedLen(rawSize)) * 0.125))
}

func fitImageDimensions(width, height, maxWidth, maxHeight int) (int, int) {
	if width <= 0 || height <= 0 {
		return width, height
	}
	scale := 1.0
	if width > maxWidth {
		scale = math.Min(scale, float64(maxWidth)/float64(width))
	}
	if height > maxHeight {
		scale = math.Min(scale, float64(maxHeight)/float64(height))
	}
	if scale >= 1.0 {
		return width, height
	}
	fittedWidth := max(1, int(math.Round(float64(width)*scale)))
	fittedHeight := max(1, int(math.Round(float64(height)*scale)))
	return fittedWidth, fittedHeight
}

func resizeImage(img image.Image, width, height int) image.Image {
	srcBounds := img.Bounds()
	if srcBounds.Dx() == width && srcBounds.Dy() == height {
		return img
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, srcBounds, draw.Over, nil)
	return dst
}

func buildImageCandidates(img image.Image, mediaType string, width, height int, targetBytes int) []encodedImageCandidate {
	candidates := make([]encodedImageCandidate, 0, 6)
	alphaPreferred := hasAlpha(img)
	seen := map[string]bool{}

	addCandidate := func(data []byte, candidateMediaType string) {
		if len(data) == 0 {
			return
		}
		key := fmt.Sprintf("%s:%d", candidateMediaType, len(data))
		if seen[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, encodedImageCandidate{
			Data:      data,
			MediaType: candidateMediaType,
			Width:     width,
			Height:    height,
		})
	}

	if alphaPreferred || mediaType == "image/png" {
		if pngData, err := encodePNGImage(img); err == nil {
			addCandidate(pngData, "image/png")
			if len(pngData) <= targetBytes {
				return candidates
			}
		}
	}

	opaque := flattenImageForJPEG(img)
	for _, quality := range []int{80, 60, 40, 20} {
		jpegData, err := encodeJPEGImage(opaque, quality)
		if err != nil {
			continue
		}
		addCandidate(jpegData, "image/jpeg")
		if len(jpegData) <= targetBytes {
			return candidates
		}
	}

	if !alphaPreferred {
		switch mediaType {
		case "image/gif":
			if gifData, err := encodeGIFImage(img); err == nil {
				addCandidate(gifData, "image/gif")
			}
		case "image/png":
			if pngData, err := encodePNGImage(img); err == nil {
				addCandidate(pngData, "image/png")
			}
		}
	}

	return candidates
}

func pickBestImageCandidate(candidates []encodedImageCandidate, maxTokens int) *encodedImageCandidate {
	var best *encodedImageCandidate
	for i := range candidates {
		candidate := candidates[i]
		if estimateImageTokens(len(candidate.Data)) > maxTokens {
			continue
		}
		if best == nil || len(candidate.Data) < len(best.Data) {
			best = &candidate
		}
	}
	return best
}

func smallestImageCandidate(candidates []encodedImageCandidate) *encodedImageCandidate {
	var best *encodedImageCandidate
	for i := range candidates {
		candidate := candidates[i]
		if best == nil || len(candidate.Data) < len(best.Data) {
			best = &candidate
		}
	}
	return best
}

func shrinkImageToTokenBudget(img image.Image, mediaType string, originalWidth, originalHeight, maxTokens int) *encodedImageCandidate {
	baseWidth, baseHeight := fitImageDimensions(originalWidth, originalHeight, readImageMaxWidth, readImageMaxHeight)
	// Match TS scaling sequence (1.0, 0.75, 0.5, 0.25) so Go and TS produce
	// equivalent thumbnail sizes for the same input. Mirrors
	// imageResizer:tryProgressiveResizing.
	scales := []float64{1.0, 0.75, 0.5, 0.25}
	allCandidates := make([]encodedImageCandidate, 0, len(scales)*4)

	for _, scale := range scales {
		width := max(readImageMinDimension, int(math.Round(float64(baseWidth)*scale)))
		height := max(readImageMinDimension, int(math.Round(float64(baseHeight)*scale)))
		if originalWidth < readImageMinDimension {
			width = max(1, int(math.Round(float64(baseWidth)*scale)))
		}
		if originalHeight < readImageMinDimension {
			height = max(1, int(math.Round(float64(baseHeight)*scale)))
		}
		width = min(width, baseWidth)
		height = min(height, baseHeight)

		scaled := resizeImage(img, width, height)
		candidates := buildImageCandidates(scaled, mediaType, width, height, readImageTargetRawSize)
		if best := pickBestImageCandidate(candidates, maxTokens); best != nil {
			return best
		}
		allCandidates = append(allCandidates, candidates...)
	}

	// Ultra-compress fallback: a final 400x400 + JPEG q=20 attempt before
	// giving up. Mirrors TS imageResizer's last-resort branch which keeps
	// pathological images viewable (degraded but readable) rather than
	// rejecting outright.
	if ultra := ultraCompressFallback(img, maxTokens); ultra != nil {
		return ultra
	}

	return smallestImageCandidate(allCandidates)
}

// ultraCompressFallback returns a 400x400 JPEG-quality-20 candidate sized
// to fit maxTokens, or nil when even that doesn't fit. Mirrors the TS
// imageResizer last-resort path used when all earlier scales overflow.
func ultraCompressFallback(img image.Image, maxTokens int) *encodedImageCandidate {
	const ultraDim = 400
	const ultraQuality = 20
	scaled := resizeImage(img, ultraDim, ultraDim)
	flat := flattenImageForJPEG(scaled)
	data, err := encodeJPEGImage(flat, ultraQuality)
	if err != nil {
		return nil
	}
	if estimateImageTokens(len(data)) > maxTokens {
		return nil
	}
	return &encodedImageCandidate{
		Data:      data,
		MediaType: "image/jpeg",
		Width:     ultraDim,
		Height:    ultraDim,
	}
}

func encodePNGImage(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeJPEGImage(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeGIFImage(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func flattenImageForJPEG(img image.Image) image.Image {
	bounds := img.Bounds()
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(dst, bounds, img, bounds.Min, draw.Over)
	return dst
}

func hasAlpha(img image.Image) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a != 0xffff {
				return true
			}
		}
	}
	return false
}

func createImageMetadataText(dimensions *readImageDimensions) string {
	if dimensions == nil {
		return ""
	}
	if dimensions.OriginalWidth <= 0 || dimensions.OriginalHeight <= 0 || dimensions.DisplayWidth <= 0 || dimensions.DisplayHeight <= 0 {
		return ""
	}
	if dimensions.OriginalWidth == dimensions.DisplayWidth && dimensions.OriginalHeight == dimensions.DisplayHeight {
		return ""
	}
	scaleFactor := float64(dimensions.OriginalWidth) / float64(dimensions.DisplayWidth)
	return fmt.Sprintf(
		"[Image: original %dx%d, displayed at %dx%d. Multiply coordinates by %.2f to map to original image.]",
		dimensions.OriginalWidth,
		dimensions.OriginalHeight,
		dimensions.DisplayWidth,
		dimensions.DisplayHeight,
		scaleFactor,
	)
}
