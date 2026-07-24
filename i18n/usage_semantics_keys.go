package i18n

const (
	KeyUsageSession                            Key = "usage.session"
	KeyUsageSessionUnknownCost                 Key = "usage.session.unknown_cost"
	KeyUsageSessionNoCache                     Key = "usage.session.no_cache"
	KeyUsageSessionNoCacheUnknownCost          Key = "usage.session.no_cache.unknown_cost"
	KeyUsageSessionCompacted                   Key = "usage.session.compacted"
	KeyUsageSessionCompactedUnknownCost        Key = "usage.session.compacted.unknown_cost"
	KeyUsageSessionCompactedNoCache            Key = "usage.session.compacted.no_cache"
	KeyUsageSessionCompactedNoCacheUnknownCost Key = "usage.session.compacted.no_cache.unknown_cost"
	KeyUsageSessionUnavailable                 Key = "usage.session.unavailable"
	KeyUsageContext                            Key = "usage.model_context"
	KeyUsageContextCompact                     Key = "usage.model_context.compact"
	KeyUsageContextPlain                       Key = "usage.model_context.plain"
	KeyUsageContextEstimate                    Key = "usage.model_context.estimated"
	KeyUsageContextEstimateCompact             Key = "usage.model_context.estimated.compact"
	KeyUsageContextEstimatePlain               Key = "usage.model_context.estimated.plain"
	KeyUsageContextLowerBound                  Key = "usage.model_context.lower_bound"
	KeyUsageContextLowerBoundCompact           Key = "usage.model_context.lower_bound.compact"
	KeyUsageContextLowerBoundPlain             Key = "usage.model_context.lower_bound.plain"
	KeyUsageContextUnknown                     Key = "usage.model_context.unknown"

	KeyUsageLastRequest                Key = "usage.last_request"
	KeyUsageLastRequestUnknown         Key = "usage.last_request.unknown"
	KeyUsageCumulativeSession          Key = "usage.cumulative_session"
	KeyUsageCumulativeUnknown          Key = "usage.cumulative_session.unknown_cost"
	KeyUsageCumulativeUnavailable      Key = "usage.cumulative_session.unavailable"
	KeyUsageScopedCompact              Key = "usage.scoped.compact"
	KeyUsageScopedCompactUnknownCost   Key = "usage.scoped.compact.unknown_cost"
	KeyUsageEffectiveContext           Key = "usage.effective_model_context"
	KeyUsageEffectiveContextCompact    Key = "usage.effective_model_context.compact"
	KeyUsageEffectiveContextPlain      Key = "usage.effective_model_context.plain"
	KeyUsageEffectiveEstimate          Key = "usage.effective_model_context.estimated"
	KeyUsageEffectiveEstimateCompact   Key = "usage.effective_model_context.estimated.compact"
	KeyUsageEffectiveEstimatePlain     Key = "usage.effective_model_context.estimated.plain"
	KeyUsageEffectiveLowerBound        Key = "usage.effective_model_context.lower_bound"
	KeyUsageEffectiveLowerBoundCompact Key = "usage.effective_model_context.lower_bound.compact"
	KeyUsageEffectiveLowerBoundPlain   Key = "usage.effective_model_context.lower_bound.plain"
	KeyUsageEffectiveUnknown           Key = "usage.effective_model_context.unknown"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyUsageSession,
		"Session: in %s · %d%% cached · out %s · $%.4f",
		"会话：输入 %s · 已缓存 %d%% · 输出 %s · $%.4f",
		"Sitzung: %s ein · %d%% im Cache · %s aus · $%.4f",
		"セッション: 入力 %s · %d%% キャッシュ済み · 出力 %s · $%.4f",
		"세션: 입력 %s · %d%% 캐시됨 · 출력 %s · $%.4f",
		"Сеанс: вход %s · %d%% из кэша · выход %s · $%.4f")
	add(KeyUsageSessionUnknownCost,
		"Session: in %s · %d%% cached · out %s · cost unknown",
		"会话：输入 %s · 已缓存 %d%% · 输出 %s · 费用未知",
		"Sitzung: %s ein · %d%% im Cache · %s aus · Kosten unbekannt",
		"セッション: 入力 %s · %d%% キャッシュ済み · 出力 %s · 料金不明",
		"세션: 입력 %s · %d%% 캐시됨 · 출력 %s · 비용 알 수 없음",
		"Сеанс: вход %s · %d%% из кэша · выход %s · стоимость неизвестна")
	add(KeyUsageSessionNoCache,
		"Session: in %s · out %s · $%.4f",
		"会话：输入 %s · 输出 %s · $%.4f",
		"Sitzung: %s ein · %s aus · $%.4f",
		"セッション: 入力 %s · 出力 %s · $%.4f",
		"세션: 입력 %s · 출력 %s · $%.4f",
		"Сеанс: вход %s · выход %s · $%.4f")
	add(KeyUsageSessionNoCacheUnknownCost,
		"Session: in %s · out %s · cost unknown",
		"会话：输入 %s · 输出 %s · 费用未知",
		"Sitzung: %s ein · %s aus · Kosten unbekannt",
		"セッション: 入力 %s · 出力 %s · 料金不明",
		"세션: 입력 %s · 출력 %s · 비용 알 수 없음",
		"Сеанс: вход %s · выход %s · стоимость неизвестна")
	add(KeyUsageSessionCompacted,
		"Session: in %s (%s total) · %d%% cached · out %s · $%.4f",
		"会话：输入 %s（累计 %s）· 已缓存 %d%% · 输出 %s · $%.4f",
		"Sitzung: %s ein (%s gesamt) · %d%% im Cache · %s aus · $%.4f",
		"セッション: 入力 %s（合計 %s）· %d%% キャッシュ済み · 出力 %s · $%.4f",
		"세션: 입력 %s (총 %s) · %d%% 캐시됨 · 출력 %s · $%.4f",
		"Сеанс: вход %s (всего %s) · %d%% из кэша · выход %s · $%.4f")
	add(KeyUsageSessionCompactedUnknownCost,
		"Session: in %s (%s total) · %d%% cached · out %s · cost unknown",
		"会话：输入 %s（累计 %s）· 已缓存 %d%% · 输出 %s · 费用未知",
		"Sitzung: %s ein (%s gesamt) · %d%% im Cache · %s aus · Kosten unbekannt",
		"セッション: 入力 %s（合計 %s）· %d%% キャッシュ済み · 出力 %s · 料金不明",
		"세션: 입력 %s (총 %s) · %d%% 캐시됨 · 출력 %s · 비용 알 수 없음",
		"Сеанс: вход %s (всего %s) · %d%% из кэша · выход %s · стоимость неизвестна")
	add(KeyUsageSessionCompactedNoCache,
		"Session: in %s (%s total) · out %s · $%.4f",
		"会话：输入 %s（累计 %s）· 输出 %s · $%.4f",
		"Sitzung: %s ein (%s gesamt) · %s aus · $%.4f",
		"セッション: 入力 %s（合計 %s）· 出力 %s · $%.4f",
		"세션: 입력 %s (총 %s) · 출력 %s · $%.4f",
		"Сеанс: вход %s (всего %s) · выход %s · $%.4f")
	add(KeyUsageSessionCompactedNoCacheUnknownCost,
		"Session: in %s (%s total) · out %s · cost unknown",
		"会话：输入 %s（累计 %s）· 输出 %s · 费用未知",
		"Sitzung: %s ein (%s gesamt) · %s aus · Kosten unbekannt",
		"セッション: 入力 %s（合計 %s）· 出力 %s · 料金不明",
		"세션: 입력 %s (총 %s) · 출력 %s · 비용 알 수 없음",
		"Сеанс: вход %s (всего %s) · выход %s · стоимость неизвестна")
	add(KeyUsageSessionUnavailable,
		"Session: usage unknown", "会话：用量未知", "Sitzung: Nutzung unbekannt",
		"セッション: 使用量不明", "세션: 사용량 알 수 없음", "Сеанс: использование неизвестно")
	add(KeyUsageContext,
		"Context: %s %d%% (%s/%s)", "上下文：%s %d%%（%s/%s）", "Kontext: %s %d%% (%s/%s)",
		"コンテキスト: %s %d%%（%s/%s）", "컨텍스트: %s %d%% (%s/%s)", "Контекст: %s %d%% (%s/%s)")
	add(KeyUsageContextCompact,
		"Context: %s %d%%", "上下文：%s %d%%", "Kontext: %s %d%%",
		"コンテキスト: %s %d%%", "컨텍스트: %s %d%%", "Контекст: %s %d%%")
	add(KeyUsageContextPlain,
		"Context: %d%% (%s/%s)", "上下文：%d%%（%s/%s）", "Kontext: %d%% (%s/%s)",
		"コンテキスト: %d%%（%s/%s）", "컨텍스트: %d%% (%s/%s)", "Контекст: %d%% (%s/%s)")
	add(KeyUsageContextEstimate,
		"Context: %s ≈%d%% (%s/%s)", "上下文：%s ≈%d%%（%s/%s）", "Kontext: %s ≈%d%% (%s/%s)",
		"コンテキスト: %s ≈%d%%（%s/%s）", "컨텍스트: %s ≈%d%% (%s/%s)", "Контекст: %s ≈%d%% (%s/%s)")
	add(KeyUsageContextEstimateCompact,
		"Context: %s ≈%d%%", "上下文：%s ≈%d%%", "Kontext: %s ≈%d%%",
		"コンテキスト: %s ≈%d%%", "컨텍스트: %s ≈%d%%", "Контекст: %s ≈%d%%")
	add(KeyUsageContextEstimatePlain,
		"Context: ≈%d%% (%s/%s)", "上下文：≈%d%%（%s/%s）", "Kontext: ≈%d%% (%s/%s)",
		"コンテキスト: ≈%d%%（%s/%s）", "컨텍스트: ≈%d%% (%s/%s)", "Контекст: ≈%d%% (%s/%s)")
	add(KeyUsageContextLowerBound,
		"Context: %s ≥%d%% (at least %s/%s)", "上下文：%s ≥%d%%（至少 %s/%s）", "Kontext: %s ≥%d%% (mindestens %s/%s)",
		"コンテキスト: %s ≥%d%%（少なくとも %s/%s）", "컨텍스트: %s ≥%d%% (최소 %s/%s)", "Контекст: %s ≥%d%% (не менее %s/%s)")
	add(KeyUsageContextLowerBoundCompact,
		"Context: %s ≥%d%%", "上下文：%s ≥%d%%", "Kontext: %s ≥%d%%",
		"コンテキスト: %s ≥%d%%", "컨텍스트: %s ≥%d%%", "Контекст: %s ≥%d%%")
	add(KeyUsageContextLowerBoundPlain,
		"Context: ≥%d%% (at least %s/%s)", "上下文：≥%d%%（至少 %s/%s）", "Kontext: ≥%d%% (mindestens %s/%s)",
		"コンテキスト: ≥%d%%（少なくとも %s/%s）", "컨텍스트: ≥%d%% (최소 %s/%s)", "Контекст: ≥%d%% (не менее %s/%s)")
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
	add(KeyUsageEffectiveEstimate,
		"Estimated effective context: %s %d%% (%s/%s)",
		"有效上下文估算：%s %d%%（%s/%s）",
		"Geschätzter effektiver Kontext: %s %d%% (%s/%s)",
		"有効コンテキストの推定: %s %d%%（%s/%s）",
		"유효 컨텍스트 추정: %s %d%% (%s/%s)",
		"Оценка эффективного контекста: %s %d%% (%s/%s)")
	add(KeyUsageEffectiveEstimateCompact,
		"Est. effective ctx: %s %d%%",
		"有效上下文估算：%s %d%%",
		"Gesch. effektiver Kontext: %s %d%%",
		"有効コンテキスト推定: %s %d%%",
		"유효 컨텍스트 추정: %s %d%%",
		"Оценка контекста: %s %d%%")
	add(KeyUsageEffectiveEstimatePlain,
		"Estimated effective context: %d%% (%s/%s)",
		"有效上下文估算：%d%%（%s/%s）",
		"Geschätzter effektiver Kontext: %d%% (%s/%s)",
		"有効コンテキストの推定: %d%%（%s/%s）",
		"유효 컨텍스트 추정: %d%% (%s/%s)",
		"Оценка эффективного контекста: %d%% (%s/%s)")
	add(KeyUsageEffectiveLowerBound,
		"Effective context lower bound: %s %d%% (at least %s/%s)",
		"有效上下文下界：%s %d%%（至少 %s/%s）",
		"Untergrenze des effektiven Kontexts: %s %d%% (mindestens %s/%s)",
		"有効コンテキストの下限: %s %d%%（少なくとも %s/%s）",
		"유효 컨텍스트 하한: %s %d%% (최소 %s/%s)",
		"Нижняя граница эффективного контекста: %s %d%% (не менее %s/%s)")
	add(KeyUsageEffectiveLowerBoundCompact,
		"Effective ctx ≥ %s %d%%",
		"有效上下文 ≥ %s %d%%",
		"Effektiver Kontext ≥ %s %d%%",
		"有効コンテキスト ≥ %s %d%%",
		"유효 컨텍스트 ≥ %s %d%%",
		"Эффективный контекст ≥ %s %d%%")
	add(KeyUsageEffectiveLowerBoundPlain,
		"Effective context lower bound: %d%% (at least %s/%s)",
		"有效上下文下界：%d%%（至少 %s/%s）",
		"Untergrenze des effektiven Kontexts: %d%% (mindestens %s/%s)",
		"有効コンテキストの下限: %d%%（少なくとも %s/%s）",
		"유효 컨텍스트 하한: %d%% (최소 %s/%s)",
		"Нижняя граница эффективного контекста: %d%% (не менее %s/%s)")
	add(KeyUsageEffectiveUnknown,
		"Effective model context: unknown",
		"模型有效上下文：未知",
		"Effektiver Modellkontext: unbekannt",
		"モデルの有効コンテキスト: 不明",
		"모델 유효 컨텍스트: 알 수 없음",
		"Эффективный контекст модели: неизвестен")
}
