package input

import "fmt"

const (
	// warnCharThreshold is the input size (in characters) above which a warning
	// is shown to the user.
	warnCharThreshold = 50_000

	// charsPerToken is a rough approximation used for the token estimate shown
	// in the warning message.
	charsPerToken = 4
)

// CheckInputSize returns a formatted warning message when input exceeds
// warnCharThreshold characters, or an empty string if no warning is needed.
//
// Example output:
//
//	⚠️  Large input detected (~52K chars, ~13K tokens). Continue? [Y/n]
func CheckInputSize(input string) string {
	n := len(input)
	if n <= warnCharThreshold {
		return ""
	}

	charK := (n + 500) / 1000
	tokenK := (n/charsPerToken + 500) / 1000

	return fmt.Sprintf("⚠️  Large input detected (~%dK chars, ~%dK tokens). Continue? [Y/n]", charK, tokenK)
}
