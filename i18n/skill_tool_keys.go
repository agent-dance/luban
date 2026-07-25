package i18n

// Semantic copy emitted by the Skill tool's authoritative registry boundary.
// Skill IDs, revisions, rule names, and registry outcomes remain format values.
const (
	KeyToolSkillDescription               Key = "tool.skill.description"
	KeyToolSkillInputSelectorDescription  Key = "tool.skill.input.selector.description"
	KeyToolSkillInputRevisionDescription  Key = "tool.skill.input.revision.description"
	KeyToolSkillInputArgumentsDescription Key = "tool.skill.input.arguments.description"
	KeyToolSkillAllowedTools              Key = "tool.skill.allowed_tools"

	KeySkillToolInvalidInput       Key = "tool.skill.invalid_input"
	KeySkillToolRequired           Key = "tool.skill.required"
	KeySkillToolInvalidSelector    Key = "tool.skill.invalid_selector"
	KeySkillToolExplicitUserOrigin Key = "tool.skill.explicit_user_origin"
	KeySkillToolUnavailable        Key = "tool.skill.unavailable"
	KeySkillToolRecursive          Key = "tool.skill.recursive"
	KeySkillToolNotFound           Key = "tool.skill.not_found"
	KeySkillToolAvailable          Key = "tool.skill.available"
	KeySkillToolNoneInstalled      Key = "tool.skill.none_installed"
	KeySkillToolAmbiguous          Key = "tool.skill.ambiguous"
	KeySkillToolShadowed           Key = "tool.skill.shadowed"
	KeySkillToolPolicyDeniedModel  Key = "tool.skill.policy_denied_model"
	KeySkillToolPolicyDeniedUser   Key = "tool.skill.policy_denied_user"
	KeySkillToolStale              Key = "tool.skill.stale"
	KeySkillToolRegistryFailure    Key = "tool.skill.registry_failure"
)

func init() {
	for key, values := range map[Key][6]string{
		KeyToolSkillDescription: {
			"Execute a skill in the current conversation. Available skills are listed in the conversation. Match user requests and slash-command references against that list. A matching skill is a blocking requirement: invoke it before any response, and never mention it without invoking it. Pass the stable skill ID or exact name and optional arguments. Do not invoke a skill that is already running or use this tool for built-in CLI commands.",
			"在当前对话中执行技能。可用技能会列在对话中；请将用户请求和斜杠命令引用与该列表进行匹配。匹配到技能时必须先调用它，这是回复前的阻塞要求；不要只提及技能而不调用。请传入稳定技能 ID 或准确名称以及可选参数。不要重复调用正在运行的技能，也不要用此工具执行内置 CLI 命令。",
			"Führt einen Skill in der aktuellen Unterhaltung aus. Die verfügbaren Skills sind in der Unterhaltung aufgeführt. Gleiche Benutzeranfragen und Verweise auf Slash-Befehle mit dieser Liste ab. Ein passender Skill muss zwingend vor jeder Antwort aufgerufen werden; erwähne ihn niemals, ohne ihn aufzurufen. Übergib die stabile Skill-ID oder den exakten Namen sowie optionale Argumente. Rufe keinen bereits laufenden Skill auf und verwende dieses Tool nicht für integrierte CLI-Befehle.",
			"現在の会話でスキルを実行します。利用可能なスキルは会話内に示されます。ユーザーの依頼やスラッシュコマンドへの言及をその一覧と照合してください。該当するスキルの呼び出しは必須で、応答より先に実行します。呼び出さずにスキルへ言及しないでください。安定したスキル ID または正確な名前と、必要に応じて引数を渡します。実行中のスキルを重ねて呼び出したり、組み込み CLI コマンドにこのツールを使ったりしないでください。",
			"현재 대화에서 스킬을 실행합니다. 사용 가능한 스킬은 대화에 표시됩니다. 사용자 요청과 슬래시 명령 언급을 그 목록과 대조하세요. 일치하는 스킬 호출은 필수이며 어떤 응답보다 먼저 실행해야 합니다. 호출하지 않은 스킬을 언급하지 마세요. 안정적인 스킬 ID 또는 정확한 이름과 선택적 인수를 전달합니다. 이미 실행 중인 스킬을 다시 호출하거나 내장 CLI 명령에 이 도구를 사용하지 마세요.",
			"Выполняет навык в текущем диалоге. Доступные навыки перечислены в диалоге. Сопоставьте с этим списком запрос пользователя или упоминание slash-команды. Подходящий навык необходимо вызвать до любого ответа; не упоминайте навык, не вызвав его. Передайте стабильный ID или точное имя навыка и, при необходимости, аргументы. Не вызывайте уже выполняющийся навык и не используйте этот инструмент для встроенных команд CLI.",
		},
		KeyToolSkillInputSelectorDescription: {
			"Stable skill ID or exact name from the current catalog, without a leading slash",
			"当前目录中的稳定技能 ID 或准确名称，不含开头的斜杠",
			"Stabile Skill-ID oder exakter Name aus dem aktuellen Katalog, ohne führenden Schrägstrich",
			"現在のカタログにある安定したスキル ID または正確な名前（先頭のスラッシュは付けない）",
			"현재 카탈로그의 안정적인 스킬 ID 또는 정확한 이름(앞쪽 슬래시 제외)",
			"Стабильный ID или точное имя навыка из текущего каталога, без начальной косой черты",
		},
		KeyToolSkillInputRevisionDescription: {
			"Optional skill revision from the latest catalog; a changed revision is rejected",
			"最新目录中的可选技能修订号；修订号变化时将拒绝执行",
			"Optionale Skill-Revision aus dem neuesten Katalog; eine geänderte Revision wird abgelehnt",
			"最新カタログの任意のスキルリビジョン。リビジョンが変わっている場合は拒否されます",
			"최신 카탈로그의 선택적 스킬 리비전이며, 리비전이 변경되면 실행이 거부됩니다",
			"Необязательная ревизия навыка из последнего каталога; изменившаяся ревизия отклоняется",
		},
		KeyToolSkillInputArgumentsDescription: {
			"Optional arguments forwarded to the skill's argument placeholders; when no placeholder exists, the arguments are appended to the skill body",
			"转发给技能参数占位符的可选参数；若不存在占位符，参数将追加到技能正文",
			"Optionale Argumente für die Argumentplatzhalter des Skills; sind keine Platzhalter vorhanden, werden die Argumente an den Skill-Inhalt angehängt",
			"スキルの引数プレースホルダーに渡す任意の引数。プレースホルダーがない場合はスキル本文に追記されます",
			"스킬의 인수 자리표시자로 전달할 선택적 인수이며, 자리표시자가 없으면 스킬 본문에 덧붙입니다",
			"Необязательные аргументы для подстановки в навык; если заполнителей нет, аргументы добавляются к тексту навыка",
		},
		KeyToolSkillAllowedTools: {
			"Allowed tools: %s",
			"允许使用的工具：%s",
			"Zulässige Tools: %s",
			"使用できるツール: %s",
			"허용된 도구: %s",
			"Разрешённые инструменты: %s",
		},
		KeySkillToolInvalidInput: {
			"Error: invalid Skill input",
			"错误：Skill 输入无效",
			"Fehler: ungültige Skill-Eingabe",
			"エラー: Skill の入力が無効です",
			"오류: Skill 입력이 올바르지 않습니다",
			"Ошибка: недопустимые входные данные Skill",
		},
		KeySkillToolRequired: {
			"Error: 'skill' parameter is required",
			"错误：必须提供 'skill' 参数",
			"Fehler: Der Parameter 'skill' ist erforderlich",
			"エラー: 'skill' パラメーターは必須です",
			"오류: 'skill' 매개변수가 필요합니다",
			"Ошибка: требуется параметр 'skill'",
		},
		KeySkillToolInvalidSelector: {
			"Error: invalid skill selector",
			"错误：技能选择器无效",
			"Fehler: ungültige Skill-Auswahl",
			"エラー: スキルの指定が無効です",
			"오류: 스킬 선택자가 올바르지 않습니다",
			"Ошибка: недопустимый селектор навыка",
		},
		KeySkillToolExplicitUserOrigin: {
			"Error: explicit skill invocation requires user origin",
			"错误：显式技能调用必须使用用户来源",
			"Fehler: Ein ausdrücklicher Skill-Aufruf erfordert den Benutzerursprung",
			"エラー: 明示的なスキル呼び出しにはユーザー起点が必要です",
			"오류: 명시적 스킬 호출에는 사용자 출처가 필요합니다",
			"Ошибка: для явного вызова навыка требуется источник пользователя",
		},
		KeySkillToolUnavailable: {
			"Error: the Skill service is unavailable",
			"错误：技能服务不可用",
			"Fehler: Der Skill-Dienst ist nicht verfügbar",
			"エラー: スキルサービスを利用できません",
			"오류: 스킬 서비스를 사용할 수 없습니다",
			"Ошибка: служба навыков недоступна",
		},
		KeySkillToolRecursive: {
			"Error: skill %q is already running in session %q (recursive invocation refused)",
			"错误：技能 %q 已在会话 %q 中运行（已拒绝递归调用）",
			"Fehler: Skill %q wird bereits in Sitzung %q ausgeführt (rekursiver Aufruf abgelehnt)",
			"エラー: スキル %q はセッション %q ですでに実行中です（再帰呼び出しを拒否しました）",
			"오류: 스킬 %q이(가) 세션 %q에서 이미 실행 중입니다(재귀 호출 거부됨)",
			"Ошибка: навык %q уже выполняется в сеансе %q (рекурсивный вызов отклонен)",
		},
		KeySkillToolNotFound: {
			"Error: skill %q not found.",
			"错误：未找到技能 %q。",
			"Fehler: Skill %q wurde nicht gefunden.",
			"エラー: スキル %q が見つかりません。",
			"오류: 스킬 %q을(를) 찾을 수 없습니다.",
			"Ошибка: навык %q не найден.",
		},
		KeySkillToolAvailable: {
			" Available skills: %s",
			" 可用技能：%s",
			" Verfügbare Skills: %s",
			" 利用可能なスキル: %s",
			" 사용 가능한 스킬: %s",
			" Доступные навыки: %s",
		},
		KeySkillToolNoneInstalled: {
			" No skills are currently installed.",
			" 当前未安装任何技能。",
			" Derzeit sind keine Skills installiert.",
			" 現在インストールされているスキルはありません。",
			" 현재 설치된 스킬이 없습니다.",
			" Сейчас навыки не установлены.",
		},
		KeySkillToolAmbiguous: {
			"Error: skill %q is ambiguous; use one of these stable IDs: %s",
			"错误：技能 %q 存在歧义；请使用以下稳定 ID 之一：%s",
			"Fehler: Skill %q ist mehrdeutig; verwende eine dieser stabilen IDs: %s",
			"エラー: スキル %q は曖昧です。次の安定 ID のいずれかを使用してください: %s",
			"오류: 스킬 %q이(가) 모호합니다. 다음 안정 ID 중 하나를 사용하세요: %s",
			"Ошибка: навык %q неоднозначен; используйте один из стабильных ID: %s",
		},
		KeySkillToolShadowed: {
			"Error: skill %q is shadowed by %s and cannot be invoked",
			"错误：技能 %q 已被 %s 遮蔽，无法调用",
			"Fehler: Skill %q wird von %s überschattet und kann nicht aufgerufen werden",
			"エラー: スキル %q は %s によって隠されているため呼び出せません",
			"오류: 스킬 %q은(는) %s에 의해 가려져 호출할 수 없습니다",
			"Ошибка: навык %q перекрыт навыком %s и не может быть вызван",
		},
		KeySkillToolPolicyDeniedModel: {
			"Error: skill %q is disabled for this session or model invocation",
			"错误：技能 %q 已在当前会话中禁用，或已禁止模型调用",
			"Fehler: Skill %q ist für diese Sitzung oder für Modellaufrufe deaktiviert",
			"エラー: スキル %q はこのセッション、またはモデルからの呼び出しで無効です",
			"오류: 스킬 %q은(는) 이 세션 또는 모델 호출에서 비활성화되어 있습니다",
			"Ошибка: навык %q отключен для этого сеанса или для вызова моделью",
		},
		KeySkillToolPolicyDeniedUser: {
			"Error: skill %q is disabled for explicit user invocation",
			"错误：技能 %q 已禁止用户显式调用",
			"Fehler: Skill %q ist für ausdrückliche Benutzeraufrufe deaktiviert",
			"エラー: スキル %q はユーザーから明示的に呼び出せません",
			"오류: 스킬 %q은(는) 사용자의 명시적 호출이 비활성화되어 있습니다",
			"Ошибка: навык %q отключен для явного вызова пользователем",
		},
		KeySkillToolStale: {
			"Error: skill %q changed after it was selected; current revision is %d",
			"错误：技能 %q 在选中后已发生变化；当前修订号为 %d",
			"Fehler: Skill %q wurde nach der Auswahl geändert; aktuelle Revision: %d",
			"エラー: スキル %q は選択後に変更されました。現在のリビジョンは %d です",
			"오류: 스킬 %q이(가) 선택된 후 변경되었습니다. 현재 리비전은 %d입니다",
			"Ошибка: навык %q изменился после выбора; текущая ревизия: %d",
		},
		KeySkillToolRegistryFailure: {
			"Error: skill %q could not be prepared from the current registry",
			"错误：无法从当前注册表准备技能 %q",
			"Fehler: Skill %q konnte nicht aus der aktuellen Registry vorbereitet werden",
			"エラー: 現在のレジストリからスキル %q を準備できませんでした",
			"오류: 현재 레지스트리에서 스킬 %q을(를) 준비할 수 없습니다",
			"Ошибка: не удалось подготовить навык %q из текущего реестра",
		},
	} {
		semanticTranslations[key] = skillToolTranslations(values[0], values[1], values[2], values[3], values[4], values[5])
	}
}

func skillToolTranslations(en, zh, de, ja, ko, ru string) map[Language]string {
	return map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
}
