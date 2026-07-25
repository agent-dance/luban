package toolbase

import (
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestNewBase64ImageBlock(t *testing.T) {
	block, ok := NewBase64ImageBlock("encoded", "").(types.ImageBlock)
	if !ok {
		t.Fatalf("block type = %T", block)
	}
	if block.Type != types.ContentTypeImage || block.Source == nil || block.Source.Type != "base64" || block.Source.MediaType != "image/png" || block.Source.Data != "encoded" {
		t.Fatalf("block = %#v", block)
	}
}
