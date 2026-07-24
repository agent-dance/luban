package commands

import (
	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/input"
	"github.com/agent-dance/luban/types"
)

// PasteCmd implements the /paste slash command.
// It checks the clipboard for an image, prompts the user for confirmation
// (via ctx.Confirm), and if confirmed, populates ImageBlock so the REPL can
// send it as a query.
type PasteCmd struct {
	// ImageBlock is set after a successful Execute call and can be read by
	// the REPL to inject the image into a query.
	ImageBlock *types.ImageBlock
}

func (c *PasteCmd) Name() string      { return "paste" }
func (c *PasteCmd) Aliases() []string { return nil }
func (c *PasteCmd) Description() string {
	return builtinCommandDescription("paste")
}

func (c *PasteCmd) Execute(ctx *Context, _ string) error {
	c.ImageBlock = nil

	if !input.HasClipboardImage() {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyCommandPasteNoImage))
		return nil
	}

	// Use ctx.Confirm instead of reading stdin directly, because stdin may
	// be owned by readline (non-TUI) or go-tui (TUI mode). If no Confirm
	// callback is provided, default to rejecting (safe default).
	if ctx.Confirm == nil {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyCommandPasteConfirmUnavailable))
		return nil
	}
	if !ctx.Confirm(i18n.Format(ctx.Language, i18n.KeyCommandPasteConfirm, brand.DisplayName)) {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyCommandPasteCancelled))
		return nil
	}

	b64Data, mediaType, err := input.GetClipboardImage()
	if err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyCommandPasteReadError, err))
		return nil
	}
	if b64Data == "" {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyCommandPasteEmpty))
		return nil
	}

	block := types.ImageBlock{
		Type: types.ContentTypeImage,
		Source: &types.ImageSource{
			Type:      "base64",
			MediaType: mediaType,
			Data:      b64Data,
		},
	}
	c.ImageBlock = &block
	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyCommandPasteReady, brand.DisplayName))
	return nil
}
