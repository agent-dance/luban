package catalog

import "testing"

func TestValidationSeverityFailsClosed(t *testing.T) {
	tests := []struct {
		name      string
		severity  string
		wantFatal bool
		wantWarn  bool
	}{
		{name: "warning", severity: validationSeverityWarning, wantWarn: true},
		{name: "case-insensitive warning", severity: " WARNING ", wantWarn: true},
		{name: "fatal", severity: validationSeverityFatal, wantFatal: true},
		{name: "unknown", severity: "future-severity", wantFatal: true},
		{name: "empty", wantFatal: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validation := ValidationError{MCPErrorMetadata: MCPErrorMetadata{Severity: test.severity}}
			if got := validation.IsFatal(); got != test.wantFatal {
				t.Fatalf("IsFatal() = %t, want %t", got, test.wantFatal)
			}
			if got := validation.IsWarning(); got != test.wantWarn {
				t.Fatalf("IsWarning() = %t, want %t", got, test.wantWarn)
			}
		})
	}
}

func TestMCPConfigParseResultFatalQueries(t *testing.T) {
	result := &MCPConfigParseResult{Errors: []ValidationError{
		{MCPErrorMetadata: MCPErrorMetadata{ServerName: "warning-only", Severity: validationSeverityWarning}},
		{Message: "fatal-message", MCPErrorMetadata: MCPErrorMetadata{ServerName: "invalid", Severity: validationSeverityFatal}},
	}}

	validation, ok := result.FirstFatalValidation()
	if !ok || validation.MCPErrorMetadata.ServerName != "invalid" {
		t.Fatalf("FirstFatalValidation() = (%#v, %t)", validation, ok)
	}
	if result.HasFatalValidationForServer("warning-only") {
		t.Fatal("warning-only server was classified as fatal")
	}
	if !result.HasFatalValidationForServer("invalid") {
		t.Fatal("fatal server was not classified as fatal")
	}
}
