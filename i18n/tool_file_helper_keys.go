package i18n

// Semantic copy returned by filesystem helpers whose errors flow into
// user-visible file tool results. Paths, settings fields, concrete types, and
// underlying filesystem/schema errors remain raw format arguments.
const (
	KeyToolFileHelperStatFailed                 Key = "tool.file.helper.stat_failed"
	KeyToolFileHelperResolveSymlinkFailed       Key = "tool.file.helper.resolve_symlink_failed"
	KeyToolFileHelperPathIsDirectory            Key = "tool.file.helper.path_is_directory"
	KeyToolFileHelperReadFailed                 Key = "tool.file.helper.read_failed"
	KeyToolFileHelperWriteTargetOutsideAllowed  Key = "tool.file.helper.write_target_outside_allowed"
	KeyToolFileHelperCreatedAfterCheck          Key = "tool.file.helper.created_after_check"
	KeyToolFileHelperRecheckWriteTargetFailed   Key = "tool.file.helper.recheck_write_target_failed"
	KeyToolFileHelperSymlinkTargetChanged       Key = "tool.file.helper.symlink_target_changed"
	KeyToolFileHelperWriteTargetReplaced        Key = "tool.file.helper.write_target_replaced"
	KeyToolFileHelperEditTargetReplacedBefore   Key = "tool.file.helper.edit_target_replaced_before_read"
	KeyToolFileHelperEditTargetChangedWhileRead Key = "tool.file.helper.edit_target_changed_while_read"
	KeyToolFileHelperEditTargetOutsideAllowed   Key = "tool.file.helper.edit_target_outside_allowed"
	KeyToolFileHelperRecheckEditTargetFailed    Key = "tool.file.helper.recheck_edit_target_failed"
	KeyToolFileHelperEditThroughSymlink         Key = "tool.file.helper.edit_through_symlink"
	KeyToolFileHelperEditTargetReplacedAfter    Key = "tool.file.helper.edit_target_replaced_after_read"
	KeyToolFileHelperEditTargetChangedAfter     Key = "tool.file.helper.edit_target_changed_after_read"
	KeyToolFileHelperPathOutsideAllowed         Key = "tool.file.helper.path_outside_allowed"
	KeyToolFileHelperVerifyFDPathFailed         Key = "tool.file.helper.verify_fd_path_failed"
	KeyToolFileHelperSettingsInvalidJSON        Key = "tool.file.helper.settings.invalid_json"
	KeyToolFileHelperSettingsTopLevelObject     Key = "tool.file.helper.settings.top_level_object"
	KeyToolFileHelperSettingsSkillOverrides     Key = "tool.file.helper.settings.invalid_skill_overrides"
	KeyToolFileHelperSettingsMissingKey         Key = "tool.file.helper.settings.missing_required_key"
	KeyToolFileHelperSettingsUnknownKey         Key = "tool.file.helper.settings.unknown_top_level_key"
	KeyToolFileHelperSkillOverridesObject       Key = "tool.file.helper.settings.skill_overrides_object"
	KeyToolFileHelperSkillOverrideKey           Key = "tool.file.helper.settings.skill_override_key"
	KeyToolFileHelperSkillOverrideRecord        Key = "tool.file.helper.settings.skill_override_record"
	KeyToolFileHelperSkillOverrideShape         Key = "tool.file.helper.settings.skill_override_shape"
	KeyToolFileHelperSkillOverrideField         Key = "tool.file.helper.settings.skill_override_field"
	KeyToolFileHelperSkillOverrideMissingField  Key = "tool.file.helper.settings.skill_override_missing_field"
	KeyToolFileHelperSkillOverrideStringField   Key = "tool.file.helper.settings.skill_override_string_field"
	KeyToolFileHelperSkillOverrideLastNonOff    Key = "tool.file.helper.settings.skill_override_last_non_off"
)

var toolFileHelperKeys = []Key{
	KeyToolFileHelperStatFailed,
	KeyToolFileHelperResolveSymlinkFailed,
	KeyToolFileHelperPathIsDirectory,
	KeyToolFileHelperReadFailed,
	KeyToolFileHelperWriteTargetOutsideAllowed,
	KeyToolFileHelperCreatedAfterCheck,
	KeyToolFileHelperRecheckWriteTargetFailed,
	KeyToolFileHelperSymlinkTargetChanged,
	KeyToolFileHelperWriteTargetReplaced,
	KeyToolFileHelperEditTargetReplacedBefore,
	KeyToolFileHelperEditTargetChangedWhileRead,
	KeyToolFileHelperEditTargetOutsideAllowed,
	KeyToolFileHelperRecheckEditTargetFailed,
	KeyToolFileHelperEditThroughSymlink,
	KeyToolFileHelperEditTargetReplacedAfter,
	KeyToolFileHelperEditTargetChangedAfter,
	KeyToolFileHelperPathOutsideAllowed,
	KeyToolFileHelperVerifyFDPathFailed,
	KeyToolFileHelperSettingsInvalidJSON,
	KeyToolFileHelperSettingsTopLevelObject,
	KeyToolFileHelperSettingsSkillOverrides,
	KeyToolFileHelperSettingsMissingKey,
	KeyToolFileHelperSettingsUnknownKey,
	KeyToolFileHelperSkillOverridesObject,
	KeyToolFileHelperSkillOverrideKey,
	KeyToolFileHelperSkillOverrideRecord,
	KeyToolFileHelperSkillOverrideShape,
	KeyToolFileHelperSkillOverrideField,
	KeyToolFileHelperSkillOverrideMissingField,
	KeyToolFileHelperSkillOverrideStringField,
	KeyToolFileHelperSkillOverrideLastNonOff,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyToolFileHelperStatFailed,
		"failed to stat file: %v",
		"无法获取文件状态：%v",
		"Dateistatus konnte nicht ermittelt werden: %v",
		"ファイルの状態を取得できませんでした: %v",
		"파일 상태를 확인할 수 없습니다: %v",
		"Не удалось получить сведения о файле: %v")
	add(KeyToolFileHelperResolveSymlinkFailed,
		"failed to resolve symlink %q: %v",
		"无法解析符号链接 %q：%v",
		"Symbolischer Link %q konnte nicht aufgelöst werden: %v",
		"シンボリックリンク %q を解決できませんでした: %v",
		"심볼릭 링크 %q을(를) 확인할 수 없습니다: %v",
		"Не удалось разрешить символическую ссылку %q: %v")
	add(KeyToolFileHelperPathIsDirectory,
		"path %q is a directory, not a file",
		"路径 %q 是目录，不是文件",
		"Pfad %q ist ein Verzeichnis, keine Datei",
		"パス %q はファイルではなくディレクトリです",
		"경로 %q은(는) 파일이 아니라 디렉터리입니다",
		"Путь %q ведёт к каталогу, а не к файлу")
	add(KeyToolFileHelperReadFailed,
		"failed to read file: %v",
		"无法读取文件：%v",
		"Datei konnte nicht gelesen werden: %v",
		"ファイルを読み込めませんでした: %v",
		"파일을 읽을 수 없습니다: %v",
		"Не удалось прочитать файл: %v")
	add(KeyToolFileHelperWriteTargetOutsideAllowed,
		"write target changed outside allowed directories: %v",
		"写入目标已变更到允许目录之外：%v",
		"Das Schreibziel wurde außerhalb der zulässigen Verzeichnisse geändert: %v",
		"書き込み先が許可されたディレクトリの外に変更されました: %v",
		"쓰기 대상이 허용된 디렉터리 밖으로 변경되었습니다: %v",
		"Цель записи была изменена и теперь находится вне разрешённых каталогов: %v")
	add(KeyToolFileHelperCreatedAfterCheck,
		"file was created after it was checked; read it before writing",
		"文件在检查后被创建；请在写入前先读取该文件",
		"Die Datei wurde nach der Prüfung erstellt; lies sie vor dem Schreiben ein",
		"確認後にファイルが作成されました。書き込む前に読み込んでください",
		"확인 후 파일이 생성되었습니다. 쓰기 전에 파일을 읽으세요",
		"Файл был создан после проверки; прочитайте его перед записью")
	add(KeyToolFileHelperRecheckWriteTargetFailed,
		"failed to recheck write target: %v",
		"无法重新检查写入目标：%v",
		"Schreibziel konnte nicht erneut geprüft werden: %v",
		"書き込み先を再確認できませんでした: %v",
		"쓰기 대상을 다시 확인할 수 없습니다: %v",
		"Не удалось повторно проверить цель записи: %v")
	add(KeyToolFileHelperSymlinkTargetChanged,
		"symlink target changed after it was read; read it again before writing",
		"符号链接的目标在读取后发生了变化；请在写入前重新读取",
		"Das Ziel des symbolischen Links wurde nach dem Lesen geändert; lies die Datei vor dem Schreiben erneut ein",
		"読み込み後にシンボリックリンクの参照先が変更されました。書き込む前にもう一度読み込んでください",
		"읽은 후 심볼릭 링크 대상이 변경되었습니다. 쓰기 전에 다시 읽으세요",
		"Цель символической ссылки изменилась после чтения; прочитайте файл повторно перед записью")
	add(KeyToolFileHelperWriteTargetReplaced,
		"file was replaced after it was read; read it again before writing",
		"文件在读取后被替换；请在写入前重新读取",
		"Die Datei wurde nach dem Lesen ersetzt; lies sie vor dem Schreiben erneut ein",
		"読み込み後にファイルが置き換えられました。書き込む前にもう一度読み込んでください",
		"읽은 후 파일이 교체되었습니다. 쓰기 전에 다시 읽으세요",
		"Файл был заменён после чтения; прочитайте его повторно перед записью")
	add(KeyToolFileHelperEditTargetReplacedBefore,
		"file was replaced before it could be read; read it again before editing",
		"文件在读取前被替换；请在编辑前重新读取",
		"Die Datei wurde ersetzt, bevor sie gelesen werden konnte; lies sie vor dem Bearbeiten erneut ein",
		"読み込む前にファイルが置き換えられました。編集する前にもう一度読み込んでください",
		"파일을 읽기 전에 교체되었습니다. 편집하기 전에 다시 읽으세요",
		"Файл был заменён до чтения; прочитайте его повторно перед редактированием")
	add(KeyToolFileHelperEditTargetChangedWhileRead,
		"file changed while it was being read; read it again before editing",
		"文件在读取过程中发生了变化；请在编辑前重新读取",
		"Die Datei wurde während des Lesens geändert; lies sie vor dem Bearbeiten erneut ein",
		"読み込み中にファイルが変更されました。編集する前にもう一度読み込んでください",
		"파일을 읽는 동안 변경되었습니다. 편집하기 전에 다시 읽으세요",
		"Файл изменился во время чтения; прочитайте его повторно перед редактированием")
	add(KeyToolFileHelperEditTargetOutsideAllowed,
		"edit target changed outside allowed directories: %v",
		"编辑目标已变更到允许目录之外：%v",
		"Das Bearbeitungsziel wurde außerhalb der zulässigen Verzeichnisse geändert: %v",
		"編集先が許可されたディレクトリの外に変更されました: %v",
		"편집 대상이 허용된 디렉터리 밖으로 변경되었습니다: %v",
		"Цель редактирования была изменена и теперь находится вне разрешённых каталогов: %v")
	add(KeyToolFileHelperRecheckEditTargetFailed,
		"failed to recheck edit target: %v",
		"无法重新检查编辑目标：%v",
		"Bearbeitungsziel konnte nicht erneut geprüft werden: %v",
		"編集先を再確認できませんでした: %v",
		"편집 대상을 다시 확인할 수 없습니다: %v",
		"Не удалось повторно проверить цель редактирования: %v")
	add(KeyToolFileHelperEditThroughSymlink,
		"refusing to edit through symlink %q",
		"拒绝通过符号链接 %q 编辑文件",
		"Bearbeitung über den symbolischen Link %q wird verweigert",
		"シンボリックリンク %q 経由の編集は拒否されました",
		"심볼릭 링크 %q을(를) 통한 편집을 거부합니다",
		"Редактирование через символическую ссылку %q запрещено")
	add(KeyToolFileHelperEditTargetReplacedAfter,
		"file was replaced after it was read; read it again before editing",
		"文件在读取后被替换；请在编辑前重新读取",
		"Die Datei wurde nach dem Lesen ersetzt; lies sie vor dem Bearbeiten erneut ein",
		"読み込み後にファイルが置き換えられました。編集する前にもう一度読み込んでください",
		"읽은 후 파일이 교체되었습니다. 편집하기 전에 다시 읽으세요",
		"Файл был заменён после чтения; прочитайте его повторно перед редактированием")
	add(KeyToolFileHelperEditTargetChangedAfter,
		"file changed after it was read; read it again before editing",
		"文件在读取后发生了变化；请在编辑前重新读取",
		"Die Datei wurde nach dem Lesen geändert; lies sie vor dem Bearbeiten erneut ein",
		"読み込み後にファイルが変更されました。編集する前にもう一度読み込んでください",
		"읽은 후 파일이 변경되었습니다. 편집하기 전에 다시 읽으세요",
		"Файл изменился после чтения; прочитайте его повторно перед редактированием")
	add(KeyToolFileHelperPathOutsideAllowed,
		"path is outside allowed directories (resolved to %s)",
		"路径位于允许目录之外（解析为 %s）",
		"Der Pfad liegt außerhalb der zulässigen Verzeichnisse (aufgelöst zu %s)",
		"パスは許可されたディレクトリの外です（解決先: %s）",
		"경로가 허용된 디렉터리 밖에 있습니다(확인된 경로: %s)",
		"Путь находится вне разрешённых каталогов (разрешён в %s)")
	add(KeyToolFileHelperVerifyFDPathFailed,
		"cannot verify fd path: %v",
		"无法验证文件描述符路径：%v",
		"Pfad des Dateideskriptors kann nicht geprüft werden: %v",
		"ファイル記述子のパスを確認できません: %v",
		"파일 디스크립터 경로를 확인할 수 없습니다: %v",
		"Не удалось проверить путь файлового дескриптора: %v")
	add(KeyToolFileHelperSettingsInvalidJSON,
		"settings.json validation failed after edit: invalid JSON (%v). Refusing to write a settings file the runtime cannot parse.",
		"编辑后的 settings.json 校验失败：JSON 无效（%v）。拒绝写入 runtime 无法解析的设置文件。",
		"Die Prüfung von settings.json nach der Bearbeitung ist fehlgeschlagen: ungültiges JSON (%v). Eine Einstellungsdatei, die die Runtime nicht parsen kann, wird nicht geschrieben.",
		"編集後の settings.json の検証に失敗しました: JSON が無効です（%v）。runtime が解析できない設定ファイルは書き込みません。",
		"편집 후 settings.json 검증에 실패했습니다. JSON이 올바르지 않습니다(%v). runtime이 해석할 수 없는 설정 파일은 쓰지 않습니다.",
		"Проверка settings.json после редактирования завершилась ошибкой: недопустимый JSON (%v). Файл настроек, который runtime не может разобрать, не будет записан.")
	add(KeyToolFileHelperSettingsTopLevelObject,
		"settings.json validation failed after edit: top-level value must be an object, got %T",
		"编辑后的 settings.json 校验失败：顶层值必须是对象，实际为 %T",
		"Die Prüfung von settings.json nach der Bearbeitung ist fehlgeschlagen: Der Wert auf oberster Ebene muss ein Objekt sein, erhalten wurde %T",
		"編集後の settings.json の検証に失敗しました: トップレベルの値は object である必要がありますが、%T でした",
		"편집 후 settings.json 검증에 실패했습니다. 최상위 값은 object여야 하지만 %T입니다",
		"Проверка settings.json после редактирования завершилась ошибкой: значение верхнего уровня должно быть объектом, получено %T")
	add(KeyToolFileHelperSettingsSkillOverrides,
		"settings.json validation failed after edit: invalid skillOverrides (%v)",
		"编辑后的 settings.json 校验失败：skillOverrides 无效（%v）",
		"Die Prüfung von settings.json nach der Bearbeitung ist fehlgeschlagen: ungültige skillOverrides (%v)",
		"編集後の settings.json の検証に失敗しました: skillOverrides が無効です（%v）",
		"편집 후 settings.json 검증에 실패했습니다. skillOverrides가 올바르지 않습니다(%v)",
		"Проверка settings.json после редактирования завершилась ошибкой: недопустимые skillOverrides (%v)")
	add(KeyToolFileHelperSettingsMissingKey,
		"settings.json validation failed after edit: missing required key %q. The runtime cannot start without a populated permissions block.",
		"编辑后的 settings.json 校验失败：缺少必需的 key %q。permissions block 未配置时，runtime 无法启动。",
		"Die Prüfung von settings.json nach der Bearbeitung ist fehlgeschlagen: Der erforderliche Key %q fehlt. Ohne einen ausgefüllten permissions-Block kann die Runtime nicht starten.",
		"編集後の settings.json の検証に失敗しました: 必須 key %q がありません。permissions block が設定されていないと runtime は起動できません。",
		"편집 후 settings.json 검증에 실패했습니다. 필수 key %q이(가) 없습니다. permissions block이 설정되지 않으면 runtime을 시작할 수 없습니다.",
		"Проверка settings.json после редактирования завершилась ошибкой: отсутствует обязательный key %q. Runtime не может запуститься без заполненного блока permissions.")
	add(KeyToolFileHelperSettingsUnknownKey,
		"settings.json validation failed after edit: top-level key %q is not part of the published schema (additionalProperties:false).",
		"编辑后的 settings.json 校验失败：顶层 key %q 不在已发布的 schema 中（additionalProperties:false）。",
		"Die Prüfung von settings.json nach der Bearbeitung ist fehlgeschlagen: Der Key %q auf oberster Ebene gehört nicht zum veröffentlichten Schema (additionalProperties:false).",
		"編集後の settings.json の検証に失敗しました: トップレベル key %q は公開 schema に含まれていません（additionalProperties:false）。",
		"편집 후 settings.json 검증에 실패했습니다. 최상위 key %q은(는) 공개된 schema에 포함되지 않습니다(additionalProperties:false).",
		"Проверка settings.json после редактирования завершилась ошибкой: key верхнего уровня %q отсутствует в опубликованной schema (additionalProperties:false).")
	add(KeyToolFileHelperSkillOverridesObject,
		"must be an object, got %T",
		"必须是对象，实际为 %T",
		"muss ein Objekt sein, erhalten wurde %T",
		"object である必要がありますが、%T でした",
		"object여야 하지만 %T입니다",
		"должно быть объектом, получено %T")
	add(KeyToolFileHelperSkillOverrideKey,
		"key %q: %v",
		"key %q：%v",
		"Key %q: %v",
		"key %q: %v",
		"key %q: %v",
		"key %q: %v")
	add(KeyToolFileHelperSkillOverrideRecord,
		"%s: %v",
		"%s：%v",
		"%s: %v",
		"%s: %v",
		"%s: %v",
		"%s: %v")
	add(KeyToolFileHelperSkillOverrideShape,
		"override must be a visibility string or object, got %T",
		"override 必须是 visibility string 或对象，实际为 %T",
		"Override muss ein visibility-String oder Objekt sein, erhalten wurde %T",
		"override は visibility string または object である必要がありますが、%T でした",
		"override는 visibility string 또는 object여야 하지만 %T입니다",
		"override должен быть строкой visibility или объектом, получено %T")
	add(KeyToolFileHelperSkillOverrideField,
		"field %q is not allowed",
		"不允许使用字段 %q",
		"Feld %q ist nicht zulässig",
		"フィールド %q は使用できません",
		"필드 %q은(는) 허용되지 않습니다",
		"Поле %q не разрешено")
	add(KeyToolFileHelperSkillOverrideMissingField,
		"missing required field %q",
		"缺少必需字段 %q",
		"Erforderliches Feld %q fehlt",
		"必須フィールド %q がありません",
		"필수 필드 %q이(가) 없습니다",
		"Отсутствует обязательное поле %q")
	add(KeyToolFileHelperSkillOverrideStringField,
		"field %q must be a string, got %T",
		"字段 %q 必须是字符串，实际为 %T",
		"Feld %q muss ein String sein, erhalten wurde %T",
		"フィールド %q は string である必要がありますが、%T でした",
		"필드 %q은(는) string이어야 하지만 %T입니다",
		"Поле %q должно быть строкой, получено %T")
	add(KeyToolFileHelperSkillOverrideLastNonOff,
		"%v: last_non_off is valid only for off overrides",
		"%v：last_non_off 仅对 off override 有效",
		"%v: last_non_off ist nur für off-Overrides gültig",
		"%v: last_non_off は off override の場合にのみ有効です",
		"%v: last_non_off는 off override에만 유효합니다",
		"%v: last_non_off допустим только для override со значением off")
}
