package provider

import (
	"os"
	"strings"
)

// parseHeaderLines parses newline-delimited curl-style headers in the form
// "Name: Value". Invalid lines are ignored.
func parseHeaderLines(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	headers := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		value := strings.TrimSpace(line[colon+1:])
		if name == "" {
			continue
		}
		headers[name] = value
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func loadEnvHeaders(envKey string) map[string]string {
	return parseHeaderLines(os.Getenv(envKey))
}
