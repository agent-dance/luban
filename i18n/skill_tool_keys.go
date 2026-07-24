package i18n

// Semantic copy emitted by the Skill tool's authoritative registry boundary.
// Skill IDs, revisions, rule names, and registry outcomes remain format values.
const (
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
	KeySkillToolDenyRule           Key = "tool.skill.deny_rule"
)

func init() {
	for key, values := range map[Key][6]string{
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
		KeySkillToolDenyRule: {
			"Error: skill %q execution blocked by deny rule %q",
			"错误：技能 %q 的执行被拒绝规则 %q 阻止",
			"Fehler: Die Ausführung von Skill %q wurde durch die Ablehnungsregel %q blockiert",
			"エラー: スキル %q の実行は拒否ルール %q によってブロックされました",
			"오류: 스킬 %q 실행이 거부 규칙 %q에 의해 차단되었습니다",
			"Ошибка: выполнение навыка %q заблокировано правилом запрета %q",
		},
	} {
		semanticTranslations[key] = skillToolTranslations(values[0], values[1], values[2], values[3], values[4], values[5])
	}
}

func skillToolTranslations(en, zh, de, ja, ko, ru string) map[Language]string {
	return map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
}
