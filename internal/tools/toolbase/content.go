package toolbase

import "github.com/agent-dance/luban/types"

// NewBase64ImageBlock builds an image content block from already encoded data.
func NewBase64ImageBlock(data, mediaType string) types.ContentBlock {
	if mediaType == "" {
		mediaType = "image/png"
	}
	return types.ImageBlock{
		Type: types.ContentTypeImage,
		Source: &types.ImageSource{
			Type:      "base64",
			MediaType: mediaType,
			Data:      data,
		},
	}
}
