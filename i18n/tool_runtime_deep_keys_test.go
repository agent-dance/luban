package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestToolRuntimeDeepKeysCoverEveryLanguageAndPreserveRawValues(t *testing.T) {
	tests := []struct {
		key  Key
		args []any
		raw  []string
	}{
		{KeyToolRuntimeBackgroundTaskRunningOtherProcess, []any{"Task", "task-17"}, []string{"Task", "task-17"}},
		{KeyToolRuntimeBackgroundTaskNotRunning, []any{"Task", "task-17", "paused"}, []string{"Task", "task-17", "paused"}},
		{KeyToolRuntimeBackgroundTaskNotFound, []any{"Task", "ID", "task-17"}, []string{"Task", "ID", "task-17"}},
		{KeyToolRuntimeBackgroundOutputDirCreateFailed, []any{errors.New("raw-path-cause")}, []string{"raw-path-cause"}},
		{KeyToolRuntimeBackgroundCommandStartFailed, []any{errors.New("raw-bash-cause")}, []string{"raw-bash-cause"}},
		{KeyToolRuntimeCronSentinelReserved, []any{"sentinel", "<<dynamic>>", "ScheduleWakeup", "<<static>>", "Cron"}, []string{"sentinel", "<<dynamic>>", "ScheduleWakeup", "<<static>>", "Cron"}},
		{KeyToolRuntimeCronPromptSentinelUnknown, []any{"prompt sentinel", "<<unknown>>"}, []string{"prompt sentinel", "<<unknown>>"}},
		{KeyToolRuntimeTeamUniqueNameGenerationFailed, []any{"team"}, []string{"team"}},
	}

	for _, tt := range tests {
		for _, lang := range AllLanguages() {
			got := Format(lang, tt.key, tt.args...)
			if got == "" || got == "["+string(tt.key)+"]" || strings.Contains(got, "%!") {
				t.Fatalf("Format(%s, %q) = %q", lang.Code(), tt.key, got)
			}
			for _, raw := range tt.raw {
				if !strings.Contains(got, raw) {
					t.Errorf("Format(%s, %q) = %q; missing raw value %q", lang.Code(), tt.key, got, raw)
				}
			}
		}
	}
}

func TestToolRuntimeDeepKeysPreserveEnglishCompatibility(t *testing.T) {
	cause := errors.New("raw cause")
	tests := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyToolRuntimeBackgroundTaskRunningOtherProcess, []any{"Task", "task-17"}, "Task task-17 is running in another process and cannot be stopped from this session"},
		{KeyToolRuntimeBackgroundTaskNotRunning, []any{"Task", "task-17", "paused"}, "Task task-17 is not running (status: paused)"},
		{KeyToolRuntimeBackgroundTaskNotFound, []any{"Task", "ID", "task-17"}, "No Task found with ID: task-17"},
		{KeyToolRuntimeBackgroundOutputDirCreateFailed, []any{cause}, "create background task output dir: raw cause"},
		{KeyToolRuntimeBackgroundCommandStartFailed, []any{cause}, "start background command: raw cause"},
		{KeyToolRuntimeCronSentinelReserved, []any{"sentinel", "<<autonomous-loop-dynamic>>", "ScheduleWakeup", "<<autonomous-loop>>", "Cron"}, "sentinel \"<<autonomous-loop-dynamic>>\" is reserved for ScheduleWakeup; use a plain prompt or \"<<autonomous-loop>>\" with Cron"},
		{KeyToolRuntimeCronPromptSentinelUnknown, []any{"prompt sentinel", "<<bogus>>"}, "unknown prompt sentinel \"<<bogus>>\""},
		{KeyToolRuntimeTeamUniqueNameGenerationFailed, []any{"team"}, "failed to generate a unique team name"},
	}

	for _, tt := range tests {
		if got := Format(LangEN, tt.key, tt.args...); got != tt.want {
			t.Errorf("Format(LangEN, %q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestToolRuntimeDeepWrappedErrorsPreserveCause(t *testing.T) {
	cause := errors.New("raw-start-cause")
	err := WrapError(KeyToolRuntimeBackgroundCommandStartFailed, cause)
	if !errors.Is(err, cause) {
		t.Fatal("WrapError did not preserve the background command cause")
	}
	if got := err.Error(); !strings.Contains(got, cause.Error()) || strings.Contains(got, "%!") {
		t.Fatalf("wrapped error = %q", got)
	}
}
