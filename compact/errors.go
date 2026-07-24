package compact

import (
	"context"
	"errors"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

var (
	ErrCompactNotEnoughMessages = errors.New("compact: not enough messages")
	ErrCompactUserAbort         = errors.New("compact: user abort")
	ErrCompactIncomplete        = errors.New("compact: incomplete response")
	ErrCompactPromptTooLong     = errors.New("compact: prompt too long")
	ErrCompactAPI               = errors.New("compact: api error")
	ErrCompactNoSummary         = errors.New("compact: no summary response")
)

const (
	MessageNotEnoughMessages = "Not enough messages to compact."
	MessageUserAbort         = "API Error: Request was aborted."
	MessageIncomplete        = "Compaction interrupted · This may be due to network issues — please try again."
	MessagePromptTooLong     = "Conversation too long. Press esc twice to go up a few messages and try again."
	MessageNoSummary         = "Failed to generate conversation summary - response did not contain valid text content"
)

// CompactError carries a stable compaction failure category while preserving
// the underlying provider/context error for errors.Is/errors.As callers.
type CompactError struct {
	Kind    error
	Message string
	Cause   error
}

func (e *CompactError) Error() string {
	if e == nil {
		return "compact failed"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Kind != nil {
		return e.Kind.Error()
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return "compact failed"
}

func (e *CompactError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *CompactError) Is(target error) bool {
	return e != nil && e.Kind != nil && target == e.Kind
}

func compactError(kind error, message string, cause error) error {
	return &CompactError{Kind: kind, Message: message, Cause: cause}
}

func compactUserAbortError(cause error) error {
	if cause == nil {
		cause = context.Canceled
	}
	return compactError(ErrCompactUserAbort, MessageUserAbort, cause)
}

func compactSummaryAPIError(summary string) error {
	return compactError(ErrCompactAPI, strings.TrimSpace(summary), nil)
}

func isCompactUserAbortCause(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, ErrCompactUserAbort)
}

func startsWithAPIErrorPrefix(text string) bool {
	text = strings.TrimSpace(text)
	return strings.HasPrefix(text, "API Error") ||
		strings.HasPrefix(text, "Please run /login · API Error")
}

func IsCompactNotEnoughMessagesError(err error) bool {
	return errors.Is(err, ErrCompactNotEnoughMessages)
}

func IsCompactUserAbortError(err error) bool {
	return isCompactUserAbortCause(err)
}

func IsCompactIncompleteResponseError(err error) bool {
	return errors.Is(err, ErrCompactIncomplete)
}

func IsCompactAPIError(err error) bool {
	return errors.Is(err, ErrCompactAPI)
}

func IsCompactNoSummaryError(err error) bool {
	return errors.Is(err, ErrCompactNoSummary)
}

// HasUserErrorCategory reports whether err has first-party compaction copy
// that should be localized before it reaches a renderer.
func HasUserErrorCategory(err error) bool {
	return IsCompactNotEnoughMessagesError(err) ||
		errors.Is(err, ErrCompactUserAbort) ||
		IsCompactIncompleteResponseError(err) ||
		errors.Is(err, ErrCompactPromptTooLong) ||
		IsCompactNoSummaryError(err)
}

type localizedCompactUserError struct {
	text string
	err  error
}

func (e *localizedCompactUserError) Error() string { return e.text }
func (e *localizedCompactUserError) Unwrap() error { return e.err }

// LocalizeUserError preserves the original error chain while replacing its
// display text with semantic copy for the active runtime language.
func LocalizeUserError(lang i18n.Language, err error) error {
	if err == nil {
		return nil
	}
	return &localizedCompactUserError{text: FormatCompactUserError(lang, err), err: err}
}

// FormatCompactUserError returns localized user-facing text for manual compact
// surfaces while preserving the original errors for diagnostics and matching.
func FormatCompactUserError(lang i18n.Language, err error) string {
	switch {
	case err == nil:
		return ""
	case IsCompactNotEnoughMessagesError(err):
		return i18n.Text(lang, i18n.KeyAuxCompactNotEnoughMessages)
	case IsCompactUserAbortError(err):
		return i18n.Text(lang, i18n.KeyAuxCompactCancelled)
	case IsCompactIncompleteResponseError(err):
		return i18n.Text(lang, i18n.KeyAuxCompactInterrupted)
	case errors.Is(err, ErrCompactPromptTooLong):
		return i18n.Text(lang, i18n.KeyAuxCompactConversationLong)
	case IsCompactNoSummaryError(err):
		return i18n.Text(lang, i18n.KeyAuxCompactSummaryMissing)
	default:
		return i18n.Format(lang, i18n.KeyAuxCompactFailed, err)
	}
}
