package interaction

// SendUserMessageAttachment is renderer-facing attachment metadata.
// Attachment bytes deliberately never enter the model-visible tool result.
type SendUserMessageAttachment struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsImage bool   `json:"isImage"`
}

// SendUserMessageOutput is the surface-neutral value shared by the tool
// dispatcher and presentation renderers. The provider sees only the tool's
// compact acknowledgement; local renderers consume this typed value.
type SendUserMessageOutput struct {
	Message     string                      `json:"message"`
	Attachments []SendUserMessageAttachment `json:"attachments,omitempty"`
	SentAt      string                      `json:"sentAt,omitempty"`
}
