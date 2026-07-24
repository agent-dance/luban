package i18n

const (
	KeyToolWebPolicyRegionBlocked        Key = "tool.web_policy.region_blocked"
	KeyToolWebPolicyRateLimited          Key = "tool.web_policy.rate_limited"
	KeyToolWebPolicyRateLimitedWithLimit Key = "tool.web_policy.rate_limited_with_limit"
)

var toolWebPolicyKeys = []Key{
	KeyToolWebPolicyRegionBlocked,
	KeyToolWebPolicyRateLimited,
	KeyToolWebPolicyRateLimitedWithLimit,
}

func init() {
	addToolWebPolicy(KeyToolWebPolicyRegionBlocked,
		"Web search is only available in the US",
		"Web 搜索仅在美国可用",
		"Die Websuche ist nur in den USA verfügbar",
		"Web 検索は米国でのみ利用できます",
		"Web 검색은 미국에서만 사용할 수 있습니다",
		"Web-поиск доступен только в США")
	addToolWebPolicy(KeyToolWebPolicyRateLimited,
		"Web search rate limit exceeded; try again in a minute",
		"Web 搜索请求过于频繁，请一分钟后重试",
		"Das Limit für Websuchen wurde überschritten; versuchen Sie es in einer Minute erneut",
		"Web 検索の回数制限を超えました。1 分後にもう一度お試しください",
		"Web 검색 요청 한도를 초과했습니다. 1분 후 다시 시도해 주세요",
		"Превышен лимит Web-поиска; повторите попытку через минуту")
	addToolWebPolicy(KeyToolWebPolicyRateLimitedWithLimit,
		"Web search rate limit exceeded; try again in a minute (limit=%d/min)",
		"Web 搜索请求过于频繁，请一分钟后重试（上限：每分钟 %d 次）",
		"Das Limit für Websuchen wurde überschritten; versuchen Sie es in einer Minute erneut (Limit: %d/min)",
		"Web 検索の回数制限を超えました。1 分後にもう一度お試しください（上限: %d 回/分）",
		"Web 검색 요청 한도를 초과했습니다. 1분 후 다시 시도해 주세요(한도: 분당 %d회)",
		"Превышен лимит Web-поиска; повторите попытку через минуту (лимит: %d/мин)")
}

func addToolWebPolicy(key Key, en, zh, de, ja, ko, ru string) {
	semanticTranslations[key] = map[Language]string{
		LangEN: en,
		LangZH: zh,
		LangDE: de,
		LangJA: ja,
		LangKO: ko,
		LangRU: ru,
	}
}
