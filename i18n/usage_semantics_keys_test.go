package i18n

import "testing"

func TestUsageSemanticsNarrowCJKUsesFullLabels(t *testing.T) {
	tests := []struct {
		lang Language
		key  Key
		args []any
		want string
	}{
		{LangZH, KeyUsageSessionNarrow, []any{"9.4K", 40, "302", "$", 0.04}, "会话：输入 9.4K · 缓存 40% · 输出 302 · $0.04"},
		{LangZH, KeyUsageSessionNarrowUnknownCost, []any{"9.4K", 40, "302"}, "会话：输入 9.4K · 缓存 40% · 输出 302 · 费用未知"},
		{LangZH, KeyUsageSessionCompactedNarrow, []any{"9.4K", "12K", 40, "302", "$", 0.04}, "会话：输入 9.4K/12K · 缓存 40% · 输出 302 · $0.04"},
		{LangZH, KeyUsageSessionCompactedNarrowUnknownCost, []any{"9.4K", "12K", 40, "302"}, "会话：输入 9.4K/12K · 缓存 40% · 输出 302 · 费用未知"},
		{LangZH, KeyUsageSessionNarrowNoCache, []any{"9.4K", "302", "$", 0.04}, "会话：输入 9.4K · 输出 302 · $0.04"},
		{LangZH, KeyUsageSessionNarrowNoCacheUnknownCost, []any{"9.4K", "302"}, "会话：输入 9.4K · 输出 302 · 费用未知"},
		{LangZH, KeyUsageSessionCompactedNarrowNoCache, []any{"9.4K", "12K", "302", "$", 0.04}, "会话：输入 9.4K/12K · 输出 302 · $0.04"},
		{LangZH, KeyUsageSessionCompactedNarrowNoCacheUnknownCost, []any{"9.4K", "12K", "302"}, "会话：输入 9.4K/12K · 输出 302 · 费用未知"},
		{LangJA, KeyUsageSessionNarrow, []any{"9.4K", 40, "302", "$", 0.04}, "セッション: 入力 9.4K · キャッシュ 40% · 出力 302 · $0.04"},
		{LangJA, KeyUsageSessionNarrowUnknownCost, []any{"9.4K", 40, "302"}, "セッション: 入力 9.4K · キャッシュ 40% · 出力 302 · 料金不明"},
		{LangJA, KeyUsageSessionCompactedNarrow, []any{"9.4K", "12K", 40, "302", "$", 0.04}, "セッション: 入力 9.4K/12K · キャッシュ 40% · 出力 302 · $0.04"},
		{LangJA, KeyUsageSessionCompactedNarrowUnknownCost, []any{"9.4K", "12K", 40, "302"}, "セッション: 入力 9.4K/12K · キャッシュ 40% · 出力 302 · 料金不明"},
		{LangKO, KeyUsageSessionNarrow, []any{"9.4K", 40, "302", "$", 0.04}, "세션: 입력 9.4K · 캐시 40% · 출력 302 · $0.04"},
		{LangKO, KeyUsageSessionNarrowUnknownCost, []any{"9.4K", 40, "302"}, "세션: 입력 9.4K · 캐시 40% · 출력 302 · 비용 알 수 없음"},
		{LangKO, KeyUsageSessionCompactedNarrow, []any{"9.4K", "12K", 40, "302", "$", 0.04}, "세션: 입력 9.4K/12K · 캐시 40% · 출력 302 · $0.04"},
		{LangKO, KeyUsageSessionCompactedNarrowUnknownCost, []any{"9.4K", "12K", 40, "302"}, "세션: 입력 9.4K/12K · 캐시 40% · 출력 302 · 비용 알 수 없음"},
	}
	for _, tt := range tests {
		if got := Format(tt.lang, tt.key, tt.args...); got != tt.want {
			t.Errorf("Format(%s, %s) = %q, want %q", tt.lang, tt.key, got, tt.want)
		}
	}
}

func TestUsageSemanticsFormatsNativeCurrencySymbol(t *testing.T) {
	got := Format(LangZH, KeyUsageSessionNarrow, "9.4K", 40, "302", "¥", 0.04)
	if want := "会话：输入 9.4K · 缓存 40% · 输出 302 · ¥0.04"; got != want {
		t.Fatalf("Format() = %q, want %q", got, want)
	}
}

func TestUsageSemanticsKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyUsageSession, KeyUsageSessionUnknownCost, KeyUsageSessionNoCache, KeyUsageSessionNoCacheUnknownCost,
		KeyUsageSessionCompacted, KeyUsageSessionCompactedUnknownCost, KeyUsageSessionCompactedNoCache,
		KeyUsageSessionCompactedNoCacheUnknownCost, KeyUsageSessionUnavailable,
		KeyUsageSessionNarrow, KeyUsageSessionNarrowUnknownCost,
		KeyUsageSessionNarrowNoCache, KeyUsageSessionNarrowNoCacheUnknownCost,
		KeyUsageSessionCompactedNarrow, KeyUsageSessionCompactedNarrowUnknownCost,
		KeyUsageSessionCompactedNarrowNoCache, KeyUsageSessionCompactedNarrowNoCacheUnknownCost,
		KeyUsageContext, KeyUsageContextCompact, KeyUsageContextPlain,
		KeyUsageContextEstimated, KeyUsageContextEstimatedCompact, KeyUsageContextEstimatedPlain,
		KeyUsageContextLowerBound, KeyUsageContextLowerBoundCompact, KeyUsageContextLowerBoundPlain,
		KeyUsageContextUnknown,
		KeyCommandContextUsageExact, KeyCommandContextUsageEstimated, KeyCommandContextUsageLowerBound,
		KeyCommandContextRemainingEstimated, KeyCommandContextRemainingUpperBound,
		KeyCommandContextSourceProvider, KeyCommandContextSourceEstimate, KeyCommandContextSourceLowerBound,
		KeyUsageLastRequest, KeyUsageLastRequestUnknown,
		KeyUsageCumulativeSession, KeyUsageCumulativeUnknown,
		KeyUsageCumulativeUnavailable, KeyUsageScopedCompact, KeyUsageScopedCompactUnknownCost,
		KeyUsageEffectiveContext, KeyUsageEffectiveContextCompact,
		KeyUsageEffectiveContextPlain, KeyUsageEffectiveUnknown,
	}
	for _, lang := range AllLanguages() {
		for _, key := range keys {
			if got := Text(lang, key); got == "" || got == string(key) {
				t.Fatalf("missing %s translation for %s", key, lang)
			}
		}
	}
}
