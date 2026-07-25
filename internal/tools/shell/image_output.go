package shell

import (
	"encoding/base64"
	"regexp"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// dataUriPattern matches a data URI like `data:image/png;base64,<payload>`.
// We pin to the start of the string and require an explicit base64 token —
// the TS isImageOutput is similarly strict to avoid mistaking a payload that
// happens to contain `data:` as a plot URI.
var dataUriPattern = regexp.MustCompile(
	`(?s)^\s*data:(image/(?:png|jpe?g|gif|webp|bmp|svg\+xml));base64,([A-Za-z0-9+/=\n\r]+)\s*$`,
)

// isImageOutput reports whether `stdout` is a data URI carrying a base64-
// encoded image. When true, the caller should replace the text response with
// the image block from buildImageToolResult so the model receives the
// image rather than a giant text blob.
func isImageOutput(stdout string) bool {
	return dataUriPattern.MatchString(stdout)
}

// parseDataURI extracts the media type and base64 payload from a data URI.
// Returns ("", "", false) when the input is not a recognised image data URI.
func parseDataURI(stdout string) (mediaType, data string, ok bool) {
	m := dataUriPattern.FindStringSubmatch(stdout)
	if len(m) != 3 {
		return "", "", false
	}
	mediaType = m[1]
	// Strip whitespace from the base64 payload — multi-line URIs are common.
	data = strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t', ' ':
			return -1
		}
		return r
	}, m[2])
	if _, err := base64.StdEncoding.DecodeString(data); err != nil {
		return "", "", false
	}
	return mediaType, data, true
}

// buildImageToolResult turns the data URI in `stdout` into a ToolResult
// carrying a ContentBlocks array with one ImageBlock plus an optional text
// block describing the image. The text block is only attached when `caption`
// is non-empty so the model still has scannable context.
func buildImageToolResult(stdout, caption string) (types.ToolResult, bool) {
	mediaType, data, ok := parseDataURI(stdout)
	if !ok {
		return types.ToolResult{}, false
	}
	blocks := []types.ContentBlock{
		types.ImageBlock{
			Type: types.ContentTypeImage,
			Source: &types.ImageSource{
				Type:      "base64",
				MediaType: mediaType,
				Data:      data,
			},
		},
	}
	if strings.TrimSpace(caption) != "" {
		blocks = append(blocks, types.TextBlock{
			Type: types.ContentTypeText,
			Text: caption,
		})
	}
	return types.ToolResult{
		Content:       toolRuntimeFormat(i18n.KeyToolShellImagePlaceholder, mediaType, len(data)),
		ContentBlocks: blocks,
		Metadata: map[string]string{
			"isImage":        "true",
			"imageMediaType": mediaType,
		},
	}, true
}
