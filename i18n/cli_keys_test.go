package i18n

import (
	"strings"
	"testing"
)

func TestCLISemanticCopyCoversEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyCLIUsage, KeyCLIHelpCodingSurface, KeyCLIOptions, KeyCLIExamples, KeyCLIExampleInteractive,
		KeyCLIExamplePrint, KeyCLIExampleModel, KeyCLIExampleAllowedDir,
		KeyCLIFlagDefault, KeyCLIError, KeyCLIParseFailure,
		KeyCLIInvalidSessionChars, KeyCLIInvalidSessionParent,
		KeyCLIInputModeSDKPrint, KeyCLIStdinReadFailure, KeyCLIStdinTooLarge,
		KeyCLIScreenReaderSDK, KeyCLIScreenReaderPrint, KeyCLIScreenReaderOutput,
		KeyCLIScreenReaderTerminal, KeyCLIWorkingDirectoryError,
		KeyCLIFlagModel, KeyCLIFlagProvider, KeyCLIFlagAPI, KeyCLIFlagReasoningEffort, KeyCLIFlagPinnedModel, KeyCLIFlagPrint,
		KeyCLIFlagResume, KeyCLIFlagSessionID, KeyCLIFlagMaxTurns,
		KeyCLIFlagSystemPrompt, KeyCLIFlagAllowedDir, KeyCLIFlagAllowAll,
		KeyCLIFlagAllowedTools, KeyCLIFlagDisallowedTools, KeyCLIFlagSandbox, KeyCLIFlagForceSandboxTools,
		KeyCLIFlagSDK, KeyCLIFlagVersion, KeyCLIFlagVerbose, KeyCLIFlagDebugFile,
		KeyCLIFlagNoColor, KeyCLIFlagOutputFormat, KeyCLIFlagQuiet,
		KeyCLIFlagScreenReader, KeyCLIFlagAgents, KeyCLIFlagPromptDump,
		KeyCLIFlagPromptDumpJSON, KeyCLIFlagLanguage, KeyCLIFlagOutputStyle,
		KeyCLIFlagAllowedDomains, KeyCLIFlagDisallowedDomains,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || strings.HasPrefix(got, "[") {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}

func TestCLIInputTransportCopyCoversEveryLanguage(t *testing.T) {
	for _, key := range []Key{KeyCLIInputModeSDKPrint, KeyCLIStdinReadFailure, KeyCLIStdinTooLarge} {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || strings.HasPrefix(got, "[") {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}
