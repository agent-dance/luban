package i18n

const (
	KeyToolConfigDescription        Key = "tool.config.description"
	KeyToolConfigSettingDescription Key = "tool.config.input.setting.description"
	KeyToolConfigValueDescription   Key = "tool.config.input.value.description"
	KeyToolConfigSettingRequired    Key = "tool.config.setting_required"
	KeyToolConfigNullValue          Key = "tool.config.null_value"
	KeyToolConfigNotSet             Key = "tool.config.not_set"
	KeyToolConfigValue              Key = "tool.config.value"
	KeyToolConfigSaveFailed         Key = "tool.config.save_failed"
	KeyToolConfigUpdated            Key = "tool.config.updated"
)

var toolConfigKeys = [...]Key{
	KeyToolConfigDescription,
	KeyToolConfigSettingDescription,
	KeyToolConfigValueDescription,
	KeyToolConfigSettingRequired,
	KeyToolConfigNullValue,
	KeyToolConfigNotSet,
	KeyToolConfigValue,
	KeyToolConfigSaveFailed,
	KeyToolConfigUpdated,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyToolConfigDescription,
		"Read or write LUBAN Code configuration settings",
		"读取或写入 LUBAN Code 配置设置",
		"LUBAN Code-Konfigurationseinstellungen lesen oder schreiben",
		"LUBAN Code の構成設定を読み書きします",
		"LUBAN Code 구성 설정을 읽거나 씁니다",
		"Чтение или запись параметров конфигурации LUBAN Code")
	add(KeyToolConfigSettingDescription,
		"Setting key. Known keys: %s",
		"设置键。已知键：%s",
		"Einstellungsschlüssel. Bekannte Schlüssel: %s",
		"設定キー。既知のキー: %s",
		"설정 키. 알려진 키: %s",
		"Ключ настройки. Известные ключи: %s")
	add(KeyToolConfigValueDescription,
		"Value to write. Omit it to read the current setting.",
		"要写入的值；省略此字段可读取当前设置。",
		"Zu schreibender Wert. Zum Lesen der aktuellen Einstellung weglassen.",
		"書き込む値。現在の設定を読み取る場合は省略してください。",
		"쓸 값입니다. 현재 설정을 읽으려면 생략하세요.",
		"Записываемое значение. Не указывайте его, чтобы прочитать текущую настройку.")
	add(KeyToolConfigSettingRequired,
		"Error: 'setting' must be a non-empty string",
		"错误：setting 必须是非空字符串",
		"Fehler: setting muss eine nicht leere Zeichenfolge sein",
		"エラー: setting には空でない文字列を指定してください",
		"오류: setting은 비어 있지 않은 문자열이어야 합니다",
		"Ошибка: setting должен быть непустой строкой")
	add(KeyToolConfigNullValue,
		"Error: 'value' must not be null; omit it to read the current setting",
		"错误：value 不得为 null；如需读取当前设置，请省略该字段",
		"Fehler: value darf nicht null sein; zum Lesen der aktuellen Einstellung weglassen",
		"エラー: value に null は指定できません。現在の設定を読み取る場合は省略してください",
		"오류: value는 null일 수 없습니다. 현재 설정을 읽으려면 생략하세요",
		"Ошибка: value не может быть null; не указывайте его, чтобы прочитать текущую настройку")
	add(KeyToolConfigNotSet,
		"(not set) %s",
		"（未设置）%s",
		"(nicht gesetzt) %s",
		"（未設定）%s",
		"(설정되지 않음) %s",
		"(не задано) %s")
	add(KeyToolConfigValue, "%s = %s", "%s = %s", "%s = %s", "%s = %s", "%s = %s", "%s = %s")
	add(KeyToolConfigSaveFailed,
		"Error: failed to save config: %s",
		"错误：无法保存配置：%s",
		"Fehler: Konfiguration konnte nicht gespeichert werden: %s",
		"エラー: 設定を保存できませんでした: %s",
		"오류: 설정을 저장하지 못했습니다: %s",
		"Ошибка: не удалось сохранить конфигурацию: %s")
	add(KeyToolConfigUpdated,
		"Config updated: %s = %s",
		"配置已更新：%s = %s",
		"Konfiguration aktualisiert: %s = %s",
		"設定を更新しました: %s = %s",
		"설정이 업데이트되었습니다: %s = %s",
		"Конфигурация обновлена: %s = %s")
}
