package web

import (
	"strings"

	"github.com/agent-dance/luban/i18n"
)

func formatRedirectMarker(originalURL, redirectURL string, code int, prompt string) string {
	originalURL = strings.TrimSpace(originalURL)
	redirectURL = strings.TrimSpace(redirectURL)
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolWebRedirectMarker,
		originalURL, redirectURL, code, redirectStatusText(code), redirectURL, prompt)
}

func redirectStatusText(code int) string {
	switch code {
	case 301:
		return "Moved Permanently"
	case 308:
		return "Permanent Redirect"
	case 307:
		return "Temporary Redirect"
	default:
		return "Found"
	}
}
