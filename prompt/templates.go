package prompt

import (
	"strings"

	"github.com/agent-dance/luban/brand"
)

const originalProductName = "Claude Code"

func productName() string {
	return brand.DisplayName
}

func brandPromptText(text string) string {
	return strings.ReplaceAll(text, originalProductName, productName())
}
