package app

import (
	"errors"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// rootLocalizedError keeps machine-checkable causes available through
// errors.Is/errors.As while presenting localized first-party context. Raw OS,
// process, parser, and hook details are retained verbatim after the prefix.
type rootLocalizedError struct {
	message string
	cause   error
}

func (e rootLocalizedError) Error() string { return e.message }
func (e rootLocalizedError) Unwrap() error { return e.cause }

func rootRuntimeError(key i18n.Key, args ...any) error {
	return errors.New(i18n.Format(i18n.DetectOrLoadLanguage(), key, args...))
}

func rootRuntimeWrap(key i18n.Key, cause error, args ...any) error {
	message := i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
	if cause != nil && strings.TrimSpace(cause.Error()) != "" {
		message += ": " + cause.Error()
	}
	return rootLocalizedError{message: message, cause: cause}
}

func rootRuntimeErrorWithCause(key i18n.Key, cause error, args ...any) error {
	return rootLocalizedError{
		message: i18n.Format(i18n.DetectOrLoadLanguage(), key, args...),
		cause:   cause,
	}
}

func rootRuntimeWrapWithRawDetail(key i18n.Key, cause error, detail string, args ...any) error {
	message := i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
	if cause != nil && strings.TrimSpace(cause.Error()) != "" {
		message += ": " + cause.Error()
	}
	if detail = strings.TrimSpace(detail); detail != "" {
		message += ": " + detail
	}
	return rootLocalizedError{message: message, cause: cause}
}
