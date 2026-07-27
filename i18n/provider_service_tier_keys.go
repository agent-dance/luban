package i18n

const (
	KeyProviderServiceTierInvalid     Key = "provider.service_tier.invalid"
	KeyProviderServiceTierUnsupported Key = "provider.service_tier.unsupported"
	KeyProviderServiceTierMismatch    Key = "provider.service_tier.mismatch"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyProviderServiceTierInvalid,
		"The requested service tier %q is invalid; only default is supported",
		"请求的 service tier %q 无效；当前仅支持 default",
		"Der angeforderte Service-Tier %q ist ungültig; unterstützt wird nur default",
		"指定された service tier %q は無効です。現在サポートされるのは default のみです",
		"요청한 service tier %q은(는) 올바르지 않습니다. 현재 default만 지원합니다",
		"Запрошенный service tier %q недопустим; поддерживается только default")
	add(KeyProviderServiceTierUnsupported,
		"Provider %s with model %s does not explicitly support a pinned service tier",
		"Provider %s 的模型 %s 未明确支持固定 service tier",
		"Provider %s mit Modell %s unterstützt keinen ausdrücklich fixierten Service-Tier",
		"Provider %s のモデル %s は固定 service tier を明示的にサポートしていません",
		"Provider %s의 모델 %s은(는) 고정 service tier를 명시적으로 지원하지 않습니다",
		"Провайдер %s с моделью %s явно не поддерживает фиксированный service tier")
	add(KeyProviderServiceTierMismatch,
		"The provider returned service tier %q instead of the requested %q",
		"Provider 返回的 service tier 为 %q，与请求的 %q 不一致",
		"Der Provider hat den Service-Tier %q statt des angeforderten %q zurückgegeben",
		"Provider が service tier %q を返しましたが、要求値は %q です",
		"Provider가 service tier %q을(를) 반환했지만 요청값은 %q입니다",
		"Провайдер вернул service tier %q вместо запрошенного %q")
}
