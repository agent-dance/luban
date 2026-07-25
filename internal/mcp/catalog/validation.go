package catalog

import "strings"

const (
	validationSeverityWarning = "warning"
	validationSeverityFatal   = "fatal"
)

// IsWarning reports whether a validation diagnostic is explicitly
// non-blocking. Unknown severities fail closed through IsFatal.
func (e ValidationError) IsWarning() bool {
	return strings.EqualFold(strings.TrimSpace(e.MCPErrorMetadata.Severity), validationSeverityWarning)
}

// IsFatal reports whether a validation diagnostic must block registration.
// Treating unknown severities as fatal prevents newly introduced diagnostics
// from being silently ignored by older consumers.
func (e ValidationError) IsFatal() bool {
	return !e.IsWarning()
}

// FirstFatalValidation returns the first registration-blocking diagnostic.
func (r *MCPConfigParseResult) FirstFatalValidation() (ValidationError, bool) {
	if r == nil {
		return ValidationError{}, false
	}
	for _, validation := range r.Errors {
		if validation.IsFatal() {
			return validation, true
		}
	}
	return ValidationError{}, false
}

// HasFatalValidationForServer reports whether a specific parsed server is
// invalid and therefore must not be registered.
func (r *MCPConfigParseResult) HasFatalValidationForServer(serverName string) bool {
	if r == nil {
		return false
	}
	for _, validation := range r.Errors {
		if validation.MCPErrorMetadata.ServerName == serverName && validation.IsFatal() {
			return true
		}
	}
	return false
}

// FatalConfigValidationError preserves the structured fatal diagnostic while
// satisfying error-returning initialization boundaries.
type FatalConfigValidationError struct {
	Validation ValidationError
}

func (e *FatalConfigValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Validation.Message
}
