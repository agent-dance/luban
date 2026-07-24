package i18n

import "testing"

func TestPresentationSemanticKeysAreLocalized(t *testing.T) {
	tests := []struct {
		language Language
		key      Key
		args     []any
		want     string
	}{
		{LangZH, KeyPresentationRunning, []any{"⠋", "Bash", 1.2}, "⠋ 正在运行 Bash…（1.2 秒）"},
		{LangJA, KeyPresentationDetailsAvailable, nil, "詳細あり"},
		{LangRU, KeyPresentationAggregateOperations, []any{"Чтение", "2"}, "Чтение · 2 операций"},
	}
	for _, tt := range tests {
		if got := Format(tt.language, tt.key, tt.args...); got != tt.want {
			t.Errorf("Format(%s, %q) = %q, want %q", tt.language.Code(), tt.key, got, tt.want)
		}
	}
}
