package i18n

const (
	KeyCLIFlagServiceTier    Key = "cli.flag.service_tier"
	KeyCLIServiceTierInvalid Key = "cli.service_tier.invalid"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyCLIFlagServiceTier,
		"Provider service tier; contract-bound runs use default",
		"Provider service tier；受测评契约约束的运行使用 default",
		"Provider-Service-Tier; vertraglich gebundene Läufe verwenden default",
		"Provider の service tier。契約固定の実行では default を使用します",
		"Provider service tier. 계약 고정 실행에서는 default를 사용합니다",
		"Service tier провайдера; запуски с фиксированным контрактом используют default")
	add(KeyCLIServiceTierInvalid,
		"Unsupported service tier %q; use default",
		"不支持 service tier %q；请使用 default",
		"Nicht unterstützter Service-Tier %q; verwenden Sie default",
		"service tier %q はサポートされていません。default を使用してください",
		"service tier %q은(는) 지원되지 않습니다. default를 사용하세요",
		"Service tier %q не поддерживается; используйте default")
}
