package i18n

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestToolBackgroundAgentKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolBackgroundAgentKeys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Errorf("%s is missing for %s: %q", key, lang.Code(), got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestToolBackgroundAgentEnglishCompatibility(t *testing.T) {
	rawCause := errors.New("raw-cause-42")
	tests := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyToolBackgroundTaskCanceled, nil, "background task canceled"},
		{KeyToolBackgroundCommandTimedOut, nil, "background command timed out"},
		{KeyToolBackgroundCommandTimedOutAfter, []any{3 * time.Second}, "background command timed out after 3s"},
		{KeyToolBackgroundOutputOpenFailed, []any{rawCause}, "open background output file: raw-cause-42"},
		{KeyToolBackgroundAgentEmptyOutput, nil, "(Subagent completed but returned no output.)"},
		{KeyToolBackgroundAgentFailed, nil, "agent failed"},
		{KeyToolBackgroundAgentCanceledWithCause, []any{context.Canceled}, "context canceled"},
		{KeyToolBackgroundAgentTimedOutWithCause, []any{context.DeadlineExceeded}, "context deadline exceeded"},
	}
	for _, test := range tests {
		if got := Format(LangEN, test.key, test.args...); got != test.want {
			t.Errorf("Format(LangEN, %s) = %q, want %q", test.key, got, test.want)
		}
	}
}

func TestToolBackgroundAgentErrorsUseRuntimeLanguageAndPreserveCause(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	cause := errors.New("raw-cause-42")
	openErr := WrapError(KeyToolBackgroundOutputOpenFailed, cause)
	cancelErr := WrapError(KeyToolBackgroundAgentCanceledWithCause, context.Canceled)
	timeoutErr := WrapError(KeyToolBackgroundAgentTimedOutWithCause, context.DeadlineExceeded)
	for name, test := range map[string]struct {
		err   error
		cause error
	}{
		"open": {openErr, cause}, "cancel": {cancelErr, context.Canceled}, "timeout": {timeoutErr, context.DeadlineExceeded},
	} {
		if !errors.Is(test.err, test.cause) {
			t.Errorf("%s error did not preserve its cause", name)
		}
	}

	detectedLanguageCache.Store(int32(LangEN))
	english := []string{openErr.Error(), cancelErr.Error(), timeoutErr.Error()}
	detectedLanguageCache.Store(int32(LangZH))
	chinese := []string{openErr.Error(), cancelErr.Error(), timeoutErr.Error()}
	for i := range english {
		if english[i] == chinese[i] {
			t.Errorf("runtime language did not change error %d: %q", i, english[i])
		}
	}
	if !strings.Contains(chinese[0], "raw-cause-42") {
		t.Fatalf("localized open error omitted raw cause: %q", chinese[0])
	}
	if !errors.Is(cancelErr, context.Canceled) {
		t.Fatal("localized cancel error did not preserve context.Canceled")
	}
	if !errors.Is(timeoutErr, context.DeadlineExceeded) {
		t.Fatal("localized timeout error did not preserve context.DeadlineExceeded")
	}
}
