package file

import (
	"os"
	"strconv"
	"strings"
)

const fileReadMaxOutputTokensEnv = "LUBAN_CODE_FILE_READ_MAX_OUTPUT_TOKENS"

// FileReadingLimits is the effective text/notebook Read budget.
type FileReadingLimits struct {
	MaxTokens    int
	MaxSizeBytes int64
}

func defaultFileReadingLimits() FileReadingLimits {
	return FileReadingLimits{
		MaxTokens:    defaultReadMaxTokens,
		MaxSizeBytes: defaultReadMaxSizeBytes,
	}
}

func resolveFileReadingLimits() FileReadingLimits {
	limits := defaultFileReadingLimits()
	if raw := strings.TrimSpace(os.Getenv(fileReadMaxOutputTokensEnv)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limits.MaxTokens = parsed
		}
	}
	return limits
}
