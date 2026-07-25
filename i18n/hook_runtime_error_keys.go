package i18n

const (
	KeyHookURLInvalid            Key = "hook.http.url_invalid"
	KeyHookSchemeNotAllowed      Key = "hook.http.scheme_not_allowed"
	KeyHookHostnameMissing       Key = "hook.http.hostname_missing"
	KeyHookDNSLookupFailed       Key = "hook.http.dns_lookup_failed"
	KeyHookBlockedIP             Key = "hook.http.blocked_ip"
	KeyHookSSRFBlocked           Key = "hook.http.ssrf_blocked"
	KeyHookRedirectLimit         Key = "hook.http.redirect_limit"
	KeyHookRedirectBlocked       Key = "hook.http.redirect_blocked"
	KeyHookRequestBuildFailed    Key = "hook.http.request_build_failed"
	KeyHookResponseReadFailed    Key = "hook.http.response_read_failed"
	KeyHookResponseTruncated     Key = "hook.http.response_truncated"
	KeyHookAttemptsFailed        Key = "hook.http.attempts_failed"
	KeyHookConfigSettingsParse   Key = "hook.config.settings_parse_failed"
	KeyHookConfigEventInvalid    Key = "hook.config.event_invalid"
	KeyHookConfigKindUnknown     Key = "hook.config.kind_unknown"
	KeyHookLifecycleApplyMissing Key = "hook.lifecycle.apply_missing"
	KeyHookLifecycleRollback     Key = "hook.lifecycle.rollback_failed"
	KeyHookLifecycleBlocked      Key = "hook.lifecycle.blocked"
	KeyHookBlockedDefault        Key = "hook.blocked_default"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyHookURLInvalid, "Invalid hook URL", "hook URL 无效", "Ungültige Hook-URL", "hook URL が無効です", "잘못된 hook URL입니다", "Недопустимый URL hook")
	add(KeyHookSchemeNotAllowed, "URL scheme %q is not allowed; use http or https", "不允许 URL scheme %q；请使用 http 或 https", "URL-Schema %q ist nicht zulässig; verwende http oder https", "URL scheme %q は許可されていません。http または https を使用してください", "URL scheme %q은(는) 허용되지 않습니다. http 또는 https를 사용하세요", "Scheme URL %q запрещена; используйте http или https")
	add(KeyHookHostnameMissing, "The hook URL has no hostname", "hook URL 缺少 hostname", "Die Hook-URL enthält keinen Hostnamen", "hook URL に hostname がありません", "hook URL에 hostname이 없습니다", "В URL hook отсутствует hostname")
	add(KeyHookDNSLookupFailed, "DNS lookup failed for %q", "%q 的 DNS 查询失败", "DNS-Abfrage für %q fehlgeschlagen", "%q の DNS 検索に失敗しました", "%q의 DNS 조회에 실패했습니다", "Ошибка DNS-поиска для %q")
	add(KeyHookBlockedIP, "The URL resolves to blocked IP %s in a private or internal range", "该 URL 解析到私有或内部范围中的受阻 IP %s", "Die URL wird zur gesperrten IP-Adresse %s in einem privaten oder internen Bereich aufgelöst", "URL はプライベートまたは内部範囲のブロック対象 IP %s に解決されます", "URL이 사설 또는 내부 범위의 차단된 IP %s(으)로 확인됩니다", "URL разрешается в заблокированный IP %s из частного или внутреннего диапазона")
	add(KeyHookSSRFBlocked, "SSRF protection blocked %s because it resolves to private IP %s", "SSRF 防护已阻止 %s，因为它解析到私有 IP %s", "Der SSRF-Schutz hat %s blockiert, da es zur privaten IP %s aufgelöst wird", "SSRF 保護により %s をブロックしました。プライベート IP %s に解決されます", "SSRF 보호가 %s을(를) 차단했습니다. 사설 IP %s(으)로 확인됩니다", "Защита SSRF заблокировала %s: адрес разрешается в частный IP %s")
	add(KeyHookRedirectLimit, "Too many redirects; maximum is 3", "重定向次数过多；最多允许 3 次", "Zu viele Weiterleitungen; maximal 3 sind zulässig", "リダイレクトが多すぎます。上限は 3 回です", "리디렉션이 너무 많습니다. 최대 3회까지 허용됩니다", "Слишком много перенаправлений; максимум — 3")
	add(KeyHookRedirectBlocked, "Redirect blocked", "重定向已被阻止", "Weiterleitung blockiert", "リダイレクトをブロックしました", "리디렉션이 차단되었습니다", "Перенаправление заблокировано")
	add(KeyHookRequestBuildFailed, "Could not build the hook request", "无法构建 hook 请求", "Hook-Anfrage konnte nicht erstellt werden", "hook リクエストを作成できませんでした", "hook 요청을 만들 수 없습니다", "Не удалось сформировать запрос hook")
	add(KeyHookResponseReadFailed, "Could not read the hook response", "无法读取 hook 响应", "Hook-Antwort konnte nicht gelesen werden", "hook 応答を読み取れませんでした", "hook 응답을 읽을 수 없습니다", "Не удалось прочитать ответ hook")
	add(KeyHookResponseTruncated, "Hook response was truncated after exceeding the %d-byte limit", "hook 响应超过 %d 字节限制，已截断", "Hook-Antwort wurde nach Überschreiten des Limits von %d Byte gekürzt", "hook 応答は %d バイトの上限を超えたため切り詰められました", "hook 응답이 %d바이트 제한을 초과하여 잘렸습니다", "Ответ hook усечён после превышения лимита в %d байт")
	add(KeyHookAttemptsFailed, "HTTP hook failed after %d attempt(s): %v", "HTTP hook 在尝试 %d 次后失败：%v", "HTTP-Hook nach %d Versuch(en) fehlgeschlagen: %v", "HTTP hook は %d 回試行後に失敗しました: %v", "HTTP hook가 %d회 시도 후 실패했습니다: %v", "HTTP hook завершился ошибкой после %d попыток: %v")
	add(KeyHookConfigSettingsParse, "Could not parse hook settings %s", "无法解析 hook 设置 %s", "Hook-Einstellungen %s konnten nicht geparst werden", "hook 設定 %s を解析できませんでした", "hook 설정 %s을(를) 파싱할 수 없습니다", "Не удалось разобрать настройки hook %s")
	add(KeyHookConfigEventInvalid, "Invalid hook configuration for event %q", "事件 %q 的 hook 配置无效", "Ungültige Hook-Konfiguration für Ereignis %q", "イベント %q の hook 設定が無効です", "이벤트 %q의 hook 구성이 올바르지 않습니다", "Недопустимая конфигурация hook для события %q")
	add(KeyHookConfigKindUnknown, "Unknown hook kind: %q", "未知 hook 类型：%q", "Unbekannte Hook-Art: %q", "不明な hook 種類です: %q", "알 수 없는 hook 종류: %q", "Неизвестный тип hook: %q")
	add(KeyHookLifecycleApplyMissing, "%s lifecycle transition has no apply function", "%s 生命周期转换缺少 apply 函数", "Für den Lebenszyklusübergang %s fehlt die Apply-Funktion", "%s のライフサイクル遷移に apply 関数がありません", "%s 수명 주기 전환에 apply 함수가 없습니다", "У перехода жизненного цикла %s отсутствует функция apply")
	add(KeyHookLifecycleRollback, "Could not roll back the lifecycle transition", "无法回滚生命周期转换", "Lebenszyklusübergang konnte nicht zurückgesetzt werden", "ライフサイクル遷移をロールバックできませんでした", "수명 주기 전환을 롤백할 수 없습니다", "Не удалось откатить переход жизненного цикла")
	add(KeyHookLifecycleBlocked, "Hook %s blocked the lifecycle transition: %s", "Hook %s 阻止了生命周期转换：%s", "Hook %s hat den Lebenszyklusübergang blockiert: %s", "Hook %s がライフサイクル遷移をブロックしました: %s", "Hook %s이(가) 수명 주기 전환을 차단했습니다: %s", "Hook %s заблокировал переход жизненного цикла: %s")
	add(KeyHookBlockedDefault, "Blocked by hook", "已被 hook 阻止", "Durch Hook blockiert", "hook によりブロックされました", "hook에 의해 차단됨", "Заблокировано hook")
}
