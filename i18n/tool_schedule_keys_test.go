package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestToolScheduleKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolScheduleKeys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestToolScheduleKeyFormatting(t *testing.T) {
	cause := errors.New("disk-cause-42")
	tests := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyToolScheduleInvalidExpression, []any{"*/5 * * * *", cause}, `Invalid cron expression "*/5 * * * *": disk-cause-42`},
		{KeyToolScheduleTooMany, []any{50}, "Scheduled job limit reached (50). Cancel a job before creating another."},
		{KeyToolScheduleStoreReadFailed, []any{cause}, "Could not read .luban-code/schedule/jobs.json: disk-cause-42"},
		{KeyToolScheduleStoreVersion, []any{3}, "Schedule data version 3 is not supported."},
		{KeyToolScheduleEnqueueFailed, []any{"job-42", cause}, "Could not queue scheduled job job-42: disk-cause-42"},
		{KeyToolScheduleStopFailed, []any{cause}, "Could not stop the schedule service: disk-cause-42"},
		{KeyToolScheduleExecutionDescription, []any{"job-42"}, "Run scheduled job job-42."},
		{KeyToolScheduleCreatedRecurring, []any{"job-42", "2030-01-02T03:04:05Z"}, "Created recurring scheduled job job-42; next run: 2030-01-02T03:04:05Z."},
		{KeyToolScheduleListRow, []any{"job-42", "*/5 * * * *", "soon", "persistent"}, "job-42 | */5 * * * * | next: soon | persistent"},
	}
	for _, test := range tests {
		if got := Format(LangEN, test.key, test.args...); got != test.want {
			t.Errorf("Format(LangEN, %s) = %q, want %q", test.key, got, test.want)
		}
	}

	for _, lang := range AllLanguages() {
		for _, test := range tests {
			if got := Format(lang, test.key, test.args...); strings.Contains(got, "%!") {
				t.Errorf("Format(%s, %s) has invalid formatting: %q", lang.Code(), test.key, got)
			}
		}
	}
}

func TestToolScheduleCopyUsesCurrentContract(t *testing.T) {
	forbidden := []string{"claude", "typescript", "scheduled_tasks", "7 days", "legacy"}
	for _, key := range toolScheduleKeys {
		for _, lang := range AllLanguages() {
			copy := strings.ToLower(Text(lang, key))
			for _, fragment := range forbidden {
				if strings.Contains(copy, fragment) {
					t.Errorf("Text(%s, %s) contains stale fragment %q: %q", lang.Code(), key, fragment, copy)
				}
			}
		}
	}

	for _, key := range []Key{
		KeyToolScheduleSchemaDurable,
		KeyToolScheduleStoreReadFailed,
		KeyToolScheduleStoreWriteFailed,
		KeyToolScheduleStoreCorrupt,
		KeyToolScheduleCreatedPersisted,
	} {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); !strings.Contains(got, ".luban-code/schedule/jobs.json") {
				t.Errorf("Text(%s, %s) omitted the current store path: %q", lang.Code(), key, got)
			}
		}
	}
}
