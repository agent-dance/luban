package i18n

// Semantic copy owned by the TUI skill-routing surface. Skill names, stable
// IDs, authored summaries, paths, and raw Skill tool errors remain values.
const (
	KeyTUISkillsMenuUnavailable    Key = "tui.skills.menu_unavailable"
	KeyTUISkillsMenuOpenFailed     Key = "tui.skills.menu_open_failed"
	KeyTUISkillsInvalidSelector    Key = "tui.skills.invalid_selector"
	KeyTUISkillsBackendUnavailable Key = "tui.skills.backend_unavailable"
	KeyTUISkillsSnapshotFailed     Key = "tui.skills.snapshot_failed"
	KeyTUISkillsNotFound           Key = "tui.skills.not_found"
	KeyTUISkillsAmbiguous          Key = "tui.skills.ambiguous"
	KeyTUISkillsUnavailable        Key = "tui.skills.unavailable"
	KeyTUISkillsInvokerUnavailable Key = "tui.skills.invoker_unavailable"
	KeyTUISkillsInvocationFailed   Key = "tui.skills.invocation_failed"
	KeyTUISkillsInvocationRejected Key = "tui.skills.invocation_rejected"
	KeyTUISkillsEmptyEnvelope      Key = "tui.skills.empty_envelope"
)

func init() {
	for key, values := range map[Key][6]string{
		KeyTUISkillsMenuUnavailable: {
			"The interactive skills menu is unavailable in this runtime.",
			"当前运行环境不支持交互式技能菜单。",
			"Das interaktive Skill-Menü ist in dieser Laufzeit nicht verfügbar.",
			"この実行環境では対話型スキルメニューを利用できません。",
			"이 런타임에서는 대화형 스킬 메뉴를 사용할 수 없습니다.",
			"Интерактивное меню навыков недоступно в этой среде.",
		},
		KeyTUISkillsMenuOpenFailed: {
			"Could not open the skills menu: %v",
			"无法打开技能菜单：%v",
			"Das Skill-Menü konnte nicht geöffnet werden: %v",
			"スキルメニューを開けませんでした: %v",
			"스킬 메뉴를 열 수 없습니다: %v",
			"Не удалось открыть меню навыков: %v",
		},
		KeyTUISkillsInvalidSelector: {
			"%q is not a valid skill selector.",
			"%q 不是有效的技能选择器。",
			"%q ist kein gültiger Skill-Selektor.",
			"%q は有効なスキル指定ではありません。",
			"%q은(는) 올바른 스킬 선택자가 아닙니다.",
			"%q не является допустимым идентификатором навыка.",
		},
		KeyTUISkillsBackendUnavailable: {
			"The live skill catalog is unavailable in this runtime.",
			"当前运行环境中实时技能目录不可用。",
			"Der Live-Skill-Katalog ist in dieser Laufzeit nicht verfügbar.",
			"この実行環境ではライブスキルカタログを利用できません。",
			"이 런타임에서는 실시간 스킬 카탈로그를 사용할 수 없습니다.",
			"Актуальный каталог навыков недоступен в этой среде.",
		},
		KeyTUISkillsSnapshotFailed: {
			"Could not read the current skill catalog: %v",
			"无法读取当前技能目录：%v",
			"Der aktuelle Skill-Katalog konnte nicht gelesen werden: %v",
			"現在のスキルカタログを読み込めませんでした: %v",
			"현재 스킬 카탈로그를 읽을 수 없습니다: %v",
			"Не удалось прочитать текущий каталог навыков: %v",
		},
		KeyTUISkillsNotFound: {
			"Skill %q was not found. Use /skills list to inspect the current catalog.",
			"未找到技能 %q。请使用 /skills list 查看当前目录。",
			"Skill %q wurde nicht gefunden. Mit /skills list lässt sich der aktuelle Katalog anzeigen.",
			"スキル %q が見つかりません。/skills list で現在のカタログを確認してください。",
			"스킬 %q을(를) 찾을 수 없습니다. /skills list로 현재 카탈로그를 확인하세요.",
			"Навык %q не найден. Текущий каталог можно посмотреть командой /skills list.",
		},
		KeyTUISkillsAmbiguous: {
			"Skill name %q matches multiple stable IDs: %s. Invoke one of those IDs explicitly.",
			"技能名称 %q 对应多个稳定 ID：%s。请显式调用其中一个 ID。",
			"Der Skill-Name %q passt zu mehreren stabilen IDs: %s. Eine dieser IDs muss ausdrücklich aufgerufen werden.",
			"スキル名 %q は複数の安定 ID に一致します: %s。いずれかの ID を明示して実行してください。",
			"스킬 이름 %q이(가) 여러 고정 ID와 일치합니다: %s. 해당 ID 중 하나를 명시적으로 호출하세요.",
			"Имени навыка %q соответствуют несколько стабильных ID: %s. Укажите один из них явно.",
		},
		KeyTUISkillsUnavailable: {
			"Skill %q is not available for explicit user invocation in the current catalog.",
			"当前目录不允许用户显式调用技能 %q。",
			"Skill %q ist im aktuellen Katalog nicht für einen ausdrücklichen Benutzeraufruf verfügbar.",
			"現在のカタログではスキル %q をユーザーが明示的に実行できません。",
			"현재 카탈로그에서는 사용자가 스킬 %q을(를) 명시적으로 호출할 수 없습니다.",
			"Навык %q недоступен для явного вызова пользователем в текущем каталоге.",
		},
		KeyTUISkillsInvokerUnavailable: {
			"Explicit skill invocation is unavailable in this runtime.",
			"当前运行环境不支持显式调用技能。",
			"Der ausdrückliche Skill-Aufruf ist in dieser Laufzeit nicht verfügbar.",
			"この実行環境ではスキルを明示的に実行できません。",
			"이 런타임에서는 스킬을 명시적으로 호출할 수 없습니다.",
			"Явный вызов навыков недоступен в этой среде.",
		},
		KeyTUISkillsInvocationFailed: {
			"Could not invoke skill %q: %v",
			"无法调用技能 %q：%v",
			"Skill %q konnte nicht aufgerufen werden: %v",
			"スキル %q を実行できませんでした: %v",
			"스킬 %q을(를) 호출할 수 없습니다: %v",
			"Не удалось вызвать навык %q: %v",
		},
		KeyTUISkillsInvocationRejected: {
			"Skill %q rejected the explicit invocation.",
			"技能 %q 拒绝了此次显式调用。",
			"Skill %q hat den ausdrücklichen Aufruf abgelehnt.",
			"スキル %q は明示的な実行を拒否しました。",
			"스킬 %q이(가) 명시적 호출을 거부했습니다.",
			"Навык %q отклонил явный вызов.",
		},
		KeyTUISkillsEmptyEnvelope: {
			"Skill %q returned no model instruction, so sampling was not started.",
			"技能 %q 未返回模型指令，因此未开始采样。",
			"Skill %q hat keine Modellanweisung geliefert; daher wurde kein Sampling gestartet.",
			"スキル %q からモデル指示が返されなかったため、サンプリングを開始しませんでした。",
			"스킬 %q이(가) 모델 지침을 반환하지 않아 샘플링을 시작하지 않았습니다.",
			"Навык %q не вернул инструкцию для модели, поэтому выборка не была запущена.",
		},
	} {
		semanticTranslations[key] = map[Language]string{
			LangEN: values[0], LangZH: values[1], LangDE: values[2],
			LangJA: values[3], LangKO: values[4], LangRU: values[5],
		}
	}
}
