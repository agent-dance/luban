// Package tools — read_image_errors.go implements numeric error
// categories for image processing failures. Mirrors TS classifyImageError
// (src/utils/imageResizer.ts) which assigns one of 8 stable category IDs
// to each failure so analytics can distinguish "library missing" from
// "image too large" without parsing free-form messages.
package tools

import (
	"errors"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// ImageErrorCategory enumerates the stable category IDs used by TS.
type ImageErrorCategory int

const (
	ImageErrCategoryUnknown      ImageErrorCategory = 0
	ImageErrCategoryModuleLoad   ImageErrorCategory = 1
	ImageErrCategoryDecodeFailed ImageErrorCategory = 2
	ImageErrCategoryTooLarge     ImageErrorCategory = 3
	ImageErrCategoryDimensionOOM ImageErrorCategory = 4
	ImageErrCategoryEncodeFailed ImageErrorCategory = 5
	ImageErrCategoryEmpty        ImageErrorCategory = 6
	ImageErrCategoryFormat       ImageErrorCategory = 7
	ImageErrCategoryPermission   ImageErrorCategory = 8
)

// classifiedImageError wraps an underlying error with a stable category
// label so analytics consumers can branch on Category() without parsing
// strings.
type classifiedImageError struct {
	Underlying error
	Category   ImageErrorCategory
}

func (e *classifiedImageError) Error() string {
	if e == nil || e.Underlying == nil {
		return ""
	}
	return e.Underlying.Error()
}

func (e *classifiedImageError) Unwrap() error { return e.Underlying }

// CategoryOfImageError returns the category of err, or
// ImageErrCategoryUnknown when err is not a classifiedImageError.
func CategoryOfImageError(err error) ImageErrorCategory {
	var c *classifiedImageError
	if errors.As(err, &c) {
		return c.Category
	}
	return ImageErrCategoryUnknown
}

// classifyImageError inspects err's message and wraps it in a
// classifiedImageError with the matching category. Mirrors TS keyword
// matching.
func classifyImageError(err error) error {
	if err == nil {
		return nil
	}
	// Pre-classified — leave alone.
	var already *classifiedImageError
	if errors.As(err, &already) {
		return err
	}
	if semantic, ok := i18n.DescribeSemanticError(err); ok {
		switch semantic.Key {
		case i18n.KeyToolSourceSinkReadPNGInvalidSize, i18n.KeyToolSourceSinkReadPNGTooLarge:
			return &classifiedImageError{Underlying: err, Category: ImageErrCategoryDimensionOOM}
		}
	}
	msg := strings.ToLower(err.Error())
	cat := ImageErrCategoryUnknown
	switch {
	case strings.Contains(msg, "permission") || strings.Contains(msg, "access denied"):
		cat = ImageErrCategoryPermission
	case strings.Contains(msg, "is empty"):
		cat = ImageErrCategoryEmpty
	case strings.Contains(msg, "exceed") && strings.Contains(msg, "dimension"),
		strings.Contains(msg, "exceed the maximum allowed"):
		cat = ImageErrCategoryDimensionOOM
	case strings.Contains(msg, "exceed") && strings.Contains(msg, "token"):
		cat = ImageErrCategoryTooLarge
	case strings.Contains(msg, "exceed") && (strings.Contains(msg, "size") || strings.Contains(msg, "bytes")):
		cat = ImageErrCategoryTooLarge
	case strings.Contains(msg, "decode") || strings.Contains(msg, "decoding"):
		cat = ImageErrCategoryDecodeFailed
	case strings.Contains(msg, "encode") || strings.Contains(msg, "encoding"):
		cat = ImageErrCategoryEncodeFailed
	case strings.Contains(msg, "module") || strings.Contains(msg, "library"):
		cat = ImageErrCategoryModuleLoad
	case strings.Contains(msg, "format") || strings.Contains(msg, "unsupported"):
		cat = ImageErrCategoryFormat
	}
	return &classifiedImageError{Underlying: err, Category: cat}
}
