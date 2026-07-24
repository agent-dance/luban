package i18n

const (
	KeyToolWebValidationURLTooLong           Key = "tool.web_validation.url_too_long"
	KeyToolWebValidationInvalidURL           Key = "tool.web_validation.invalid_url"
	KeyToolWebValidationUnsupportedScheme    Key = "tool.web_validation.unsupported_scheme"
	KeyToolWebValidationUserinfoForbidden    Key = "tool.web_validation.userinfo_forbidden"
	KeyToolWebValidationHostnameMissing      Key = "tool.web_validation.hostname_missing"
	KeyToolWebValidationHostnameNotPublic    Key = "tool.web_validation.hostname_not_public"
	KeyToolWebValidationResolveHostname      Key = "tool.web_validation.resolve_hostname"
	KeyToolWebValidationLoopbackAddress      Key = "tool.web_validation.loopback_address"
	KeyToolWebValidationPrivateAddress       Key = "tool.web_validation.private_address"
	KeyToolWebValidationLinkLocalAddress     Key = "tool.web_validation.link_local_address"
	KeyToolWebValidationUnspecifiedAddress   Key = "tool.web_validation.unspecified_address"
	KeyToolWebValidationCloudMetadataAddress Key = "tool.web_validation.cloud_metadata_address"
	KeyToolWebValidationRedirectLimit        Key = "tool.web_validation.redirect_limit"

	KeyToolWebDomainSafetyCheckFailed Key = "tool.web_domain.safety_check_failed"
	KeyToolWebDomainEmptyHostname     Key = "tool.web_domain.empty_hostname"
	KeyToolWebDomainBuildRequest      Key = "tool.web_domain.build_request"
	KeyToolWebDomainRequest           Key = "tool.web_domain.request"
	KeyToolWebDomainStatus            Key = "tool.web_domain.status"
	KeyToolWebDomainDecodeResponse    Key = "tool.web_domain.decode_response"
	KeyToolWebDomainMissingCanFetch   Key = "tool.web_domain.missing_can_fetch"
)

var toolWebValidationKeys = []Key{
	KeyToolWebValidationURLTooLong,
	KeyToolWebValidationInvalidURL,
	KeyToolWebValidationUnsupportedScheme,
	KeyToolWebValidationUserinfoForbidden,
	KeyToolWebValidationHostnameMissing,
	KeyToolWebValidationHostnameNotPublic,
	KeyToolWebValidationResolveHostname,
	KeyToolWebValidationLoopbackAddress,
	KeyToolWebValidationPrivateAddress,
	KeyToolWebValidationLinkLocalAddress,
	KeyToolWebValidationUnspecifiedAddress,
	KeyToolWebValidationCloudMetadataAddress,
	KeyToolWebValidationRedirectLimit,
	KeyToolWebDomainSafetyCheckFailed,
	KeyToolWebDomainEmptyHostname,
	KeyToolWebDomainBuildRequest,
	KeyToolWebDomainRequest,
	KeyToolWebDomainStatus,
	KeyToolWebDomainDecodeResponse,
	KeyToolWebDomainMissingCanFetch,
}

func init() {
	addToolWebValidation(KeyToolWebValidationURLTooLong,
		"URL exceeds max length of %d characters",
		"URL 超出 %d 个字符的长度上限",
		"URL überschreitet die maximale Länge von %d Zeichen",
		"URL が最大長の %d 文字を超えています",
		"URL이 최대 길이인 %d자를 초과합니다",
		"Длина URL превышает предел в %d символов")
	addToolWebValidation(KeyToolWebValidationInvalidURL,
		"invalid URL: %v",
		"URL 无效：%v",
		"Ungültige URL: %v",
		"URL が無効です: %v",
		"잘못된 URL: %v",
		"Недопустимый URL: %v")
	addToolWebValidation(KeyToolWebValidationUnsupportedScheme,
		"unsupported scheme %q: only http and https are allowed",
		"不支持 scheme %q：仅允许 http 和 https",
		"Nicht unterstütztes Schema %q: Nur http und https sind zulässig",
		"scheme %q はサポートされていません。http と https のみ使用できます",
		"지원하지 않는 scheme %q: http와 https만 허용됩니다",
		"Схема %q не поддерживается: разрешены только http и https")
	addToolWebValidation(KeyToolWebValidationUserinfoForbidden,
		"URL must not contain userinfo (user:password@)",
		"URL 不能包含 userinfo（user:password@）",
		"URL darf keine userinfo enthalten (user:password@)",
		"URL に userinfo（user:password@）を含めることはできません",
		"URL에는 userinfo(user:password@)를 포함할 수 없습니다",
		"URL не должен содержать userinfo (user:password@)")
	addToolWebValidation(KeyToolWebValidationHostnameMissing,
		"URL has no hostname",
		"URL 未包含 hostname",
		"URL enthält keinen hostname",
		"URL に hostname がありません",
		"URL에 hostname이 없습니다",
		"В URL отсутствует hostname")
	addToolWebValidation(KeyToolWebValidationHostnameNotPublic,
		"URL hostname %q is not a public domain",
		"URL hostname %q 不是公开 domain",
		"Der URL-hostname %q ist keine öffentliche Domain",
		"URL の hostname %q は公開 domain ではありません",
		"URL hostname %q은(는) 공개 domain이 아닙니다",
		"Hostname URL %q не является публичным доменом")
	addToolWebValidation(KeyToolWebValidationResolveHostname,
		"failed to resolve hostname %q: %v",
		"解析 hostname %q 失败：%v",
		"Hostname %q konnte nicht aufgelöst werden: %v",
		"hostname %q を解決できませんでした: %v",
		"hostname %q을(를) 확인하지 못했습니다: %v",
		"Не удалось разрешить hostname %q: %v")
	addToolWebValidation(KeyToolWebValidationLoopbackAddress,
		"URL resolves to loopback address %s",
		"URL 解析到 loopback 地址 %s",
		"URL wird zur Loopback-Adresse %s aufgelöst",
		"URL は loopback アドレス %s に解決されます",
		"URL이 loopback 주소 %s(으)로 확인됩니다",
		"URL разрешается в loopback-адрес %s")
	addToolWebValidation(KeyToolWebValidationPrivateAddress,
		"URL resolves to private address %s",
		"URL 解析到 private 地址 %s",
		"URL wird zur privaten Adresse %s aufgelöst",
		"URL は private アドレス %s に解決されます",
		"URL이 private 주소 %s(으)로 확인됩니다",
		"URL разрешается в частный адрес %s")
	addToolWebValidation(KeyToolWebValidationLinkLocalAddress,
		"URL resolves to link-local address %s",
		"URL 解析到 link-local 地址 %s",
		"URL wird zur Link-Local-Adresse %s aufgelöst",
		"URL は link-local アドレス %s に解決されます",
		"URL이 link-local 주소 %s(으)로 확인됩니다",
		"URL разрешается в link-local-адрес %s")
	addToolWebValidation(KeyToolWebValidationUnspecifiedAddress,
		"URL resolves to unspecified address %s",
		"URL 解析到 unspecified 地址 %s",
		"URL wird zur nicht spezifizierten Adresse %s aufgelöst",
		"URL は unspecified アドレス %s に解決されます",
		"URL이 unspecified 주소 %s(으)로 확인됩니다",
		"URL разрешается в неопределённый адрес %s")
	addToolWebValidation(KeyToolWebValidationCloudMetadataAddress,
		"URL resolves to cloud metadata endpoint %s",
		"URL 解析到 cloud metadata endpoint %s",
		"URL wird zum Cloud-Metadata-Endpunkt %s aufgelöst",
		"URL は cloud metadata endpoint %s に解決されます",
		"URL이 cloud metadata endpoint %s(으)로 확인됩니다",
		"URL разрешается в endpoint метаданных облака %s")
	addToolWebValidation(KeyToolWebValidationRedirectLimit,
		"stopped after %d redirects",
		"已在 %d 次重定向后停止",
		"Nach %d Weiterleitungen abgebrochen",
		"%d 回のリダイレクト後に停止しました",
		"%d회 리디렉션 후 중단했습니다",
		"Остановлено после %d перенаправлений")

	addToolWebValidation(KeyToolWebDomainSafetyCheckFailed,
		"Unable to verify if domain %s is safe to fetch. This may be due to network restrictions or enterprise security policies blocking claude.ai.",
		"无法确认 domain %s 是否可安全获取。这可能是网络限制，或 enterprise security policy 阻止访问 claude.ai 导致的。",
		"Es konnte nicht geprüft werden, ob die Domain %s sicher abgerufen werden kann. Möglicherweise verhindern Netzwerkeinschränkungen oder Sicherheitsrichtlinien des Unternehmens den Zugriff auf claude.ai.",
		"domain %s を安全に取得できるか確認できませんでした。ネットワーク制限または enterprise security policy により claude.ai へのアクセスがブロックされている可能性があります。",
		"domain %s을(를) 안전하게 가져올 수 있는지 확인하지 못했습니다. 네트워크 제한 또는 enterprise security policy가 claude.ai 액세스를 차단했을 수 있습니다.",
		"Не удалось проверить, безопасно ли получать данные с домена %s. Возможно, доступ к claude.ai заблокирован сетевыми ограничениями или корпоративными политиками безопасности.")
	addToolWebValidation(KeyToolWebDomainEmptyHostname,
		"empty hostname",
		"hostname 为空",
		"Hostname ist leer",
		"hostname が空です",
		"hostname이 비어 있습니다",
		"Hostname пуст")
	addToolWebValidation(KeyToolWebDomainBuildRequest,
		"domain_info preflight: build request: %v",
		"domain_info preflight：构建请求失败：%v",
		"domain_info preflight: Anfrage konnte nicht erstellt werden: %v",
		"domain_info preflight: リクエストを構築できませんでした: %v",
		"domain_info preflight: 요청을 구성하지 못했습니다: %v",
		"domain_info preflight: не удалось сформировать запрос: %v")
	addToolWebValidation(KeyToolWebDomainRequest,
		"domain_info request: %v",
		"domain_info 请求失败：%v",
		"domain_info-Anfrage fehlgeschlagen: %v",
		"domain_info リクエストに失敗しました: %v",
		"domain_info 요청 실패: %v",
		"Ошибка запроса domain_info: %v")
	addToolWebValidation(KeyToolWebDomainStatus,
		"domain check returned status %d",
		"domain 检查返回 status %d",
		"Domain-Prüfung gab Status %d zurück",
		"domain チェックが status %d を返しました",
		"domain 확인에서 status %d을(를) 반환했습니다",
		"Проверка домена вернула status %d")
	addToolWebValidation(KeyToolWebDomainDecodeResponse,
		"decode domain_info response: %v",
		"解析 domain_info 响应失败：%v",
		"domain_info-Antwort konnte nicht decodiert werden: %v",
		"domain_info レスポンスをデコードできませんでした: %v",
		"domain_info 응답을 디코딩하지 못했습니다: %v",
		"Не удалось декодировать ответ domain_info: %v")
	addToolWebValidation(KeyToolWebDomainMissingCanFetch,
		"domain_info response missing boolean can_fetch",
		"domain_info 响应缺少 boolean can_fetch",
		"In der domain_info-Antwort fehlt das boolesche Feld can_fetch",
		"domain_info レスポンスに boolean の can_fetch がありません",
		"domain_info 응답에 boolean can_fetch가 없습니다",
		"В ответе domain_info отсутствует логическое поле can_fetch")
}

func addToolWebValidation(key Key, en, zh, de, ja, ko, ru string) {
	semanticTranslations[key] = map[Language]string{
		LangEN: en,
		LangZH: zh,
		LangDE: de,
		LangJA: ja,
		LangKO: ko,
		LangRU: ru,
	}
}
