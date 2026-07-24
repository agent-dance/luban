package i18n

const (
	KeySkillsUserErrorStoreUnavailable    Key = "skills.user_error.store_unavailable"
	KeySkillsUserErrorCatalogChanged      Key = "skills.user_error.catalog_changed"
	KeySkillsUserErrorInvalidIdentifier   Key = "skills.user_error.invalid_identifier"
	KeySkillsUserErrorInvalidContent      Key = "skills.user_error.invalid_content"
	KeySkillsUserErrorInvalidCatalogState Key = "skills.user_error.invalid_catalog_state"
	KeySkillsUserErrorInvalidVisibility   Key = "skills.user_error.invalid_visibility"
	KeySkillsUserErrorInvalidPolicy       Key = "skills.user_error.invalid_policy"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeySkillsUserErrorStoreUnavailable,
		"Skill settings are unavailable in this runtime.",
		"当前运行环境无法使用 Skill 设置。",
		"Die Skill-Einstellungen sind in dieser Laufzeitumgebung nicht verfügbar.",
		"この実行環境では Skill 設定を利用できません。",
		"현재 런타임에서는 Skill 설정을 사용할 수 없습니다.",
		"В этой среде выполнения настройки Skill недоступны.")
	add(KeySkillsUserErrorCatalogChanged,
		"The skill catalog changed. Refresh and try again.",
		"Skill 目录已发生变化。请刷新后重试。",
		"Der Skill-Katalog wurde geändert. Aktualisiere ihn und versuche es erneut.",
		"Skill カタログが変更されました。更新してから再試行してください。",
		"Skill 카탈로그가 변경되었습니다. 새로고침한 후 다시 시도하세요.",
		"Каталог Skill изменился. Обновите его и повторите попытку.")
	add(KeySkillsUserErrorInvalidIdentifier,
		"The skill identifier or source is invalid.",
		"Skill 标识或来源无效。",
		"Die Skill-ID oder -Quelle ist ungültig.",
		"Skill の識別子または取得元が無効です。",
		"Skill 식별자 또는 소스가 올바르지 않습니다.",
		"Недопустимый идентификатор или источник Skill.")
	add(KeySkillsUserErrorInvalidContent,
		"The skill content could not be verified.",
		"无法验证 Skill 内容。",
		"Der Inhalt des Skills konnte nicht verifiziert werden.",
		"Skill の内容を検証できませんでした。",
		"Skill 콘텐츠를 검증할 수 없습니다.",
		"Не удалось проверить содержимое Skill.")
	add(KeySkillsUserErrorInvalidCatalogState,
		"The skill catalog is inconsistent. Refresh and try again.",
		"Skill 目录状态不一致。请刷新后重试。",
		"Der Skill-Katalog ist inkonsistent. Aktualisiere ihn und versuche es erneut.",
		"Skill カタログの状態が不整合です。更新してから再試行してください。",
		"Skill 카탈로그 상태가 일치하지 않습니다. 새로고침한 후 다시 시도하세요.",
		"Состояние каталога Skill несогласованно. Обновите его и повторите попытку.")
	add(KeySkillsUserErrorInvalidVisibility,
		"The requested skill visibility is invalid.",
		"请求的 Skill 可见性设置无效。",
		"Die angeforderte Sichtbarkeit des Skills ist ungültig.",
		"指定された Skill の表示範囲が無効です。",
		"요청한 Skill 공개 범위가 올바르지 않습니다.",
		"Запрошенный режим видимости Skill недопустим.")
	add(KeySkillsUserErrorInvalidPolicy,
		"The skill policy is invalid.",
		"Skill 策略无效。",
		"Die Skill-Richtlinie ist ungültig.",
		"Skill ポリシーが無効です。",
		"Skill 정책이 올바르지 않습니다.",
		"Политика Skill недопустима.")
}
