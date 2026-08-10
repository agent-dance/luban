package i18n

const (
	KeyUsageSession                                  Key = "usage.session"
	KeyUsageSessionUnknownCost                       Key = "usage.session.unknown_cost"
	KeyUsageSessionNoCache                           Key = "usage.session.no_cache"
	KeyUsageSessionNoCacheUnknownCost                Key = "usage.session.no_cache.unknown_cost"
	KeyUsageSessionCompacted                         Key = "usage.session.compacted"
	KeyUsageSessionCompactedUnknownCost              Key = "usage.session.compacted.unknown_cost"
	KeyUsageSessionCompactedNoCache                  Key = "usage.session.compacted.no_cache"
	KeyUsageSessionCompactedNoCacheUnknownCost       Key = "usage.session.compacted.no_cache.unknown_cost"
	KeyUsageSessionNarrow                            Key = "usage.session.narrow"
	KeyUsageSessionNarrowUnknownCost                 Key = "usage.session.narrow.unknown_cost"
	KeyUsageSessionNarrowNoCache                     Key = "usage.session.narrow.no_cache"
	KeyUsageSessionNarrowNoCacheUnknownCost          Key = "usage.session.narrow.no_cache.unknown_cost"
	KeyUsageSessionCompactedNarrow                   Key = "usage.session.compacted.narrow"
	KeyUsageSessionCompactedNarrowUnknownCost        Key = "usage.session.compacted.narrow.unknown_cost"
	KeyUsageSessionCompactedNarrowNoCache            Key = "usage.session.compacted.narrow.no_cache"
	KeyUsageSessionCompactedNarrowNoCacheUnknownCost Key = "usage.session.compacted.narrow.no_cache.unknown_cost"
	KeyUsageSessionUnavailable                       Key = "usage.session.unavailable"
	KeyUsageContext                                  Key = "usage.model_context"
	KeyUsageContextCompact                           Key = "usage.model_context.compact"
	KeyUsageContextPlain                             Key = "usage.model_context.plain"
	KeyUsageContextEstimated                         Key = "usage.model_context.estimated"
	KeyUsageContextEstimatedCompact                  Key = "usage.model_context.estimated.compact"
	KeyUsageContextEstimatedPlain                    Key = "usage.model_context.estimated.plain"
	KeyUsageContextLowerBound                        Key = "usage.model_context.lower_bound"
	KeyUsageContextLowerBoundCompact                 Key = "usage.model_context.lower_bound.compact"
	KeyUsageContextLowerBoundPlain                   Key = "usage.model_context.lower_bound.plain"
	KeyUsageContextUnknown                           Key = "usage.model_context.unknown"

	KeyUsageLastRequest              Key = "usage.last_request"
	KeyUsageLastRequestUnknown       Key = "usage.last_request.unknown"
	KeyUsageCumulativeSession        Key = "usage.cumulative_session"
	KeyUsageCumulativeUnknown        Key = "usage.cumulative_session.unknown_cost"
	KeyUsageCumulativeUnavailable    Key = "usage.cumulative_session.unavailable"
	KeyUsageScopedCompact            Key = "usage.scoped.compact"
	KeyUsageScopedCompactUnknownCost Key = "usage.scoped.compact.unknown_cost"
	KeyUsageEffectiveContext         Key = "usage.effective_model_context"
	KeyUsageEffectiveContextCompact  Key = "usage.effective_model_context.compact"
	KeyUsageEffectiveContextPlain    Key = "usage.effective_model_context.plain"
	KeyUsageEffectiveUnknown         Key = "usage.effective_model_context.unknown"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyUsageSession,
		"Session total: in %s · %d%% cached · out %s · %s%.4f",
		"会话累计：输入 %s · 缓存 %d%% · 输出 %s · %s%.4f",
		"Sitzung gesamt: %s ein · %d%% im Cache · %s aus · %s%.4f",
		"セッション累計: 入力 %s · キャッシュ %d%% · 出力 %s · %s%.4f",
		"세션 누계: 입력 %s · 캐시 %d%% · 출력 %s · %s%.4f",
		"Сеанс суммарно: вход %s · %d%% из кэша · выход %s · %s%.4f")
	add(KeyUsageSessionUnknownCost,
		"Session total: in %s · %d%% cached · out %s · cost unknown",
		"会话累计：输入 %s · 缓存 %d%% · 输出 %s · 费用未知",
		"Sitzung gesamt: %s ein · %d%% im Cache · %s aus · Kosten unbekannt",
		"セッション累計: 入力 %s · キャッシュ %d%% · 出力 %s · 料金不明",
		"세션 누계: 입력 %s · 캐시 %d%% · 출력 %s · 비용 알 수 없음",
		"Сеанс суммарно: вход %s · %d%% из кэша · выход %s · стоимость неизвестна")
	add(KeyUsageSessionNoCache,
		"Session total: in %s · out %s · %s%.4f",
		"会话累计：输入 %s · 输出 %s · %s%.4f",
		"Sitzung gesamt: %s ein · %s aus · %s%.4f",
		"セッション累計: 入力 %s · 出力 %s · %s%.4f",
		"세션 누계: 입력 %s · 출력 %s · %s%.4f",
		"Сеанс суммарно: вход %s · выход %s · %s%.4f")
	add(KeyUsageSessionNoCacheUnknownCost,
		"Session total: in %s · out %s · cost unknown",
		"会话累计：输入 %s · 输出 %s · 费用未知",
		"Sitzung gesamt: %s ein · %s aus · Kosten unbekannt",
		"セッション累計: 入力 %s · 出力 %s · 料金不明",
		"세션 누계: 입력 %s · 출력 %s · 비용 알 수 없음",
		"Сеанс суммарно: вход %s · выход %s · стоимость неизвестна")
	add(KeyUsageSessionCompacted,
		"Session: in %s (%s total) · %d%% cached · out %s · %s%.4f",
		"会话：输入 %s（累计 %s）· 缓存 %d%% · 输出 %s · %s%.4f",
		"Sitzung: %s ein (%s gesamt) · %d%% im Cache · %s aus · %s%.4f",
		"セッション: 入力 %s（合計 %s）· キャッシュ %d%% · 出力 %s · %s%.4f",
		"세션: 입력 %s (총 %s) · 캐시 %d%% · 출력 %s · %s%.4f",
		"Сеанс: вход %s (всего %s) · %d%% из кэша · выход %s · %s%.4f")
	add(KeyUsageSessionCompactedUnknownCost,
		"Session: in %s (%s total) · %d%% cached · out %s · cost unknown",
		"会话：输入 %s（累计 %s）· 缓存 %d%% · 输出 %s · 费用未知",
		"Sitzung: %s ein (%s gesamt) · %d%% im Cache · %s aus · Kosten unbekannt",
		"セッション: 入力 %s（合計 %s）· キャッシュ %d%% · 出力 %s · 料金不明",
		"세션: 입력 %s (총 %s) · 캐시 %d%% · 출력 %s · 비용 알 수 없음",
		"Сеанс: вход %s (всего %s) · %d%% из кэша · выход %s · стоимость неизвестна")
	add(KeyUsageSessionCompactedNoCache,
		"Session: in %s (%s total) · out %s · %s%.4f",
		"会话：输入 %s（累计 %s）· 输出 %s · %s%.4f",
		"Sitzung: %s ein (%s gesamt) · %s aus · %s%.4f",
		"セッション: 入力 %s（合計 %s）· 出力 %s · %s%.4f",
		"세션: 입력 %s (총 %s) · 출력 %s · %s%.4f",
		"Сеанс: вход %s (всего %s) · выход %s · %s%.4f")
	add(KeyUsageSessionCompactedNoCacheUnknownCost,
		"Session: in %s (%s total) · out %s · cost unknown",
		"会话：输入 %s（累计 %s）· 输出 %s · 费用未知",
		"Sitzung: %s ein (%s gesamt) · %s aus · Kosten unbekannt",
		"セッション: 入力 %s（合計 %s）· 出力 %s · 料金不明",
		"세션: 입력 %s (총 %s) · 출력 %s · 비용 알 수 없음",
		"Сеанс: вход %s (всего %s) · выход %s · стоимость неизвестна")
	add(KeyUsageSessionUnavailable,
		"Session total: usage unknown", "会话累计：用量未知", "Sitzung gesamt: Nutzung unbekannt",
		"セッション累計: 使用量不明", "세션 누계: 사용량 알 수 없음", "Сеанс суммарно: использование неизвестно")
	add(KeyUsageSessionNarrow,
		"Total: in %s · %d%% cached · out %s · %s%.2f", "会话累计：输入 %s · 缓存 %d%% · 输出 %s · %s%.2f", "Gesamt: %s ein · %d%% im Cache · %s aus · %s%.2f",
		"セッション累計: 入力 %s · キャッシュ %d%% · 出力 %s · %s%.2f", "세션 누계: 입력 %s · 캐시 %d%% · 출력 %s · %s%.2f", "Итого: вход %s · %d%% из кэша · выход %s · %s%.2f")
	add(KeyUsageSessionNarrowUnknownCost,
		"Total: in %s · %d%% cached · out %s · cost unknown", "会话累计：输入 %s · 缓存 %d%% · 输出 %s · 费用未知", "Gesamt: %s ein · %d%% im Cache · %s aus · Kosten unbekannt",
		"セッション累計: 入力 %s · キャッシュ %d%% · 出力 %s · 料金不明", "세션 누계: 입력 %s · 캐시 %d%% · 출력 %s · 비용 알 수 없음", "Итого: вход %s · %d%% из кэша · выход %s · стоимость неизвестна")
	add(KeyUsageSessionNarrowNoCache,
		"Total: in %s · out %s · %s%.2f", "会话累计：输入 %s · 输出 %s · %s%.2f", "Gesamt: %s ein · %s aus · %s%.2f",
		"セッション累計: 入力 %s · 出力 %s · %s%.2f", "세션 누계: 입력 %s · 출력 %s · %s%.2f", "Итого: вход %s · выход %s · %s%.2f")
	add(KeyUsageSessionNarrowNoCacheUnknownCost,
		"Total: in %s · out %s · cost unknown", "会话累计：输入 %s · 输出 %s · 费用未知", "Gesamt: %s ein · %s aus · Kosten unbekannt",
		"セッション累計: 入力 %s · 出力 %s · 料金不明", "세션 누계: 입력 %s · 출력 %s · 비용 알 수 없음", "Итого: вход %s · выход %s · стоимость неизвестна")
	add(KeyUsageSessionCompactedNarrow,
		"S: in %s/%s · %d%% cached · out %s · %s%.2f", "会话：输入 %s/%s · 缓存 %d%% · 输出 %s · %s%.2f", "S: %s/%s ein · %d%% im Cache · %s aus · %s%.2f",
		"セッション: 入力 %s/%s · キャッシュ %d%% · 出力 %s · %s%.2f", "세션: 입력 %s/%s · 캐시 %d%% · 출력 %s · %s%.2f", "С: вход %s/%s · %d%% из кэша · выход %s · %s%.2f")
	add(KeyUsageSessionCompactedNarrowUnknownCost,
		"S: in %s/%s · %d%% cached · out %s · cost unknown", "会话：输入 %s/%s · 缓存 %d%% · 输出 %s · 费用未知", "S: %s/%s ein · %d%% im Cache · %s aus · Kosten unbekannt",
		"セッション: 入力 %s/%s · キャッシュ %d%% · 出力 %s · 料金不明", "세션: 입력 %s/%s · 캐시 %d%% · 출력 %s · 비용 알 수 없음", "С: вход %s/%s · %d%% из кэша · выход %s · стоимость неизвестна")
	add(KeyUsageSessionCompactedNarrowNoCache,
		"S: in %s/%s · out %s · %s%.2f", "会话：输入 %s/%s · 输出 %s · %s%.2f", "S: %s/%s ein · %s aus · %s%.2f",
		"セッション: 入力 %s/%s · 出力 %s · %s%.2f", "세션: 입력 %s/%s · 출력 %s · %s%.2f", "С: вход %s/%s · выход %s · %s%.2f")
	add(KeyUsageSessionCompactedNarrowNoCacheUnknownCost,
		"S: in %s/%s · out %s · cost unknown", "会话：输入 %s/%s · 输出 %s · 费用未知", "S: %s/%s ein · %s aus · Kosten unbekannt",
		"セッション: 入力 %s/%s · 出力 %s · 料金不明", "세션: 입력 %s/%s · 출력 %s · 비용 알 수 없음", "С: вход %s/%s · выход %s · стоимость неизвестна")
	add(KeyUsageContext,
		"Context: %s %d%% (%s/%s)", "上下文：%s %d%%（%s/%s）", "Kontext: %s %d%% (%s/%s)",
		"コンテキスト: %s %d%%（%s/%s）", "컨텍스트: %s %d%% (%s/%s)", "Контекст: %s %d%% (%s/%s)")
	add(KeyUsageContextCompact,
		"Context: %s %d%%", "上下文：%s %d%%", "Kontext: %s %d%%",
		"コンテキスト: %s %d%%", "컨텍스트: %s %d%%", "Контекст: %s %d%%")
	add(KeyUsageContextPlain,
		"Context: %d%% (%s/%s)", "上下文：%d%%（%s/%s）", "Kontext: %d%% (%s/%s)",
		"コンテキスト: %d%%（%s/%s）", "컨텍스트: %d%% (%s/%s)", "Контекст: %d%% (%s/%s)")
	add(KeyUsageContextEstimated,
		"Context: %s ≈%d%% (%s/%s)", "上下文：%s ≈%d%%（%s/%s）", "Kontext: %s ≈%d%% (%s/%s)",
		"コンテキスト: %s ≈%d%%（%s/%s）", "컨텍스트: %s ≈%d%% (%s/%s)", "Контекст: %s ≈%d%% (%s/%s)")
	add(KeyUsageContextEstimatedCompact,
		"Context: %s ≈%d%%", "上下文：%s ≈%d%%", "Kontext: %s ≈%d%%",
		"コンテキスト: %s ≈%d%%", "컨텍스트: %s ≈%d%%", "Контекст: %s ≈%d%%")
	add(KeyUsageContextEstimatedPlain,
		"Context: ≈%d%% (%s/%s)", "上下文：≈%d%%（%s/%s）", "Kontext: ≈%d%% (%s/%s)",
		"コンテキスト: ≈%d%%（%s/%s）", "컨텍스트: ≈%d%% (%s/%s)", "Контекст: ≈%d%% (%s/%s)")
	add(KeyUsageContextLowerBound,
		"Context: %s ≥%d%% (%s/%s)", "上下文：%s ≥%d%%（%s/%s）", "Kontext: %s ≥%d%% (%s/%s)",
		"コンテキスト: %s ≥%d%%（%s/%s）", "컨텍스트: %s ≥%d%% (%s/%s)", "Контекст: %s ≥%d%% (%s/%s)")
	add(KeyUsageContextLowerBoundCompact,
		"Context: %s ≥%d%%", "上下文：%s ≥%d%%", "Kontext: %s ≥%d%%",
		"コンテキスト: %s ≥%d%%", "컨텍스트: %s ≥%d%%", "Контекст: %s ≥%d%%")
	add(KeyUsageContextLowerBoundPlain,
		"Context: ≥%d%% (%s/%s)", "上下文：≥%d%%（%s/%s）", "Kontext: ≥%d%% (%s/%s)",
		"コンテキスト: ≥%d%%（%s/%s）", "컨텍스트: ≥%d%% (%s/%s)", "Контекст: ≥%d%% (%s/%s)")
	add(KeyUsageContextUnknown,
		"Context: unknown", "上下文：未知", "Kontext: unbekannt", "コンテキスト: 不明", "컨텍스트: 알 수 없음", "Контекст: неизвестен")
	add(KeyUsageLastRequest,
		"Last request: %s input · %s output · %d%% cache hit",
		"最近请求：输入 %s · 输出 %s · 缓存命中 %d%%",
		"Letzte Anfrage: %s Eingabe · %s Ausgabe · %d%% Cache-Treffer",
		"直近のリクエスト: 入力 %s · 出力 %s · キャッシュヒット %d%%",
		"최근 요청: 입력 %s · 출력 %s · 캐시 적중 %d%%",
		"Последний запрос: вход %s · выход %s · попадание в кэш %d%%")
	add(KeyUsageLastRequestUnknown,
		"Last request: usage unknown",
		"最近请求：用量未知",
		"Letzte Anfrage: Nutzung unbekannt",
		"直近のリクエスト: 使用量不明",
		"최근 요청: 사용량 알 수 없음",
		"Последний запрос: использование неизвестно")
	add(KeyUsageCumulativeSession,
		"Cumulative session: %s input · %s output · %d%% cache hit · $%.4f",
		"会话累计：输入 %s · 输出 %s · 缓存命中 %d%% · $%.4f",
		"Sitzung kumuliert: %s Eingabe · %s Ausgabe · %d%% Cache-Treffer · $%.4f",
		"セッション累計: 入力 %s · 出力 %s · キャッシュヒット %d%% · $%.4f",
		"세션 누계: 입력 %s · 출력 %s · 캐시 적중 %d%% · $%.4f",
		"Сеанс суммарно: вход %s · выход %s · попадание в кэш %d%% · $%.4f")
	add(KeyUsageCumulativeUnknown,
		"Cumulative session: %s input · %s output · %d%% cache hit · cost unknown",
		"会话累计：输入 %s · 输出 %s · 缓存命中 %d%% · 费用未知",
		"Sitzung kumuliert: %s Eingabe · %s Ausgabe · %d%% Cache-Treffer · Kosten unbekannt",
		"セッション累計: 入力 %s · 出力 %s · キャッシュヒット %d%% · 料金不明",
		"세션 누계: 입력 %s · 출력 %s · 캐시 적중 %d%% · 비용 알 수 없음",
		"Сеанс суммарно: вход %s · выход %s · попадание в кэш %d%% · стоимость неизвестна")
	add(KeyUsageCumulativeUnavailable,
		"Cumulative session: usage unknown",
		"会话累计：用量未知",
		"Sitzung kumuliert: Nutzung unbekannt",
		"セッション累計: 使用量不明",
		"세션 누계: 사용량 알 수 없음",
		"Сеанс суммарно: использование неизвестно")
	add(KeyUsageScopedCompact,
		"Req: %s in · %s out · %d%% cache | Session: $%.4f",
		"请求：输入 %s · 输出 %s · 缓存 %d%% | 会话累计：$%.4f",
		"Anfrage: %s ein · %s aus · %d%% Cache | Sitzung: $%.4f",
		"リクエスト: 入力 %s · 出力 %s · キャッシュ %d%% | セッション累計: $%.4f",
		"요청: 입력 %s · 출력 %s · 캐시 %d%% | 세션 누계: $%.4f",
		"Запрос: вход %s · выход %s · кэш %d%% | Сеанс: $%.4f")
	add(KeyUsageScopedCompactUnknownCost,
		"Req: %s in · %s out · %d%% cache | Session cost: unknown",
		"请求：输入 %s · 输出 %s · 缓存 %d%% | 会话累计费用：未知",
		"Anfrage: %s ein · %s aus · %d%% Cache | Sitzungskosten: unbekannt",
		"リクエスト: 入力 %s · 出力 %s · キャッシュ %d%% | セッション累計料金: 不明",
		"요청: 입력 %s · 출력 %s · 캐시 %d%% | 세션 누계 비용: 알 수 없음",
		"Запрос: вход %s · выход %s · кэш %d%% | Стоимость сеанса: неизвестна")
	add(KeyUsageEffectiveContext,
		"Effective model context: %s %d%% (%s/%s)",
		"模型有效上下文：%s %d%%（%s/%s）",
		"Effektiver Modellkontext: %s %d%% (%s/%s)",
		"モデルの有効コンテキスト: %s %d%%（%s/%s）",
		"모델 유효 컨텍스트: %s %d%% (%s/%s)",
		"Эффективный контекст модели: %s %d%% (%s/%s)")
	add(KeyUsageEffectiveContextCompact,
		"Effective ctx: %s %d%%",
		"有效上下文：%s %d%%",
		"Effektiver Kontext: %s %d%%",
		"有効コンテキスト: %s %d%%",
		"유효 컨텍스트: %s %d%%",
		"Эффективный контекст: %s %d%%")
	add(KeyUsageEffectiveContextPlain,
		"Effective model context: %d%% (%s/%s)",
		"模型有效上下文：%d%%（%s/%s）",
		"Effektiver Modellkontext: %d%% (%s/%s)",
		"モデルの有効コンテキスト: %d%%（%s/%s）",
		"모델 유효 컨텍스트: %d%% (%s/%s)",
		"Эффективный контекст модели: %d%% (%s/%s)")
	add(KeyUsageEffectiveUnknown,
		"Effective model context: unknown",
		"模型有效上下文：未知",
		"Effektiver Modellkontext: unbekannt",
		"モデルの有効コンテキスト: 不明",
		"모델 유효 컨텍스트: 알 수 없음",
		"Эффективный контекст модели: неизвестен")
}
