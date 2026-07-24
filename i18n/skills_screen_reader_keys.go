package i18n

// Semantic copy used by the screen-reader skill router. Skill names, stable
// IDs, catalog revisions, and backend errors remain format values.
const (
	KeyScreenReaderSkillCatalogUnavailable Key = "screen_reader.skills.catalog_unavailable"
	KeyScreenReaderSkillInvokerUnavailable Key = "screen_reader.skills.invoker_unavailable"
	KeyScreenReaderSkillLookupFailed       Key = "screen_reader.skills.lookup_failed"
	KeyScreenReaderSkillInvalidSelector    Key = "screen_reader.skills.invalid_selector"
	KeyScreenReaderSkillNotFound           Key = "screen_reader.skills.not_found"
	KeyScreenReaderSkillAmbiguous          Key = "screen_reader.skills.ambiguous"
	KeyScreenReaderSkillUnavailable        Key = "screen_reader.skills.unavailable"
	KeyScreenReaderSkillInvocationFailed   Key = "screen_reader.skills.invocation_failed"
	KeyScreenReaderSkillInvocationRejected Key = "screen_reader.skills.invocation_rejected"
	KeyScreenReaderSkillEmptyEnvelope      Key = "screen_reader.skills.empty_envelope"
	KeyScreenReaderSkillTranscriptInvoke   Key = "screen_reader.skills.transcript_invocation"
	KeyScreenReaderSkillArgumentsProvided  Key = "screen_reader.skills.arguments_provided"
	KeyScreenReaderSkillArgumentsOmitted   Key = "screen_reader.skills.arguments_omitted"
)

func init() {
	for key, values := range map[Key][6]string{
		KeyScreenReaderSkillCatalogUnavailable: {
			"The live skill catalog is unavailable in screen-reader mode.",
			"屏幕阅读器模式下无法使用实时技能目录。",
			"Der Live-Skill-Katalog ist im Screenreader-Modus nicht verfügbar.",
			"スクリーンリーダーモードではライブスキルカタログを利用できません。",
			"스크린 리더 모드에서 실시간 스킬 카탈로그를 사용할 수 없습니다.",
			"Каталог навыков в реальном времени недоступен в режиме экранного диктора.",
		},
		KeyScreenReaderSkillInvokerUnavailable: {
			"Explicit skill invocation is unavailable in screen-reader mode.",
			"屏幕阅读器模式下无法显式调用技能。",
			"Der ausdrückliche Skill-Aufruf ist im Screenreader-Modus nicht verfügbar.",
			"スクリーンリーダーモードではスキルを明示的に呼び出せません。",
			"스크린 리더 모드에서 스킬을 명시적으로 호출할 수 없습니다.",
			"Явный вызов навыка недоступен в режиме экранного диктора.",
		},
		KeyScreenReaderSkillLookupFailed: {
			"The current skill catalog could not be read: %v",
			"无法读取当前技能目录：%v",
			"Der aktuelle Skill-Katalog konnte nicht gelesen werden: %v",
			"現在のスキルカタログを読み取れませんでした: %v",
			"현재 스킬 카탈로그를 읽을 수 없습니다: %v",
			"Не удалось прочитать текущий каталог навыков: %v",
		},
		KeyScreenReaderSkillInvalidSelector: {
			"Skill command %s is not a valid skill name or stable skill ID.",
			"技能命令 %s 不是有效的技能名称或稳定技能 ID。",
			"Der Skill-Befehl %s ist weder ein gültiger Skill-Name noch eine stabile Skill-ID.",
			"スキルコマンド %s は有効なスキル名または安定 ID ではありません。",
			"스킬 명령 %s은(는) 올바른 스킬 이름이나 안정 스킬 ID가 아닙니다.",
			"Команда навыка %s не является допустимым именем навыка или стабильным ID.",
		},
		KeyScreenReaderSkillNotFound: {
			"Unknown command %s. Use /help, or run /skills list to inspect explicit skill commands.",
			"未知命令 %s。请使用 /help，或运行 /skills list 查看可显式调用的技能命令。",
			"Unbekannter Befehl %s. Verwende /help oder führe /skills list aus, um ausdrücklich aufrufbare Skills anzuzeigen.",
			"不明なコマンド %s です。/help を使うか、/skills list を実行して明示的に呼び出せるスキルを確認してください。",
			"알 수 없는 명령 %s입니다. /help를 사용하거나 /skills list를 실행하여 명시적으로 호출할 수 있는 스킬을 확인하세요.",
			"Неизвестная команда %s. Используйте /help или выполните /skills list, чтобы просмотреть навыки для явного вызова.",
		},
		KeyScreenReaderSkillAmbiguous: {
			"Skill command %s is ambiguous. Invoke one of these stable IDs instead: %s.",
			"技能命令 %s 存在歧义。请改用以下稳定 ID 之一调用：%s。",
			"Der Skill-Befehl %s ist mehrdeutig. Rufe stattdessen eine dieser stabilen IDs auf: %s.",
			"スキルコマンド %s は曖昧です。代わりに次の安定 ID のいずれかを呼び出してください: %s。",
			"스킬 명령 %s은(는) 모호합니다. 대신 다음 안정 ID 중 하나를 호출하세요: %s.",
			"Команда навыка %s неоднозначна. Вместо нее вызовите один из стабильных ID: %s.",
		},
		KeyScreenReaderSkillUnavailable: {
			"Skill %s is not available for explicit user invocation in the current catalog.",
			"当前目录中的技能 %s 不可供用户显式调用。",
			"Skill %s ist im aktuellen Katalog nicht für ausdrückliche Benutzeraufrufe verfügbar.",
			"現在のカタログではスキル %s をユーザーから明示的に呼び出せません。",
			"현재 카탈로그에서 스킬 %s을(를) 사용자가 명시적으로 호출할 수 없습니다.",
			"Навык %s в текущем каталоге недоступен для явного вызова пользователем.",
		},
		KeyScreenReaderSkillInvocationFailed: {
			"Skill %s could not be invoked: %v",
			"无法调用技能 %s：%v",
			"Skill %s konnte nicht aufgerufen werden: %v",
			"スキル %s を呼び出せませんでした: %v",
			"스킬 %s을(를) 호출할 수 없습니다: %v",
			"Не удалось вызвать навык %s: %v",
		},
		KeyScreenReaderSkillInvocationRejected: {
			"Skill %s rejected the explicit invocation.",
			"技能 %s 拒绝了显式调用。",
			"Skill %s hat den ausdrücklichen Aufruf abgelehnt.",
			"スキル %s は明示的な呼び出しを拒否しました。",
			"스킬 %s이(가) 명시적 호출을 거부했습니다.",
			"Навык %s отклонил явный вызов.",
		},
		KeyScreenReaderSkillEmptyEnvelope: {
			"Skill %s returned no invocation instructions, so no model turn was started.",
			"技能 %s 未返回调用指令，因此未启动模型轮次。",
			"Skill %s hat keine Aufrufanweisungen zurückgegeben; daher wurde kein Modellturn gestartet.",
			"スキル %s から呼び出し指示が返されなかったため、モデルのターンは開始されませんでした。",
			"스킬 %s이(가) 호출 지침을 반환하지 않아 모델 턴을 시작하지 않았습니다.",
			"Навык %s не вернул инструкций вызова, поэтому ход модели не был запущен.",
		},
		KeyScreenReaderSkillTranscriptInvoke: {
			"Explicit skill invocation: %s (%s)",
			"显式技能调用：%s（%s）",
			"Ausdrücklicher Skill-Aufruf: %s (%s)",
			"明示的なスキル呼び出し: %s（%s）",
			"명시적 스킬 호출: %s(%s)",
			"Явный вызов навыка: %s (%s)",
		},
		KeyScreenReaderSkillArgumentsProvided: {
			"arguments provided",
			"已提供参数",
			"Argumente angegeben",
			"引数あり",
			"인수 제공됨",
			"аргументы переданы",
		},
		KeyScreenReaderSkillArgumentsOmitted: {
			"arguments omitted",
			"未提供参数",
			"Argumente ausgelassen",
			"引数なし",
			"인수 생략됨",
			"аргументы не переданы",
		},
	} {
		semanticTranslations[key] = skillsScreenReaderTranslations(values[0], values[1], values[2], values[3], values[4], values[5])
	}
}

func skillsScreenReaderTranslations(en, zh, de, ja, ko, ru string) map[Language]string {
	return map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
}
